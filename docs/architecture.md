# Architecture

Hypercube runs one team per workspace: an orchestrator in the original
checkout, and workers in their own Git worktrees or existing directories.
Outside Git, a workspace is an ordinary directory and each worker must be
given an existing directory.

## Processes and terminals

`hypercube` resolves the current workspace, starts its team if necessary, and
attaches to its tmux session. Inside tmux it switches clients instead of
nesting an attachment. `hypercube start <dir>` starts a team detached.

The session belongs to tmux and survives detaching, closing a terminal, or
exiting the Hypercube command. tmux owns pane PTYs, input, rendering,
scrollback, selection, mouse handling, hyperlinks, resizing, zoom, and
terminal notifications. Hypercube does not emulate a terminal or proxy its
input and output. Normal tmux bindings apply: Ctrl-b arrows select a pane,
Ctrl-b z zooms, Ctrl-b d detaches, and Ctrl-b s selects a session.

The orchestrator occupies the left 40% of the window; workers stack on the
right. The window is rearranged when workers are added or removed. Pane
titles show the name, harness, model, effort, directory, activity, and tool
approval attention. Options and hooks are scoped to Hypercube's own sessions
and windows.

A new pane briefly runs a placeholder until its options and identity are
installed, then `respawn-pane` replaces it with `hypercube agent <prompt>`.
That short-lived wrapper records the pane ID, sets activity before any
harness hook can run, builds per-launch configuration, and replaces itself
with the harness using `exec`. The native harness occupies the pane directly.

`remain-on-exit` keeps launch errors visible. A `pane-died` hook runs
`hypercube exited <state> <pane> <status>`. It verifies the pane's live identity,
then restarts the original command with the saved conversation and the
agent's identity environment. Three consecutive exits within ten seconds
leave the agent exited until the workspace starts again. A failure to build
or execute the harness leaves it exited immediately. The tmux session keeps
the launch environment so restarts use the same PATH and configuration.

## State and lifecycle

A workspace name is its directory basename, sanitized, followed by the first
six hex characters of its path's SHA-256. Its files are in
`$XDG_STATE_HOME/hypercube`, or `~/.local/state/hypercube`:

- `<name>.json`: agents, saved harness session IDs, queued messages, activity,
  pane IDs, waiter generations, and launch bookkeeping.
- `<name>.lock`: the lock for atomic state updates.
- `<name>.json.session.lock`: serializes start, stop, delete, worker creation
  and removal, and relaunches.
- `opencode-plugin/index.js`: the embedded OpenCode plugin, with this
  executable's path inserted.

Hooks, waiters, and MCP servers are short-lived processes or children of the
harness. Each gets `HYPERCUBE_AGENT_ID` and `HYPERCUBE_STATE`; no network
server or custom terminal host is needed.

State updates take an exclusive `flock`, load the current state, and rename a
complete temporary file into place. Readers need no lock. An update that
fails writes nothing; an unchanged update leaves the modification time alone.
A lock whose file was unlinked while it waited is reopened and locked again,
so waiters cannot update through different lock-file inodes.

`stop [dir]` kills only the workspace's identified tmux session. `delete [dir]`
first refuses dirty worktrees or branches not merged into the checkout's
HEAD. It then stops the session, rechecks each worktree before removing it,
removes the branch, and forgets each successfully removed worker. Failed
cleanup remains recorded for another attempt. Workers in existing directories
have those directories left alone. The state is removed last. `--all` lists
targets and asks for confirmation; failures do not prevent the remaining
targets from being processed. `status` lists known workspaces; `running`
prints the paths of running workspaces for reinstall scripts. Stop and delete
also resolve saved workspaces whose checkout has moved or disappeared.

A saved pane number is not proof of ownership. Before using a pane, Hypercube
checks its live session name, `@hypercube_state`, and `@hypercube_id` options.
Worker removal finds the actual pane by its identity, so a stale saved number
cannot close an unrelated pane or leave the agent's actual pane behind.

Saved session IDs and queued messages retain their JSON representation.
State containing extra teams or scheduled tasks is refused on startup without
rewriting it. A still-running host holding the workspace's `.host.lock` also
blocks startup, preventing concurrent owners of the same state.

## Harness integration and messaging

Claude Code and OpenCode retain their own UI, sessions, transcripts,
permissions, credentials, and native subagents. Hypercube configures each
launch through arguments and environment overrides, leaving the user's
settings and HOME unchanged. See [Harness support](harness-support.md).

Each harness starts `hypercube mcp`, a line-delimited stdio JSON-RPC server.
The tools are `send_message`, `list_agents`, `create_agent`, and `remove_agent`.
Only the orchestrator can create or remove workers. Worker creation validates
a named profile, reserves the name, creates an optional worktree, queues its
brief, and launches the pane. Failed launches roll back; failed cleanup keeps
the agent recorded. Worktrees start from the orchestrator's HEAD, initialize
submodules, and copy ignored paths selected by `.worktreeinclude`.

`send_message` appends to the recipient's queue. It never sends pane keys.
The harness's prompt, tool-result, and stop hooks deliver messages during a
turn, including steering from other agents. A stop with pending messages
continues the turn; an empty stop marks the agent idle. A failed worker turn
marks it idle, preserves its own queue, and sends the orchestrator a bounded
failure summary.

An idle harness runs `hypercube wait`. Each waiter increments an agent's
persisted generation; older waiters exit, and a waiter exits if its parent
ends. An idle agent with queued messages is rechecked under the state lock;
the waiter writes those messages to stderr, marks the agent busy, and exits
with status 2. Failed writes leave the messages queued. Claude Code's
`asyncRewake` hook and OpenCode's plugin use that output to start a native
turn. A draft prompt being typed stays in the editor.

Native subagent hooks cannot consume the main conversation's queue, overwrite
its saved session, change its activity, or replace its waiter. A native
subagent's tool approval still flags attention in the same pane. Claude Code's
status line clears attention when its approval dialog is answered.
