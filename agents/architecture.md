# Architecture

`hypercube ui` owns session state and is the only database writer. Native
harnesses execute the agents' work. Plugins connect the two, translating each
harness's protocol into Hypercube commands and transcript events.

![The Electron desktop app contains the browser interface and connects to the
backend, which owns SQLite state. Each agent runs under a separate process
holder, with a harness plugin, native harness, and MCP tools that call back
into the backend.](images/architecture.png)

The holder keeps the agent processes alive across backend restarts. Agent tools
return to the same backend that handles user actions; only the backend writes
to SQLite.

## Plugin protocol

Each `hypercube-harness-<name>` executable runs beside `hypercube`. The app
starts it with `serve` and exchanges commands and events as JSON Lines. The
plugin owns the native protocol; the core and browser interface do not name
individual harnesses.

The [protocol types](../internal/harness/protocol.go) define commands, replies,
transcript items, and usage fields. Native mappings live in the
[Claude Code](../plugins/claude), [OpenCode](../plugins/opencode), and
[Pi](../plugins/pi) plugins. Restart a session to
load a newly installed plugin.

Two one-shot commands, defined in [discovery](../internal/harness/discovery.go),
describe the harness. `describe` reports whether the native executable is
installed; the app runs it for every plugin at startup and when the browser or
`list_models` lists harnesses, and gives the installed names to new sessions'
instructions. `models` lists the models a harness offers in a directory and,
for one model, its efforts. The runtime saves catalogs in SQLite by harness,
directory, and model. The browser uses saved catalogs for labels and refreshes
them when the selector opens; `list_models` asks the harness directly. The
plugin, not the core, checks a session's model and effort in its native start
and configure, and its start and configure replies report the settings the
harness accepted.

A session saves its harness, model, and effort. A child takes its parent's
current settings, with any overrides its parent passes, resolved in
[selection](../internal/session/selection.go) when the child is created or
proposed; a proposal saves the resolved settings. A launch counts as started
once the transaction that saves its start reply commits.

Integration research for the supported harnesses is in
[Claude Code](research/harnesses/claude-code.md),
[OpenCode](research/harnesses/opencode.md), and
[Pi](research/harnesses/pi.md). These are version-specific research notes;
the plugin code defines the current integration.

## Why agents outlive the window

An agent plugin runs under a `hypercube-hold` process that owns its input and
output independently of the app. Panel shells use holders too. A service
restart, development rebuild, SIGTERM, or app crash therefore leaves those
processes alive. On startup, the app attaches to surviving launches and
finishes those that exited or were lost.

Each session's agent starts when the session is opened, prompted, or sent a
message, and ends after the session has been idle and unviewed for 15 minutes.
SIGINT to the backend deliberately ends agents and shells; each session starts
again when next used. Quitting the desktop app ends neither.

An agent whose plugin exits after starting is relaunched with its saved
conversation. Three quick exits—within ten seconds of starting—stop automatic
relaunches. A failed start stays exited with an error in its transcript. The
app never substitutes a new conversation for one it cannot resume.

See [hold](../internal/hold/hold.go) and
[session launch](../internal/session/launch.go).

## One owner for user actions and agent tools

Each harness starts `hypercube-mcp` as a stdio MCP server. It forwards tool
requests to the app with the session ID. Browser actions reach the same session
runtime, so both use one owner of lifecycle and saved state. The runtime
serializes its operations, releasing its lock before waiting on a plugin:
a harness can call back into an app tool while a command is in flight.

The app records commands before sending them. A reply is applied in the same
transaction that removes its command. After reconnection, unanswered commands
are resent in ID order. Queued messages are delivered one at a time and removed
when the plugin accepts them. Acceptance means the harness took the input,
including into a native queue; it does not mean the model processed it.

The [session runtime](../internal/session/runtime.go),
[delivery code](../internal/session/deliver.go), and
[store](../internal/store/store.go) define these transitions.

## Saved state and the interface

The database at `$XDG_STATE_HOME/hypercube/hypercube.db` (normally
`~/.local/state/hypercube/hypercube.db`) contains workspaces, sessions, queued
messages, commands, and transcripts. Native conversation IDs connect sessions
to the harnesses' own storage.

Transcript items have stable IDs. Streaming changes publish from memory;
durable changes commit with their output position and unfinished items so
replay applies each record's effects once. A later update replaces the item
in place.

The browser receives state snapshots over a WebSocket, plus the selected
session's whole transcript on connection or selection. Subsequent transcript
updates contain only new or changed items. Items carry links to their images
rather than the image data, and the page loads an image when it shows it. A
reconnect takes fresh snapshots.
Requests to open a file wait until a page views that session.

See [transcripts](../internal/session/transcript.go) and
[the browser interface](../internal/ui/ui.go).

## Backend, desktop app, and terminals

`hypercube ui` is the backend: a background process that serves the interface
and runs the agents, with no window of its own. It runs as a login service or
in a terminal. It finds its plugins, `hypercube-mcp`, and `hypercube-hold`
beside its real executable. The `hypercube mcp` and `hypercube hold` commands
run the same code for processes started by earlier versions.

The Electron app, `Hypercube Desktop`, is a client of the backend. It shows the
interface in a window, with the menus, notifications, and the count of waiting
sessions: on macOS a menu bar icon and the Dock badge, on Linux the launcher
badge. Closing the window hides it on macOS and quits the
app on Linux. It also runs the actions that need the user's
graphical session: Add Workspace's folder chooser and opening folders and files
in other applications. Quitting, restarting, or
losing the desktop app leaves the backend, agents, messaging, and coordination
running. Its signed application bundle, `Hypercube.app`, contains the desktop
app, `hypercube`, its MCP bridge and holder, and the plugins; macOS notifications
require that bundle. The Linux bundle is the directory `Hypercube` with the same
executables, and the pages and launcher files in `resources/`.

The desktop app connects to the backend's loopback `/desktop` WebSocket and
dials again after a restart. The backend sends an alert for each new request or
reply that the notification preferences in `config.json` allow. Without a
desktop app, alerts are dropped. The page shows current requests when the app
opens. The backend's side is in
[internal/desktop](../internal/desktop/desktop.go) and the app in
[desktop](../desktop/main.js).

[Panel terminals](../internal/terminal/terminal.go) start on demand for each
session or workspace. Session shells use their working directories; workspace
shells use the registered checkout. Their holders own the pseudo-terminals and
bounded scrollback, which browser connections read directly. A shell ends on
exit, deletion of its owner, or the backend’s SIGINT. Archiving a session also
ends its shell.

Managed worktrees live inside the checkout so harnesses inherit folder trust.
Paths and branches include session IDs. This separates working copies without
restricting an agent's access to the local filesystem. Git resource handling
lives in [workspace](../internal/workspace/workspace.go).
