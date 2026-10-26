# Hypercube

Hypercube runs a team of local coding agents in tmux. Each agent is a native
harness TUI (Claude Code, Codex, OpenCode, or Ox) in its own tmux pane. Agents
message each other through an MCP tool and receive messages through harness
hooks. One orchestrator works in the original checkout and manages workers in
Git worktrees. In a directory that is not a Git checkout, workers work directly
in existing directories.

Kyle is the only user. Favor small implementations that are easy to understand
and change over generalized infrastructure or speculative requirements.

## Principles

- The harnesses own their UI, transcripts, sessions, permissions, usage, and
  native delegation. Hypercube launches them and moves messages between them.
- Hypercube supplies identities, worktrees or worker directories, messages, and
  lifecycle tools.
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

- `hypercube` attaches to the workspace's tmux session. The workspace is the
  Git checkout containing the working directory, or the working directory
  itself when it is not in a Git checkout. If no session exists, it creates one
  and launches the orchestrator and workers, resuming their saved harness
  sessions. The orchestrator starts idle. Each worker starts with the nudge as
  its first prompt, so it picks up messages queued while it was down.
- `hypercube stop` in a workspace kills the workspace's tmux session, which
  ends its agents. The state file, worktrees, and queued messages stay, so the
  next `hypercube` resumes the team.
- `hypercube down` does what `stop` does, then removes each worker's worktree
  and branch and deletes the state file. If any worker has uncommitted changes
  or a branch not merged into the checkout's HEAD, it refuses and changes
  nothing. In a `no_git` workspace it deletes only the state file and leaves
  workers' directories alone.
- `hypercube status` lists every workspace with a state file, most recently
  active first: its directory, whether its tmux session is running, its number
  of agents, when it was created, and when it was last active. Creation is the
  lock file's modification time, since the lock is created with the state and
  never written. Last activity is the state file's modification time, since
  every hook writes the state.
- `hypercube mcp` is the stdio MCP server. Harnesses launch it; users do not.
- `hypercube statusline` is the status line command of Claude Code panes.
  Claude Code passes rate limits only to status line commands. It saves the
  5-hour and 7-day limits in the global tmux option `@hypercube_claude_limits`
  and prints the agent's context usage and branch, with `*` when its checkout
  has uncommitted changes.
- `hypercube limits` prints the status bar text: Claude Code's saved limits and
  the rate limits in the most recently written Codex session log, each with
  its reset time. tmux runs it for the status bar.
- `hypercube hook <event>` is the hook entry point. It reads the harness's hook
  JSON on stdin and writes the harness's hook response on stdout.

## Layout

- One tmux session per workspace, named from the checkout path.
- The session has one window with a pane per agent. The orchestrator's
  pane fills the left 40% and the workers' panes are stacked on the right.
  Pane borders are heavy blue lines, and each pane's top border shows the
  agent's name, harness, model, effort, and working directory in black on
  light blue.
- The session has the mouse and `allow-passthrough` on, so harness
  notifications reach the terminal. Creating the session also turns on the
  tmux server's `extended-keys` (`csi-u`, with `extkeys` for `xterm*`
  terminals), so Shift+Enter reaches the harnesses, and `focus-events`. These
  server options affect every session on that tmux server.
- The status bar is black on light blue, like the pane labels. It shows the
  checkout's path on the left and `hypercube limits` on the right.
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
- `no_git`: set when the first `hypercube` creates the orchestrator in a
  directory that is not a Git checkout. Its workers work directly in existing
  directories, with an empty branch. A state file without it stays in Git mode.

Every change holds an exclusive `flock` on `<workspace>.lock` while it reads
the file, applies the change, and writes it back through a temporary file and
`rename`.

## Configuration

`~/.config/hypercube/config.json` (`XDG_CONFIG_HOME` overrides the parent).
The orchestrator and the worker defaults each need a harness, model, and effort.
`harness` is `claude`, `codex`, `opencode`, or `ox`. A missing value is an error. `create_agent` uses the worker defaults unless the
orchestrator passes its own. The optional `models` text describes the models the
orchestrator can give workers and when to use each. It is free text, added as
written to the orchestrator's SessionStart context.

```json
{
  "orchestrator": {"harness": "claude", "model": "opus", "effort": "high"},
  "worker": {"harness": "codex", "model": "gpt-6.1-sol", "effort": "medium"},
  "models": "- harness codex, model gpt-6.1-sol, effort medium: coding.\n- harness opencode, model openrouter/deepseek/deepseek-v4.1-flash, effort high: light tasks; very fast.\n"
}
```

Effort is passed through as Claude Code's `--effort`, Codex's
`model_reasoning_effort`, OpenCode's model variant, and Ox's `effort`.

Ox models use its qualified `provider:model` ID, such as
`openrouter:deepseek/deepseek-v4.1-flash`. Ox rejects a model or effort its
catalog does not list, and the pane shows the error.

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

In a `no_git` workspace, `tools/list` returns these tools with different
`create_agent` and `remove_agent` descriptions:

- `create_agent(name, dir, harness?, model?, effort?, message?)` requires
  `dir`, an existing directory given as an absolute path, a path starting with
  `~/`, or a path relative to the orchestrator's directory. The worker works
  directly in it, and Hypercube creates no branch or worktree. Two workers may
  share a directory. In a Git workspace, `dir` is ignored.
- `remove_agent(id)` only closes the pane and deletes the agent and its queued
  messages. The directory is left as it is.

## Message delivery

Every message goes into the queue first. Hooks deliver the message text; the
tmux nudge only prompts an idle agent to run a turn.

- **SessionStart:** returns the runtime facts as additional context: the agent's ID, role, working directory, and
  team, the MCP tools, and how worktrees and Git integration work. In a
  `no_git` workspace they describe directory workers instead, and the
  orchestrator is told to instruct each worker how to handle branches and
  commits. The orchestrator also gets the configured `models` text.
- **UserPromptSubmit:** records the harness session ID, marks the agent busy,
  and returns queued messages as additional context. Claude Code saves no
  conversation until the first prompt, so an ID from SessionStart may not
  resume.
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
  `--settings` carries the hooks and the `hypercube statusline` status line. The checkout must already be trusted in
  Claude Code; the first launch in a new checkout shows Claude Code's trust
  dialog in the orchestrator's pane. A directory worker's directory must also
  be trusted, or its pane shows the trust dialog.
- **Codex:** `codex [resume] --dangerously-bypass-approvals-and-sandbox
  --dangerously-bypass-hook-trust` with `-c` overrides for `model`,
  `model_reasoning_effort`, `mcp_servers.hypercube`, `hooks.<event>`, and
  `projects={"<checkout>"={trust_level="trusted"}}`, followed by the session ID
  when resuming and the prompt. Codex keys folder trust on the main checkout,
  which covers the worktrees. A directory outside Git is trusted as it is.
  Codex gives MCP servers a filtered environment,
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
- **Ox:** `ox [--resume <id>] [-- <prompt>]` with `OX_SETTINGS` set to launch
  settings: the model, the effort, `auto` mode, the MCP server, and the hooks
  in Claude Code's format. Ox applies them over the user's settings files
  without saving them, and runs SessionStart before the first turn of each
  session in its server process.

`hypercube hook` writes the Claude Code response format for every harness.
Codex and Ox read the same format, and the OpenCode plugin reads
`hookSpecificOutput.additionalContext` and `reason`.

## To verify

- Nudges hold up when a harness dialog is open.

## Packages

Go. The sole executable is `cmd/hypercube`. Packages under `internal/`:

- `store`: the locked state file, agents, and the message queue.
- `tmux`: sessions, panes, layout, environment, and `send-keys`.
- `workspace`: checkout discovery, worker branches and worktrees, and worker
  directories.
- `harness`: configuration, launch commands, and hook configuration for each
  harness, and the OpenCode plugin.
- `mcp`: the stdio MCP server and tools.
- `hook`: hook event handling and the runtime facts.
- `limits`: Claude Code's and Codex's usage limits for the status bar.

## Smoke testing

For harness smoke tests, use Codex `gpt-6-luna`, Claude `haiku`, OpenCode
`openrouter/~deepseek/deepseek-flash-latest`, and Ox
`openrouter:~deepseek/deepseek-flash-latest`.

When a task is done, stop and destroy every demo or test session it started:
kill its tmux session or server, and delete its state file, lock file,
repositories, and temporary directories. `hypercube status` must not list any
of them afterward.

## Validation

Before pushing anything other than a Markdown-only change, all of this must pass:

```sh
go build ./... && go vet ./... && staticcheck ./... && test -z "$(gofmt -l .)" && go test ./...
```

Use disposable local repositories and isolated `XDG_STATE_HOME` and
`XDG_CONFIG_HOME` for smoke tests, and a separate tmux server: set
`TMUX_TMPDIR` to a temporary directory and unset `TMUX`.
