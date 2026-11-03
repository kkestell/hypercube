# Architecture

Hypercube supplies coordination around native coding harnesses. The
[user guide](usage.md) defines workspaces and sessions; this document explains
the ownership decisions behind them.

## Ownership

The app process, `hypercube ui`, manages sessions and is the only process
that writes the database. Each agent plugin and panel shell runs under a
`hypercube hold` process that owns its input and output and outlives the app.
Plugins keep their native harnesses alive across turns and speak the
[serve protocol](harness-support.md#plugin-protocol). Closing the browser
changes nothing.

Quit and SIGINT end all agents and panel shells. SIGTERM, service
restarts, development rebuilds, and crashes leave them running. At startup
the app attaches to surviving launches, finishes those that exited, and
restarts lost launches. A running family whose agents were all lost, or that
has no launches after Quit or a reboot, starts awaiting the user's prompt.
Stopping a session clears its running mark.

An agent whose plugin exits after starting is started again with its saved
conversation, unless it has exited three times within ten seconds of starting.
A start that fails stays ended, with the error in the agent's transcript; the
app never replaces a conversation it could not resume with a new one.

The browser view reads everything from the app: Hypercube's data and the
selected session's transcript. Agents reach Hypercube's tools through
`hypercube mcp`, the stdio server each plugin's harness runs. It forwards each
request, with the session's ID, to a route in the app, which runs the tool. MCP
and the browser's actions share one owner of session state. `open_file` requests
wait in the app until a page views the agent's session and arrive over that
page's data socket.

With `--menu`, the app process also runs the macOS application: the menu bar
icon and the Hypercube window, a web view of the browser view. It runs as
`Hypercube.app`, whose `Contents/MacOS` holds `hypercube` and the harness
plugins, because macOS delivers notifications only to a signed application
bundle. `Runtime.WatchAlerts` reports each new request and finished turn, and
the window turns it into a notification unless it has keyboard focus. Each session
can have a browser, a second web view in that window, which the agent drives
with `browser_navigate` and `browser_eval` and the user sees in the session's
Browser tab. The page reports where the tab's body is, and the window places
the session's browser over it. Browsers last until the app quits and share
one website data store, so logins persist across sessions and restarts.

Child worktrees live inside the checkout so harnesses can inherit its folder
trust. Parent session IDs qualify their paths and branches to avoid collisions.

Agents are trusted local processes with the user's credentials. A top-level
session can message its own children by name and any other running top-level
session across all workspaces in the app by ID; a child can message only its
parent, by ID. Stopped and ended recipients are refused. `list_sessions` lists
only running addressable recipients, with workspace paths and parent IDs.
Top-level sessions in the same workspace share its checkout, files, Git index,
and branch. Messaging scope does not isolate agents from the filesystem or
shell tools.

## Persistence and concurrency

Everything Hypercube knows lives in one SQLite database,
`$XDG_STATE_HOME/hypercube/hypercube.db`, defaulting to
`~/.local/state/hypercube/hypercube.db`: workspaces, sessions, queued
messages, commands, and each session's transcript. The app serializes its own
operations; it never holds that lock while waiting on a plugin, since a native request
can call back into a tool.

A transcript is an ordered list of items: the user's messages, text,
reasoning, tool calls and results, errors, attention requests, and child status
updates. Streaming changes are published from memory. Durable changes commit
with their output position and any unfinished items, so replay after a restart
applies each record's effects once. A later item with the same ID replaces the
earlier one in its place.

The database keeps harness conversation IDs, queued messages, and transcripts
across launches. The app records every command before sending it, resends
unanswered commands in ID order when attaching, and applies each reply while
deleting its command
in the same transaction. It sends one queued message at a time and removes it
after the plugin accepts it; a refused message stays queued, and an unanswered
delivery holds back later messages for that launch. Acceptance means
the native harness took the text, not that the model processed it.

The state WebSocket sends the view a new snapshot of its data when the data
changes. It also sends the transcript of the selected session: the whole chat
on connecting and on selecting, then the items that are new or changed. A
reconnecting page takes new snapshots.

## Panel shell

`internal/terminal` manages one shell per session, started on demand in the
session's directory. Its holder owns the pseudo-terminal and keeps bounded
scrollback, which each browser connection reads directly. The shell survives
app restarts, hiding the panel, and closing the browser. It ends when it exits,
its session stops or is deleted, or the user quits the app.

## Source ownership

Internal commands, data fields, protocol messages, and algorithms are
documented by the implementation rather than copied here.

| Source | Responsibility |
| --- | --- |
| [cmd/hypercube](../cmd/hypercube/main.go) | The command line and the app process. |
| [session](../internal/session/runtime.go) | The runtime that runs agents: [workspace and session](../internal/session/sessions.go) and [child session](../internal/session/children.go) lifecycle, [launch and relaunch](../internal/session/launch.go), [events](../internal/session/event.go), [delivery](../internal/session/deliver.go), restore, and shutdown. |
| [store](../internal/store/store.go) | The database schema, transactions, queues, and transcripts. |
| [workspace](../internal/workspace/workspace.go) | Directory resolution and Git resources. |
| [hold](../internal/hold/hold.go) | Detached children, input, retained output, and exit records. |
| [harness](../internal/harness/harness.go) | Configuration, plugin discovery, and the [serve protocol](../internal/harness/protocol.go) client. |
| [mcp](../internal/mcp/mcp.go) | Agent tool schemas and operations, and the bridge to the app. |
| [desktop](../internal/desktop/desktop.go) | The macOS application: the menu bar icon, the window, and the sessions' [browsers](../internal/desktop/browser.m). |
| [report](../internal/report/report.go) | Instructions, message attribution, titles, and failure reports. |
| [terminal](../internal/terminal/terminal.go) | Panel shells. |
| [ui](../internal/ui/ui.go) | The browser view: its data stream, transcripts, actions, and panel terminal. |
| [plugins](../plugins) | The Claude Code, OpenCode, Codex, and Pi harness plugins. |
