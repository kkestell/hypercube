# One exit path per launch, and internal/session split by concern

## Goal

A launch of an agent's plugin can end in four ways today, each handled
differently: `failed` on a first start, `ended` (which releases a lock its
caller took) on an exit or a failed relaunch, `AddWorker`'s own cleanup, and
`l.exit` for an exit while starting. `runtime.go` (817 lines) mixes launch,
exit, events, delivery, user actions, transcripts, and shutdown, and
`session.go` mixes session lifecycle with workers.

When done, every launch that ends by itself goes through one function, `end`,
which takes and releases `mu` itself. Each file holds one concern. The
remaining audit findings for `internal/session` are fixed, and the session
tests use public behavior only.

## Related code

- `internal/session/runtime.go` — `live`, `register`, `launch`, `failed`,
  `exited`, `ended`, `event`, delivery, user actions, transcripts, `detach`,
  `Shutdown`.
- `internal/session/session.go` — package doc, workspaces, `Create`, `start`,
  `Stop`, `Delete`, `Rename`, `Seen`, `SetStatus`, `Restore`, `AddWorker`,
  `RemoveWorker`.
- `internal/session/feed.go`, `inactivity.go`, `messages.go` — unchanged in
  shape.
- `internal/report/report.go` — `Format`.
- `internal/harness/harness.go` — `Launch` calls `onExit` once, after
  `done` closes; `Client.Kill` waits for `done`, so it must not be called with
  `mu` held (the reader may be blocked in `r.event`).
- `agents/reviews/go-audit.md` — the `internal/session` and `internal/report`
  sections.

## Decisions

### Launch states

A `live` is one launch. It is in one of three states:

1. **Starting**: in `r.live`, `starting` set. `client` is nil until the
   plugin process exists. Events are applied; deliveries and user actions wait
   (`kickLocked` and `running` refuse a starting launch).
2. **Ready**: the plugin replied to start. `starting` is clear, `launched` is
   the time it became ready, deliveries run.
3. **Over**: not in `r.live`. Anything from a launch that is no longer
   current is ignored: events, exits, deliveries, and `launch` finishing.

A launch becomes over in one of two ways:

- **It ends by itself** (failed to start, or its plugin exited): `end`.
- **It is stopped** (Stop, Delete, RemoveWorker, idle stop, Shutdown):
  `detachLocked`, as today. A stop writes no note and never relaunches, and
  it handles several launches in one transaction, so it stays separate from
  `end`.

### `end`, the single exit path

```go
// end records that l's launch is over because it failed to start or its
// plugin exited: it saves l's unfinished items, notes why in the transcript,
// and marks the agent ended. An agent whose plugin exits after starting is
// launched again, resuming its conversation, unless its session stopped or it
// ended maxQuickExits times in a row within quickExit of starting. end ignores
// a launch that was stopped or has ended already.
func (r *Runtime) end(l *live, cause error)
```

`end` locks `mu`, returns if `!r.current(l)`, deletes `l` from `r.live`, sets
`l.cause`, writes in one transaction (drained items, the note, `markEnded`,
reading `s.Running`), and, to relaunch, registers the next launch and starts
`go r.launch(next)`, all before unlocking with `defer`. The note is "Could not
start the agent: …" for a launch that was starting and "The agent ended: …"
otherwise.

Callers:

- the plugin's exit: `harness.Launch(path, …, func(err error) { r.end(l, err) })`.
  `exited` and the `l.exit` field go away; an exit while starting ends the
  launch at once.
- `launch`, when starting fails.
- `AddWorker`, when its worktree cannot be made (the launch never started).

`launch` keeps the plugin's client to itself until the launch is ready:

```go
// launch starts l's plugin and waits until its harness is ready. A launch that
// fails to start ends. launch returns why it ended, or errStopped if it was
// stopped first.
func (r *Runtime) launch(l *live) error {
	client, err := r.startPlugin(l)
	if err != nil {
		r.end(l, err)
		if client != nil {
			client.Kill()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.current(l) {
		return cmp.Or(l.cause, errStopped)
	}
	// ready: clear starting, set launched, mark the agent idle, kick delivery
}
```

`end` runs before `Kill`, so the cause recorded is the start error and not the
kill that follows. `startPlugin` reads the start arguments, finds and launches
the plugin, sets `l.client` under `mu` only if `l` is still current (otherwise
it returns `errStopped` and the client, which `launch` kills), and sends start.
`live.cause` replaces `live.exit`: why the launch ended by itself, nil for a
stopped launch.

`r.closing` checks leave `launch` and `end`: after `Shutdown`, `r.live` is empty
and nothing registers again (`start` checks `closing`, `AddWorker` needs a live
lead, `end` needs a current launch).

`Start` and `AddWorker` report the error `launch` returns, as today. The
relaunch goroutine ignores it, since `end` recorded it.

### Behavior choice: a launch that never started is not relaunched

Today a failed first start stays ended, while a failed relaunch counts as a
quick exit and is retried up to the limit, with the note "The agent ended: …".
With one exit path the rule is: a launch that fails to start stays ended, with
"Could not start the agent: …"; only an exit after starting relaunches. So a
failed relaunch is no longer retried, and its note reads "Could not start the
agent". Keeping today's retry would need a second flag on `live` meaning "a
start failure here counts as a quick exit", which is the split this change
removes. Quick exits then count only exits after start, which matches their
name. A failed start also now saves items the plugin streamed before failing
(`failed` dropped them).

An exit right after the start reply but before `launch` marks the launch
ready is still a failed start, as today, so the stand-in's `startCrashDelay`
stays.

### Who holds `mu`

`mu` guards `r.live`, every mutable field of a `live`, `r.seqs`, `r.gen`,
`r.viewers`, and `r.closing`. It is never held while waiting on a plugin
(`Do`, `Shutdown`, `Kill`). Every function that takes `mu` releases it before
returning; none hands it to another. Helpers that need it say "The caller
holds mu": `current`, `register`, `nextSeq`, `note`, `kickLocked`,
`kickSessionLocked`, `detachLocked`, `stopLocked`, `sessionLive`, `running`
(today's `ready`, renamed because ready is now a launch state), `next`,
`delivered`. Writes that save transcript items run under `mu`, as now, so
changes publish in order.

`deliver` holds `mu` except while waiting on the plugin:

```go
func (r *Runtime) deliver(l *live, client *harness.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer func() { l.delivering = false }()
	for {
		out, ok := r.next(l)
		if !ok {
			return
		}
		r.mu.Unlock()
		sent := request(client, harness.Command{Command: harness.CmdInput, Text: out.text})
		r.mu.Lock()
		if !r.delivered(l, out, sent) {
			return
		}
	}
}
```

`delivering` is cleared in the same critical section that finds the queue
empty, as now, so a message queued meanwhile is never stranded.

### `next` and `report.Format`

`report.Format(tx, m store.Message) (text, from string)` formats one message
and returns its sender's address when an agent sent it. `next` returns
`(outgoing, bool)`:

```go
// outgoing is a queued message ready to send.
type outgoing struct {
	message int64        // the queued message's ID
	text    string       // what the plugin receives
	item    harness.Item // what the transcript shows
	seq     int64        // the item's position, taken before sending so the
	                     // reply's items follow it
}
```

`delivered` dequeues and saves `out.item`, or notes the failure; it returns
false to stop.

### Transcript positions

`register` stops reading the database. `start` and `AddWorker` call
`r.loadSeq(tx, id)` inside the transaction that marks the agents for launch,
so a read error fails that `Start` or `AddWorker` and numbering never restarts
at 1. `loadSeq` reads `LastSeq` only when `r.seqs` lacks the agent, since the
runtime's count can be ahead of the database while a write is in flight. A
relaunch's agent is always loaded already.

### Small fixes

- `Delete` checks the `Agents` error before ranging, and checks worktrees
  inside one `if present` instead of `continue` per worker.
- `AddWorker` returns `store.Agent{}` with every error (the MCP caller ignores
  the agent on error).

### File split

| File | Holds |
| --- | --- |
| `runtime.go` | package doc, constants, errors, `Runtime`, `New`, `logf` |
| `launch.go` | `live`, `pendingItem`, `register`, `loadSeq`, `nextSeq`, `current`, `sessionLive`, `launch`, `startPlugin`, `startArgs`, `end`, `markEnded`, `drain`, `note` |
| `event.go` | `event`, `applyEvent` |
| `deliver.go` | `Deliver`, `kickLocked`, `kickSessionLocked`, `deliver`, `outgoing`, `next`, `delivered` |
| `input.go` | `request`, `running`, `checkPrompt`, `Input`, `Interrupt`, `Answer` |
| `transcript.go` | `TranscriptItem`, `Request`, `Requests`, `Transcript` |
| `sessions.go` (from `session.go`) | `AddWorkspace`, `DeleteWorkspace`, `Create`, `Start`, `start`, `Restore`, `Stop`, `stopLocked`, `Delete`, `detach`, `detachLocked`, `stopClients`, `Shutdown`, `Rename`, `Seen`, `update`, `Shell` |
| `workers.go` | `AddWorker`, `RemoveWorker`, `SetStatus` |
| `messages.go`, `feed.go`, `inactivity.go` | unchanged |

Package doc, in `runtime.go`: "Package session runs Hypercube's workspaces,
sessions, and agents. A Runtime owns each agent's plugin process, delivers
queued messages and the user's input, records what the agents report, and is
the only writer of the database."

## Test plan

Tests move to match the files: `fixture_test.go` (the one fixture),
`launch_test.go`, `event_test.go`, `deliver_test.go`, `input_test.go`,
`sessions_test.go`, `workers_test.go`, and the existing `feed_test.go`,
`inactivity_test.go`, `messages_test.go`. No test reads `rt.live` or `mu` or
calls `event`, `save`, `putItem`, or `nextSeq`.

- **One fixture.** `open`, `seed`, and `count` go; `newFixture` gains
  `f.seed(workers ...store.Agent)`, which adds a stopped session `s` with lead
  `a` in `f.workspace` and checks every setup error. `TestSendMessageScope`,
  `TestSetStatus`, `TestDelete`, and `TestDeleteMissingCheckout` use it
  (`TestDeleteMissingCheckout` removes `f.root`).
- **No hidden index skipping.** `fixture.created` goes. `inputs` and
  `transcript` return everything; assertions name the creation turn
  (`"user:start|text:ack|…"`) or slice it off visibly where a test is about
  later items.
- **Fixed sleeps become triggered checks.**
  - `TestRefusedMessageStaysQueued`: after the first refusal, call `Deliver`
    and wait for a second refusal; the inputs are then two `!reject this` and
    the queue still holds both messages. (The refusal note is saved in the
    critical section that clears `delivering`, so this `Deliver` always
    starts an attempt.)
  - `TestRestoredSessionHoldsMessagesUntilThePrompt`: call `Deliver` after the
    restart and check the queue; the user's prompt then goes out before the
    held message.
  - `TestAgentThatKeepsEndingStaysEnded`: wait for `maxQuickExits` "The agent
    ended" notes; then `AddWorker` on the lead fails with "the session is not
    running", which shows no relaunch is registered (the decision is made in
    the same critical section that writes the note).
- **Public behavior instead of internals.**
  - `TestFailedResumeIsAnError`: instead of `rt.live`, a second `Start`
    launches again and fails again.
  - `TestRelaunchAfterTheProcessEnds`: the direct `rt.event` call on the old
    launch goes; nothing public can make an ended plugin send events.
  - `TestRequestsNewestFirst`: the lead and then a worker run `!ask`;
    `Requests` lists the worker's request first.
  - `TestStreamedChangesKeepTheirPositionAndText`: driven by `!stream`.
  - `TestFailedWritePublishesNothing`: renaming a missing session fails and
    nothing is published.
  - `inactivity_test.go`'s `settled` goes. The checks that a session is not
    yet stopped do not depend on delivery; the checks that it stops retry
    `stopIdleSessions` with `f.until`.
- `TestTranscriptKeepsOrderAndReplacesItems`: one condition accepting either
  order of the last two items.
- `report_test.go`: `TestFormatAttributesMessages` checks an agent's message
  (text and address) and the user's (no address).

## Implementation plan

1. `report.Format`: one message, returns `(text, from string)`; update
   `report_test.go`.
2. Move code into the files above without changing it; build.
3. Add `live.cause`, `end`, `startPlugin`, `startArgs`; rewrite `launch`;
   delete `failed`, `exited`, `ended`, `live.exit`; update `start`'s goroutine,
   `AddWorker`'s failure path, and the relaunch.
4. `loadSeq` in `start` and `AddWorker`; `register` no longer reads.
5. `outgoing`, `next`, `delivered`, and `deliver` as above; rename `ready` to
   `running`.
6. `Delete` and `AddWorker` fixes; package doc.
7. Tests as in the test plan.
