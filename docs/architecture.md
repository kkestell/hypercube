# Architecture

`hypercube ui` owns session state and is the only database writer. Native
harnesses execute the agents' work. Plugins connect the two, translating each
harness's protocol into Hypercube commands and transcript events.

```text
Desktop app or browser ←→ hypercube ui ←→ SQLite
                          ↕
                    hypercube hold
                          ↕
                    Harness plugin
                          ↕
                    Native harness
                          ↕
                    hypercube mcp ←→ hypercube ui
```

## Why agents outlive the window

An agent plugin runs under a `hypercube hold` process that owns its input and
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

Each harness starts `hypercube mcp` as a stdio MCP server. It forwards tool
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
updates contain only new or changed items. A reconnect takes fresh snapshots.
Requests to open a file wait until a page views that session.

See [transcripts](../internal/session/transcript.go) and
[the browser interface](../internal/ui/ui.go).

## Backend, desktop app, and terminals

`hypercube ui` is the backend: a background process that serves the interface
and runs the agents, with no window of its own. It runs as a login service or
in a terminal. The Electron app, `Hypercube Desktop`, is a client of the
backend. It shows the interface in a window, with the menus, menu bar icon,
Dock badge, notifications, and agents' browsers. Quitting, restarting, or
losing the desktop app leaves the backend, agents, messaging, and coordination
running. Its signed application bundle, `Hypercube.app`, contains the desktop
app, `hypercube`, and the plugins; macOS notifications require that bundle.

The desktop app connects to the backend's loopback `/desktop` WebSocket and
dials again after a restart. The backend sends navigate, eval, and close
requests for agents' browsers and an alert for each new request or reply; the
app answers each request with its ID. Without a desktop app, browser calls
fail with a message asking the user to open it, and alerts are dropped. The
page shows current requests when the app opens. The backend's side is in
[internal/desktop](../internal/desktop/desktop.go) and the app in
[desktop](../desktop/main.js).

Each session's browser is a desktop window with a toolbar page above the
site. The site page has no access to the app, and every browser shares one
persistent profile, retaining logins across sessions and restarts. Closing the
window, calling `browser_close`, or quitting the desktop app ends a browser.

[Panel terminals](../internal/terminal/terminal.go) start on demand, one per
session, in its working directory. Their holders own the pseudo-terminals and
bounded scrollback, which browser connections read directly. A shell ends on
exit, session stop or deletion, or the backend's SIGINT.

Managed worktrees live inside the checkout so harnesses inherit folder trust.
Paths and branches include session IDs. This separates working copies without
restricting an agent's access to the local filesystem. Git resource handling
lives in [workspace](../internal/workspace/workspace.go).
