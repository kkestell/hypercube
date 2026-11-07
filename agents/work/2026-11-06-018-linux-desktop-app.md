# Desktop app on Linux

## Plan

`agents/plans/2026-11-06-017-linux-desktop-app.md`

## Summary

The Electron app runs on Linux: window manager frame and menu bar, the File,
Edit, View, Session, Help menus, close-quits, `app.setBadgeCount`, Linux
notifications, the Linux bundle layout and `systemctl --user` checks in
`createSetup(platform)`, and the new waiting-page actions (`restart`, `install`).
`scripts/linux-install` and `scripts/linux-uninstall` quit a running desktop app
before replacing or deleting the bundle, and `make install-service` opens the app
afterward. `scripts/linux/test-desktop` drives the installed app in the test
container and saves screenshots to `bin/linux-test/`, which `scripts/linux/run`
copies out and the CI job uploads. The plan's goal is met.

## Departures from the plan

- `quit_desktop` fails when the app is still running after ten seconds: it
  prints `install: the desktop app (process N) did not quit` (or `uninstall:`)
  and exits 1. This is the change the assignment asked for. The failure path
  was not run.
- `quit_desktop` skips Chromium's child processes by finding `--type=` anywhere
  in `cmdline`, not as a separate argument. Chromium's child processes rewrite
  their `cmdline` into one space-separated string, so a NUL-separated check
  matched every child.
- Step 4 checks the menu structure with the page not yet loaded: File ends with
  Quit and Help ends with About. Settings… arrives with the page's items, so
  step 5 checks it after the backend page loads.
- Step 6 does not select the session before the second prompt, and the badge
  is checked with a third prompt that ends on a question. The badge counts
  sessions with a waiting question (`waitingCount`), so a finished reply did not
  raise it in the run, and the plan's expectation that the reply does
  was wrong. The notification click opens the session, which the test checks
  through the window title.
- Step 7 right-clicks the workspace's name in the sidebar. The session rows sit
  inside the workspace element, so a right click on the element's first match
  opened the session menu.
- `scripts/screenshots/main.js` passes `platform: process.platform` to
  `createMainWindow`, which now takes it.
- `createSetup(platform, execPath)` takes the executable path as an optional
  argument so the tests can place a bundle in a temporary directory.
- `scripts/linux/run` replaces `bin/linux-test/` only when the command made one,
  so `make linux-check` leaves the last `make linux-test` screenshots in place.
- The CI artifact step uses `actions/upload-artifact@v7`, the current release.

## Decisions

- `test-desktop` runs `make install-service` through an asynchronous spawn.
  `execFileSync` blocks the event loop, which reaps the app (a child of the
  script); an unreaped exited app stays a zombie and `kill -0` still succeeds.
- `test-desktop` deletes `~/.local/state/hypercube` and `~/desktop-project` at
  the start and refuses to run while the service is active, so earlier runs'
  workspaces do not show in the sidebar.
- The Linux About panel uses `applicationVersion` as `0.1.0 (abc1234)` and the
  bundle's PNG as `iconPath`.

## Checks run

- `make linux-test` — passed: `test-bundle`, `test-install` (the installer's
  `make install-service` opens the app and `make uninstall` quits it), and
  `test-desktop`; `bin/linux-test/` holds the screenshots and the bus and file
  manager logs.
- `make check` — passed on macOS, including `desktop/setup.test.js` for both
  platforms' installer paths.
- `make linux-check` — passed.
- CI's `linux-test` job — not run; nothing is pushed.
- macOS diff read: `window.js`, `menu.js`, `attention.js`, `setup.js`, and
  `main.js` keep the macOS template, hidden title bar and traffic lights,
  hide-on-close with the Dock calls (now inside `window.js`), tray, `subtitle`
  notifications, and `launchctl` paths. The Dock badge now comes from
  `app.setBadgeCount`, as the plan decided. `make install`, `make place`,
  `make quit-desktop`, `make install-service`, and `make uninstall` were not
  run on the Mac.

## Manual verification

Screenshots from `scripts/linux/run make linux-test`, in `bin/linux-test/`. The
window is 1280×800 on a 1920×1080 openbox desktop with a blue title bar and
minimize, maximize, and close buttons.

- `waiting-install.png`: the frame and a File, Edit, View, Session, Help menu
  bar under it. The page says "Waiting for Hypercube", the address and log
  path, and "Install Hypercube with `…/bin/Hypercube/resources/install`, then
  open it from your applications to set up the service."
- `waiting-headless.png`: the same frame; "Start it with `hypercube ui 4290`."
- `waiting-setup.png`: "Set Up Background Service starts Hypercube at login
  from `~/.local/share/hypercube`" and the Set Up Background Service button; no
  Login Items sentence.
- `window.png`: after setup, the backend's page with an empty workspace list and
  "Add a workspace to get started"; the tab bars start directly under the menu
  bar with the browser's spacing.
- `notification.png`: the window titled "first — desktop-project — Hypercube"
  with the session's two replies and the changed files; a dunst notification at
  the top right reads "first", "desktop-project", "second done".
- `workspace-menu.png`: the workspace's context menu with Open in VS Code, Show
  in Files, and Delete Workspace, and no Open in Terminal.
- `waiting-restart.png`: "The service is installed but not answering. Start it
  with `systemctl --user restart com.github.kkestell.hypercube`."
- `reinstall.png`: the new app, started by `make install-service`, showing the
  same session as before the reinstall.

`test-desktop` also asserts `WM_CLASS` and a nonzero frame, the menu
structure, `Notify` on the bus for the reply and a `LauncherEntry.Update` with count 1
for the question,
`code <repo>`, `xdg-open <repo>`, and `ShowItems ['file://<repo>/first.txt']`,
that `wmctrl -c` exits the app and leaves the service's `MainPID` and the
agent's holder running, that reinstalling ends the old app and starts a new one
with the holder alive, and that `make uninstall` leaves no process from the
bundle.

## Follow-up work

- Test the `quit_desktop` timeout, which needs a stand-in that ignores SIGTERM.
