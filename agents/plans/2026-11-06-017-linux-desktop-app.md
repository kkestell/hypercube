# Desktop app on Linux

## Goal

The Electron app in `desktop/` runs on Linux. Today it throws at the first
window show (`app.dock` is undefined), and its window, menus, closing, bundle
paths, service checks, and waiting page assume macOS.

When the work is done, on Linux:

- The window has the window manager's frame and Electron's menu bar. The page's
  top row uses its normal spacing.
- The menu bar is File, Edit, View, Session, Help, laid out as VS Code lays out
  its Linux menus: Settings and Quit in File, About at the end of Help.
- Closing the window quits the desktop app. The service and its agents keep
  running. There is no tray icon.
- The launcher badge comes from `app.setBadgeCount`. Notifications and their
  click-through work.
- The app finds its bundle, `resources/`, and the service installer in the
  Linux layout.
- The waiting page offers to set up the service, gives the `systemctl --user`
  command for a service that is installed but not answering, and shows the log.
- `make install-service` quits a running desktop app before replacing the
  bundle and opens the app afterward.
- "Show in Files", the open-folder item, and Open in VS Code work.

A new test in the Linux test container drives the installed app against the
installed service and saves screenshots. macOS behavior does not change.

Keyboard shortcuts and key labels belong to the next task, "Keys and labels".
Until then, Linux menus show accelerators built from the macOS bindings.

## Related code

- `desktop/main.js` — starts the window, menu, attention, and setup handlers;
  derives the bundle as `execPath/../../..`; decides the waiting page's
  action, including the macOS AppTranslocation check; shows About.
- `desktop/window.js` — the window: `titleBarStyle: 'hidden'`, traffic-light
  position, the title-bar size it reports to the page, and the close handler
  that hides the window.
- `desktop/dock.js` — Dock show and hide; used only by `window.js`.
- `desktop/menu.js` — the macOS menu template with `appMenu`, `services`,
  `hide`, `hideOthers`, `unhide`, `window`, and `front` roles.
- `desktop/attention.js` — tray icon, Dock badge, tray title, notifications.
- `desktop/setup.js`, `desktop/setup.test.js` — `launchctl print` check and
  the installer at `Contents/Resources/install-service`.
- `desktop/actions.js` — already takes `platform`; the Linux open actions use
  `shell.openPath`, `shell.showItemInFolder`, and the `code` command.
- `desktop/com.github.kkestell.hypercube.desktop`, `desktop/package.json`
  (`desktopName`) — launcher identity.
- `internal/ui/web/desktop/waiting.tsx`, `internal/ui/web/native.d.ts`
  (`SetupStatus`) — the waiting page and its bridge types.
- `internal/ui/web/app/title-bar.ts` — the page sizes its top row from the
  reported title bar; `{height: 0}` gives the browser spacing.
- `internal/ui/web/app/desktop-actions.ts` — "Show in Files" on Linux; Open in
  Terminal is macOS-only.
- `internal/session/alerts.go` — a finished turn's reply raises an alert
  (title: session title; subtitle: workspace, then parent title).
- `scripts/linux-install`, `scripts/linux-uninstall`, `make/linux.mk`,
  `make/macos.mk` (`quit-desktop`, `open`).
- `scripts/linux/run`, `scripts/linux/Dockerfile`, `scripts/linux/test-install`,
  `Makefile` (`linux-test`), `.github/workflows/check.yml`.

## Decisions

### Facts checked in the test container

Electron 45.2.3 on Debian 13 under Xvfb (ARM64):

- The packaged bundle throws `Cannot read properties of undefined (reading
  'show')` from `dock.js` at the first window show.
- A default `BrowserWindow` gets the window manager's frame (openbox) and
  GTK's menu bar. A root-window screenshot (`import -window root`) shows both.
  With `desktopName` set, the window's `WM_CLASS` is
  `com.github.kkestell.hypercube`, matching the launcher entry's
  `StartupWMClass`.
- A window-manager close (`wmctrl -c`) emits `close`, then the app quits when
  no `window-all-closed` listener exists. SIGTERM to the main process runs
  `before-quit`, the window's `close`, and `will-quit`, then exits.
- `app.setBadgeCount(3)` returns true and emits
  `com.canonical.Unity.LauncherEntry.Update` with
  `application://com.github.kkestell.hypercube.desktop` and `count` 3, without
  libunity installed.
- With dunst as the notification server, `Notification.show()` sends
  `Notify` with the app name, title, body, a `default` action, and the
  `desktop-entry` hint; `dunstctl action 0` sends `ActionInvoked default` and
  Electron emits `click`. `subtitle` is not sent on Linux. Without any
  notification server, `show()` blocks the main process for D-Bus's timeout.
- `shell.openPath(dir)` runs `xdg-open dir`. `shell.showItemInFolder(file)`
  calls `org.freedesktop.FileManager1.ShowItems` with the file's URI when a
  file manager owns that name (Nautilus, Dolphin, Nemo, and Thunar do);
  without one, Electron runs `xdg-open .` in the file's grandparent folder.
- `systemctl --user show -p LoadState --value <unit>` prints `not-found` for a
  unit with no file and `loaded` once installed.
- Playwright's `_electron.launch({executablePath})` starts the packaged
  `hypercube-desktop` with its `--inspect=0 --remote-debugging-port=0` flags.
  The launch could not complete because of the crash above; the first test
  run confirms it.

### Where platform differences live

`main.js` reads `process.platform` once and passes `platform` to each owner,
as it already does for `actions.js`. Each owner keeps its macOS and Linux
parts side by side, chosen in one place:

| Owner | macOS | Linux |
| --- | --- | --- |
| `window.js` | hidden title bar, traffic-light inset, close hides, Dock follows the window | frame and menu bar, window icon, close quits |
| `menu.js` | current template, unchanged | File, Edit, View, Session, Help |
| `attention.js` | tray icon, tray title and tooltip | no tray; subtitle folded into the body |
| `setup.js` | bundle at `execPath/../../..`, `Contents/Resources/install-service`, `launchctl`, AppTranslocation | bundle at `dirname(execPath)`, `resources/install-service`, `systemctl --user`, installed location |
| `actions.js` | unchanged | unchanged |
| `waiting.tsx` | Login Items sentence | no Login Items sentence |

`dock.js` is deleted; its two calls move into `window.js` under macOS, the
only place that uses them.

### Window

- macOS keeps `titleBarStyle: 'hidden'`, `trafficLightPosition`, and the
  52×98 title bar. Linux passes neither option and adds
  `icon: <resourcesPath>/com.github.kkestell.hypercube.png`, which window
  managers that read `_NET_WM_ICON` show; GNOME and KDE take the icon from the
  launcher entry.
- The title-bar size is `{height: 0, inset: 0}` on Linux, so `titleBarVars`
  returns nothing and the page uses its browser spacing. The page code does
  not change.
- Close: macOS keeps hide-on-close and `app.dock.hide()` when no window is
  visible. On Linux the handler only saves the bounds; the window closes and
  Electron's default quits the app. `main.js` adds no `window-all-closed`
  listener.
- `show()` calls `app.dock.show()` only on macOS.

### Menus

macOS keeps its template as is. Linux, following VS Code's Linux menus:

```text
File     Settings… (the page's App items) | Quit
Edit     role editMenu
View     the page's View items | Toggle Full Screen, Toggle Developer Tools
Session  the page's Session items
Help     the page's Help items | About
```

`setMenu(items, run, platform)` builds one of two templates from the same
`section()` helper. Quit is `{role: 'quit'}`. About runs `run('about')` as on
macOS.

`showAbout` in `main.js`: on Linux, `setAboutPanelOptions` has no `version`
field, so the commit goes into `applicationVersion` (`0.1.0 (abc1234)`), and
`iconPath` names the bundle's PNG. macOS options stay as they are.

### Badge and notifications

- `setBadge(count)` calls `app.setBadgeCount(count)` on both platforms. On
  macOS this sets the same Dock badge `app.dock.setBadge` did ('' for zero).
  The tray title and tooltip update only where the tray exists (macOS).
- `createAttention` creates the tray only on macOS.
- On Linux, where `subtitle` is dropped, the notification body is the
  subtitle and the text on separate lines, so the workspace and parent still
  show. macOS keeps `subtitle`.

### Bundle, service, and the waiting page

`setup.js` becomes `createSetup(platform)` and owns everything that depends
on the bundle layout and service manager. It returns `service`,
`defaultPort`, `bundle`, `status(port)`, and `install(shell)`:

- `bundle`: macOS `path.resolve(execPath, '../../..')`; Linux
  `path.dirname(execPath)`.
- Installer: macOS `Contents/Resources/install-service`; Linux
  `resources/install-service`. Same environment as today (`APP`, `SERVICE`,
  `PORT`, `SHELL`).
- Installed: macOS `launchctl print gui/<uid>/<service>` succeeds; Linux
  `systemctl --user show -p LoadState --value <service>` prints `loaded`. A
  unit file present means installed; the service answering is what the
  waiting page is waiting for.
- Where setup may run: macOS refuses a bundle under `/AppTranslocation/`
  (`move`). Linux refuses a bundle other than the installed one,
  `realpath($XDG_DATA_HOME/hypercube)` (`install`), because the unit names
  the bundle's `hypercube`, and the installer is what puts the bundle there.

`status(port)` returns the action and the command the page shows:

| Action | When | Command |
| --- | --- | --- |
| `headless` | port is not the default | `hypercube ui <port>` |
| `restart` | the service is installed | macOS `launchctl kickstart gui/<uid>/<service>`; Linux `systemctl --user restart <service>` |
| `move` | macOS, translocated | none |
| `install` | Linux, not the installed bundle | `<bundle>/resources/install` |
| `setup` | otherwise | none |

`restart` replaces `kickstart`. `SetupStatus` becomes `{address, log, bundle,
action, command}`; `port`, `uid`, and `service` go, since the commands carry
them. `setup-install` still refuses unless the action is `setup`.

`waiting.tsx`:

- `headless`: "Start it with `<command>`."
- `restart`: "The service is installed but not answering. Start it with
  `<command>`."
- `move`: unchanged.
- `install`: "Install Hypercube with `<command>`, then open it from your
  applications to set up the service."
- `setup`: the current sentence and button. The Login Items sentence shows
  only when `globalThis.hypercube?.platform === 'darwin'`; Linux desktops have
  no common equivalent.
- The opening sentence keeps the address and log path on both platforms.

### Reinstalling

Replacing the bundle under a running desktop app breaks it: Electron loads
`resources/ui/waiting.html` and other files after start. So the quit belongs
to whatever replaces or removes the bundle:

- `scripts/linux-install`, before it moves the old bundle aside (only when it
  replaces one), and `scripts/linux-uninstall`, before deleting the bundle,
  each run a `quit_desktop` function: for each `/proc/<pid>` whose `exe` is
  the installed `hypercube-desktop` (resolved with `pwd -P`) and whose
  `cmdline` has no `--type=` argument (Chromium's child processes), send
  SIGTERM and poll until it exits, up to ten seconds. SIGTERM is Electron's
  normal quit, so the window saves its bounds. The function is eight lines;
  each script keeps its own copy, since the two scripts run from different
  places (the bundle being installed and the checkout or installed bundle).
- `make/linux.mk`'s `install-service` opens the app after the service
  installer, as `make/macos.mk` runs `open`:
  `if [ -n "$$DISPLAY$$WAYLAND_DISPLAY" ]; then setsid -f "$(INSTALLED)/hypercube-desktop" >/dev/null 2>&1 </dev/null; fi`.
  A shell without a graphical session skips it.

`make install` alone therefore also quits a running app and does not reopen
it; `make install-service` reopens it.

`scripts/linux/test-install` runs under Xvfb, so its `make install-service`
now opens the app, and its `make uninstall` quits it; its existing "no
process runs from the bundle" check covers that.

### Desktop actions on Linux

`actions.js` needs no change. The test checks the real paths: Open in VS Code
runs `code <dir>`, "Show in Files" on a workspace runs `xdg-open <dir>`, and
"Show in Files" on a file calls `FileManager1.ShowItems`. Electron's fallback
without a file manager service (opening the grandparent folder) is left
alone; every major desktop provides the service.

### The Linux desktop test

`scripts/linux/test-desktop` is a Node script (Playwright is installed
globally in the image, `NODE_PATH` set) with a header comment. It shells out
for `make`, `systemctl`, `curl`, `wmctrl`, `xprop`, `dunstctl`, and `import`.
Waits poll a condition with a deadline. On failure it prints the service log
and the bus log.

Observing the desktop:

- **Window manager:** `openbox`, so windows have frames, `wmctrl -c` closes
  like the close button, and `_NET_FRAME_EXTENTS` is set.
- **Notifications:** `dunst`, a real notification server. `dbus-monitor
  --session "interface=org.freedesktop.Notifications"
  "interface=com.canonical.Unity.LauncherEntry"` writes the `Notify` calls and
  badge signals to `bus.log`. `dunstctl action 0` clicks the newest
  notification's default action.
- **File manager:** `scripts/linux/file-manager`, a 20-line Python script
  (`python3-gi`) that owns `org.freedesktop.FileManager1` and prints each
  `ShowItems` call.
- **Other applications:** a directory of stub `code` and `xdg-open` scripts
  that append their arguments to a file, first on the app's `PATH`.
- **Screenshots:** `import -window root` after each visible state, into
  `bin/linux-test/`.

Steps, starting from a clean home after `test-install`:

1. `make install` (no service). Start openbox, dunst, `dbus-monitor`, and the
   file-manager stub. Write the scripted scene and shell configuration as
   `test-install` does, and build `./plugins/scripted` over the installed
   `hypercube-harness-claude`.
2. **Not the installed bundle.** Launch `bin/Hypercube/hypercube-desktop`.
   The waiting page shows `…/bin/Hypercube/resources/install`.
   `waiting-install.png`. Close the app.
3. **Another port.** Launch the installed app with `--port=4290`. The page
   shows `hypercube ui 4290`. `waiting-headless.png`. Close.
4. **Not installed.** Launch the installed app. The page shows Set Up
   Background Service and the log path. Check the window: `WM_CLASS` is
   `com.github.kkestell.hypercube`, `_NET_FRAME_EXTENTS` is nonzero,
   `isMenuBarVisible()` is true, and the menu (from
   `Menu.getApplicationMenu()` in the main process) is File, Edit, View,
   Session, Help with Settings… and Quit in File and About last in Help.
   `waiting-setup.png`.
5. **Setup.** Click Set Up Background Service. The unit becomes active, the
   backend answers `/version`, and the window loads `http://localhost:4280/`.
   `window.png`.
6. **Notification and badge.** Add a workspace and start a scripted session
   with `curl`, as `test-install` does, and wait for its first turn. Minimize
   the window, send the second prompt, and wait for its file. `bus.log` has a
   `Notify` whose summary is the session's title, whose body has the
   workspace name and the reply, and whose hints name
   `com.github.kkestell.hypercube`, and a `LauncherEntry.Update` for
   `application://com.github.kkestell.hypercube.desktop` with a count of at
   least 1. `dunstctl action 0` shows and focuses the window on that session.
7. **Open actions.** Right-click the workspace in the sidebar: the menu has
   Open in VS Code and Show in Files and no Open in Terminal. Each records its
   call: `code <repo>` and `xdg-open <repo>`. "Show in Files" from a file's
   context menu in the files panel prints `ShowItems ['file://<repo>/first.txt']`.
8. **Closing.** `wmctrl -c Hypercube`. The app's main process exits; the
   unit's `MainPID` is unchanged and answers, and the agent's holder is
   alive.
9. **Installed but not answering.** `systemctl --user stop` the unit and
   launch the installed app. The page shows `systemctl --user restart
   com.github.kkestell.hypercube` and the log path. `waiting-restart.png`. Run
   that command; the window loads the backend.
10. **Reinstall.** With the app from step 9 running, `make install-service`.
    The old app's process exits, a new `hypercube-desktop` main process runs
    from the installed bundle with a "Hypercube" window, and the holder is
    alive. `reinstall.png`.
11. `make uninstall`. No process runs from the bundle.

The app is launched with `_electron.launch({executablePath, args, env})`,
where `env` puts the stubs first on `PATH`. Launches never overlap, because
of the single-instance lock.

### Getting the screenshots out

`scripts/linux/run` copies the container's `~/hypercube/bin/linux-test/`, if
the command created it, to the checkout's `bin/linux-test/` when the command
ends, pass or fail, replacing the previous copy (with `tar` over `docker
exec`, as it copies the checkout in). The CI `linux-test` job uploads
`bin/linux-test/` with `actions/upload-artifact` under `if: always()`.

## Test plan

- `make linux-test` passes: `test-bundle`, `test-install` (now with the app
  opened and quit by the installer), and `test-desktop`. The screenshots in
  `bin/linux-test/` show the frame, menu bar, page, and each waiting-page
  state; inspect them.
- `make check` passes on macOS and in `make linux-check`, including
  `desktop/setup.test.js` covering both platforms' installer paths through
  `createSetup`.
- CI's `linux-test` job passes on `ubuntu-24.04` and uploads the screenshots.
- macOS: read the diff of `window.js`, `menu.js`, `attention.js`, `setup.js`,
  and `main.js` against the old behavior; the macOS template, title bar,
  hide-on-close, tray, and `launchctl` paths are the same. Do not run
  `make install`, `make place`, `make quit-desktop`, `make install-service`,
  or `make uninstall` on the Mac.

## Implementation plan

1. `setup.js`: `createSetup(platform)` with the bundle, installer, installed
   check, and `status(port)` above; update `setup.test.js` to create the
   installer at each platform's path and run `install` for both.
2. `main.js`: pass `platform` to the window, menu, attention, and setup; use
   `setup.status` and `setup.bundle`; Linux About options.
3. `window.js`: per-platform chrome, title-bar size, close, and Dock calls;
   delete `dock.js`.
4. `menu.js`: the Linux template.
5. `attention.js`: tray only on macOS, `app.setBadgeCount`, Linux body.
6. `native.d.ts` and `waiting.tsx`: the new `SetupStatus` and actions.
7. `scripts/linux-install`, `scripts/linux-uninstall`: `quit_desktop`.
   `make/linux.mk`: open the app after `install-service`, with an updated
   target comment.
8. `scripts/linux/Dockerfile`: add `openbox`, `dunst`, `dbus-bin`,
   `imagemagick`, `wmctrl`, `x11-utils`, `python3-gi`, `gir1.2-glib-2.0`.
   `scripts/linux/run`: copy `bin/linux-test/` out.
9. `scripts/linux/file-manager`, `scripts/linux/test-desktop`; add
   `test-desktop` to `linux-test` in the `Makefile`; upload the artifact in
   `.github/workflows/check.yml`.
10. Run `make linux-test` and `make check`; inspect the screenshots.
11. Update documentation.

## Documentation updates

- `agents/development.md` — "Test on Linux": `make linux-test` also runs
  `scripts/linux/test-desktop`, what it checks, the desktop services it
  starts, and that screenshots and logs land in `bin/linux-test/`.
- `agents/architecture.md` — "Backend, desktop app, and terminals": the
  desktop app shows the menu bar icon and Dock badge on macOS and a launcher
  badge on Linux; on Linux, closing the window quits it.
- `agents/research/linux.md` — under platform facts, `showItemInFolder`'s
  behavior with and without `org.freedesktop.FileManager1`, and that
  notifications on Linux drop `subtitle`.
