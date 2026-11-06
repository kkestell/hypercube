# Child session choices, fork placement, and completed results

## Goal

When the user requests a separate new task, offer **New session / Do it here**
and wait for the answer. Choosing New session authorizes that task; only then
does the agent supply its brief and start the child. Choosing Do it here
starts no child and returns the work to the existing session. Continuing an
existing task, including corrections and small follow-ups, stays here.

Make forks follow the original session's folder setup. Let the original agent
receive completed results while another child waits for the user, naming that
child in the delivered message.

Keep automated-turn guidance, configured harness/model choices, fresh-child
placement, report contents and clipping, and existing suggestion cards. Do not
add proposal expiry, agent tools for proposal management, automatic placement
preferences, or Desktop's permission-mode and turn-authorship machinery.

## Related code

- `internal/mcp/mcp.go` — `create_session` schema and the unchecked
  `requested: true` launch path.
- `internal/session/proposals.go`, `children.go`, `child_options.go` — saved
  user actions, child registration, and working-directory selection.
- `internal/session/input.go`, `launch.go`, `restore.go`, `event.go` — answers,
  pending questions, launch lifetime, and changes that release queued results.
- `internal/session/reports.go`, `deliver.go` — report batching and the
  current requirement that every child be idle without an open question.
- `internal/harness/protocol.go`, `plugins/codex/events.go` — attention items
  and Codex questions that leave the agent working.
- `internal/ui/web/transcript/` — existing pinned question forms, answer
  actions, and saved proposal display.
- `internal/report/report.go` — instructions governing when agents offer,
  propose, start, and resume work.

## Baseline and decisions

### Record the choice in the existing transcript

Reuse attention items, the pinned question form, and the existing answer HTTP
action. Keep child choices in `internal/session/offers.go`, owned by the
runtime. Identify app-owned questions through saved offer metadata, and route
their answers locally rather than sending them to a plugin. There is no new
question service, transport, or task database.

No tool call waits for the answer. The offer is a saved transcript question
that stays open until the user answers it, or until the parent session is
removed; interrupting, stopping, reloading, and backend reattachment leave it
open. The answer reaches the parent as an automatic message through the
existing delivery path, whenever it comes: New session carries the `offer_id`,
and Do it here carries no ID. A late answer therefore authorizes nothing
unexpected. Keep native questions and existing proposals intact.

### Separate offering from supplying the brief

Keep one `create_session` tool with distinct validated forms, replacing
`requested`:

- `initiation: offer` takes a title only, records the app-owned question, and
  returns at once. The agent's instructions tell it to end its turn and wait
  for the answer message. Neither answer creates a child. This preserves the
  agreed order without introducing an empty running session solely to fill it
  later.
- `offer_id` plus the existing child settings and `message` fills the accepted
  task's brief, including the needed background. Offers start fresh. Check the
  owning parent, accepted decision, title, and unused approval before creating
  files or processes. Record consumption and the new child together in the
  existing child-registration transaction. The agent cannot substitute an
  unaccepted offer or start the same offer twice.
- `initiation: own_initiative` keeps today's saved proposal and Start / Dismiss
  behavior. It launches nothing. No suggestion-card
  lifecycle features are added.
- An explicit request to start another session also uses this form. The saved
  proposal already holds the exact settings, its Start action is the user's
  approval, and child registration already claims it atomically and reopens it
  after a cleaned-up launch failure. No separate direct-start approval is added.

This closes the existing unchecked path: no tool form launches a child without
an accepted offer or the user's Start click. Hypercube has no equivalent of
Desktop's app-level classifier and bypass modes, so a direct request needs a
click where Desktop's selected mode might not. Do not describe this as
permission-mode parity. Adding input-authorship tracking or interpreting prompt
text in the app is out of scope.

Move child-tool registration, schema, and handlers out of `mcp.go` into
`internal/mcp/children.go` so the conditional request forms remain readable.
Reject combinations that mix offering, filling an accepted offer, and
proposing. Remove the old boolean from the tool contract;
it must no longer provide a route around an outstanding choice.

### Derive fork placement from the original session

Use the existing top-level session's `Base` to distinguish its managed worktree
from the original checkout; `Branch` also exists for ordinary checkouts and
is not that distinction.

- A fork of a session in the original checkout shares `parent.Dir`, including
  its uncommitted files. This also works outside Git without requiring `dir`.
- A fork of a session in its own managed worktree gets a new managed worktree
  from that session's HEAD, through the existing worktree creation and cleanup
  functions. Uncommitted changes remain in the original working copy.
- New fork requests cannot select `dir`. Fresh children retain their existing
  directory choices and defaults. For already-saved proposals, allow an old
  explicit directory only when it matches the newly required placement;
  otherwise show a clear error rather than silently changing the saved choice.
- Keep same-harness validation and configured model/effort selection. Keep
  OpenCode's evidenced restriction: a fork requiring another folder fails
  clearly and cleans up through the current failed-launch path. Do not silently
  change it to fresh context or a shared folder. Update its obsolete advice to
  use `dir: "."`.

Only correct the existing proposal's fork-folder description where the new
rule would make it inaccurate; do not redesign or pin suggestion cards.

### Distinguish a waiting child from a working child

The plugins' synchronous questions remain open while their session can still
be marked busy. Codex also sends asynchronous questions that do not stop work
(`plugins/codex/events.go:agentMessage`); those must not make a working child
look ready.

Add optional `Async` metadata to attention items and set it on Codex's native
asynchronous questions. The existing question/approval paths in Claude,
OpenCode, Pi, and Codex server requests wait for their response; they keep the
synchronous default. Keep this distinction in the protocol, without checking
harness names or native item-ID conventions in the runtime.

OpenCode can withdraw a form it no longer waits for
(`plugins/opencode/requests.go:withdraw`); its items currently stay open until
the turn ends. Close the withdrawn form's items when withdrawal occurs, so an
open synchronous question always means the agent is waiting for its answer.
Update `TestWithdrawnFormIsRefused` to expect the items closed.

Allow a child with a synchronous main-conversation question to count as waiting
for the user even while its turn is busy. An idle child with an open question
also no longer blocks delivery. Retain checks for startup, pending inputs,
queued messages, and commands still being accepted. A busy child with only an
asynchronous question continues to hold results back. The original agent must
still be idle, have no open question or pending input, and be eligible for
normal delivery.

Return the waiting children's names with the existing readiness decision and
include them in the same report batch. Tell the original agent that those
children need the user and that it may continue other requested work; it must
not answer their questions or work around their approvals. Keep the report
snapshot and command registration together as today. Reconsider delivery when
questions open or close as well as when work ends. No queued result means no
new automatic turn. Do not add polling, a settling timer, or a second queue.

## Implementation plan

1. Add runtime-owned child choices in `internal/session/offers.go` and their
   tests. Answer offers locally in `input.go` and queue the answer message for
   the parent; keep open offers out of launch-scoped pending items, so ending
   or reattaching a launch leaves them open (`restore.go`).
2. Extend `children.go` so accepted offers are checked and consumed during
   child registration. Reuse the existing failure cleanup and proposal-claim
   pattern; an ordinary cleaned-up launch failure leaves the accepted offer
   retryable.
3. Move child-tool handling into `internal/mcp/children.go`, implement the three
   request forms, and update `mcp_test.go`. Keep existing Start / Dismiss HTTP
   actions unchanged.
4. Change fork placement in `child_options.go` and its description in
   `children.go`. Cover original checkout, managed worktree, non-Git folders,
   conflicting directory overrides, and restart of the child's own native
   conversation in `children_test.go` and `reports_test.go`. Update the
   OpenCode restriction message in `plugins/opencode/main.go` and its existing
   test in `main_test.go`.
5. Add the asynchronous-question marker to `internal/harness/protocol.go` and
   `plugins/codex/events.go`; verify its native mapping in the existing Codex
   question tests in `plugins/codex/main_test.go`. Close withdrawn OpenCode
   form items in `plugins/opencode/requests.go` and update its test. Extend the existing harness
   stand-in's question responses to exercise both kinds through the runtime.
6. Update `reports.go`, `deliver.go`, and `event.go` to release completed
   results when the remaining children need the user, including their names.
   Replace the test that requires questions to hold every report, while keeping
   coverage for parent work and actual child work delaying delivery.
7. Rewrite the affected instructions in `internal/report/report.go` around
   the offer/answer/brief order, unchanged proposals, inherited fork placement,
   and continuing with completed results. Keep automated-turn guidance as it
   stands. Correct the fork-folder text in `proposal-item.tsx` and the
   `create_session` tool summary in `transcript-tree.ts`; extend the existing
   transcript interaction tests for the New session / Do it here choices.
   Regenerate the UI distribution through its existing build.

## Test plan

- Through the real MCP handler and runtime, make an offer; the call returns
  at once and the question is saved. Assert no child, process, branch, or
  worktree exists. Answer Do it here: the parent receives a message without an
  ID, and still no child exists. Answer New session in a separate case: the
  parent receives the `offer_id`; supply the brief and observe exactly one
  fresh child with that brief and the selected profile.
- Attempt a fill before acceptance, after Do it here, from another parent,
  and after consumption. None launches a child. An explicit start request saves
  a proposal and launches nothing until Start. `requested: true` cannot
  reproduce the original unchecked start.
- Browser reload, interruption, stopping, and backend reattachment leave an
  unanswered offer open, and an answer given later reaches the parent once it
  runs. Duplicate answers and duplicate fills cannot start another child. Exercise ordinary failed-start retry using
  the existing cleanup behavior, without adding crash-recovery machinery.
- Existing proposal Start / Dismiss behavior and saved proposals still work.
  Use the current tests; do not add expiry, quota, or proposal-management tests.
- Fork a plain checkout with an uncommitted file and verify the child sees
  it. Fork a managed worktree after a commit and verify a separate worktree
  starts at that commit. A non-Git fork shares the folder. A conflicting
  directory or different harness is refused. OpenCode's incompatible native
  fork reports its limitation without a leftover worktree.
- With Alice finished and Bob waiting on a synchronous question, deliver
  Alice's result once and name Bob as needing the user. Leave Bob's request
  open and unanswered. Keep results queued while the parent works, another
  child works, or a busy child has only an asynchronous question. Once that
  work ends, deliver the results normally. Preserve late-arriving report
  snapshots and explicit-message behavior through existing tests.
- The visible New session / Do it here form stays above the composer, records
  the selected answer, and cannot submit twice while answering. Exercise the
  actual live choice and resulting child with the user during implementation
  verification; repository checks alone do not establish this behavior. Leave
  screenshots and visual judgment to the user.

## Documentation updates

- `docs/usage.md` — new-task choice, waiting for the answer, accepted brief,
  and explicit start requests saved as suggestion cards.
- `docs/introduction.md` — inherited fork folders and completed results while
  another agent needs the user.
- `docs/harness-support.md` — fork placement versus OpenCode's native
  restriction, asynchronous questions, and the app-owned child-start check.

Loading the new tool contract and instructions requires restarting active
sessions after installation. Existing sessions and proposals remain saved;
the plan adds optional transcript metadata without a database schema change.
