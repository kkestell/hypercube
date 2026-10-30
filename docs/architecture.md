# Architecture

Hypercube supplies coordination around native coding harnesses. The
[user guide](usage.md) defines the workspace
and team model; this document explains the ownership decisions behind it.

## Ownership

tmux is the process and terminal owner so agents can outlive the command that
starts them. A short-lived launch wrapper installs identity and launch state
before replacing itself with the harness. There is no persistent Hypercube
server between the harness and its terminal.

Worker worktrees live inside the checkout so harnesses can inherit its folder
trust. Team names qualify new worker paths and branches to avoid collisions
between teams.

Live tmux markers establish ownership at the point of use. Saved pane
numbers, window indexes, and editable display names cannot establish it:
tmux can reuse numbers, and users can rename or reorder windows.

Agents are trusted local processes with the user's credentials. Team scoping
in MCP prevents accidental cross-team operations, with lead-to-lead messages
as the one exception so that leads sharing a checkout can coordinate. It does
not isolate agents from the filesystem or from tools they can invoke through
a shell.

## Persistence and concurrency

Workspace state lives under `$XDG_STATE_HOME/hypercube`, defaulting to
`~/.local/state/hypercube`. Workspace names derive from their absolute paths,
so equal directory basenames do not collide. The naming algorithm belongs to
[workspace.go](../internal/workspace/workspace.go). Failed Hypercube tool
calls and Claude Code auto mode denials are appended there to `tools.log` as
JSON lines.

State keeps harness conversation IDs and queued messages across launches;
tmux targets and launch bookkeeping are replaceable. Lifecycle operations
serialize separately from state updates so a multi-step launch or cleanup
does not block hooks from delivering messages.

An update publishes a complete file atomically. Queue delivery writes its
response before committing removal, so a failed output write leaves the
messages available for retry. This protects the write boundary; it is not an
acknowledgement that the harness or model processed the message.

Waiter generations let a newly started waiter retire earlier ones without a
daemon. Waiters also exit when their parent ends. Each idle waiter currently
polls and parses the whole state file, so idle overhead grows with both
agent count and workspace state size.

## Source ownership

Internal commands, data fields, hook payloads, and algorithms are documented by
the implementation rather than copied here.

| Source | Responsibility |
| --- | --- |
| [cmd/hypercube](../cmd/hypercube/main.go) | CLI and workspace, team, and relaunch lifecycle. |
| [store](../internal/store/store.go) | State schema, locks, atomic updates, and queues. |
| [workspace](../internal/workspace/workspace.go) | Directory resolution and Git resources. |
| [harness](../internal/harness/harness.go) | Per-launch commands and native settings. |
| [OpenCode plugin](../internal/harness/opencode.js) | OpenCode event adaptation and wake-ups. |
| [tmux](../internal/tmux/tmux.go) | Live ownership checks, panes, windows, titles, and bindings. |
| [mcp](../internal/mcp/mcp.go) | Agent tool schemas and operations. |
| [hook](../internal/hook/hook.go) | Context, teamwork guidance, steering, and turn events. |
| [wait](../internal/wait/wait.go) | Idle delivery and waiter replacement. |
| [statusline](../internal/statusline/statusline.go) | Claude's native status line and approval clearing. |
