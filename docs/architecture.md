# Architecture

`hypercube ui` owns session state and is the only database writer. Native
harnesses execute the agents' work. Plugins connect the two, translating each
harness's protocol into Hypercube commands and transcript events.

```text
Browser interface ←→ hypercube ui ←→ SQLite
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
processes alive. On startup, the app attaches to surviving launches, finishes
those that exited, and restarts lost ones.

Quit and SIGINT deliberately end agents and shells. A family's running mark
remains, so its sessions launch again next time, awaiting a prompt. Stopping
a family also ends its processes but clears that mark.

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

## Native views and terminals

With `--menu`, the app also owns the macOS window, menu bar icon, and
notifications. Its signed application bundle contains `hypercube` and the
plugins; macOS notifications require that bundle.

Each session's browser has its own native window, navigation controls, and web
view. AppKit owns their layout. Browsers share the default website data store,
retaining logins across sessions and restarts. Closing the window, calling
`browser_close`, or quitting the app ends a browser.
The implementation is in [desktop](../internal/desktop/desktop.go).

[Panel terminals](../internal/terminal/terminal.go) start on demand, one per
session, in its working directory. Their holders own the pseudo-terminals and
bounded scrollback, which browser connections read directly. A shell ends on
exit, session stop or deletion, or Quit.

Managed worktrees live inside the checkout so harnesses inherit folder trust.
Paths and branches include session IDs. This separates working copies without
restricting an agent's access to the local filesystem. Git resource handling
lives in [workspace](../internal/workspace/workspace.go).
