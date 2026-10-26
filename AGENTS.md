# Hypercube

Hypercube runs a team of local coding agents in tmux. Each agent is a native
harness TUI (Claude Code, Codex, or OpenCode) in its own tmux pane. Agents
message each other through an MCP tool and receive messages through harness
hooks. One orchestrator works in the original checkout and manages workers in
Git worktrees.

Kyle is the only user. Favor small implementations that are easy to understand
and change over generalized infrastructure or speculative requirements.

## Principles

- The harnesses own their UI, transcripts, sessions, permissions, usage, and
  native delegation. Hypercube launches them and moves messages between them.
- Hypercube supplies identities, worktrees, messages, and lifecycle tools.
  Agents decide how to divide work, collaborate, integrate, and assess
  completion. There is no task registry, workflow, or reporting protocol.
- Configure harnesses only through per-launch flags, environment, and
  configuration overrides. The user's global and project settings, credentials,
  and `HOME` stay untouched.
- macOS only. Agents and Hypercube are trusted local processes running with the
  user's credentials.
- There is no daemon. Every Hypercube process is short-lived or owned by a
  harness, and they share one state file per workspace.

## Commands

- `hypercube` in a Git checkout attaches to the workspace's tmux session. If no
  session exists, it creates one and launches the orchestrator and
  workers, resuming their saved harness sessions. The orchestrator starts idle.
  Each worker starts with the nudge as its first prompt, so it picks up
  messages queued while it was down.
- `hypercube mcp` is the stdio MCP server. Harnesses launch it; users do not.
- `hypercube hook <event>` is the hook entry point. It reads the harness's hook
  JSON on stdin and writes the harness's hook response on stdout.

## Layout

- One tmux session per workspace, named from the checkout path.
- The session has one window with a pane per agent. The orchestrator's
  pane fills the left 40% and the workers' panes are stacked on the right.
  Each pane's border shows the agent's name.
- Each pane's environment carries `HYPERCUBE_AGENT_ID` and `HYPERCUBE_STATE`.
  The harness inherits them, and so do the `hypercube hook` processes it
  starts. The MCP server configuration also sets them explicitly. That
  environment is the agent's identity.

## State

Each workspace has one JSON file at
`~/.local/state/hypercube/<workspace>.json`, named from the checkout path.
`XDG_STATE_HOME` overrides the parent directory. `HYPERCUBE_STATE` holds the
file's path.

- `agents`: id, name, role (`orchestrator` or `worker`), harness, model,
  effort, working directory, branch, harness session ID, tmux target, and
  activity (`busy` or `idle`).
- `messages`: id, sender, recipient, and text. A message is removed when it is
  delivered.

Every change holds an exclusive `flock` on `<workspace>.lock` while it reads
the file, applies the change, and writes it back through a temporary file and
`rename`.

## Configuration

`~/.config/hypercube/config.yaml` (`XDG_CONFIG_HOME` overrides the parent).
The orchestrator and the worker defaults each need a harness, model, and effort.
A missing value is an error. `create_agent` uses the worker defaults unless the
orchestrator passes its own.

```yaml
orchestrator:
  harness: claude      # claude, codex, or opencode
  model: opus
  effort: high
worker:
  harness: codex
  model: gpt-6-astra
  effort: medium
```

Effort is passed through as Claude Code's `--effort`, Codex's
`model_reasoning_effort`, and OpenCode's model variant.

OpenCode models use its full `provider/model` ID. For example, the OpenRouter
model `meta/muse-spark-1.3-contributor` is
`openrouter/meta/muse-spark-1.3-contributor` in Hypercube. The OpenCode plugin
rejects a different model on the first prompt before delivering queued messages;
OpenCode otherwise silently falls back to a recent model for unavailable IDs.

## MCP tools

- `send_message(to, text)` queues a message and wakes the recipient if it is
  idle.
- `list_agents()` returns the team: id, name, role, harness, activity,
  directory, and branch.
- `create_agent(name, harness?, model?, effort?, message?)`, orchestrator
  only. It creates branch `hypercube/<name>` from `HEAD` in a worktree at
  `<checkout>/.hypercube/<name>`, records the agent as busy, and launches its
  harness in a new pane with the nudge as its first prompt. A `message` is
  queued for the new agent before launch, so the nudge delivers it.
  `/.hypercube/` is listed in the checkout's `.git/info/exclude`. Worktrees
  inside the checkout inherit the folder trust the harnesses already grant the
  checkout.
- `remove_agent(id)`, orchestrator only. It runs `git worktree remove` and
  `git branch -d`, closes the pane, and deletes the agent and its queued
  messages. Git refuses dirty or unmerged work, and that refusal is returned
  to the orchestrator.

## Message delivery

Every message goes into the queue first. Hooks deliver the message text; the
tmux nudge only prompts an idle agent to run a turn.

- **SessionStart:** records the harness session ID and returns the runtime
  facts as additional context: the agent's ID, role, working directory, and
  team, the MCP tools, and how worktrees and Git integration work.
- **UserPromptSubmit:** marks the agent busy and returns queued messages as
  additional context.
- **PostToolUse:** returns queued messages as additional context, so messages
  reach a running turn.
- **Stop:** if messages are queued, blocks the stop with the messages as the
  reason, and the agent continues. Otherwise it marks the agent idle.

A message is delivered, and removed, when a hook returns it. Each message is
shown to the model as `Message from <name> (<id>):` followed by the text.

`send_message` to an idle agent runs `tmux send-keys -t <target>` with the fixed
text `Check your Hypercube messages.` followed by Enter. The target is the
agent's tmux pane ID. A nudge sent while the user has unsent text in the pane
is appended to that text and submits both. The resulting
UserPromptSubmit hook delivers the queue. A duplicate nudge finds an empty
queue and is harmless.

## Harnesses

Each harness has a launch command built from the agent record. It sets the
model, effort, permission bypass, the `hypercube` stdio MCP server, the hooks,
and the resume session ID when one is saved.

- **Claude Code:** `claude --mcp-config <json> --settings <json> --model <m>
  --effort <e> --dangerously-skip-permissions [--resume <id>] [prompt]`.
  `--settings` carries the hooks. The checkout must already be trusted in
  Claude Code; the first launch in a new checkout shows Claude Code's trust
  dialog in the orchestrator's pane.
- **Codex:** `codex [resume] --dangerously-bypass-approvals-and-sandbox
  --dangerously-bypass-hook-trust` with `-c` overrides for `model`,
  `model_reasoning_effort`, `mcp_servers.hypercube`, `hooks.<event>`, and
  `projects={"<checkout>"={trust_level="trusted"}}`, followed by the session ID
  when resuming and the prompt. Codex keys folder trust on the main checkout,
  which covers the worktrees. Codex gives MCP servers a filtered environment,
  so the server config forwards `TMUX` through `env_vars` for `send-keys`.
- **OpenCode:** `opencode --standalone --auto [--session <id>] [--prompt <p>]`
  with `OPENCODE_CONFIG_CONTENT` set to a generated config: the model, the
  `build` agent's model with the effort as its variant, the MCP server, and the
  Hypercube plugin. `--standalone` gives each agent a private server that reads
  that config, and `--auto` bypasses permission prompts. The plugin is written
  to `~/.local/state/hypercube/opencode-plugin/index.js` with the `hypercube`
  path embedded. It calls `hypercube hook` for top-level sessions only:
  - `session.hook("prompt")`: SessionStart on the first prompt of each session,
    then UserPromptSubmit. The returned context is appended to the prompt text.
  - `tool.hook("execute.after")`: PostToolUse, appended to the tool result.
  - `session.execution.succeeded`, `failed`, and `interrupted` events: Stop. A
    block reason is sent back with `session.prompt`, which starts a new turn.

`hypercube hook` writes the Claude Code response format for every harness.
Codex reads the same format, and the OpenCode plugin reads
`hookSpecificOutput.additionalContext` and `reason`.

## To verify

- Nudges hold up when a harness dialog is open.

## Packages

Go. The sole executable is `cmd/hypercube`. Packages under `internal/`:

- `store`: the locked state file, agents, and the message queue.
- `tmux`: sessions, panes, layout, environment, and `send-keys`.
- `workspace`: checkout discovery and worker branches and worktrees.
- `harness`: configuration, launch commands, and hook configuration for each
  harness, and the OpenCode plugin.
- `mcp`: the stdio MCP server and tools.
- `hook`: hook event handling and the runtime facts.

## Smoke testing

For harness smoke tests, use Codex `gpt-6-luna`, Claude `haiku`, and OpenCode
`openrouter/~deepseek/deepseek-flash-latest`.

## Validation

Before pushing, all of this must pass:

```sh
go build ./... && go vet ./... && staticcheck ./... && test -z "$(gofmt -l .)" && go test ./...
```

Use disposable local repositories and isolated `XDG_STATE_HOME` and
`XDG_CONFIG_HOME` for smoke tests, and a separate tmux server: set
`TMUX_TMPDIR` to a temporary directory and unset `TMUX`.
