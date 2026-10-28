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

They are the agent's identity for `hypercube mcp` and `hypercube hook`.

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

Saved session IDs are passed to the harness on subsequent launches. The
session ID is recorded at the first prompt, when the harness has a
conversation to resume.

## Claude Code

```
claude --mcp-config <json> --settings <json> --model <model> --effort <effort>
  --dangerously-skip-permissions [--resume <session>] [prompt]
```

- `--mcp-config` registers the `hypercube` stdio server: `exe mcp`, with
  `HYPERCUBE_AGENT_ID` and `HYPERCUBE_STATE` in its environment.
- `--settings` carries a `hooks` entry for each event running
  `'<exe>' hook <event>`, and a `statusLine` running `'<exe>' statusline`,
  with `--limits` for the orchestrator. The status line shows the model, the
  effort while thinking is on, and the context window's tokens in use, size,
  and percentage; `--limits` adds bars for the 5-hour and 7-day usage limits.
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

Each Pi hook call sends `{ "session_id": "<current session ID>" }` on stdin.
The extension's per-session SessionStart tracking lasts for the loaded
extension; a new process or extension reload starts fresh tracking.
