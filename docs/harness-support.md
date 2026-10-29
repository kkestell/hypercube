# Harness support

`harness::command(exe, agent, state_path, prompt)` builds a `Launch`: the
`argv`, the `env` the host adds to its own, and the `cwd` (the agent's
directory). `exe` is the `hypercube` executable, which each harness runs as its
MCP server and hook command. The harness inherits the host's environment,
including `HOME` and any credential environment variables, with the launch's
`env` entries added. The pty sets `TERM=xterm-256color` and
`COLORTERM=truecolor`.

A non-empty `prompt` starts the first turn. Workers launch with
`Check your Hypercube messages.`; the orchestrator opens ready for user
input.

Each `Launch` carries two identity environment variables, which the harness's
child processes inherit:

- `HYPERCUBE_AGENT_ID`: the agent's ID.
- `HYPERCUBE_STATE`: the path of the workspace's state file.

They are the agent's identity for `hypercube mcp`, `hypercube hook`, and
`hypercube wait`.

Hook events are `SessionStart`, `UserPromptSubmit`, `PostToolUse`, `Stop`,
`StopFailure`, and, for Claude Code only, `PermissionRequest`.
`hypercube hook <event>` reads the harness's hook JSON on stdin and writes the
response on stdout. `hookSpecificOutput.additionalContext` carries text for the
model, and `decision: "block"` with `reason` keeps an agent running with a
queued message.

- `SessionStart` supplies the agent's identity, working directory, team, and
  tools. The orchestrator also gets the configured worker profiles and team
  guidance.
- `UserPromptSubmit` delivers queued messages, saves a non-empty harness
  `session_id`, and marks the agent busy.
- `PostToolUse` delivers queued messages during a turn.
- `Stop` delivers queued messages as a block reason, or marks the agent idle
  when its queue is empty.
- `StopFailure` marks the agent idle without taking its queued messages. A
  worker's failure queues a message from that worker to its team's
  orchestrator, stating that its turn failed and including the error text,
  trimmed to 2,000 characters. An orchestrator's failure queues no message.
- `PermissionRequest` sets the agent's attention flag, which puts a red dot
  on its workspace's tab in the bar, and writes nothing, so the user decides.

Each harness runs `hypercube wait` in the background when a session starts
and after each turn's stop. When the agent is idle with messages queued, the
waiter writes them to stderr and exits with status 2, and the harness starts a
turn with them.

Saved session IDs are passed to the harness on subsequent launches. The
session ID is recorded at the first prompt, when the harness has a
conversation to resume.

## Claude Code

```
claude --mcp-config <json> --settings <json> --model <model> --effort <effort>
  --permission-mode auto [--resume <session>] [prompt]
```

- `--mcp-config` registers the `hypercube` stdio server: `exe mcp`, with
  `HYPERCUBE_AGENT_ID` and `HYPERCUBE_STATE` in its environment.
- `--settings` carries a `hooks` entry for each event running
  `'<exe>' hook <event>`, and a `statusLine` running `'<exe>' statusline`,
  with `--limits` for the orchestrator. The status line shows the model, the
  effort while thinking is on, and the context window's tokens in use, size,
  and percentage; `--limits` adds bars for the 5-hour and 7-day usage limits.
- The `SessionStart`, `Stop`, and `StopFailure` entries also run `'<exe>' wait`
  with `asyncRewake` and a 20-day `timeout`. Claude Code runs it in the
  background, and when it exits with status 2, wakes the session with its stderr
  as a system reminder, even when the session is idle. The prompt being typed
  stays in the editor.
- `StopFailure` runs when a turn ends due to an API error. Its input carries
  the error type in `error`, with optional `error_details` and
  `last_assistant_message`. The failure message uses the first non-empty
  error details, assistant message, or error type, or just the failure notice
  if none is present.
- `--permission-mode auto` starts the session in auto mode. A tool call auto
  mode does not allow opens an approval dialog, after the `PermissionRequest`
  hook.
- No hook runs when the user answers the dialog with a denial. Claude Code
  runs the status line when the dialog is answered either way, and not while
  it is open, so `'<exe>' statusline` clears the attention flag.
- `--resume` is present when the agent has a saved session ID.

Claude Code handles folder trust in its pane. Worktree workers live under
`<checkout>/.hypercube/<name>`, within the checkout's trusted folder. A worker
in another directory uses that directory's trust decision.

## OpenCode

```
opencode --standalone --auto --session <session> [--prompt <prompt>]
```

OpenCode 2.x is required. `--standalone` gives each agent a private server
that reads the launch's configuration, and `--auto` makes the TUI approve
every permission request itself, so an OpenCode agent never waits for an
approval. The launch adds three variables to the identity
ones:

- `OPENCODE_CONFIG_CONTENT`: the configuration. It sets the model, the `build`
  agent's model with the effort as its variant (`<model>#<effort>`), the
  `hypercube` local MCP server, `exe mcp` with `HYPERCUBE_AGENT_ID` and
  `HYPERCUBE_STATE` in its environment and Code Mode off so its tools are
  called directly, and the Hypercube plugin.
- `OPENCODE_CLI_CONFIG_CONTENT`: the TUI settings, merged over the user's
  `cli.json`. It turns session tabs off, so the pane shows only the agent's
  session.
- `HYPERCUBE_SESSION_ID`: the session the launch opens.

The session is the agent's saved session ID or, before one is saved,
`ses_hypercube_<agent ID>`. OpenCode creates a session with a missing ID on
its first prompt, so the plugin knows the agent's session before it exists.

The plugin is written to `opencode-plugin/index.js` in the state directory,
with the `exe` path embedded, each time an OpenCode agent launches; an
up-to-date copy is left untouched. It calls `exe hook` for top-level sessions
only, since subagent sessions have a parent:

- `session.hook("prompt")`: SessionStart the first time the loaded plugin
  sees a session, then UserPromptSubmit on every prompt. The combined context
  is appended to the prompt text. Before SessionStart, the plugin refuses the
  prompt if the session's model is not the requested one, since OpenCode
  falls back to a recent model for an unavailable ID.
- `tool.hook("execute.after")`: PostToolUse for a completed tool, appended to
  its result.
- The `session.execution.succeeded` and `interrupted` events:
  Stop. A block reason is sent back with `session.prompt`, which starts a new
  turn. Otherwise the plugin starts a waiter.
- `session.execution.failed`: StopFailure, with `data.error.type` as `error`
  and `data.error.message` as `error_details`, then a waiter. An interrupted
  turn, such as the user pressing Esc, sends no failure notice.

The plugin also starts a waiter when it loads if the launch's session exists.
A new session gets its first waiter at the end of its first turn. When a
waiter exits with status 2, the plugin sends its stderr with `session.prompt`.
The server queues the prompt and starts a turn, and text being typed in the
TUI's editor stays there. The prompt runs the `prompt` hook like any other.
The plugin's cleanup kills the current waiter.

Each hook call sends `{ "session_id": "<session ID>" }` on stdin, with
`error` and `error_details` added for StopFailure.
