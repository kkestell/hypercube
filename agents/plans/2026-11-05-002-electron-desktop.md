# Electron desktop app

## Goal

Replace the Cocoa/WKWebView shell, including agents' browser windows, with an
idiomatic Electron app on macOS. Keep the current React UI, the Go backend, the
database and configuration, and session and harness behavior. Linux is a later
change.

The Go backend is a persistent background service. The Electron app is a
desktop client of that service: it shows the window, tray, badge,
notifications, and agents' browsers. Quitting, restarting, or disconnecting the
desktop app never stops the backend, agents, messaging, or coordination.

Build the app the way Electron apps are normally built: standard lifecycle,
native menus with roles and accelerators, standard title-bar customization and
notifications. Do not reproduce the old window's behavior where that would need
custom emulation. Each material visible difference is decided by the user
before the task that depends on it starts; the plan records the decision. A
difference not listed below goes to the user before it is implemented.

## Visible differences

Approved:

- Browser storage resets: unsent drafts, view preferences, and website logins
  start fresh, with no importer.
- Agent browser toolbar: the back, forward, reload, address, and open in
  default browser controls are a small local web page above the site instead
  of AppKit controls.
- Menu shortcuts: the native menus own the shortcuts they show (⌘N, ⌘B, ⌘J,
  ⌘⌥B, ⌘⌥↑/↓, ⌘,, ⌘/, zoom), so they work wherever focus is in the window,
  including text fields, terminals, and dialogs. Shortcuts not in the menus
  keep the page's rules.
- Quit: every quit of the desktop app (⌘Q, the app menu, the tray, the Dock,
  logout) ends only the desktop app. Agents, shells, messaging, and
  coordination keep running in the backend. Per-session Stop is unchanged.

- First run: when no backend answers on the default port and no login service
  is loaded, the desktop's waiting page offers **Set Up Background Service**,
  which runs `install-service` for this app. The login service is installed
  only by that button, `make install-service`, or running `install-service`
  by hand.
- Login: the backend starts at login under launchd. The desktop app starts
  when the user opens it; the tray appears at login only if the user adds
  Hypercube to System Settings → Login Items. Opening the app shows its
  window, since opening it is now always a user's choice.

Consequences of the service boundary, stated in the docs:

- With the desktop app closed, agents keep working and messaging. There are no
  notifications, badge, or tray, and agents' browser tool calls fail with "the
  browser needs the Hypercube app; open it". Alerts raised while it is closed
  are not shown later; the badge and the page show the current requests when
  it opens.
- Quitting the desktop app closes agents' browser windows and fails their
  pending calls.
- Ending every agent at once is a service command, not a menu item:
  `launchctl kill SIGINT gui/$(id -u)/com.github.kkestell.hypercube`, or
  Ctrl-C for a terminal `hypercube ui`.

## Related code

- `cmd/hypercube/main.go` — command dispatch, `ui [port]`, signals.
- `internal/desktop/` — at `HEAD`, Cocoa windows, menus, notifications, badge,
  and agent browsers through Cgo. The uncommitted task 1 draft replaced them
  with a stdio `Conn`; this plan replaces that transport.
- `internal/ui/ui.go` — routes and `localOnly`; `internal/ui/socket.go` — the
  `/state` socket pattern.
- `internal/mcp/browser.go` — the `Browser` interface and `errNoWindow`.
- `internal/session/alerts.go` — `Alert` and `WatchAlerts`.
- `internal/ui/web/` — `data-socket.ts` (1 s reconnect, reload on a new build),
  `app/window.ts`, `native.d.ts`, `app/title-bar.ts`, `app/badge.ts`,
  `app/shortcuts.ts`, `app/shortcuts-dialog.tsx`, `app/app.tsx`,
  `notifications/notifications.ts`, `shared/panel-tabs.tsx`,
  `editor/editor-tabs.tsx`, `main.tsx`, `state.ts`.
- `cmd/hypercube/integration_test.go` — `TestAppProcess`.
- `Makefile`, `scripts/dev`, `scripts/install-service`, `scripts/release`,
  `.github/workflows/release.yml`.
- `docs/installation.md`, `docs/usage.md`, `docs/architecture.md`, `AGENTS.md`.

## Decisions

### Processes

Two independent processes, each with one lifecycle:

- **Backend**: `hypercube ui [port]`, unchanged in role. It owns the database,
  agents, holders, terminals, MCP, and the HTTP/WebSocket server, and it always
  accepts a desktop connection. As a login service it runs under launchd. In a
  terminal it runs in the foreground (headless), exactly as today. SIGINT ends
  agents and shells; SIGTERM detaches from them; both keep running marks.
- **Desktop**: the Electron executable `Hypercube Desktop`. It never starts
  the backend as its child and never stops it. It connects to the backend's
  loopback server, loads the page from it, and holds one desktop connection.

Only the backend needs the harness login environment; Electron runs with
whatever environment Finder, launchd, or the dev script gives it.

The cost compared with the drafted child-stdio design: desktop launch is a
socket connect instead of a login shell, a database open, and a session
restore; desktop quit costs the backend nothing instead of a detach and later
reattach of every holder. The service has one installer, run only on explicit
request. The transport reuses the existing server, `localOnly`, and the
drafted request bookkeeping instead of a new pipe and `SIGPIPE` handling.

### Bundle

`Hypercube.app/Contents/MacOS/` holds the Electron executable `Hypercube
Desktop` (APFS is case-insensitive, so it cannot be `Hypercube` beside
`hypercube`), `hypercube`, and the `hypercube-harness-*` plugins at their
current paths. Plugin discovery, holders, MCP, and agent launches are
unchanged.

`@electron/packager` builds the bundle under Bun (`bunx --bun
electron-packager`; it needs `--app-version`). The Makefile then copies the Go
binaries into `Contents/MacOS`, sets the version fields, copies
`install-service`, and signs ad hoc with `codesign --force --deep --sign -`.
`codesign --verify --deep --strict` passes on that layout. Packager writes
`CFBundleExecutable`, `CFBundleIdentifier`, and `CFBundleName`;
`cmd/hypercube/Info.plist` is deleted. Pin `electron` 45.2.3 and
`@electron/packager` 20.3.0 as exact devDependencies in `desktop/package.json`
with `bun.lock`; the app has no runtime dependencies.

### Login service

`scripts/install-service` stays the one installer, run by `make
install-service`, the setup button, and a downloaded app's user. It takes `APP`
(the bundle path), `SERVICE`, `PORT`, and `SHELL`, writes the plist, boots out
the loaded copy (SIGTERM, so agents keep running), waits for the port as
today, and bootstraps. The plist runs the backend through the login shell in
the tcsh-safe form the Finder launch used: `Program` is the shell;
`ProgramArguments` are `-<shell name>`, `-i`, `-c`, `exec
"$HYPERCUBE_APP_EXECUTABLE" ui "$HYPERCUBE_PORT"`; `EnvironmentVariables`
holds both values. `RunAtLoad`, `KeepAlive` with `SuccessfulExit` false, and
the log path are unchanged. Task 2 verifies that launchd passes
`ProgramArguments[0]` as `argv[0]` with tcsh before relying on it.

### Desktop startup

1. `app.requestSingleInstanceLock()`; a second instance exits, and the first
   shows its window on `second-instance`. `userData` is set before `ready` to
   `$XDG_STATE_HOME/hypercube/chromium` (default `~/.local/state`), so the dev
   app has its own profile and lock.
2. Create the tray and the main window, which shows a local waiting page
   (`desktop/waiting.html`, with its own preload) until the backend answers.
   The window shows at launch, on `activate`, on `second-instance`, and from
   the tray's Show item.
3. Open the desktop connection (below) to port `--port=N`, default 4280. On
   its first open, the main window loads `http://localhost:<port>/`.
4. While the connection is not open, the waiting page names the address and
   the service log (`$XDG_STATE_HOME/hypercube/ui.log`), and says how to start
   the backend: `launchctl kickstart gui/<uid>/com.github.kkestell.hypercube`
   when the service is loaded (`launchctl print` of it succeeds), otherwise
   **Set Up Background Service** on port 4280, or `hypercube ui <port>` on any
   other port.
5. **Set Up Background Service** runs `Contents/Resources/install-service`
   with `APP` (the bundle containing `process.execPath`, shown on the page as
   the app that will run at login), `SERVICE=com.github.kkestell.hypercube`,
   `PORT=4280`, and `SHELL` from `os.userInfo().shell`. The button exists only
   when the service is not loaded, so it never replaces a loaded service. A
   nonzero exit shows the installer's stderr on the page; on success the
   connection opens on its next attempt.

The desktop compares no builds and manages no service otherwise. A backend
that never answers leaves the waiting page up; there is no other supervisor or
failure channel.

### Desktop connection

`GET /desktop` on the backend is a WebSocket, behind `localOnly` like every
route. Electron's main process opens it with the global `WebSocket`; a request
without an `Origin` header passes `localOnly`'s GET rule, and a web page cannot
open it from another origin. Local processes are trusted, so there is no
token.

Each message is one JSON text frame.

Backend to desktop:

```json
{"type":"navigate","id":1,"session":"<id>","url":"<address>"}
{"type":"eval","id":2,"session":"<id>","script":"<function body>"}
{"type":"close","id":3,"session":"<id>"}
{"type":"alert","session":"<id>","title":"…","subtitle":"…","text":"…"}
```

Desktop to backend, one reply per request:

```json
{"id":1,"url":"<final URL>","title":"<title>"}
{"id":2,"result":"<JSON.stringify of the value, or undefined>"}
{"id":3}
{"id":1,"error":"<message>"}
```

Go side: `internal/desktop` keeps the drafted `Conn` bookkeeping (request IDs,
the pending map, dropping the late reply of a cancelled call, failing pending
calls when the connection ends) with the WebSocket as its transport, and adds
`Desktop`, which holds the current connection under a mutex:

- `Desktop` implements `mcp.Browser`. With no connection, calls fail with
  `ErrNoApp` ("the browser needs the Hypercube app; open it"), which replaces
  `mcp.errNoWindow`; `mcp` no longer special-cases a nil browser.
- `Desktop.Alert` sends to the current connection, or does nothing without
  one.
- `Desktop.ServeHTTP` accepts the socket, calls `SetReadLimit(-1)` because an
  eval reply carries a whole page's result and the peer is trusted, makes it
  current, and reads replies until it closes, then fails that connection's
  pending calls and clears it if it is still current. A newer connection
  replaces an older one and closes it. The single-instance lock means one
  desktop per backend in normal use; the replacement rule only decides which
  socket wins.
- `Desktop.Shutdown` closes the current connection and refuses new ones. The app
  calls it in `close`, before `server.Close`, because closing the HTTP server
  does not close upgraded connections.
- `ui.Handler(db, rt, d *desktop.Desktop)` mounts `GET /desktop` and passes
  `d` to `mcp.Handler`. `main.go` creates one `Desktop` and passes `d.Alert`
  as `startApp`'s notify, so `WatchAlerts` always runs.

Desktop side: on close, the main process reconnects after one second, as the
page's `/state` socket does; that covers backend restarts by the installer,
the dev script, and launchd. Agent browser windows stay open across a
reconnect; requests of the closed connection are dropped.

### Lifecycle

- Desktop quit, from anywhere: Electron quits. `before-quit` sets a flag so
  the main window's hide-on-close does not cancel it. The socket closes; the
  backend fails that connection's pending browser calls and continues.
- Backend restart (installer, launchd, dev rebuild): the desktop's socket and
  page reconnect; the page reloads on a new build as it does today.
- `make install-service`: quit the running desktop app by bundle ID (only when
  it is running), install the bundle, run `install-service`, then `open` the
  installed app.
- Downloaded-app upgrade: replace the app and run its `install-service`, as
  today.

### Main window

A `BrowserWindow` with a sandboxed, context-isolated preload. The app keeps
today's tray and Dock behavior with standard APIs: a
`Tray` with a template image (`desktop/trayTemplate.png` and `@2x`, rendered
once from `menubar.svg`, which is deleted) has Show Hypercube and Quit
Hypercube items and shows the waiting count as its title. Showing the main
window or an agent browser calls `app.dock.show()`; hiding or closing the last
visible window calls `app.dock.hide()`. Closing the main window hides it. Its
title follows the page. Navigation away from the backend origin and
`window.open` go to `shell.openExternal`. A crashed renderer reloads. The
normal bounds persist in `window.json` under `userData` and are restored when
they fit a display. Chromium's file inputs replace the old open-panel code.

The preload exposes one global, `hypercube`, with `postMessage(message)` and
`onEvent(callback)`. Page events still arrive as `hypercube-notification`,
`hypercube-shortcut`, and `hypercube-title-bar` window events: `window.ts`
re-dispatches each `onEvent(name, detail)` call, because a `CustomEvent` built
in the isolated world loses its detail in the page. Main accepts messages only
from the main window's main frame. Page messages: `{type: 'title-bar'}`,
`{type: 'badge', count}`, `{type: 'shortcuts', items}`.

Menus: the app menu has the page's App items, About, Services, Hide, Hide
Others, Show All, and Quit Hypercube (`quit` role); Edit uses the `editMenu`
role; Session, View, and Help come from the page's `shortcuts` items with
accelerators converted from their binding, and clicking or pressing them sends
`hypercube-shortcut`. View adds Zoom In/Out/Actual Size, handled in main with
`setZoomLevel` so the title bar can be re-reported, Enter Full Screen, and
Toggle Developer Tools. Window uses the `windowMenu` role, which also lists
agent browsers. In the window, the page's keyboard handler skips `menu: true`
shortcuts, so each runs once.

Title bar: the tab bars stay in the title-bar row with `titleBarStyle:
'hidden'` and a fixed `trafficLightPosition`; dragging and double-clicking the
row's empty space use Electron's drag regions. Main reports `{height, inset}`
as fixed constants chosen with that position to clear the buttons, divided by
the zoom factor, and `{0, 0}` in full screen. Top-row tab bars get `[app-region:drag]` and their
buttons `[app-region:no-drag]`; `titleBarMouseDown` and the
`drag`/`zoom`/`page-zoom` messages are deleted.

Badge and notifications: `app.dock.setBadge` and the tray title show the
waiting count. An alert shows an Electron `Notification` unless the main
window is focused; main keeps a reference until it is clicked or closed, and a
click shows the window and sends `hypercube-notification`.

### Agent browsers

Each session's browser is a `BaseWindow` (1000×720, min 500×300) titled
`<page title> — <session>`, with two `WebContentsView`s: a 44px toolbar page
from `desktop/` with its own preload, and the site in the `persist:browser`
partition with no preload, whose permission request handler denies requests.
It shows without taking focus from another app.

- Navigate: an address without `://` that does not start with `about:` gets
  `https://`; an invalid URL fails with `<address> is not a URL`. Await
  `loadURL`, then reply with `getURL()` and `getTitle()`; a rejection replies
  with its message.
- Eval: `executeJavaScript` of the body wrapped in an async function, returning
  `JSON.stringify(value) ?? 'undefined'`; a rejection replies with its message.
- Close: close the window if it exists, then reply.
- Closing the window fails its pending requests with `The browser was closed`.
  `window.open` loads in the same view.
- The toolbar keeps a typed address while the field has focus; Escape restores
  the page's address.

## Implementation plan

Each task is reviewed before dependent work starts. File lists bound each
task's ownership; split or merge desktop modules by concern as the code
warrants.

### 1. Backend desktop connection

Revises the uncommitted task 1 draft. Owns `internal/desktop/`,
`internal/ui/ui.go` and its tests, `internal/mcp/` browser files and
`mcp.Handler`'s signature, `cmd/hypercube/main.go`, `main_test.go`,
`integration_test.go`.

- Keep the draft's deletion of every `.m`, `.h`, and Cgo file and of
  `launchApp` and the no-argument case. Remove the stdio transport: the
  `--desktop` flag, `Ready`, `Out`, the write mutex (coder/websocket writes are
  safe concurrently), `SIGPIPE`, and the stdout/stderr startup-line move.
  `serveUI` returns to `HEAD`'s headless select over signals and the server,
  with `parseUIArgs` taking only a port, plus one `Desktop` passed to
  `startApp`.
- Implement `Desktop`, the WebSocket `Conn`, `ErrNoApp`, the read limit,
  `Desktop.Shutdown` in `app.close`, and the `/desktop` route as specified.
  Delete `errNoWindow` and the nil-browser checks. Put each doc comment on the
  declaration it describes (the draft's `close` comment sits on `detach`).
- Tests in `internal/desktop` through an `httptest` server and a WebSocket
  client: each request's reply and error, a cancelled call dropping its late
  reply, closing the socket failing pending calls, calls without a desktop
  failing with `ErrNoApp`, a reply larger than 32 KiB arriving whole, a newer
  connection replacing an older one, and `Shutdown` ending the current one.
  `internal/ui` asserts a cross-origin `/desktop` request is refused.
  `TestAppProcess` drops the Finder login-shell checks and keeps its headless
  start and signal cases.

### 2. Electron app, setup page, main window, and page connection

Owns `desktop/` except the agent browser
modules, `internal/ui/web/` files listed under Related code with their tests,
`internal/ui/dist/`, `Makefile`, `scripts/dev`, `scripts/install-service`,
`.gitignore`.

- `desktop/`: `package.json` (`main`, `packageManager` matching
  `internal/ui`, devDependencies), `bun.lock`, `waiting.html` and its preload,
  and CommonJS modules for startup and the waiting page's setup, the desktop
  connection, the main
  window and menus, and the preload. Agent browser requests reply `error: "not
  implemented"` until task 3.
- Page: `window.ts` returns `globalThis.hypercube` and installs the event
  re-dispatch from `main.tsx`; `native.d.ts` declares it. Update `badge.ts` and
  its test, `title-bar.ts` and its callers, `shortcuts.ts`,
  `shortcuts-dialog.tsx`, and `app.tsx` per Decisions. Regenerate `dist`.
- `install-service` as specified under Login service.
- Makefile `bundle`: packager into a temporary directory, replace `$(APP)`,
  build the Go binaries into `$(MACOS)`, set version fields and
  `CFBundleGetInfoString`, copy `install-service`, sign. `install-service` per
  Lifecycle. `check` adds `cd desktop && bun install --frozen-lockfile && bun
  test`. Ignore `desktop/node_modules/`.
- `scripts/dev`: `up` is unchanged and runs the backend directly. `--app` also
  builds `bin/HypercubeDev.app` and runs `Hypercube Desktop --port=$PORT`
  under `DEV_ENV`, after the backend answers. A rebuild restarts the backend
  (SIGTERM) and the desktop app; `stop` and `clean` stop both. The pid checks
  match each started command; `sources` includes `desktop/` without
  `node_modules`.
- Tests: a bun test of the installer's arguments and environment from a bundle
  path and the user's shell. The tcsh `argv[0]` check is manual, with a
  disposable service label.

### 3. Agent browsers

Owns the agent browser modules, the toolbar page and its preload in
`desktop/`, and their wiring into the desktop connection.

- Implement Agent browsers as specified; export the address normalization for
  a bun test.

### 4. Release and documentation

Owns `scripts/release`, `.github/workflows/release.yml`, `docs/`, `AGENTS.md`.

- `release`: unchanged identity check; package with the new `bundle`.
- `release.yml`: add `oven-sh/setup-bun` before packaging.
- Docs: `installation.md` — Bun as a source build dependency; first run's
  **Set Up Background Service**; the backend starting at login and the desktop
  only when opened or added to Login Items; `make install-service`; the
  downloaded-app upgrade; running `hypercube ui` headless and
  attaching a desktop with `--port`; ending every agent and removing the
  service; the browser profile row in Files on disk; backup step 1's quit.
  `usage.md` — quit, browser window, zoom, and notification text that changed.
  `architecture.md` — the backend and desktop processes and the desktop
  connection, replacing Native views and the Quit statements. `AGENTS.md` —
  add `desktop/` to Code; `make dev-app` and install-service lines that
  mention the native app.

### Tasks 2–4 as built

Tasks 2–4 were carried out as one task. Departures from the text above:

- The waiting page and the browser toolbar are React pages in `internal/ui`
  (`web/desktop/`, built with the same stylesheet and shadcn components as the
  main page). `make bundle` copies `internal/ui/dist` to `Contents/Resources/ui`
  and the app loads both from there; the toolbar and waiting page have no
  separate HTML or CSS in `desktop/`.
- One preload serves the main window, the waiting page's setup requests and the
  page bridge, with the app checking each message's sender; the toolbar has its
  own preload.
- `menubar.svg` moved to `desktop/` and `make bundle` renders the tray template
  images into `bin/`, as it does the app icon; no raster is committed.
- The app is packaged without an asar archive, so `make bundle` can set
  `productName` in the copied `package.json` (the app name that menus show).
- The waiting page's guidance is one `action` chosen in main: `headless` for a
  port other than 4280, then `kickstart` when the service is loaded, `move` when
  the app runs from an App Translocation path, and otherwise `setup`.
- `make install-service` quits the desktop app with an AppleScript quit by
  bundle ID, only when it is running, through the new `quit-desktop` and `place`
  targets.

## Test plan

- Go and bun tests listed per task.
- Dev app with isolated state (`make dev-app`): quitting and reopening the
  desktop while an agent works and messages, with the agent unaffected; a dev
  rebuild reconnecting the window; window close and reopen; each menu shortcut
  and Edit command in the composer, terminal, and a dialog; title-bar drag and
  double-click; tab drag and drop; zoom; full screen; the badge; a
  notification click selecting its session.
- Agent browser against local pages: navigate, a fragment change, async eval
  and a thrown exception, back and forward, closing during a navigation, a
  call with the desktop closed failing with the open-the-app message, and a
  cookie surviving a desktop restart.
- Packaged app: `codesign --verify --deep --strict`; plugins found beside
  `hypercube`; with no service loaded, the waiting page's setup button
  installs it and agents get the login environment, including with tcsh as
  the login shell; with the service loaded but stopped, the page shows the
  kickstart command and no button; `make install-service` replacing the
  service while agents keep running; a second launch shows the first
  instance's window; `hypercube version` from the bundle.
- The backend uses the existing database and configuration locations.
