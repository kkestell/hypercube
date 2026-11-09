# Child sessions with automatic coordination

## Goal

Give Hypercube children the behavior described for Claude Desktop: fresh or
forked context, automatic reports containing the first 400 characters of each
successful final response, parent continuation after all children are idle,
and proposed tasks the user can start from a card. Starting a child directly
requires the agent to identify an explicit user request. Agents are instructed
to propose children from automatically triggered turns. Keep the existing choice of models, harnesses, and working folders.

## Baseline and decisions

Extend the existing session runtime, transcript items, queued messages, and
plugin start arguments. Do not introduce another transport, scheduler, task
database, or approval service. Existing session lifecycle and Git cleanup stay
owned by their current modules.

- Native forks preserve a harness's saved conversation. A fork uses the same
  harness as its parent and may choose a different model. Fresh children can
  use any configured harness.
  Claude supports `--resume ... --fork-session`; Codex supports `thread/fork`;
  Pi supports `--fork`; OpenCode advertises ACP `session/fork` but requires the
  source folder. Refuse unsupported combinations with a clear error rather
  than silently substituting transcript copying. Forking conversation context
  does not copy uncommitted files into a worktree.
- Forks inherit native context but start with a fresh Hypercube transcript.
  Copying the UI history would create a second, potentially different snapshot
  of a live native turn and is not required for inherited context.
- Successful child finals already arrive as `Event.Final` alongside `Idle`.
  Resolve those IDs after saving the event's items and queue the report in
  that same transaction. Clip by Unicode characters, excluding progress and
  native subagent text. Turns without an identifiable final generate no
  success report; existing failure reporting remains.
- Keep each automatic report in its own queued row until delivery. Ordinary
  explicit messages can pass blocked report rows. Once the parent and all live
  non-ended children are idle, take one immutable batch, combine its rows and
  record its input command in the same transaction. Never append to a batch
  referenced by an in-flight command. New reports get new rows. Starting
  children and child questions block reports; children whose processes have
  exited without a successor do not. Reconsider delivery on family idle,
  acceptance, exit, and end events. No pending report means no automatic wake.
- The agent interprets whether the user explicitly requested delegation; the
  harness protocols cannot correlate every tool call with its causal input.
  Latest-input tracking would falsely claim to enforce turn provenance and is
  omitted. Direct starts trust the agent to identify explicit requests; other
  starts require clicking a proposal. Click authorization is independent.
- A proposal is a host-owned transcript item containing the child settings
  and brief. It has Start and Dismiss actions. Start uses the saved settings
  through the existing child creation path; it is the user's authorization
  and does not depend on the parent's latest input origin. Neither pending
  proposals nor dismissal block or wake the agent. Completed cards name the
  child. A failed start leaves the proposal actionable and shows the error.

## Related code

- `internal/session/children.go`, `input.go`, `commands.go`, `event.go`,
  `deliver.go`, `launch.go`, `removal.go` — creation, completion,
  input delivery, and lifecycle.
- `internal/store/store.go`, `migrate.go` — saved sessions and queued messages.
- `internal/harness/protocol.go` and `plugins/{claude,codex,opencode,pi}/` —
  native start/resume and final response signals.
- `internal/mcp/mcp.go`, `internal/report/report.go` — tool schema and agent
  instructions; replace the requirement to explicitly report every success.
- `internal/ui/actions.go`, `web/transcript/` — proposal actions and display.

## Implementation plan

1. Add the fork source and pending-input state to sessions, and distinguish
   automatic report messages from ordinary messages. Migrate the preceding
   schema with defaults that preserve its saved data. Keep SQL scan/write
   definitions together and update the existing upgrade tests.
2. Extend plugin start arguments with an optional native fork source, used
   only until the child has a conversation of its own. Implement native forks
   in each plugin and verify the exact API/CLI arguments. Validate same-harness
   before launching. The OpenCode plugin enforces its folder restriction;
   an allocated worktree is cleaned up through the existing failed-start path.
3. Extract child request validation/creation so direct starts and approved
   proposals use one path. Add fresh/forked context to the MCP arguments;
   record proposals when `requested` is false and document the boundary for
   direct starts as decided by the user. Do not copy the UI transcript.
4. Record automatic reports at successful child completion and batch them in
   the existing queue. Add the all-idle delivery condition without delaying
   explicit messages. Use the same condition after ending a child. Split
   report coordination out of `event.go` so event application remains readable.
5. Add the proposal transcript type and a small stock-component card, with
   Start and Dismiss handled by session HTTP actions. Read saved proposals on
   the backend; never accept replacement child settings from the action.
   Render automatic report delivery as a system notification rather than a
   user-authored bubble.
6. Rewrite agent instructions around explicit user authorization, optional
   proposals, automatic success reports, and optional explicit coordination.
   Update the existing guides and protocol description where behavior changes.

## Test plan

- A successful child's final reaches its parent without `send_message`;
  progress and nested-agent text do not. Unicode clipping keeps 400 characters.
- With two working children, the first report waits; the last completion
  delivers one batch. Parent work, open child questions, queued child input,
  and an ended child affect delivery correctly. Replayed events do not report
  twice through the existing checkpoint mechanism.
- Explicit messages still arrive independently. Automatic report handling
  follows the chosen direct-start policy; do not claim verified turn attribution.
- A proposal starts no process, survives transcript reload, starts only from
  its user action, cannot start twice, and remains usable after a failed start.
  Browser tests exercise the card's visible actions and reported errors.
- Each plugin opens an independent native conversation with the selected
  settings, reports the new saved ID, and resumes that ID after restart.
  Different-harness forks fail before filesystem changes; native folder
  restrictions fail through the existing launch cleanup. Forked transcript
  history stays native; the child UI transcript begins with its assignment.
- Upgrade the previous schema twice without losing sessions, messages, or
  history. Run the repository checks after building generated UI assets, and
  open the development browser view for the user's visual check.

## Documentation updates

Update `docs/introduction.md`, `docs/usage.md`, and `docs/harness-support.md`,
plus the existing plugin protocol documentation if it lists start arguments.
Loading changed plugin arguments and instructions requires restarting active
sessions after installation.
