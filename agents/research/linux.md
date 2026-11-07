# Linux support findings

What the current code does on Linux, from a test run and a search of the
source for macOS-only behavior. Checked on 2026-11-06 at `df6ca21` plus the
uncommitted work in the checkout at that time.

## Build

- `GOOS=linux go build ./...` and `go vet ./...` pass for AMD64 and ARM64. The
  Go code needs no cgo (`modernc.org/sqlite` and `creack/pty` are pure Go), so
  any host can build the Linux executables.
- `internal/ui` dependencies have Linux builds in `bun.lock` for both
  architectures.
- Electron 45 and `@electron/packager` produce Linux builds for either
  architecture from any host.
- Packaging uses macOS-only tools: `sips` and `iconutil` for icons, `plutil`
  for the bundle, `codesign`, `lsregister`, `lsappinfo`, and `osascript`
  (`Makefile`, `scripts/release`, `scripts/dev --app`).

## Go tests on Linux

Run in a `golang:1.27.3` Debian container on ARM64 as a non-root user,
`go test -race ./...`, then the failing packages again three times.

| Test | Result | Cause |
| --- | --- | --- |
| `internal/workspace` `TestWorktreeSetUp`, `TestWorktreeIncludesLiteralPaths`, `TestWorktreeIncludesExcludeManagedWorktrees` | Fail every run | `copyIncluded` runs `cp -c -R`; GNU `cp` has no `-c`. Worktrees in a checkout with `.worktreeinclude` cannot be created. |
| `plugins/codex` `TestStartReportsAProcessThatEnds` | Fails 1 run in 3 | A harness that exits at once loses the race with the first write to its stdin, and the plugin reports `write \|1: broken pipe` (Codex: "incompatible Codex app-server; update Codex") instead of the harness's own error. The Claude Code and Pi plugins failed the same way in the first run (`TestStartFailedResume`, `TestStartReportsAProcessThatEnds`). The race exists on macOS too; Linux timing exposes it. |
| `internal/session` `TestTranscriptKeepsOrderAndReplacesItems` | Failed 1 run in 3 | The session became idle with the streamed text item still at its first version (`Hello`, not done). Passes 5 of 5 on macOS. Either the final item is stored after the idle state or the test waits on the wrong condition. |
| `internal/harness` `TestModelsCancel`, `internal/session` `TestDeletingASessionEndsItsAgents` | Fail without `docker run --init`, pass with it | Container artifact: with no init process, killed orphans stay as zombies and `kill(pid, 0)` still finds them. Run container tests with `--init`. |

Two more failures appear under `make check` (see
[the Linux checks plan](../plans/2026-11-06-012-linux-checks.md)):
`internal/hold` `TestTerminalResizeHangupAndRetention` fails every run because
Debian's `/bin/sh` (dash) makes a bare `exit` in the test's HUP trap return the
killed `sleep`'s status, and `desktop/setup.test.js` runs macOS's `plutil`.
Every other package passes, including `internal/terminal` (PTYs, holder
sockets, and session recovery).

## macOS-only behavior in the running app

| Where | What | Effect on Linux |
| --- | --- | --- |
| `internal/ui/actions.go` | Add Workspace runs `osascript` to show a folder chooser. | Adding a workspace without a path fails. |
| `internal/ui/open.go` | Open in VS Code, Reveal in Finder, Open in Terminal, and Show in Finder run `open -a` and `open -R`. | All four menu items fail. |
| `internal/workspace/workspace.go` | `cp -c -R` (above). | Worktree creation fails when `.worktreeinclude` exists. |
| `internal/terminal/terminal.go` | Panel shell falls back to `/bin/zsh` when `SHELL` is unset. | systemd sets `SHELL` for user services, but `scripts/dev` starts the backend with `env -i` and no `SHELL`, so `make dev` panel shells fail where zsh is not installed. |
| `desktop/dock.js`, `desktop/attention.js` | `app.dock.show()`, `app.dock.hide()`, `app.dock.setBadge()`, `tray.setTitle()`. | `app.dock` is undefined on Linux, so the app throws the first time it shows the window. |
| `desktop/attention.js` | A tray icon is the way back to a hidden window. | GNOME has no tray without an extension (Fedora Workstation, Debian GNOME). |
| `desktop/window.js` | Closing hides the window; `titleBarStyle: 'hidden'` with traffic-light inset. | A hidden window has no reliable way back; the title-bar layout is macOS's. |
| `desktop/menu.js` | `appMenu`, `services`, `hide`, `hideOthers`, `unhide`, `front`, `zoom` roles. | Linux has no application menu or these roles. |
| `desktop/main.js` | Bundle path is `execPath/../../..`; `launchctl` service checks; AppTranslocation check. | Wrong bundle path and service status. |
| `desktop/setup.js`, `scripts/install-service` | launchd LaunchAgent. | No service. |
| `internal/ui/web/app/shortcuts.ts`, `desktop/accelerator.js` | Bindings and labels written with ⌘ and ⌥, mapped to `metaKey` and `Command`. | Shortcuts need the Super key, which the desktop usually owns. |
| `internal/ui/web/terminal/xterm.ts` | Cmd+Backspace and Cmd/Option+Arrow editing keys. | Not used on Linux; xterm.js already sends Ctrl+Arrow and Ctrl+Backspace sequences. |
| `internal/ui/web/files/quick-open.tsx` | Quick open checks `metaKey` for P. | Same as shortcuts. |
| `internal/ui/web/desktop/waiting.tsx` | `launchctl kickstart` and System Settings instructions. | Wrong instructions. |
| `internal/harness/config.go`, Settings | `notifications.macos`, "Show macOS notifications". | Wrong name on Linux. |

## Platform facts that shape the design

- **systemd stops every process in a service's control group.** The default
  `KillMode=control-group` sends SIGTERM, then SIGKILL, to all of them;
  `Setsid` does not move a process out of the group. Agent and shell holders
  started by the backend would die on every service stop or restart.
  `KillMode=process` signals only the main process and leaves the rest
  running, as `ssh.service` does to keep sessions open across an sshd restart.
  systemd's stop signal is SIGTERM, which already leaves agents running.
- **systemd sets `HOME`, `USER`, `LOGNAME`, and `SHELL` for user services**,
  but not the user's shell configuration. Starting the backend through
  `$SHELL -l -i -c 'exec …'`, as the LaunchAgent does, gives it the PATH and
  credentials a terminal has. Interactive bash without a terminal prints
  job-control warnings to the log.
- **Ubuntu 23.10 and later restrict unprivileged user namespaces** through
  AppArmor (`kernel.apparmor_restrict_unprivileged_userns=1`). Chromium's
  sandbox needs one, so an Electron app unpacked into the home directory
  fails to start there unless an AppArmor profile allows its executable path
  or `chrome-sandbox` is owned by root with mode 4755. Both need root once;
  the profile survives upgrades because it names the path. Debian, Fedora,
  and Arch allow unprivileged user namespaces by default.
- **Launcher integration keys on the desktop file.** Electron uses
  `desktopName` in `package.json` for the Wayland app ID and the Unity
  launcher API behind `app.setBadgeCount`; the `.desktop` file name and
  `StartupWMClass` must match it for window grouping and badges. Notification
  click-through depends on the notification server supporting the default
  action, which GNOME and KDE do.
- **Electron menu accelerators on Linux use Ctrl.** With ⌘ mapped to Ctrl,
  menu keys such as Ctrl+B, Ctrl+J, Ctrl+N, Ctrl+L, Ctrl+K, and the Edit menu's
  Ctrl+C and Ctrl+V collide with shell and readline keys in the panel terminal.
  Which of the menu and the page sees a key first must be checked on Linux;
  `MenuItem.registerAccelerator: false` shows a key without registering it.
  Linux terminals use Ctrl+Shift+C and Ctrl+Shift+V for copy and paste.
- **A systemd user service may lack the graphical session's environment**
  (`DISPLAY`, `WAYLAND_DISPLAY`). GNOME and KDE import it into the user
  manager; minimal compositors may not. The desktop app always runs in the
  graphical session, and it is the supported client.
- **The renderer shows untrusted workspace files.** HTML previews are
  script-free (`sandbox` iframe and CSP), but images, PDFs, audio, and video
  from the workspace go through Chromium's decoders and viewers. Chromium's
  process sandbox contains an exploit in those; `--no-sandbox` removes it.

## Test environments on this Mac

Docker runs ARM64 Linux containers natively. Checked on 2026-11-06:

- A Debian 13 container started with `/sbin/init` and `--privileged` boots
  systemd in its own cgroup namespace and runs a lingering user's manager. A user service with `KillMode=process`
  left its child running across `systemctl --user restart`.
- Electron 45.2.3 in a `node:22-trixie` container, under `xvfb-run` with
  `ELECTRON_DISABLE_SANDBOX=1`, started, was driven by Playwright's Electron
  support (`_electron.launch`, `app.evaluate` in the main process, page
  screenshots), and reported `process.platform` as `linux`. It needed
  `xvfb`, `libgtk-3-0`, `libnss3`, `libasound2`, `libgbm1`, and `libxss1`.

Containers share Docker's kernel, so Ubuntu's AppArmor restriction on user
namespaces and Chromium's sandbox are tested only on a real Ubuntu kernel,
such as GitHub's `ubuntu-24.04` runner. Emulated AMD64 machines are too slow
for a desktop; AMD64 runs in CI.
