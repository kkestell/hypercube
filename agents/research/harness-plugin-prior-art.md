# Prior art for harness plugins

Projects that run several coding-agent harnesses, protocols that connect a
client to an agent, and conventions for executable plugins. Collected on
2026-10-30 from four passes: a prior-art survey
(`prior-art-survey-1.md`), a review of `agents/agent-harness-plugins.md`
against the sources (`prior-art-review-1.md`), and the prior-art sections of
the two independent designs (`agents/agent-harness-plugins-sol.md` and
`agents/agent-harness-plugins-opus.md`).

Tags: **[V]** read in the project's documentation or source, **[I]**
inferred.

## Orchestrators and terminal managers

### Herdr

Docs: https://herdr.dev/docs/integrations/, https://herdr.dev/docs/agents/,
https://herdr.dev/docs/add-herdr-support/, https://herdr.dev/docs/socket-api/,
https://herdr.dev/docs/plugins/

- **Integration.** Built-in support for named agents.
  `herdr integration install <agent>` writes hooks into the user's global
  harness configuration (`~/.claude/settings.json`, `~/.codex/hooks.json`,
  Pi extensions, OpenCode plugins) [V].
- **Self-reporting.** Any program can report its state, session ID, and
  resume command with `herdr pane report-agent`, found through
  `HERDR_BIN_PATH`, `HERDR_PANE_ID`, and `HERDR_SOCKET_PATH`. A `--seq` that
  must increase makes Herdr ignore stale reports. Reports run in the
  background, failures are ignored, and outside Herdr the command does
  nothing [V].
- **States.** `idle`, `working`, `blocked`, plus `done` and `unknown` in the
  socket API [V].
- **Screen detection.** Agents that do not report are read from the screen
  with detection manifests. Manifests can override rules for known agents,
  but a new agent detected from the screen needs a Herdr release. Blocked
  detection is "deliberately strict" because "automation that trusts a
  mistaken idle state can type into an approval dialog" [V].
- **Hooks used for identity only.** For Claude, Codex, and others, Herdr
  uses hooks for identity and keeps screen detection for state, because hook
  coverage of interrupts and permission cancellation is incomplete [V].
- **OpenCode.** Herdr tracks the selected root session and its descendants,
  pending forms and permissions, retries, and several panes sharing one
  server [V].
- **Delivery.** `agent prompt` types into the pane, refuses when the agent is
  blocked, and can submit and wait for the turn to finish [V].
- **Plugins.** Workflow plugins are manifests plus executable commands that
  call the Herdr CLI [V].

### agent-deck

https://github.com/asheshgoplani/agent-deck

- Built-in tool list, plus custom tools in `config.toml`. Claude and Gemini
  have full support; Codex and Pi use hooks or extensions installed with
  `agent-deck` commands [V].
- State is `running`, `waiting`, `idle`, or `error`, from hooks where
  installed and from the screen otherwise, which the README calls "slower and
  occasionally wrong" [V].
- `session send` delivers through native queueing for Claude and OpenCode 2,
  and otherwise waits for the turn to end. When OpenCode delivery cannot be
  confirmed, it reports that and does not fall back to typing [V].
- One hook handler decodes Claude, Codex, Pi, and Hermes payloads into one
  struct, and guards Stop-hook inbox drains against loops with
  `stop_hook_active` [V].
- The README warns that globally enabled channel plugins leak pollers into
  every session [V].

### Vibe Kanban

https://github.com/BloopAI/vibe-kanban, `crates/executors`. The README says
the project is being sunset [V].

- A Rust trait, `StandardCodingAgentExecutor`, with one implementation per
  harness in a `CodingAgent` enum: Claude Code, Amp, Gemini, Codex, OpenCode,
  Cursor, Qwen Code, Copilot, Droid. Methods include `spawn`,
  `spawn_follow_up(session_id)`, `normalize_logs`,
  `default_mcp_config_path`, and `get_availability_info`, which reports
  login detected, installation found, or not found [V].
- Harnesses run headless; Claude runs with `--output-format=stream-json`.
  A follow-up is a new process with `--resume <id>` [V].
- One `ExecutorApprovalService` serves every harness: `create_tool_approval`
  returns an ID, `wait_tool_approval` blocks, with cancellation tokens and a
  timeout, and a separate path for questions. The no-op approval service
  approves tools. Its ACP translation maps "approved" to an allow-once option
  and errors when the agent offers none [V].
- Each harness has its own MCP configuration key (`mcp_servers`,
  `amp.mcpServers`, `mcp`, `mcpServers`) [V].

### Claude Squad

https://github.com/smtg-ai/claude-squad

- Runs any command in tmux, one session and worktree per agent, with
  profiles [V].
- Detects prompts by matching each harness's dialog text on screen, such as
  "No, and tell Claude what to do differently", and changes by hashing the
  pane [V].
- Sends input with tmux keys. `-y` (AutoYes) accepts prompts by pressing
  Enter [V].

### ccmanager

https://github.com/kbwo/ccmanager

- One state detector per harness over the terminal buffer, for about ten
  harnesses. Claude's detector waits 1.5 s before reporting idle, because
  Claude "sometimes appears idle in terminal output while still actively
  processing" [V].
- Runs user commands when an agent's state changes [V].
- Experimental automatic approval by a language model or a user script,
  falling back to the user on error or timeout; any key cancels it [V].
- Restore reruns a saved command; it does not resume the conversation [V].

### cmux

https://github.com/manaflow-ai/cmux, https://www.cmux.dev/docs/api

- A terminal that turns OSC 9, 99, and 777 notifications and `cmux notify`
  into attention marks [V].
- `cmux hooks setup` installs hooks globally. A wrapper for Claude [V].
- `surface resume set --shell <cmd>` binds a resume command to a pane;
  restoring a layout does not restore processes [V].
- The socket API can send text and keys to a pane [V]. "cmux is a primitive,
  not a solution" [V].

### Uzi

https://github.com/devflowinc/uzi

- Agents named on the command line in tmux and worktrees. `uzi broadcast`
  sends a message to every agent with `tmux send-keys` [V].
- `uzi auto` presses Enter whenever the screen shows "Do you want to",
  "Allow command", and similar [V].

### Conductor, Crystal, Sculptor, Terragon

- **Conductor** (https://www.conductor.build/docs): runs Claude Code, Codex,
  Cursor, and OpenCode in parallel; each task has a workspace, branch,
  terminal, diff, and review. The integration mechanism is not
  documented [V].
- **Crystal** (https://github.com/stravu/crystal): deprecated in February
  2026 in favor of Nimbalyst. A registry of CLI tool definitions with nine
  capability booleans, managed in process [V].
- **Sculptor** (https://imbue.com/sculptor/): one container per agent, a
  pairing mode that syncs with the local checkout, and saved plans, chat,
  tool calls, and code [V].
- **Terragon** (https://github.com/terragon-labs/terragon-oss): shut down in
  January 2026. Runs Claude headless with `--output-format stream-json` and
  routes approvals through `--permission-prompt-tool` to an MCP tool [V].
  Sandbox copies, handoff to the local checkout, and streamed browser
  state [V].

### Toad and acpx

- **Toad** (https://github.com/batrachianai/toad): a terminal interface for
  several agents at once over ACP, with resume, shell integration, and an
  agent catalog [V].
- **acpx** (https://github.com/openclaw/acpx): persistent sessions for
  several ACP agents, custom adapter commands, permissions, and queues. One
  live owner per session; admission to the queue is acknowledged separately
  from completion [V].

## Protocols

### Agent Client Protocol

https://agentclientprotocol.com/protocol/overview, …/session-setup,
…/tool-calls, …/v1/prompt-turn; registry schema at
https://github.com/agentclientprotocol/registry

- JSON-RPC 2.0 over stdio: `initialize`, `session/new`, `session/load`,
  `session/resume`, `session/prompt`, `session/cancel` [V].
- `initialize` exchanges capabilities; an omitted capability must be treated
  as unsupported, and `session/load` and resume must be checked before use
  [V].
- `session/new` and `session/load` both take `mcpServers` entries of
  `{name, command, args, env}` [V].
- `session/update` streams message chunks, tool calls addressed by ID,
  plans, and diffs. A turn ends with a stop reason: `end_turn`,
  `max_tokens`, `max_turn_requests`, `refusal`, or `cancelled`, and
  cancellation is not an error [V].
- `session/request_permission` blocks until the client answers. It offers
  options of `{optionId, name, kind}` with kinds `allow_once`,
  `allow_always`, `reject_once`, and `reject_always`; the reply is a selected
  option or `cancelled`, and the client must reply `cancelled` when the turn
  is cancelled. The agent, not the client, owns what "always" covers. Agents
  "SHOULD NOT treat an outcome they don't recognize as approval" [V].
- The registry describes an agent as distribution data plus command, args,
  and env; behavior lives in the agent or adapter [V].
- Native ACP: Gemini, Goose, Copilot, OpenCode, Cursor, and others.
  Adapters: `claude-agent-acp` wraps the Claude Agent SDK, `codex-acp` wraps
  the Codex app server, and `pi-acp` runs `pi --mode rpc` and does not pass
  MCP servers through to Pi [V].
- Zed runs a custom agent from `{command, args, env}` [V].

### Codex app server

https://developers.openai.com/codex/app-server/

- Codex's native terminal interface can attach to an app server
  (`codex --remote`). The server offers `turn/start`, `turn/steer`,
  `thread/inject_items`, thread status, approvals, and token usage [V].
  How several clients share one thread, and what happens to a draft, is
  untested [I].

## Plugin conventions

- **git credential helpers** (https://git-scm.com/docs/gitcredentials): an
  executable found by name, the operation as an argument, data on stdin and
  stdout, messages on stderr. Unknown operations and attributes are ignored,
  "leaving room for future operations." No version handshake [V].
- **Docker credential helpers**
  (https://github.com/docker/docker-credential-helpers):
  `docker-credential-<name> get|store|erase|list` with JSON. An unknown
  action is an error, and failures are printed on stdout [V].
- **kubectl plugins**
  (https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/):
  `kubectl-<name>` on `PATH`, arguments and environment passed through.
  `kubectl plugin list` shows resolved paths and warns about shadowed
  duplicates and files that are not executable. Plugins cannot override
  built-in commands [V].
- **HashiCorp go-plugin** (https://github.com/hashicorp/go-plugin): a
  long-running subprocess that prints a handshake line and serves RPC, with
  protocol version negotiation and clear mismatch errors [V].
- **LSP**
  (https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/):
  capability exchange at `initialize`; `processId` makes the server exit when
  its client dies, alongside explicit `shutdown` and `exit` [V].
- **MCP**
  (https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle):
  version and capability negotiation, request timeouts, and stdio shutdown by
  closing stdin, then SIGTERM, then SIGKILL [V]. A server cannot start a turn;
  Claude Code's channels extension adds that, as a research preview behind an
  allowlist (https://code.claude.com/docs/en/channels) [V].
- **asdf and mise** (https://asdf-vm.com/plugins/create.html,
  https://mise.jdx.dev/plugins.html): a plugin is a directory of executable
  scripts; some are required, others optional with documented defaults;
  context arrives in environment variables [V].
- **Neovim remote plugins**
  (https://neovim.io/doc/user/remote_plugin.html): language hosts over
  msgpack-RPC, with a generated manifest so hosts start only when needed [V].

## Features Hypercube lacks

What these projects offer that Hypercube does not, whether or not it
belongs in Hypercube.

| Feature | Where | Touches the harness plugin? |
|---|---|---|
| Prompt an agent from a script or the shell and wait for its turn to finish | Herdr `agent prompt`, agent-deck `session send`, Uzi `broadcast`, cmux socket | Yes: needs a reliable end of turn, and the turn's final message to return a result. |
| Structured view of an agent's work: tool calls, plans, diffs, normalized transcripts | ACP `session/update`, Vibe Kanban `normalize_logs`, Sculptor | Yes, heavily: the plugin would have to translate each harness's transcript. |
| Token usage and cost per agent | Codex app server, harness status lines and stats (see `research/harnesses/`) | Yes: each harness reports usage differently. |
| Login and availability detection | Vibe Kanban `get_availability_info` | Yes: a `check` or `describe` result. |
| Questions from the agent answered in the host's interface | Vibe Kanban question path, ACP permission options | Yes: like approvals, with choices. |
| Automatic approval by policy, a model, or a script, falling back to the user | ccmanager, Claude Squad AutoYes, Uzi `auto` | Only through the approval command; the policy is the core's. |
| Commands run on state changes, and desktop notifications | ccmanager, cmux | No: the core has the state. |
| Per-task review: diff, branch, pull request | Conductor, Vibe Kanban | No: worktrees and branches are the core's. |
| Containers or sandboxes per agent, with handoff to the local checkout | Sculptor, Terragon | Partly: the launch would run inside the container. |
| Agents that integrate themselves without a host release | Herdr `report-agent` | Covered by executable plugins. |
| A list of installed integrations with their paths and problems | kubectl `plugin list`, Herdr explain | Yes: discovery and `describe`. |
