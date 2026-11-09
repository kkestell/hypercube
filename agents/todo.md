# Linux support rollout (completed)

This is the completed rollout checklist, retained as history. Harness counts
and task descriptions below reflect the rollout, not current support. Use
[development](development.md#test-on-linux) for current Linux tooling and
[architecture](architecture.md) for the current executable layout.

Run the same Hypercube backend, four harness plugins, browser view, and
Electron desktop app on Linux AMD64 and ARM64 across Debian, Ubuntu, Fedora,
and Arch, keeping macOS behavior. Linux uses one application layout and one
tarball per architecture, installed under the user's home directory, with a
systemd user service.

[Linux findings](research/linux.md) records what fails today and the platform
facts behind these tasks. Take each task through its own `kplan`, `kwork`,
and `kreview` cycle as described in [development](development.md), planning
from the code current at that time. From the Linux test environment task on, each
task verifies its Linux behavior there with automated tests, including what
appears on screen. This list does not authorize installing the app on the Mac or
publishing a release.

- [x] **Linux checks in CI and a passing test suite.** Add `make check` jobs
  on `ubuntu-24.04` and `ubuntu-24.04-arm`, keeping the macOS job. Make the
  suite pass there:
  - Copy `.worktreeinclude` paths with `cp -R` plus the platform's clone flag
    (`-c` on macOS, `--reflink=auto` on Linux), keeping symlinks, modes, and
    fast clones on both. Update the APFS text in `docs/configuration.md`.
  - When a harness exits before its plugin's first write, report the
    harness's exit status and stderr instead of the broken pipe, in the
    Claude Code, Codex, and Pi plugins.
  - Find why `TestTranscriptKeepsOrderAndReplacesItems` sometimes sees the
    session idle before its final item, and fix the code or the test.
  - Pass `SHELL` through `scripts/dev`'s clean environment, so panel shells
    in `make dev` do not fall back to `/bin/zsh`, which Linux may lack.
    systemd sets `SHELL` for the service. Make `scripts/dev`'s port message
    work without `lsof`, which Arch does not install by default.
- [x] **Desktop actions.** Move Add Workspace's folder chooser and the Open
  in VS Code, Reveal in Finder, Open in Terminal, and Show in Finder items from
  the backend to the desktop app, which runs in the graphical session on both
  platforms: Electron's `dialog.showOpenDialog`, `shell.showItemInFolder`,
  and `shell.openPath`, and the `code` command for VS Code. Delete the
  backend's `osascript` and `open` calls and `POST /open`; `POST /workspace`
  takes the chosen path. Use the platform's labels ("Show in Files" on
  Linux). Linux has no standard terminal application, so Open in Terminal is
  macOS-only. Its Linux items are tested with the desktop app task.
- [x] **Linux test environment.** A Debian 13 container image that boots
  systemd, with a user manager for a test user, Xvfb, Electron's runtime
  libraries, Git, Go, Bun, `rsvg-convert`, and Playwright. It runs natively on
  ARM64 in Docker on the Mac and on AMD64 in CI. One command builds the
  checkout in it and runs a test that drives the Linux desktop app with
  Playwright's Electron support and saves screenshots for agents to inspect.
  Sessions use the scripted harness plugin, which needs no credentials.
  Chromium's sandbox is off in containers. Document the command in
  `agents/development.md`. This task proves the environment with `make check`
  and `make dev`; the later tasks add their own tests to it.
- [x] **Linux bundle.** `make bundle` on Linux builds a directory with the
  Electron client (`hypercube-desktop`), `hypercube`, the four plugins, the UI
  pages, a PNG icon, the `.desktop` file, and the service installer, and
  `make release` packs one `tar.gz` per architecture. Render the icon with
  `rsvg-convert`. Set `desktopName` in `desktop/package.json` to match the
  `.desktop` file. Keep the macOS `.app`.
- [x] **Install and service.** Install the bundle in a stable path under
  `~/.local/share`, link the CLI and plugins into `~/.local/bin`, and add the
  `.desktop` file; provide `make install-service` and a removal command. The
  systemd user unit starts the backend through `$SHELL -l -i -c` and uses
  `KillMode=process`, so stopping or replacing the service leaves agents and
  shells running for the new backend to adopt. On Ubuntu 23.10 and later,
  install an AppArmor profile for the bundle's Electron executable with one
  `sudo` step. Verify login-shell environment with bash and zsh, and adoption
  across `systemctl --user restart`. Test install, service start, and adoption
  across a restart in the Linux test environment. Check the AppArmor step on
  a real Ubuntu kernel, such as GitHub's `ubuntu-24.04` runner.
- [x] **Desktop app on Linux.** Use a standard window frame with Electron's
  menu bar (File, Edit, View, Session, Help; About in Help, Quit in File).
  Closing the window quits the client; the tray stays macOS-only. Badge with
  `app.setBadgeCount`. Keep notifications and click-through. Use Dock calls
  only on macOS. Resolve the bundle and installer from the Linux layout; the
  waiting page shows `systemctl --user` commands and offers service setup.
- [x] **Keys and labels.** Take the platform from the desktop bridge and
  derive bindings and displayed keys from it: ⌘ becomes Ctrl and ⌥ becomes Alt on Linux. Apply this to the menus,
  page shortcuts, quick open, and the shortcut reference. In the panel
  terminal on Linux, only shortcuts marked for the terminal leave the shell;
  copy and paste are Ctrl+Shift+C and Ctrl+Shift+V. Keep the macOS editing
  keys in `xterm.ts` macOS-only. Rename `notifications.macos` to
  `notifications.desktop`, migrating the saved value at startup, and update
  Settings, `docs/configuration.md`, and the user's config together.
- [x] **Distribution checks.** Run the Linux test environment's install,
  service, desktop app, session, panel shell, notification, and restart tests
  in Ubuntu, Fedora, and Arch images as well as Debian: ARM64 on the Mac, and
  AMD64 in CI, where Arch's images run. Fix differences in the owning task's
  code; claim the full matrix when every check passes.
- [x] **Release.** The beta workflow builds both Linux tarballs with
  `SHA256SUMS` beside the macOS ZIP; check a downloaded tarball's
  `hypercube version` in the Linux test environment. Update `README.md`, `AGENTS.md`'s target line,
  `docs/installation.md`, and `agents/development.md` with Linux commands and
  the supported targets. Screenshots stay on macOS.
