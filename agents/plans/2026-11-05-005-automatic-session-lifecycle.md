# Automatic session lifecycle

## Goal

Sessions start and stop on their own, as in Claude Desktop. The user sees
conversations, not processes. Hypercube starts a session's agent, resuming
its saved conversation, when the user opens the session, sends it a prompt,
or a message reaches it: another agent's `send_message`, a child's report or
failure, or the user's answer to an offer. It parks a session, ending its
agent process, 15 minutes after the session last became eligible to park:
nobody views it, and it is not mid-turn and has no open question, queued
input, queued messages, or command in flight. Each session wakes and parks on
its own; only Archive and Delete act on a whole family. Start, Stop, and
Show/Hide Stopped Sessions leave the UI. Archive and a new Unarchive are the
only lifecycle actions the user sees. The composer's Stop button still
interrupts a turn.

## Baseline

The session is the unit that starts and stops. Launches are already per
session: `r.live` is keyed by session ID, `EndChild` stops one launch, and
`reportsReady` already skips children without launches. The existing start
path becomes *wake* for one session: it registers a launch when the session
has none and resumes its conversation through `StartArgs.Conversation`. All
four plugins already resume this way (`plugins/claude/main.go:88`,
`plugins/codex/main.go:118`, `plugins/opencode/main.go:174`,
`plugins/pi/main.go:107`), so no plugin changes. The existing stop path,
`selectLaunches` then `stopLaunches`, with `endLocked` suppressing a successor
for a stopping launch, becomes *park* for one session.

The additions, each for a stated requirement:

- A count of viewers per session. A viewed session is never parked. The
  count is needed because several data sockets can view one session.
- One idle timer per session, for the idle timeout.
- A `started` channel on each launch, closed when its start succeeds, which
  any number of waiters can observe. `Input` can then wait for a launch that
  opening the session already registered.
- `Unarchive`, which the user approved.
- A migration that drops the two obsolete columns.

The change deletes:

- `Runtime.Start`, `Runtime.Stop`, `start`, `stopLocked`, and `sessionLive`.
- `store.Session.Running`, `store.Session.AwaitingUser`, `Tx.Running`, and their
  columns.
- `recordRequest`'s `release` parameter.
- Restore's start of a family that lost every process, its relaunch of lost
  members, and its awaiting-user rule.
- The `start` and `stop` UI actions, `startSession`, `stopSession`, and the
  Start/Stop menu items.
- The frontend's `stopped`, `failed`, `runningFirst`, and `running`, and the
  Show Stopped Sessions state.
- The composer's refusal to send to an exited session.

## Related code

- `internal/session/sessions.go` — `Create`, `Start`/`start` (registration,
  branch refresh, launch), `Stop`/`stopLocked`, `Shell`, `stopShells`,
  `selectLaunches`, `stopLaunches`, `Quit`, `Detach`.
- `internal/session/launch.go` — `live`, `decide`, `launch`, `run` (its start
  timeout and reader callback), `endLocked` (`suppress` reads `tx.Running`),
  `sessionLive`.
- `internal/session/restore.go` — `Restore`, `restoreFamily` (`root.Running`,
  `allLost` → `Start`, relaunch of lost members), `attach`; shell holders are
  kept only for running families.
- `internal/session/deliver.go` — `Deliver`, `kickFamilyLocked`,
  `kickLocked`, `next` (refuses `AwaitingUser` and `!Running`).
- `internal/session/messages.go` — `SendMessage` refuses stopped recipients.
- `internal/session/offers.go` — `answerOffer` queues the answer and calls
  `kickLocked`.
- `internal/session/input.go` — `running`, `Input`, `Answer`.
- `internal/session/commands.go` — `reply` (start acceptance sets `ready`),
  `recordRequest` (clears `AwaitingUser`).
- `internal/session/event.go` — `apply`, `eventLocked`, `saveEvent` (queues a
  child's reports and failures).
- `internal/session/removal.go` — `removeFamily` (Archive calls `stopLocked`),
  `endArchived`.
- `internal/session/runtime.go` — `Runtime` fields and the test-shortened
  timeouts.
- `internal/store/store.go`, `internal/store/migrate.go` — session columns, the
  final attention cleanup that reads `running`, and `convertLegacy`'s insert.
- `internal/mcp/mcp.go` — `send_message` and `list_sessions` descriptions,
  `listSessions` filters on `tx.Running`.
- `internal/report/report.go:66` — the instructions mention running and stopped
  recipients.
- `internal/ui/socket.go` — `selectSession`; `internal/ui/actions.go` — `start`
  and `stop`; `internal/ui/snapshot.go` — `Running` and `Ready`.
- Frontend: `web/sessions/selection.ts`, `web/sessions/session-actions.ts`,
  `web/sessions/types.ts`, `web/sidebar/sidebar.tsx`,
  `web/sidebar/sidebar-menus.ts`, `web/sidebar/session-item.tsx`,
  `web/app/shortcuts.ts`, `web/app/app.tsx:102`, `web/agent/composer.tsx:85`,
  `web/agent/drafts.ts:25`, `web/agent/merge-button.tsx:15`,
  `web/terminal/terminal-panel.tsx:2`.

## Decisions

- **Session unit.** Waking, viewing, the idle timer, and parking each concern
  one session. Viewing a parent does not keep its idle children awake, and a
  busy child does not keep an idle, unviewed parent awake. A parked parent
  wakes when a child's report or failure is queued for it; a parked child
  wakes when its parent messages it. Only Archive and Delete stop a whole
  family.
- **Awake means having a launch.** Hypercube no longer records whether a
  session should run. A session is awake when `r.live` holds its launch, and
  across app restarts when its `Launch` is set. A parked session has neither.
- **Parkable(id)**, checked under `mu`. All of these must hold:
  - Hypercube is not closing.
  - Neither the session nor its family is being archived or deleted.
  - The session has no viewers.
  - It has a launch that is ready, not stopping, and has no outstanding
    commands (`len(l.commands) == 0`), which covers input, deliveries,
    answers, and configuration in flight.
  - The launch has no pending question (`l.attention`), and the session's
    saved `Activity` is `Idle` with no `InputPending`.
  - The session has no queued messages.

  An open offer does not block parking. Hypercube answers offers itself, the
  answer arrives as a queued message, and that message wakes the parent.
- **Idle timer.** `idleTimeout = 15 * time.Minute` is a `Runtime` field, set
  in `New`, which tests shorten like `startTimeout`. `settleLocked(id)` runs
  wherever a parking blocker can be removed. When the session is parkable, it
  replaces the session's timer with a fresh one; otherwise it stops and
  removes it. It runs:
  - after every saved event (`eventLocked`), which covers a finished turn and
    a question answered or closed without an Idle event;
  - after every command reply (`apply`), which covers start acceptance,
    delivered messages, prompts, answers, and configuration;
  - when a session's last viewer leaves;
  - for each attached launch after `Restore`.

  A launch failure leaves no launch to park, so a stale timer finds nothing
  to do. Becoming busy needs no hook: the timer callback calls
  `park(id, timer)`, which ignores a replaced or removed timer and does
  nothing unless the session is parkable, and the next blocker removal arms a
  fresh timeout. Quit and Detach stop the timers.
- **park(id, timer)** marks the session's launch stopping under `mu`, then
  stops it. `endLocked` then records no note and decides no successor, as for
  Stop today. A view, prompt, or message that arrives while the agent stops
  reaches `wake`, which waits for the stopping launch and starts a new one.
- **wake(id)** reads the session, and for a top-level checkout session its
  branch, outside `mu`, as `start` does now. Then, in a loop:
  1. Under `mu`, it refuses closing, ended sessions, and a session or family
     being deleted.
  2. If `id`'s launch is stopping, it captures the launch's `done`, releases
     `mu`, waits on `done`, and retries. `run` ends through `endLocked`,
     which takes `mu`, so `mu` is never held while waiting.
  3. If `id` has a live launch, it returns it.
  4. Otherwise, still under `mu`, it saves a top-level checkout session's
     branch, calls `decide` for `id`, and registers the launch. Registration
     stays under the lock so concurrent wakes register one launch.

  It then starts the launch's reader with `go r.run(l)` and returns the
  launch without waiting. Launch failures appear in the transcript through
  `end`, as they do now.
- **Waiting for start.** `live.started` is closed by `reply` when start
  succeeds, and by `attach` for a launch that was already ready.
  `awaitStart(l)` returns when the launch has started, or with `l.cause` when
  it has ended. `run` already enforces the start timeout. `launch` becomes
  `go r.run(l); return r.awaitStart(l)`, and its scan for the start command
  goes.
- **Delivery wakes.** `Deliver(id)` is the one delivery path for every
  queued message. Under `mu` it kicks a live launch. When the session has no
  launch, or its launch is stopping, it releases `mu` and wakes the session
  if it is not ended and has queued messages. `SendMessage`, `EndChild`, and
  an offer answer already call it or will.
- **Reports and failures wake the parent.** `saveEvent` queues a child's
  reports and failures. `eventLocked` reports whether the event could have
  queued one (an Idle state or a new error item from a child), `apply`
  returns that, and `run`'s reader calls `Deliver(l.root)` after `apply`
  returns, outside `mu`. A parent whose own start failed therefore wakes and
  retries when the next report arrives. `kickFamilyLocked` keeps delivering
  to live members under `mu`, as now.
- **Triggers.**
  - `View` wakes the session. Opening a session whose start failed retries
    it, which replaces the frontend's restart of failed sessions.
  - `Input` calls `wake`, then `awaitStart`, then sends. A prompt to a parked
    or failed session therefore starts it. `Create` stops calling `start` and
    relies on `Input`.
  - `Answer` on an offer calls `Deliver` after releasing `mu`, in place of
    `answerOffer`'s `kickLocked`.
  - `SelectProfile`, `StartProposal`, and the other user actions keep
    requiring a ready launch. The user is viewing the session, which has
    woken it, and the UI still gates them on `ready`.
  - With `AwaitingUser` gone, a woken session delivers its queued messages as
    soon as it is ready.
- **View.** `View(id) (release func(), err error)` counts a viewer in
  `viewers` (by session ID, under `mu`). For a non-ended session it calls
  `wake`. The release function decrements the count and, at zero, calls
  `settleLocked`. The data socket holds one release for its selection, swaps
  it on each selection, and releases it on close. The socket closes on an
  error from `View`, as it does for other `selectSession` errors. Wake
  refusals that are normal for viewing (closing, deleting, ended) return no
  error.
- **Restore.**
  - Holders are adopted as they are now; an ended session's launch is marked
    stopping.
  - A session whose holder was lost stays parked. `endLocked` already clears
    its launch without a successor or note.
  - After the lost launches end, Restore settles every attached launch and
    calls `Deliver` for each non-ended session, so a parked session with
    queued messages wakes.
  - A SIGINT quit leaves every session parked. That replaces the old
    awaiting-user restart.
- **Shells are independent of agents.**
  - `Shell` admits any non-ended session that is not being deleted. It no
    longer checks whether the session is running.
  - Parking leaves shells running. Archive, Delete, EndChild, and Quit end
    them as they do now.
  - Restore keeps and adopts the shell holders of every non-ended session.

  Without this, parking would kill a shell the user left running, such as a
  dev server.
- **Archive** keeps its preflight and cleanup. It calls `selectLaunches` for
  the family, as Delete already does, in place of `stopLocked`. `endLocked`
  and `endArchived` clear attention.
- **Unarchive(id)** sits beside `Archive` in `removal.go`. It claims
  `r.deleting[id]` like `removeFamily`, and refuses a child or a session that
  is not archived. For a worktree session (`Base != ""`), it runs
  `workspace.AddWorktree(workspacePath, s.Dir, s.Branch)`, which creates the
  same branch name from the checkout's HEAD. It then sets `Base` to
  `workspace.Head(s.Dir)` and calls `refreshMerges`. It clears `Ended` and
  leaves the children ended. The session stays parked until opened or sent a
  message. A Git failure, such as an existing branch of that name, is
  returned unchanged.
- **Saved state.** `migrate` drops the `running` and `awaiting_user` columns
  when they are present, using `ALTER TABLE ... DROP COLUMN` with the
  `pragma_table_info` checks that `addChildColumns` uses. The final attention
  cleanup becomes
  `UPDATE sessions SET attention = 0 WHERE attention = 1 AND (ended = 1 OR launch = '')`.
  `convertLegacy` stops inserting the two columns. Sessions the user had
  stopped have no launches, so they come back parked. Sessions that were
  running come back awake if their holders survived.
- **Sidebar.**
  - The Sessions view lists every non-archived top-level session newest
    first, with all of its children. Ended children appear dimmed, as stopped
    ones are now.
  - Status dots and dimming key on `ended`.
  - The archived family's menu has Unarchive and Delete Session.
  - A top-level session's menu has Rename, the open items, Archive Session,
    and Delete Session.
  - ⌘1–9 follow the listed order.
- **Restart of active sessions.** No plugin, holder, MCP bridge, or launch
  argument changes. `make install-service` restarts the app with Detach, and
  Restore adopts every running agent, which then parks after 15 idle minutes.
  Agents started before the install keep their old instructions and tool
  descriptions, which mention running and stopped recipients, until they next
  park and wake. No manual restart is needed.

## Implementation plan

1. `internal/store`:
   - Remove `Session.Running`, `Session.AwaitingUser`, and `Tx.Running`.
   - Remove the two columns from `schema`, `AddSession`, `SaveSession`,
     `sessionColumns`, `scanSession`, and `convertLegacy`'s insert.
   - Add the column drop to `migrate` and rewrite its attention cleanup as
     decided.
2. `internal/session/runtime.go`: add `idleTimeout`, `viewers map[string]int`,
   and the per-session idle timers to `Runtime` and `New`, with their `mu`
   documentation.
3. `internal/session/launch.go`:
   - Add `live.started` and `awaitStart`, and simplify `launch` to use them.
   - In `endLocked`, drop the `tx.Running` read; `suppress` becomes
     `r.closing || l.stopping`.
   - Delete `sessionLive`.
   - `run`'s reader calls `Deliver(l.root)` when `apply` reports that a
     child's event could have queued a report or failure.
4. `internal/session/commands.go`:
   - `reply` closes `started` on a successful start.
   - Remove `recordRequest`'s `release` parameter and its `AwaitingUser`
     write, and update its callers.
5. Add `internal/session/lifecycle.go` with `View`, `wake`, `settleLocked`,
   `park`, and the parkable check, each with a short comment on its rule.
6. `internal/session/sessions.go`:
   - Delete `Start`, `start`, `Stop`, and `stopLocked`.
   - `Create` saves the session and calls `Input`.
   - `Shell` admits non-ended sessions.
   - `Quit` and `Detach` stop the idle timers.
   - Update the `Quit` comment.
7. `internal/session/input.go`: `Input` wakes, awaits start, then sends.
   `Answer` calls `Deliver` after an offer answer.
8. `internal/session/deliver.go`, `internal/session/offers.go`,
   `internal/session/messages.go`:
   - `Deliver` wakes a session without a running launch that has queued
     messages.
   - `next` drops the `AwaitingUser` and `Running` checks.
   - `answerOffer` no longer kicks.
   - `SendMessage` refuses only ended recipients.
   - Update the doc comments.
9. `internal/session/event.go`: `eventLocked` and `apply` call
   `settleLocked` after saving, and report whether a child's event could have
   queued a message for its parent.
10. `internal/session/restore.go`: `restoreFamily` as decided, with no
    relaunch of lost members, and `Restore` keeps shell holders of every
    non-ended session.
11. `internal/session/removal.go`: Archive uses `selectLaunches`. Add
    `Unarchive`.
12. `internal/mcp/mcp.go`, `internal/report/report.go`:
    - `listSessions` lists every non-ended session in scope.
    - The `send_message` and `list_sessions` descriptions and the coordinator
      instructions drop "running" and "stopped". Ended recipients are still
      refused.
13. `internal/ui`:
    - `actions.go` removes `start` and `stop` and adds `unarchive`.
    - `socket.go` holds a `View` release per selection.
    - `snapshot.go` drops `Running`. `Ready` becomes
      `!s.Ended && s.Ready != 0`, and the `showSession` comment is updated.
14. Frontend:
    - `types.ts` drops `running`.
    - `selection.ts` deletes `stopped`, `hasRunningAgent`, `runningFirst`, and
      `failed`. `receivedWorkspaces` drops the deselect-on-stop branch and
      falls back to the first non-ended session.
    - `session-actions.ts` deletes `startSession` and `stopSession`, adds
      `unarchiveSession`, and `chooseSession` only selects.
    - `sidebar-menus.ts` builds the menus as decided.
    - `sidebar.tsx` removes `shownStopped` and `stoppedItems`, and lists
      sessions as decided.
    - `session-item.tsx` keys on `ended`.
    - `shortcuts.ts` drops `runningFirst`.
    - `composer.tsx`, `app.tsx`, `merge-button.tsx`, and `drafts.ts` drop
      their `running` and `exited` checks.
    - The `terminal-panel.tsx` comment says shells end when the session is
      archived or deleted.

## Test plan

- Session tests (`internal/session/lifecycle_test.go`, short `idleTimeout`):
  - Releasing the last view of an idle session parks it after the timeout:
    its agent's process ends, `Launch` is cleared, and the transcript gains
    no error note.
  - Viewing a parked session wakes only that session, and it resumes its
    saved conversation.
  - A session does not park while it is viewed, busy, or has an open
    question, input pending, a queued message, or a delivery in flight.
    Viewing a parent does not keep its idle child awake, and a busy child
    does not keep its idle parent awake.
  - A question closed with no Idle event arms a fresh timeout, and the
    session then parks.
  - `SendMessage` to a parked top-level session wakes it and delivers the
    message. An offer answer to a parked parent wakes and delivers.
  - A child's report to a parked parent wakes the parent and delivers the
    report.
  - `Input` to a parked session wakes it, waits for start, and sends the
    prompt. `Input` to a session whose start fails returns the start error.
  - A message queued, or a view taken, while a park is stopping the agent
    leads to a wake once the stop finishes.
  - A shell survives parking and ends on Archive and Delete.
  - `Unarchive` of a checkout session returns it to the list, and its next
    `Input` resumes the conversation.
  - `Unarchive` of a worktree session recreates the worktree on its branch at
    the checkout's HEAD and updates `Base`, and its children stay ended.
  - `Unarchive` refuses a child and a session that is not archived.
- Restart tests (`restart_test.go`):
  - A session whose holder was lost stays parked. With a queued message, it
    wakes and delivers.
  - An adopted idle session parks after the timeout.
  - Replace `TestRestartAllLostAwaitsUser` and
    `TestRestoredSessionHoldsMessagesUntilThePrompt`.
  - Delete `TestStopClearsTheFamilysAttention` and
    `TestShutdownKeepsTheRunningMarkAndRestoreResumes`.
  - Tests that used `Start` or `Stop` to reach a running or stopped state use
    `Input` or `park`.
- Store: a database with `running` and `awaiting_user` opens without them, and
  attention is cleared for sessions without a launch.
- `cmd/hypercube/integration_test.go`:
  - After SIGTERM, the agents are adopted.
  - After SIGINT and a fresh start, the sessions stay parked (no new start)
    until input wakes them.
  - Remove the `AwaitingUser` checks.
- Frontend tests (`sidebar.test.tsx`, `session-actions.test.ts`,
  `selection.test.ts`, `composer.test.tsx`, and fixtures that set `running`):
  - No Show Stopped item.
  - Every non-archived family and all of its children are listed.
  - Menus match the decisions, including Unarchive.
  - Choosing a session posts no start.
  - The composer can send to a session whose activity is `exited`.

## Documentation updates

- `docs/usage.md`:
  - **Terminal and browser:** a shell ends when its session is archived or
    deleted, or the backend ends.
  - **Find a conversation:** remove the stopped-session sentences; ended
    children appear dimmed.
  - **Resume and cleanup:** Hypercube starts a session when you open it, send
    it a prompt, or a message reaches it, and ends its agent after 15 idle
    minutes when nobody views it. After a backend restart, Hypercube
    reconnects to surviving agents, and other sessions start when next used.
    Open a session or send it a prompt to retry a failed start.
  - Cleanup table: replace the **Stop** row with **Unarchive** (conversation
    returns to the session list; a worktree session gets its worktree back on
    a new branch from the checkout's HEAD).
  - Mention Unarchive with the Archived tab.
- `docs/introduction.md:84-89`: a parent can message its children and other
  top-level sessions; ended sessions cannot receive agent messages. Each
  session starts when it is used and stops after it has been idle.
