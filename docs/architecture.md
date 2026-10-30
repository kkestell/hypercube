# Architecture

Hypercube supplies coordination around native coding harnesses. The
[user guide](usage.md) defines the workspace
and team model; this document explains the ownership decisions behind it.

## Ownership

The mux is the process and terminal owner so agents can outlive the command
that starts them. A launch wrapper runs the harness as a child and relaunches
it when it exits. There is no persistent Hypercube server between the harness
and its terminal.

The mux in `mux/` is a general-purpose multiplexer that works on its own with
plain shells and knows nothing about agents, harnesses, or Hypercube's state.
It is a separate Go module, and Hypercube reaches it only through its socket
client in `mux/client`. Its behavior is fixed: it has no configuration file,
format language, hooks, scripting, general layouts, copy mode, or restore
after its own restart. A mux restart ends every pane's process, and Hypercube
resumes agents from their saved conversations.

Worker worktrees live inside the checkout so harnesses can inherit its folder
trust. Team names qualify new worker paths and branches to avoid collisions
between teams.

A saved pane ID establishes ownership: the mux never reuses an ID, and an ID
from an earlier server names nothing. Workspaces and teams are found by
asking the mux which session and window hold their saved panes, so display
names and window order never matter.

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
pane targets and launch bookkeeping are replaceable. Lifecycle operations
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
| [mux](../internal/mux/mux.go) | Panes, windows, labels, attention, and attach through the mux's socket. |
| [mcp](../internal/mcp/mcp.go) | Agent tool schemas and operations. |
| [hook](../internal/hook/hook.go) | Context, teamwork guidance, steering, and turn events. |
| [wait](../internal/wait/wait.go) | Idle delivery and waiter replacement. |
| [statusline](../internal/statusline/statusline.go) | Claude's native status line and approval clearing. |
