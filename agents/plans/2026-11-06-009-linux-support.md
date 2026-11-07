# Linux support

## Goal and scope

Run the same Hypercube backend, harness plugins, browser interface, and Electron
desktop app on Linux while retaining macOS behavior. Support Linux AMD64 and
ARM64 on Debian, Ubuntu, Fedora, and Arch. Use a Debian AMD64 graphical virtual
machine for the first implementation and end-to-end test. Add ARM64 and the
other distributions to validation before claiming the full support matrix.

This plan is the requested deliverable. It does not authorize implementation,
installation, a VM change, or a release. Changes to `hypercube hold`, agent or
shell launch, or plugins will reach an active session only after that session
restarts. The desktop and backend service can be replaced independently.

Use one Linux application layout and one archive format per architecture rather
than distro-specific application code or four package formats. These distros
normally provide systemd user services; support for other init systems is
outside this request. The browser view can run without the desktop app or a
login service, as it does on macOS.

## Design

### Portable core and worktrees

- Build `hypercube` and all four plugins natively in the Linux VM. Keep the
  existing sibling-executable discovery and the XDG configuration/state paths.
  Fix actual Linux build or runtime failures in their owning packages; do not
  introduce a second session runtime or plugin protocol.
- `internal/workspace/workspace.go` invokes macOS `cp -c -R` for ignored files
  named by `.worktreeinclude`. Replace that command with a copy that works on
  both macOS and Linux, preserving directories, symlinks, executable modes, and
  independent copies. Plain copying is sufficient; retain copy-on-write only
  if it can remain a simple optional optimization. Update the APFS-only text in
  `docs/configuration.md`.
- Use the user's shell for panel terminals. Replace the `/bin/zsh` fallback in
  `internal/terminal/terminal.go` with a usable Linux default without changing
  the behavior when `SHELL` is set. Verify holder attachment, PTYs, signals,
  socket paths, and session recovery on Linux before changing any other core
  process code.

### Linux application and service

- Keep the macOS `.app`, LaunchAgent, and signing path. Add a Linux Electron
  bundle for each architecture with the desktop executable, `hypercube`, four
  sibling plugins, embedded UI resources, and icons. The Linux archive is a
  compressed tarball containing this one layout. Build it on Linux; do not
  rely on macOS icon tools, `plutil`, `codesign`, or a Darwin Go toolchain.
  Build outputs stay in `bin/` and are generated from source assets.
- Install the bundle in a stable user-owned location under `~/.local`, add a
  `.desktop` launcher and CLI links under `~/.local/bin`, and point the desktop
  app and service at that same bundle. Upgrading the bundle must leave running
  agents and shells intact while the backend restarts and adopts them. Provide
  commands for manual installation and removal. No root access or distro
  package manager is required.
- Add a systemd user service for `hypercube ui 4280`. Its launcher must obtain
  the user's normal interactive shell environment so harness executables and
  credentials resolve as they do from a terminal, without modifying `HOME` or
  the user's shell configuration. Confirm the invocation with bash and at
  least one other supported login shell in the VM. The desktop waiting page
  offers service setup, shows the log path, and gives Linux start/restart
  instructions. Preserve its existing macOS flow. Make service replacement
  stop the old backend with SIGTERM, not SIGINT, so holders survive.
- On Linux, keep the standard framed window and its native window controls.
  The existing hidden title bar and traffic-light inset stay macOS-only; the
  browser top row uses its normal spacing on Linux. Closing the Linux window
  quits only the desktop client, leaving the service and agents running. This
  avoids depending on a tray icon that may be absent in a desktop environment.
  If the tray is retained on Linux, it is an optional way to show or quit the
  client, not the sole way back to a hidden window. Keep native notifications
  and their click-through behavior. Use Electron's Linux badge API where the
  desktop launcher supports it; omit macOS-only Dock and tray-title calls.
- Build a normal Linux application menu. Keep the macOS application menu and
  its roles on macOS. Resolve the app's resources and service installer from
  its actual platform layout rather than deriving a macOS `.app` path on Linux.

### Browser behavior and saved preferences

- Define the platform once at the desktop bridge/browser boundary and derive
  shortcut bindings and displayed key labels from it. Use Ctrl/Alt on Linux
  and keep Command/Option on macOS. Apply the same bindings to native menus,
  browser handlers, the shortcut reference, quick open, and terminal editing
  keys. Test for conflicts with common Linux desktop shortcuts in the VM.
- Rename the saved `notifications.macos` preference to a platform-neutral
  native-notification field. Upgrade an existing config at startup when it has
  the old key but not the new one, preserving the user's choice; write only the
  new key afterward. Update the Go config, UI types and Settings text, the
  example in `docs/configuration.md`, and the user's
  `~/.config/hypercube/config.json` together as required by `AGENTS.md`. This
  is a JSON config migration, not a database schema change.

## Files and boundaries

| Concern | Files to change |
| --- | --- |
| Build, install, release | `Makefile`, `scripts/install-service` or a Linux counterpart, `scripts/release`, `scripts/dev`, `.github/workflows/check.yml`, `.github/workflows/release.yml`; add a Linux launcher/service template only where needed. |
| Desktop | `desktop/main.js`, `setup.js`, `window.js`, `dock.js`, `attention.js`, `menu.js`, `accelerator.js`, and their focused tests. |
| Core | `internal/workspace/workspace.go`, `internal/terminal/terminal.go`; touch `internal/hold`, `internal/session`, or plugins only for failures reproduced in Linux tests. |
| Browser and config | `internal/ui/web/app/shortcuts.ts`, `title-bar.ts`, `settings-preferences.tsx`, `types.ts`, `desktop/waiting.tsx`, `native.d.ts`, terminal key handling, and `internal/harness/config.go`. Keep platform rules in a small number of owners rather than scattering checks through components. |
| Guides | `README.md`, `docs/installation.md`, `docs/configuration.md`, `docs/usage.md`, `agents/architecture.md`, `agents/development.md`, and the now inaccurate macOS-only scope line in `AGENTS.md`. Change only text whose described behavior changes. |

Delete macOS assumptions from shared code when the common behavior can be
expressed once; keep genuinely platform-specific packaging and native UI code
separate. Do not add distro-specific runtime branches, a generic service
framework, or a new plugin abstraction.

## Phases

Use a separate session for each phase. Complete and verify each phase before
starting the next; keep the Debian VM and its isolated test state available
through the work. An independent architecture review of this plan against
`AGENTS.md` comes before implementation. A substantial implementation gets an
independent review of whole touched files before release. Planning does not
authorize installation or publication.

1. **Review and Debian baseline:** Review the design, then prepare a graphical
   Debian AMD64 VM with Git, the required Go and Bun versions, Electron's Linux
   runtime libraries, and a user systemd session. Record its version and display
   session. Build and run the current checkout to capture concrete failures.
   Use isolated XDG paths and a scratch repository; do not replace installed
   Hypercube state. The result is a reviewed plan and a reproducible baseline.
2. **Portable core:** Fix the copy of `.worktreeinclude` files and the shell
   fallback. Run `make check` in the VM. Exercise worktree creation, use, and
   removal with included ignored files, a panel terminal, and backend
   SIGTERM/restart adoption. Run corresponding macOS checks. The result is a
   working Linux backend and browser view without a desktop installation.
3. **Bundle, install, and service:** Build the Linux archive and install its
   bundle, launcher, CLI links, and systemd user service in the Debian VM.
   Verify manual install and removal, service start and restart, shell
   environment with bash and another supported login shell, and adoption of
   an existing agent and shell after backend replacement. The service can be
   set up manually at this stage; desktop setup follows in phase 5.
4. **Electron desktop:** Make the Linux client start from the installed bundle
   with native window controls, menus, notifications, click-through behavior,
   and platform-correct resource paths. Verify closing and reopening the
   client leaves the service, agent, and shell running. Check light and dark
   appearance in the Debian VM.
5. **Browser behavior and preferences:** Connect the desktop platform to the
   browser shortcuts, key labels, terminal editing keys, and waiting-page
   service setup. Migrate the saved native-notification preference and update
   the matching configuration example and user config. Verify shortcut
   conflicts in the Debian desktop session and repeat the end-to-end install,
   setup, session, restart, and notification flow there. Check changed shared
   behavior on macOS.
6. **Architectures and distros:** Build and run the ARM64 bundle in a Linux
   ARM64 VM. Smoke-test the same archive/install/service flow on Ubuntu,
   Fedora, and Arch VMs. Because packaging and runtime paths are shared,
   validate each distro on one architecture and both architectures on Debian;
   expand the matrix if a distro or architecture exposes a real difference.
   Do not claim the full matrix until those checks pass.
7. **Release readiness:** Run `make check` on macOS and Linux in CI. Produce
   separate Linux AMD64 and ARM64 archives, verify their embedded `hypercube
   version` identity and checksums, and attach them alongside the existing
   macOS ZIP to the draft beta. Confirm a downloaded archive in the VM. Update
   the installation and development guides with exact commands and supported
   targets. Keep screenshot generation on macOS; it is not a Linux release
   gate.
