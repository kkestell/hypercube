# Harness support

`harness::command(exe, agent, state_path, prompt)` builds a `Launch`: the
`argv`, the `env` the host adds to its own, and the `cwd` (the agent's
directory). `exe` is the `hypercube` executable, which each harness runs as its
MCP server and hook command. A non-empty `prompt` starts the first turn; the
host passes the nudge for workers. `harness::label` is the pane label: name,
harness, model, effort, and directory with the home directory written as `~`.

Every launch sets two environment variables, which the harness's child
processes inherit:

- `HYPERCUBE_AGENT_ID`: the agent's ID.
- `HYPERCUBE_STATE`: the path of the workspace's state file.

They are the agent's identity for `hypercube mcp` and `hypercube hook`.

Hook events are `SessionStart`, `UserPromptSubmit`, `PostToolUse`, and `Stop`.
`hypercube hook <event>` reads the harness's hook JSON on stdin and writes the
response on stdout. `hookSpecificOutput.additionalContext` carries text for the
model, and `decision: "block"` with `reason` keeps an agent running with a
queued message.

## Claude Code

```
claude --mcp-config <json> --settings <json> --model <model> --effort <effort>
  --dangerously-skip-permissions [--resume <session>] [prompt]
```

- `--mcp-config` registers the `hypercube` stdio server: `exe mcp`, with
  `HYPERCUBE_AGENT_ID` and `HYPERCUBE_STATE` in its environment.
- `--settings` carries a `hooks` entry for each event running
  `'<exe>' hook <event>`, and a `statusLine` command, `'<exe>' statusline`, that
  saves Claude Code's usage limits for the host's status bar.
- `--resume` is present when the agent has a saved session ID.

The checkout must be trusted in Claude Code. A directory worker's directory
must be trusted too, or its pane shows the trust dialog.

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

- `before_agent_start`: SessionStart on the first run of each session, then
  UserPromptSubmit on every run. The combined context becomes a visible custom
  message sent to the model as user text.
- `tool_result`: PostToolUse, appended to the tool result. Nested tool calls
  with `parentToolCallId` are skipped, since they never enter the transcript.
- `agent_settled`: Stop, after retries, compaction, and queued work finish. A
  block reason is sent back with `pi.sendUserMessage`, which starts a new run.
