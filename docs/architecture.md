# Architecture

Hypercube supplies coordination around native coding harnesses. The
[user guide](usage.md) defines the workspace
and team model; this document explains the ownership decisions behind it.

## Ownership

The mux is the process and terminal owner so agents can outlive the command
that starts them. There is no persistent Hypercube server between the harness
and its terminal.

The mux in `mux/` is a general-purpose multiplexer that works on its own with
plain shells and knows nothing about agents, harnesses, or Hypercube's state.
Its behavior is fixed: it has no configuration file, format language, hooks,
scripting, general layouts, copy mode, or restore after its own restart.

Worker worktrees live inside the checkout so harnesses can inherit its folder
trust.

A saved pane ID establishes ownership: the mux never reuses an ID, and an ID
from an earlier server names nothing. Workspaces and teams are found by
asking the mux which session and window hold their saved panes, so display
names and window order never matter.

Team boundaries prevent accidental cross-team operations. They do not isolate
agents from the filesystem or from tools they can invoke through a shell.

## Persistence

Workspace state lives under `$XDG_STATE_HOME/hypercube`, defaulting to
`~/.local/state/hypercube`. Failed Hypercube tool calls and Claude Code auto
mode denials are recorded there in `tools.log`.

Lifecycle operations serialize separately from state updates so a multi-step
launch or cleanup does not block hooks from delivering messages. Queue
delivery writes its response before committing removal, so a failed output
write leaves the messages available for retry. This protects the write
boundary; it is not an acknowledgement that the harness or model processed
the message.

Waiter generations let a newly started waiter retire earlier ones without a
daemon. Each idle waiter polls and parses the whole state file, so idle
overhead grows with both agent count and workspace state size.
