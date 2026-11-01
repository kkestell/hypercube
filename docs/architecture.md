# Architecture

Hypercube supplies coordination around native coding harnesses. The
[user guide](usage.md) defines workspaces and sessions; this document explains
the ownership decisions behind them.

## Ownership

The app process, `hypercube ui`, is the only process that adds or deletes
workspaces and creates, starts, stops, or deletes sessions. Each operation on a
session holds that session's lock. At startup the app starts every session the
user left running whose tmux session is missing.

tmux owns processes and terminals so agents can outlive the app. Each running
session has its own tmux session, named `hypercube-<session ID>`: its first
window holds the agents' panes, and a second window holds the browser view's
terminal panel. A short-lived launch wrapper installs identity and launch state
before replacing itself with the harness.

The browser view reads Hypercube's data from the app and only terminals from
tmux, through a control-mode client attached to the selected session's tmux
session.

Worker worktrees live inside the checkout so harnesses can inherit its folder
trust. Session IDs qualify worker paths and branches to avoid collisions
between sessions.

Live tmux markers establish ownership at the point of use. A saved pane number
cannot establish it, since tmux can reuse numbers.

Agents are trusted local processes with the user's credentials. Session scoping
in MCP prevents accidental cross-session operations, with messages between the
leads of one workspace as the one exception so that leads sharing a checkout
can coordinate. It does not isolate agents from the filesystem or from tools
they can invoke through a shell.

## Persistence and concurrency

Everything Hypercube knows lives in one SQLite database,
`$XDG_STATE_HOME/hypercube/hypercube.db`, defaulting to
`~/.local/state/hypercube/hypercube.db`: workspaces, sessions, agents, and
queued messages. Every Hypercube process opens it. Failed Hypercube tool calls
and Claude Code auto mode denials are appended beside it to `tools.log` as JSON
lines.

The database keeps harness conversation IDs and queued messages across
launches; panes and launch bookkeeping are replaceable. Writes wait for the
database lock instead of failing. Session lifecycle operations hold a separate
lock file per session, so a multi-step launch or cleanup does not block hooks
from delivering messages.

Queue delivery writes its response before committing removal, so a failed
output write leaves the messages available for retry. This protects the write
boundary; it is not an acknowledgement that the harness or model processed the
message.

Waiter generations let a newly started waiter retire earlier ones without a
daemon. Waiters also exit when their parent ends. Each idle waiter polls its
agent's row four times a second.

The app watches the database for changes and sends the view a new snapshot of
its data when one differs from the last.

## Source ownership

Internal commands, data fields, hook payloads, and algorithms are documented by
the implementation rather than copied here.

| Source | Responsibility |
| --- | --- |
| [cmd/hypercube](../cmd/hypercube/main.go) | The command line and the app process. |
| [session](../internal/session/session.go) | Workspace, session, and worker lifecycle, and relaunches. |
| [store](../internal/store/store.go) | The database schema, transactions, locks, and queues. |
| [workspace](../internal/workspace/workspace.go) | Directory resolution and Git resources. |
| [harness](../internal/harness/harness.go) | Configuration and launches through harness plugins. |
| [tmux](../internal/tmux/tmux.go) | Live ownership checks, tmux sessions, and panes. |
| [mcp](../internal/mcp/mcp.go) | Agent tool schemas and operations. |
| [report](../internal/report/report.go) | Context, teamwork guidance, steering, and turn events reported by plugins. |
| [wait](../internal/wait/wait.go) | Idle delivery and waiter replacement. |
| [ui](../internal/ui/ui.go) | The browser view: its data stream, actions, and a tmux control-mode client per page. |
| [plugins](../plugins) | The Claude Code, OpenCode, Codex, and Pi harness plugins. |
