# Native macOS notifications

## Goal

When an agent opens an attention request or finishes a turn with a reply, and
the Hypercube window does not have keyboard focus (another app is active, or
the window is hidden or minimized), macOS shows a notification. Clicking it
brings the Hypercube window forward and opens that session. While the window
has keyboard focus, no native notification appears; the existing notification
cards cover every session.

## Related code

- `internal/session/transcript.go` — `Runtime.Requests` lists open attention
  items across live sessions.
- `internal/session/event.go` — `applyEvent` sets `Unseen` when a session goes
  from busy to idle; that is a finished turn.
- `internal/session/feed.go` — `Runtime.Subscribe`; every committed write
  publishes an empty `Change`. A reader that falls behind has its channel
  closed.
- `internal/session/inactivity.go` — `StopIdleSessions`, the existing
  runtime-owned background loop started from `startApp`.
- `internal/store/store.go` — `Tx.Workspaces`, `Tx.Sessions`, `Tx.Children`,
  `Tx.LastText` (the reply text and its position).
- `internal/ui/snapshot.go` — `show`: a reply is the `LastText` of an unseen
  idle session. Alerts use the same definition.
- `internal/ui/web/sessions/session-view.jsx` — marks the shown session seen
  as soon as its page is visible, even when the window is not focused.
- `cmd/hypercube/main.go` — `startApp` and `serveUI`; `--menu` runs
  `desktop.Run`.
- `internal/desktop/desktop.go`, `app.m`, `window.m`, `desktop.h` — the Cocoa
  app, `HCWindow show`, and `HCWindow dispatch:detail:` which sends the page a
  `CustomEvent`.
- `internal/ui/web/browser/browser.js` — the pattern for window events
  (`hypercube-browser`).
- `internal/ui/web/sessions/session-actions.js` — `chooseSession`, which the
  card's eye button uses.
- `internal/ui/web/main.jsx` — starts the page's listeners.

## Decisions

- **App identity.** `UNUserNotificationCenter` serves only a process that
  runs from a signed application bundle outside `/tmp`. A bare executable,
  with or without an Info.plist embedded in the binary and with or without an
  ad-hoc signature, is refused (`requestAuthorization not allowed`, `Failed to
  find or validate client`). So `make build` produces `bin/Hypercube.app`:
  `Contents/Info.plist` and, in `Contents/MacOS`, `hypercube` and the four
  `hypercube-harness-*` plugins, signed ad hoc (each plugin, then the bundle).
  The bundle identifier is `com.github.kkestell.hypercube`, the service
  label; the display name is `Hypercube`.
- **Executable paths.** The plugins stay beside `hypercube` inside
  `Contents/MacOS`, so `harness.Find`, `hold.Start`, and the `Hypercube` path
  in `harness.StartArgs` keep using `os.Executable` unchanged. The build
  writes `bin/hypercube` and `bin/hypercube-harness-*` as symlinks into the
  bundle, and `make install` writes the same symlinks into `BINDIR`
  (`~/.local/bin`). Whether `os.Executable` reports a symlink or its target,
  the plugins are beside it, and `hypercube` on PATH still works for users and
  for agents whose launch recorded the `~/.local/bin` path.
- **Install.** `make install` replaces `APPDIR/Hypercube.app` (`APPDIR`
  defaults to `~/Applications`) with a copy of `bin/Hypercube.app`, writes
  the `BINDIR` symlinks with `ln -sf`, which replaces the regular files of
  earlier installs, and registers the bundle with Launch Services
  (`lsregister -f`). Removing the old bundle before copying leaves running
  holders on their unlinked executable, as replacing the file does today.
- **Service launch.** The LaunchAgent keeps its login-shell launch and execs
  `APPDIR/Hypercube.app/Contents/MacOS/hypercube ui PORT --menu` directly, so
  launchd still owns the app process, its environment, its log, and
  `KeepAlive`. The app's own executable path is inside the bundle.
- **Development identity.** The dev session builds
  `bin/HypercubeDev.app` with identifier `com.github.kkestell.hypercube.dev`
  and name `Hypercube Dev`, registers it, and runs its inner executable, so
  its notifications and clicks never reach the installed app. The Makefile
  takes `APP`, `APP_ID`, and `APP_NAME`; `scripts/dev` passes the dev values.
  `cmd/hypercube/Info.plist` is the source; the build copies it and sets the
  identifier and name with `plutil -replace`.
- **Gate.** Only a bundle started with `open` has got past the refusal, and
  only as a pending authorization callback; no delivery or click has been
  seen anywhere. The first task verifies the direct launch the service and dev
  session use: a registered, ad-hoc-signed bundle under `bin/`, its inner
  executable started from a shell, gets an authorization prompt, shows a
  banner, and receives the click; after a rebuild re-signs it, it delivers
  again without a new prompt. If any of these fails, stop and report.
- **Unbundled runs.** `hypercubeRun` sets up notifications only when
  `NSBundle.mainBundle.bundleIdentifier` is set, because asking for the
  notification center from a bare executable (such as `go run`) terminates
  the process.
- **Where alerts are decided.** The app process decides what is new, not the
  page, because the page's web view may be hidden or minimized when it
  matters. `Runtime.WatchAlerts` reads state on every `Change` and calls
  `notify` for each new alert. The window decides whether to show it: the
  native side drops an alert when `NSApp.isActive` and the window is key.
  Alerts the window drops are not shown later.
- **What is new.** A request alert is a `Requests` item whose ID has not been
  alerted. A reply alert is an idle session whose `LastText` position is
  greater than the last one alerted for it, checked for sessions that are
  unseen or were busy at the previous read. Checking busy-to-idle as well as
  unseen keeps the reply when the page marks the shown session seen before
  the watcher reads. Requests and replies present when the watcher starts are
  recorded without alerting, so restarts do not replay them.
- **Notification content.** Title: the session title. Subtitle: the workspace
  name, then the parent title for a child, joined by ` › `. Body: the request
  text or the reply text. The identifier is the alert key
  (`request:<session>:<item>` or `reply:<session>:<seq>`), and `userInfo`
  carries the session ID. Default sound.
- **Authorization.** `hypercubeRun` requests alert and sound authorization at
  launch. The user answers macOS's prompt once; a denial means notifications
  are not shown, with no further handling.
- **Click.** The `UNUserNotificationCenterDelegate` response handler calls
  `[window show]` and dispatches `hypercube-notification` with `{session}`.
  The page answers with `chooseSession`. `willPresentNotification` returns
  banner, list, and sound, so an alert that passed the focus check also shows
  while the app is active with its window not key.
- **Wiring.** `startApp` takes a `notify func(session.Alert)`; `serveUI`
  passes `desktop.Notify` with `--menu` and nil otherwise, and integration
  tests pass nil. A nil `notify` starts no watcher. `WatchAlerts` returns its
  read error; `startApp`'s goroutine writes it to stderr, which is the
  service log.
- **Restarts.** The harness plugins, `hypercube hold`, `hypercube mcp`, and
  the plugin protocol do not change. Running agents and panel shells keep
  running across `make install-service`: their executables were unlinked,
  not modified, and the `~/.local/bin/hypercube` path in their launch
  arguments now resolves to the bundle. Agents launched afterwards record the
  bundle path. `make install-service` restarts the app once, as usual; its
  first launch shows the macOS permission prompt, which the user answers.

## Test plan

- `internal/session/alerts_test.go`, through `WatchAlerts` with the harness
  stand-ins used by `event_test.go`:
  - an attention request opened after start produces one request alert with
    the session title, workspace subtitle, and request text; later changes do
    not repeat it.
  - a finished turn produces one reply alert with the last text; a child's
    alert has the parent title in its subtitle.
  - a reply marked seen right after the turn ends still produces its alert.
  - requests and unseen replies present before start produce no alerts.
- `internal/ui/web/notifications/notifications.test.js`: a
  `hypercube-notification` event selects its session.
- `make build` leaves `bin/hypercube` running the bundle's executable with
  the plugins beside it: `bin/hypercube ui` in a test shell finds the
  harnesses (`make check` covers the build and signing).
- Live check in `make dev` (gate in the first task, and again at the end):
  with another app focused, a request and a reply each show a banner from
  Hypercube Dev; clicking one brings the window forward on that session; with
  the window focused, none appears; with the window closed or minimized,
  banners appear.

## Implementation plan

1. Bundle and gate: add `cmd/hypercube/Info.plist` (`CFBundleExecutable`
   `hypercube`, `CFBundlePackageType` `APPL`, identifier and name set at
   build). In the `Makefile`, `build` assembles `$(APP)` from the Go builds,
   sets the identifier and name, signs the plugins and then the bundle with
   `codesign --force --sign -`, and writes the `bin/` symlinks. Add
   `-framework UserNotifications` to `internal/desktop/desktop.go` and
   `internal/desktop/notify.m` with authorization, the delegate, and
   `hypercubeNotify`. In `scripts/dev`, build with the dev `APP`, `APP_ID`,
   and `APP_NAME`, register the bundle with `lsregister -f`, and start its
   inner executable. Run the gate checks in `make dev`; stop and report if
   any fails.
2. `internal/desktop/notify.go`: `Notify(session.Alert)` passes the key,
   session, title, subtitle, and text to `hypercubeNotify`, which dispatches
   to the main queue, drops the alert when the window has focus, and adds the
   `UNNotificationRequest`. Declare the shared C functions in
   `internal/desktop/desktop.h`; `hypercubeRun` in `app.m` sets the delegate
   and requests authorization before `[app run]` when the main bundle has an
   identifier.
3. `internal/session/alerts.go`: `Alert{Key, Session, Title, Subtitle, Text}`
   and `Runtime.WatchAlerts(ctx, notify)`. Subscribe, record the starting
   state, then on each `Change` read requests and sessions and call `notify`
   for new alerts. Resubscribe when the channel closes.
4. `cmd/hypercube/main.go`: add `notify` to `startApp`, start the watcher on
   its context, pass `desktop.Notify` from `serveUI` with `--menu`; update the
   call in `cmd/hypercube/integration_test.go`.
5. `internal/ui/web/notifications/notifications.js`:
   `startNativeNotifications(store)` listens for `hypercube-notification` and
   calls `chooseSession`; call it from `internal/ui/web/main.jsx`. Run
   `make ui`.
6. Install: the `Makefile` `install` target copies `$(APP)` to
   `$(APPDIR)/Hypercube.app`, writes the `BINDIR` symlinks, and registers
   the bundle; `scripts/install-service` takes `APPDIR` and execs the
   bundle's executable. Do not run either; installing is the user's call.
7. Tests above, then the live check.

## Documentation updates

- `docs/installation.md` — `make install` puts `Hypercube.app` in
  `~/Applications` (`APPDIR`) with symlinks for `hypercube` and the plugins in
  `~/.local/bin`; `make` builds `bin/Hypercube.app` with the same symlinks in
  `bin/`; the service runs the app's executable; the first start asks for
  notification permission; the dev session runs as Hypercube Dev.
- `docs/usage.md` — the notification card paragraph: macOS notifications for
  the same requests and replies while the window lacks focus, and clicking one
  opens its session.
- `docs/introduction.md` — one sentence beside the notification cards.
- `docs/architecture.md` — the `--menu` paragraph: the app process posts
  macOS notifications as `Hypercube.app`, whose `Contents/MacOS` holds
  `hypercube` and the plugins.
