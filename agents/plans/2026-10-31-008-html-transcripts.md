# HTML transcripts through executable harness plugins

## Goal

Replace agent terminals with one HTML chat view for every harness. Remove tmux;
keep xterm.js only for the bottom-panel shell. Keep the
`hypercube-harness-<name>` executable plugin model: native protocols live in
plugins, and neither the core nor the browser names a harness.

Work in six phases: two core phases, then Claude, OpenCode, Codex, and Pi. Each
phase builds and passes `make check`. Core phases use a stand-in plugin; each
bundled harness works again when its phase lands. There is no second,
terminal-based runtime during the transition.

## Related code

- `internal/harness/harness.go` discovers plugin executables and runs their
  one-shot `launch` command.
- `internal/session/session.go` owns session and worker operations, guards them
  with file locks (`store.Lock`), and relaunches agents through tmux.
- `internal/store/store.go` holds agents, native conversation references,
  activity, and queued messages.
- `internal/report/report.go` supplies runtime instructions, message
  attribution, titles, usage, and failure reporting, driven by harness hooks.
- `internal/mcp/mcp.go` runs MCP tools inside each agent's `hypercube mcp`
  subprocess, identified by `HYPERCUBE_AGENT_ID`.
- `internal/ui/ui.go` and `internal/ui/static/index.html` serve the view and the
  tmux control connection; the panel shell is a tmux window.
- `cmd/hypercube/main.go`, `internal/menu/`, and `scripts/dev` define startup,
  quitting, and rebuilds.
- `/Users/kyle/src/gnat/internal/harness/{claude,codex,opencode}.go` and
  `ui/src/transcript.tsx` show native transports, event translation, and
  transcript rendering. Borrow behavior, not structure or permission settings.

## Decisions

### Process ownership

The app owns every plugin process and panel shell. Each plugin keeps its native
harness alive across turns. Closing the browser changes nothing.

Quitting, service restart, and a successful dev rebuild stop the app's
processes. Startup restores sessions marked running, resumes their saved
conversations, and holds queued messages until the user sends a prompt (the
existing gate). Session Stop clears the running mark; app shutdown keeps it.

Menu Quit, SIGINT, and SIGTERM all go through the same Go shutdown: send each
plugin `shutdown`, wait briefly, then kill its process group. Each plugin starts
in its own process group.

Relaunch keeps today's rule: an agent that exits three times within ten seconds
of launch stays exited. Each launch has a generation number; events from an old
generation are ignored.

Because the app becomes the only process that writes the database, session file
locks become an in-process mutex. Never hold it while waiting on a plugin, since
a native request can call back into MCP.

### Plugin protocol

Plugins gain a `serve` command: a long-lived JSONL connection on stdin/stdout,
defined in `internal/harness/protocol.go`. Stderr carries diagnostics.

- Commands carry a request ID: `start` (existing launch information, working
  directory, runtime instructions), `input`, `interrupt`, `answer`, and
  `shutdown`. Each gets a reply with the same ID: success or an error.
- An `input` reply means the native harness accepted the text, including into
  its own queue. An error means it did not, and the input stays queued.
- Events carry the native conversation ID, the main agent's work state
  (working/idle), and transcript items: text, reasoning, tool call and result,
  usage and context, errors, and attention requests. Items have IDs; a later
  event with the same ID replaces or extends the item. Nested items name their
  parent. Native subagents appear as nested items, not Hypercube agents.
- An attention request is a prompt with optional choices and optional free
  text. The browser answers with the request's ID; the plugin maps it to the
  native reply. An answer to a finished request returns an error.
- A malformed record or a reply with an unknown ID ends the connection with a
  clear error; the agent then follows the relaunch rule.

`internal/harness` holds the client only. JSON-RPC shared by OpenCode and Codex
lives in `plugins/internal/plugin/rpc.go`.

### Messages and transcripts

The store gains an ordered transcript per agent. Completed items are written to
the database; in-progress items stay in memory and are published as they
stream. Queued messages keep their current table and attribution.

The runtime sends one queued message at a time and removes it after a
successful reply. Messages arriving while an agent works are delivered at once,
as today's tool-hook delivery does; the plugin uses native steering or queueing.
An interrupted turn stays in the transcript and is not resubmitted. A failed
resume is shown as an error; the runtime never starts a fresh conversation in
its place.

Keep first-prompt titles, manual titles, unseen lead replies, worker status,
context and cost headers, cross-session lead messaging, and worktree
safeguards.

### Browser

The existing state WebSocket also carries the selected session's transcript:
a full snapshot on connect and on selection, then appended and updated items.
Reconnecting means taking a new snapshot.

Leads and expanded workers use one transcript and composer; collapsed workers
keep their status preview. Each agent's draft is kept in browser memory across
incoming messages and selection changes. Stop stays available while an agent
works.

### MCP

`hypercube mcp` stays the stdio server every plugin launches, but becomes a thin
bridge: it forwards each request with `HYPERCUBE_AGENT_ID` to one local app
route. Tool execution moves into the app so MCP and browser actions share one
session owner. Tool names, identity checks, and session scoping are unchanged.

### Panel shell

`internal/terminal` owns one shell PTY per session, started on demand in the
workspace folder, using `github.com/creack/pty`. It survives panel hiding and
browser closure, and ends on shell exit, session Stop or Delete, or app
shutdown. It keeps a bounded buffer of recent output and replays it on connect.
The browser connects through its own WebSocket for input, output, resize, and
exit.

## Implementation plan

### Phase 1 — protocol, runtime, and process ownership

1. Add `internal/harness/protocol.go`; replace launching in
   `internal/harness/harness.go` with the `serve` client. Unported plugins answer
   `serve` with an unsupported-harness error.
2. Add transcript storage to `internal/store/store.go`. Remove pane and waiter
   fields once unused.
3. Add `internal/session/runtime.go`: the app-owned service holding live plugin
   clients, delivery, event handling, relaunch, restore, and shutdown. Replace
   `store.Lock` callers with its mutex.
4. Reduce `internal/report/report.go` to instructions, attribution, titles,
   failure reporting, and applying protocol events. Remove hook-driven delivery.
5. Move tool execution in `internal/mcp/mcp.go` into the app; make
   `hypercube mcp` forward to a route registered in `internal/ui/ui.go`.
6. Wire the service into `cmd/hypercube/main.go`. Start serving HTTP before
   restoring agents. Route menu Quit and signals through shutdown. Remove the
   `agent`, `exited`, `report`, and `wait` commands, `internal/tmux`, and
   `internal/wait`.
7. Tests: protocol client, runtime, store, report, and MCP. Convert
   `cmd/hypercube/integration_test.go` to a stand-in plugin, keeping its
   multi-session, worktree, message, restore, and failure coverage.

### Phase 2 — HTML chat and panel shell

1. Add `internal/terminal/terminal.go` and its test; add `creack/pty` to
   `go.mod`.
2. Replace `/tmux` in `internal/ui/ui.go` with input, answer, interrupt, and
   terminal routes, keeping the host and origin checks.
3. Add `internal/ui/static/transcript.js`: a small DOM renderer with safe
   Markdown, tool details, and attention forms. Model and tool text never
   becomes HTML.
4. Replace agent terminals, tmux parsing, and key forwarding in
   `internal/ui/static/index.html`. Keep the sidebar, colors, split controls,
   worker summaries, and header metrics. Styles go in `internal/ui/style.css`.
5. Update `internal/ui/ui_test.go`; add `transcript.test.js` and its Bun run to
   `make check`.
6. Update `scripts/dev`: stop the app through shutdown and drop tmux setup.
   A failed build still leaves the running app alone.
7. Update `README.md`, `docs/architecture.md`, `docs/installation.md`,
   `docs/usage.md`, `docs/harness-support.md`, and the obsolete package and
   validation facts in `AGENTS.md`.

### Phase 3 — Claude Code

1. Rewrite `plugins/claude/main.go` to drive a persistent `claude -p`
   stream-JSON process. Keep the model, effort, MCP configuration, and auto
   permissions arguments.
2. Handle `can_use_tool` and `AskUserQuestion` as attention requests,
   interrupt, further input, resume, nested tools, usage, and failures. Keep
   stdin open between turns.
3. Rewrite `plugins/claude/main_test.go` with native stream fixtures: streamed
   and final text without duplicates, resume, questions, denied tools, and
   interrupts. Wire reference:
   <https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/_internal/query.py>.
4. Smoke test live in `make dev`; update `docs/harness-support.md`.

### Phase 4 — OpenCode

1. Add `plugins/internal/plugin/rpc.go` and its test. Port
   `plugins/opencode/main.go` to persistent `opencode acp`. Reject a model or
   effort the installed version does not offer.
2. Handle new and resumed sessions, `session/update`, permission requests,
   cancellation, and child sessions. Report idle only after every outstanding
   prompt has replied.
3. First check whether ACP forwards OpenCode's question tool. If not, reduce
   `plugins/opencode/opencode.js` to forwarding those questions as attention
   requests.
4. Rewrite `plugins/opencode/main_test.go` with ACP fixtures; smoke test live
   with a question; update `docs/harness-support.md`. Reference:
   <https://opencode.ai/docs/acp/>.

### Phase 5 — Codex

1. Port `plugins/codex/main.go` to persistent `codex app-server` using the
   shared RPC helper: initialize, thread start and resume, turn start, steer,
   and interrupt, item events, request replies, and usage.
2. Keep the automatic approval reviewer, workspace-write sandbox, network
   access, writable Git metadata, and trust and MCP arguments. Remaining
   approvals, questions, and MCP elicitation become attention requests.
3. Rewrite `plugins/codex/main_test.go` with app-server fixtures, including
   steering rejected after a turn completes and duplicate completed items.
   Smoke test live; update `docs/harness-support.md`. Generate schemas from the
   installed CLI; reference: <https://learn.chatgpt.com/docs/app-server>.

### Phase 6 — Pi and cleanup

1. Port `plugins/pi/main.go` to persistent `pi --mode rpc`, keeping provider,
   model, effort, and saved conversation. Before resuming, check that the saved
   session exists, since Pi otherwise creates a new one under that ID. Reduce
   `plugins/pi/pi.js` to MCP registration and instruction injection.
2. Map steering and follow-up, text and tools, extension dialogs, usage, and
   errors. Report idle on `agent_settled`.
3. Rewrite `plugins/pi/main_test.go` with RPC fixtures; smoke test live.
   Reference:
   <https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md>.
4. Remove the old launch and hook helpers from `internal/harness` and
   `plugins/internal/plugin/plugin.go`. Finish `docs/harness-support.md`.
5. Run a mixed team in a live session: worker creation, messages while working
   and idle, attention requests, worker failure, stop and start, app restart,
   and worker removal.

## Test plan

- A stand-in plugin exercises the protocol end to end: accepted and failed
  input, interleaved events and replies, malformed output, crash and relaunch
  limits, events from an old generation, and attention answers.
- Store and runtime tests cover transcript ordering, item replacement without
  duplicates, the restored-session prompt gate, and failed resume.
- Session and MCP tests cover cross-session lead scope, worktree safeguards,
  creating a worker while a tool runs, and shutdown keeping the running mark.
- Terminal tests use a real PTY running a fixed command: input, resize, output
  replay, shell exit, session Stop, and shutdown.
- Native plugin tests use recorded native protocol fixtures. Live smoke tests
  after each harness phase cover login, permissions, questions, and resume,
  which fixtures cannot. The user judges the browser's appearance.
