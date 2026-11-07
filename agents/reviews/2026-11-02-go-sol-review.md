# Go review — go-sol-review

Reviewer: go-sol-review. Baseline: `760eeae`. Reviewed HEAD:
`f55f444d68615693cbd26f08702aaf1e9c599162`.

**Conclusion: changes requested.** Two confirmed restart defects remain. No
implementation, staging of code, or broad checks were performed. This report is
independent; no other review reports or reviewers' findings were consulted.

## Findings

### 1. [P1] An early terminal reconnect replaces a surviving shell

Location: `internal/terminal/terminal.go:54–60`. Related:
`cmd/hypercube/main.go:94–95`, `internal/session/restore.go:114–145`, and
`internal/ui/terminal.go:25`.

Violation: preserve the shell across an app restart, with one owner for its
lifecycle. The new startup path depends on a separate adoption step having
finished before a shell can be opened, but HTTP starts before that step.

`startApp` begins serving before `Restore`. `restoreFamily` adopts shells only
after attaching/recovering the agents, potentially after waiting for a native
start. During that interval, `/terminal` can call `Set.Open`. Its empty in-memory
map does not contain the surviving shell, so `Open` removes the holder directory
without checking its on-disk lock, then starts a replacement. The old holder and
shell remain alive with their original, now unlinked lock/socket/output files.
Adopting later finds the replacement. This loses shell identity and retained
output, and the runtime can no longer stop the original process through its
holder directory. An already open terminal retries its connection automatically
(`internal/ui/web/terminal/terminal-panel.jsx:51–54`).

Confirmed with a temporary Go test overlay, leaving repository code untouched:
start `/bin/cat` through one `terminal.Set`, send/read `retained`, then open the
same session through a second set using the same holder directory **before**
calling `Adopt`. Read its new output and compare the recorded holder PIDs. The
race-enabled check failed:

```text
Open before Adopt replaced living holder 56152 with 56162; original still alive: true
```

Both isolated holders were cleaned up. The missing `exit.tmp` diagnostic during
cleanup is a consequence of the first holder having lost its directory.

Smallest fix: in `Set.Open`, reuse `hold.Open` and `Alive` on a cache miss before
removing the directory or launching anything. Keep proactive adoption for shells
that the user does not reopen, so Stop/Quit still own those shells. This uses the
existing holder checks; it needs no readiness flag, transport, or goroutine.
Expected production change: replace roughly 2–4 lines with 10–14 lines around the
existing open/start branch. Extend `TestInputOutputAndAdopt` to open through the
new set before adoption and assert that both PID and retained output survive.
The additional code addresses the reproduced replacement and orphaned process.

### 2. [P2] Reattachment never attempts delivery of a queue without a command

Location: `internal/session/launch.go:153–160`. Related:
`internal/session/messages.go:16–44`, `internal/session/restore.go:151–179`, and
`internal/session/deliver.go:31–44`.

Violation: existing queued messages must continue through the ordinary delivery
path after an app restart. Reconnection restores unanswered commands but omits
the queue-to-command step.

`SendMessage` commits `messages` before calling `Deliver`. If the app ends in
that interval, the saved queue has no corresponding `commands` row. On restart,
`attach` restores the ready launch and its empty command map. `run` only invokes
`send` and watches commands that already exist. An idle surviving plugin has no
new start reply or idle event to trigger `kickLocked`, so the queued message
stays undelivered until a later user prompt, message, or native event happens.
Live reattachment deliberately leaves `AwaitingUser` false, but neither
`Restore` nor `run` asks the existing queue for work.

Confirmed with a temporary race-enabled overlay of `restart_test.go`: create an
idle session; detach its runtime; persist one queued message without a command
(the exact saved state between `SendMessage`'s commit and `Deliver`); restore a
new runtime over the same database. After five seconds the queue remained, and
the stand-in log still contained only its original start/input. The check failed
with `timed out waiting for restored idle holder to deliver saved queue`.

Smallest fix: call the existing `kickLocked(l.id)` once under the existing runtime
lock after a client is attached successfully. Its current checks already handle
readiness, stopping, shutdown, a pending delivery, and `AwaitingUser`; keep those
checks as the single policy owner. Expected production change: one added call,
no new state or helper. Add a roughly 15-line regression in `restart_test.go` for
a saved queue with no command, alongside the existing replay tests. This added
code addresses the confirmed undelivered message.

## Verification

- Passed: `go test -race ./internal/session -run '^TestChildLifecycle$' -count=1`.
- Passed: focused race-enabled checks for
  `TestRestartAppliesOutputAndStreamingOnce`,
  `TestRestartKeepsHeldDeliveryAndResendsUnwrittenCommand`,
  `TestInputOutputAndAdopt`, and `TestBridgeWaitsForRestartingApp` across
  `internal/session`, `internal/terminal`, and `internal/mcp`.
- Failed as expected: the two focused temporary-overlay checks described above.
  Overlays lived only in `/tmp`; repository implementation and tests were not
  edited. All checks used isolated state and stand-in processes.
- The first Go invocation could not access the default build cache under the
  sandbox. Subsequent checks used
  `GOCACHE=/tmp/hypercube-go-sol-review-gocache` and completed normally.
- No `make check`, installed-app operation, browser rendering, or visual check.
  Generated UI bundles are outside this review, as confirmed with the parent.

## Coverage

All 86 present files below were read in full in their current state. All four
deleted files were read in full at `760eeae`. The list comes from
`git diff --name-only 760eeae HEAD`, limited to the assigned Go/native/plugin
scope; status letters below identify additions, modifications, and deletions.

```text
M	cmd/hypercube/integration_test.go
M	cmd/hypercube/main.go
A	cmd/hypercube/testmain_test.go
A	internal/desktop/app.m
A	internal/desktop/browser.go
A	internal/desktop/browser.m
A	internal/desktop/desktop.go
A	internal/desktop/desktop.h
A	internal/desktop/window.m
M	internal/harness/client_test.go
M	internal/harness/config.go
M	internal/harness/harness.go
M	internal/harness/harness_test.go
M	internal/harness/harnesstest/harnesstest.go
M	internal/harness/protocol.go
A	internal/harness/testmain_test.go
A	internal/hold/hold.go
A	internal/hold/hold_test.go
A	internal/hold/holder.go
A	internal/hold/holdtest/holdtest.go
A	internal/hold/output.go
A	internal/hold/testmain_test.go
A	internal/mcp/browser.go
A	internal/mcp/browser_test.go
M	internal/mcp/mcp.go
M	internal/mcp/mcp_test.go
A	internal/mcp/testmain_test.go
D	internal/menu/menu.go
D	internal/menu/menu.m
M	internal/report/report.go
M	internal/report/report_test.go
A	internal/session/children.go
A	internal/session/children_test.go
A	internal/session/commands.go
M	internal/session/deliver.go
M	internal/session/deliver_test.go
M	internal/session/event.go
M	internal/session/event_test.go
M	internal/session/feed.go
M	internal/session/feed_test.go
M	internal/session/fixture_test.go
M	internal/session/inactivity.go
M	internal/session/inactivity_test.go
M	internal/session/input.go
M	internal/session/input_test.go
M	internal/session/launch.go
M	internal/session/launch_test.go
M	internal/session/messages.go
M	internal/session/messages_test.go
A	internal/session/mirrors_test.go
A	internal/session/restart_test.go
A	internal/session/restore.go
M	internal/session/runtime.go
M	internal/session/sessions.go
M	internal/session/sessions_test.go
A	internal/session/testmain_test.go
M	internal/session/transcript.go
M	internal/session/transcript_test.go
D	internal/session/workers.go
D	internal/session/workers_test.go
A	internal/store/migrate.go
A	internal/store/migrate_test.go
M	internal/store/store.go
M	internal/store/store_test.go
A	internal/store/testdata/origin-schema.sql
M	internal/terminal/terminal.go
M	internal/terminal/terminal_test.go
A	internal/terminal/testmain_test.go
M	internal/ui/actions.go
M	internal/ui/files.go
M	internal/ui/files_test.go
A	internal/ui/images.go
M	internal/ui/snapshot.go
M	internal/ui/socket.go
M	internal/ui/terminal.go
A	internal/ui/testmain_test.go
M	internal/ui/ui.go
M	internal/ui/ui_test.go
M	internal/workspace/workspace.go
M	internal/workspace/workspace_test.go
M	plugins/claude/main_test.go
M	plugins/codex/main.go
M	plugins/codex/main_test.go
A	plugins/codex/storage.go
A	plugins/codex/storage_test.go
M	plugins/internal/plugin/serve.go
M	plugins/internal/plugin/serve_test.go
M	plugins/opencode/main.go
M	plugins/opencode/main_test.go
M	plugins/pi/main_test.go
```

Dependent code inspected to resolve ownership and calls included
`cmd/hypercube/main_test.go`, `internal/ui/assets.go`,
`internal/harness/image.go`, `plugins/internal/plugin/process.go`,
`plugins/internal/plugin/rpc.go`, `plugins/internal/plugin/plugin.go`,
`plugins/internal/plugin/requests.go`, `plugins/internal/plugin/form.go`,
`plugins/codex/events.go`, the startup paths in `plugins/claude/main.go` and
`plugins/pi/main.go`, `go.mod`, and the frontend terminal connection in
`internal/ui/web/terminal/terminal-panel.jsx` and `xterm.js`.

Excluded: `internal/ui/web/**` as a review scope (the two terminal modules above
were read only to trace the server behavior), generated `internal/ui/dist/**`,
frontend build/test setup, docs, plans, work logs, and other review reports.
