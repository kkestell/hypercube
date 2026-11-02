# Architecture

Hypercube supplies coordination around native coding harnesses. The
[user guide](usage.md) defines workspaces and sessions; this document explains
the ownership decisions behind them.

## Ownership

The app process, `hypercube ui`, owns every agent and every panel shell, and is
the only process that writes the database. It adds and deletes workspaces and
creates, starts, stops, and deletes sessions.

Each agent is a harness plugin process, started in its own process group, that
keeps its native harness alive across turns. The plugin speaks the
[serve protocol](harness-support.md#plugin-protocol) over its standard input
and output. Closing the browser changes nothing.

Menu Quit, SIGINT, and SIGTERM, which a service restart or a development
rebuild sends, all stop the app the same way: it sends each plugin `shutdown`,
waits briefly, then kills the plugin's process group, and ends the panel
shells. A session keeps the running mark the user last chose, so at startup the
app starts every running session and resumes its agents' saved conversations.
It delivers no queued message until the user sends a prompt to the session.
Stopping a session clears its running mark.

An agent whose plugin exits after starting is started again with its saved
conversation, unless it has exited three times within ten seconds of starting.
A start that fails stays ended, with the error in the agent's transcript; the
app never replaces a conversation it could not resume with a new one. Each start has a generation number, and events
from an earlier generation are ignored.

The browser view reads everything from the app: Hypercube's data and the
selected session's transcript. Agents reach Hypercube's tools through
`hypercube mcp`, the stdio server each plugin's harness runs. It forwards each
request, with the session's ID, to a route in the app, which runs the tool. MCP
and the browser's actions share one owner of session state.

Child worktrees live inside the checkout so harnesses can inherit its folder
trust. Parent session IDs qualify their paths and branches to avoid collisions.

Agents are trusted local processes with the user's credentials. A top-level
session can message its children and other top-level sessions in its workspace;
a child can message only its parent. This prevents accidental cross-session
operations. It does not isolate agents from the filesystem or shell tools.

## Persistence and concurrency

Everything Hypercube knows lives in one SQLite database,
`$XDG_STATE_HOME/hypercube/hypercube.db`, defaulting to
`~/.local/state/hypercube/hypercube.db`: workspaces, sessions, queued
messages, and each session's transcript. The app serializes its own operations;
the app never holds that lock while waiting on a plugin, since a native request
can call back into a tool.

A transcript is an ordered list of items: the user's messages, text,
reasoning, tool calls and results, errors, attention requests, and child status
updates. Completed items are written to the database. Items still streaming stay in memory and are
published as they change. A later item with the same ID replaces the earlier
one in its place.

The database keeps harness conversation IDs, queued messages, and transcripts
across launches. The app sends one queued message at a time and removes it
after the plugin accepts it; a refused message stays queued. Acceptance means
the native harness took the text, not that the model processed it.

The state WebSocket sends the view a new snapshot of its data when the data
changes. It also sends the transcript of the selected session: the whole chat
on connecting and on selecting, then the items that are new or changed. A
reconnecting page takes new snapshots.

## Panel shell

`internal/terminal` owns one shell on a pseudo-terminal per session. It starts
on demand in the session's directory, keeps a bounded buffer of its recent output,
and replays that buffer to each browser connection. The shell survives hiding
the panel and closing the browser, and ends when it exits, when its session
stops or is deleted, or when the app shuts down.

## Source ownership

Internal commands, data fields, protocol messages, and algorithms are
documented by the implementation rather than copied here.

| Source | Responsibility |
| --- | --- |
| [cmd/hypercube](../cmd/hypercube/main.go) | The command line and the app process. |
| [session](../internal/session/runtime.go) | The runtime that runs agents: [workspace and session](../internal/session/sessions.go) and [child session](../internal/session/children.go) lifecycle, [launch and relaunch](../internal/session/launch.go), [events](../internal/session/event.go), [delivery](../internal/session/deliver.go), restore, and shutdown. |
| [store](../internal/store/store.go) | The database schema, transactions, queues, and transcripts. |
| [workspace](../internal/workspace/workspace.go) | Directory resolution and Git resources. |
| [harness](../internal/harness/harness.go) | Configuration, plugin discovery, and the [serve protocol](../internal/harness/protocol.go) client. |
| [mcp](../internal/mcp/mcp.go) | Agent tool schemas and operations, and the bridge to the app. |
| [report](../internal/report/report.go) | Instructions, message attribution, titles, and failure reports. |
| [terminal](../internal/terminal/terminal.go) | Panel shells. |
| [ui](../internal/ui/ui.go) | The browser view: its data stream, transcripts, actions, and panel terminal. |
| [plugins](../plugins) | The Claude Code, OpenCode, Codex, and Pi harness plugins. |
