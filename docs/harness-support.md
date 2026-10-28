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

Hook events are `SessionStart`, `UserPromptSubmit`, `PostToolUse`, and `Stop`.
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
- The `SessionStart` and `Stop` entries also run `'<exe>' wait` with
  `asyncRewake` and a 20-day `timeout`. Claude Code runs it in the background,
  and when it exits with status 2, wakes the session with its stderr as a
  system reminder, even when the session is idle. The prompt being typed stays
  in the editor.
- `--permission-mode auto` starts the session in auto mode.
- `--resume` is present when the agent has a saved session ID.

Claude Code handles folder trust in its pane. Worktree workers live under
`<checkout>/.hypercube/<name>`, within the checkout's trusted folder. A worker
in another directory uses that directory's trust decision.

## Pi

```
pi --extension <path> --model <model> --thinking <effort> --approve
  [--session-id <session>] [prompt]
```

`--approve` trusts project-local resources for the launch. The extension is
written to `pi-extension/index.js` in the state directory, with the `exe` path
embedded, each time a Pi agent launches; an up-to-date copy is left untouched.

The extension registers the `hypercube` MCP server for the session with direct
tool exposure and `HYPERCUBE_AGENT_ID` and `HYPERCUBE_STATE` in its environment.
It maps Pi events onto `exe hook` calls:

- `before_agent_start`: SessionStart the first time each session ID is seen
  by the loaded extension, then UserPromptSubmit on every prompt. The combined
  context becomes a visible custom message sent to the model as user text.
- `tool_result`: PostToolUse, appended to the tool result. Nested tool calls
  with `parentToolCallId` are skipped, since they never enter the transcript.
- `agent_settled`: Stop, after retries, compaction, and queued work finish. A
  block reason is sent back with `pi.sendUserMessage`, which starts a new run.
  Otherwise the extension starts a waiter.
- `session_start`: starts a waiter. `session_shutdown` kills the current one.

When a waiter exits with status 2, the extension sends its stderr with
`pi.sendMessage` as a visible `hypercube` custom message with `triggerTurn`,
which starts a run while the agent is idle and leaves the editor alone.

Each Pi hook call sends `{ "session_id": "<current session ID>" }` on stdin.
The extension's per-session SessionStart tracking lasts for the loaded
extension; a new process or extension reload starts fresh tracking.
