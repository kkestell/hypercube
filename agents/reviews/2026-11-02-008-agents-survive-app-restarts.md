# Agents and panel shells survive app restarts

## Scope and coverage

Reviewed branch `hypercube/5b9a1be0/survive-restart-work` at 2199580 against
`main`, implementing `agents/plans/2026-11-02-006-agents-survive-app-restarts.md`.
The review covered every file the change touches: the new `internal/hold`
package, the harness client and protocol, the plugin serve loop and stand-in,
the session runtime (launch, commands, delivery, events, restore, stop, Quit,
Detach), the store schema, panel shells and their WebSocket, the MCP bridge
retry, `cmd/hypercube`, `scripts/dev`, tests, and documentation. It traced the
exactly-once path from a recorded command through the written line, the
plugin's read, and the logged reply to the applied reply, across Detach,
crashes, holder death, Stop, Quit, Restore, relaunch, and quick-exit counting.
It also checked the result against the architectural review's conclusions
that the plan adopted.

Gaps:

- No live harness or `make dev` session was run; behavior was checked with the
  stand-in plugin and the isolated test suites.

## Summary

| ID | Severity | Category    | Fix   | Title                                                      |
| -- | -------- | ----------- | ----- | ---------------------------------------------------------- |
| F1 | Medium   | Correctness | Fixed | A normal plugin exit can be read as a lost holder          |
| F2 | Medium   | Concurrency | Fixed | The runtime mutex is held while waiting for a holder       |
| F3 | Medium   | Testing     | Fixed | Tests wait the real one-minute and 30-second timeouts      |
| F4 | Low      | Architecture| Fixed | Command lines are encoded three times; the store parses protocol JSON |
| F5 | Low      | Simplicity  | Fixed | Six identical test entry points                            |
| F6 | Low      | Simplicity  | Fixed | The panel shell lifecycle has more machinery than it needs |
| F7 | Low      | Readability | Fixed | `event.go` has a test-only method, a redundant case, and a duplicate path |
| F8 | Low      | Readability | Fixed | Restore writes the lost condition twice                    |

## F1. A normal plugin exit can be read as a lost holder

Medium · Correctness · Fixed · `internal/hold/output.go:70`

At the end of the output, `Output.Read` checked for the exit record and then
for the lock. If the holder wrote its exit record and exited between the two
checks, `Read` returned `ErrLost`. The launch then ended with no note and no
relaunch, so a plugin that crashed stayed exited without a word. A launch
classified as exited at Restore could end the same way.

### Evidence

By trace: the holder writes `exit` before it releases the lock, so the old
order leaves a window. The window is microseconds; a 600-exit probe did not hit
it.

### Fix

`Read` checks the lock first and then the exit record. A launch is lost only
when its holder was dead and no exit record exists. Restore already checked in
this order, and a comment there now says why.

## F2. The runtime mutex is held while waiting for a holder

Medium · Concurrency · Fixed · `internal/session/launch.go:211`

`end` removed the holder under `r.mu`, and `Holder.Remove` waits for the
holder to exit. After a protocol error, the client stops the plugin and returns
at once, so the whole app could wait up to the holder's 3-second kill plus its
1-second drain. `run` also called `client.Stop` under `r.mu` right after
starting a holder, and `Stop` polls until the pid file exists. The start
timeout stopped the client under `r.mu` as well.

### Fix

`end` records the end and registers the next launch under `r.mu` in
`endLocked`, then closes and removes the holder and starts the next launch
outside it. `run` and the start timer call `Stop` after releasing `r.mu`.

## F3. Tests wait the real one-minute and 30-second timeouts

Medium · Testing · Fixed · `internal/session/restart_test.go:315`

`TestDeliveryTimeoutHoldsLaterMessagesUntilAcceptance` and
`TestRestoreDecidedLaunchHasStartTimeout` waited for the real timeouts. That
added about 90 seconds to every `make check`.

### Fix

`Runtime` has `startTimeout` and `requestTimeout` fields set from the
constants in `New`. The two tests set them to 2 seconds. The session package
now takes about 36 seconds instead of 124. Fields avoid the data race a
package variable caused when a test restored it while launch goroutines were
still reading it.

## F4. Command lines are encoded three times; the store parses protocol JSON

Low · Architecture · Fixed · `internal/store/store.go:436`

`record` marshalled the command. `store.AddCommand` then decoded it into a
`harness.Command`, set the ID, and encoded it again. `send` decoded the stored
line, and `Client.Send` encoded it a third time.

### Fix

`AddCommand` inserts the row and returns its ID, and `SetCommandLine` saves the
line. `record` sets the ID and builds the line once with
`harness.Command.Line`. `Client.Send` writes the stored bytes as they are.

## F5. Six identical test entry points

Low · Simplicity · Fixed · `internal/session/testmain_test.go:1`

`cmd/hypercube`, `harness`, `mcp`, `session`, `terminal`, and `ui` each had the
same `TestMain`, including an uncommented `GORACE` setting.

### Fix

`internal/hold/holdtest.Main` runs the holder when a test binary is started as
`hold`, and runs the tests otherwise. It explains the `GORACE` setting, which
makes race-enabled holders exit without sleeping. Each `TestMain` is now one
line. The `hold` package keeps its own, because its internal tests cannot
import `holdtest`.

## F6. The panel shell lifecycle has more machinery than it needs

Low · Simplicity · Fixed · `internal/terminal/terminal.go:1`

Each shell had a watcher goroutine that polled the lock every 25 ms, a reader
`WaitGroup`, `done`, `removed`, and `detached` channels, and `ended` and `exit`
fields. The directory name `shell-<session>` was built in both `restore.go`
and `terminal.go`, and `Adopt` took a directory.

### Fix

A `Shell` is its holder. `Open` returns a live shell, or removes an ended
one's directory and starts a new one. `Stop` and `Close` stop the holder and
remove it once it has exited. `Adopt` takes the session ID. `terminal.Name`
holds the directory name for both `Set` and Restore. `Set.Detach` is gone,
since a detached app leaves nothing to release. The WebSocket sends
`{"exit": n}` whenever its output ends, because it ends only when the shell
ends, when it is stopped and removed, or when the page goes away. The page
therefore still learns that a stopped shell ended. Tests check that a stopped
shell's holder directory is gone.

## F7. `event.go` has a test-only method, a redundant case, and a duplicate path

Low · Readability · Fixed · `internal/session/event.go:38`

`Runtime.event` was used only by `mirrors_test.go`. `apply` skipped the waiter
on a failed start, although `end` sends that waiter the same failure.
`eventLocked` had two `checkpoint` return paths.

### Fix

The helper is now `fixture.event` in `mirrors_test.go`. `apply` notifies every
waiter. `eventLocked` reads the session, saves it when the event changed it,
queues failures, and checkpoints in one path.

## F8. Restore writes the lost condition twice

Low · Readability · Fixed · `internal/session/restore.go:83`

`!missing && !alive && !exited` appeared twice in `restoreFamily`.

### Fix

The condition is computed once as `holderLost`.

## Checks run

- `make check` passed: builds, generated assets, 158 Bun tests, Go vet,
  formatting, and the full Go race suite.
- `TestTerminalResizeHangupAndRetention` failed 12 to 14 times in 15 repeated
  runs, both on 2199580 and with these fixes. Its shell wrote 700 KB right after
  printing its new size, so terminal rotation could delete that line before
  the test read it. The shell now waits for a second newline before the burst,
  and 15 repeated runs pass.
- Focused race runs of the two timeout tests and of `internal/terminal` passed.
- After the runs, no `hold` processes and no `hc*` test directories were left.

## Verdict

The design delivers each command, reply, and output record exactly once
across restarts and crashes, and it matches the architectural review's
conclusions. F1 and F2 were real but rare correctness and stall bugs. F3 cut
the test suite's run time. F4 through F8 removed duplication and machinery.
All are fixed, and the branch is ready to merge.
