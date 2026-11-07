# Linux support

Support the same Hypercube backend, four harness plugins, browser interface,
and Electron desktop app on Linux AMD64 and ARM64 across Debian, Ubuntu,
Fedora, and Arch, while retaining macOS behavior. Prefer one Linux application
layout and archive format per architecture, with a user-owned installation
and a systemd user service.

These are separate tasks, not one implementation plan. Take each task through
its own `kplan`, `kwork`, and `kreview` cycle with fresh agents as described in
[development](development.md). Plan the task from the code and evidence current
at that time. Do not treat later tasks or this list as authorization to install
the app or publish a release.

- [ ] **Debian AMD64 baseline and portable core.** Prepare an isolated graphical
  Debian VM, record its version and display session, and build and run the
  current checkout there with isolated XDG paths and a scratch repository.
  Fix evidenced Linux build or runtime failures in their owning packages.
  Make `.worktreeinclude` copying portable while preserving directories,
  symlinks, executable modes, and independent copies. Replace the `/bin/zsh`
  panel-shell fallback without changing behavior when `SHELL` is set. Verify
  worktrees, terminals, holders, and backend restart adoption on Linux and
  corresponding shared behavior on macOS.
- [ ] **Linux application bundle.** Build the Electron client, `hypercube`,
  sibling plugins, embedded UI, and icons natively on Linux in one bundle per
  architecture, distributed as a compressed tarball. Keep macOS packaging.
- [ ] **User installation and service.** Install the bundle under `~/.local`
  with a desktop launcher and CLI links. Provide manual install and removal.
  Add a systemd user service that receives the user's normal interactive shell
  environment and preserves agents and shells across backend replacement.
  Verify the shell invocation with bash and another supported login shell.
  Investigate systemd's default control-group shutdown before choosing the
  service stop behavior: signaling the backend alone does not establish holder
  survival. See [systemd.kill(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.kill.html).
- [ ] **Linux desktop behavior.** Use native Linux window controls and menus;
  closing the window quits only the client. Keep notifications and their
  click-through behavior, and use badge support where the launcher provides
  it. Preserve the macOS desktop behavior.
- [ ] **Browser behavior and preferences.** Derive shortcuts and key labels
  from the desktop platform, using Ctrl/Alt on Linux and Command/Option on
  macOS. Cover menus, browser handlers, quick open, the shortcut reference,
  and terminal editing keys. Add Linux service setup to the waiting page.
  Migrate `notifications.macos` to a platform-neutral setting while preserving
  existing choices; update the configuration example and user's config
  together. Verify the integrated behavior in the Debian desktop session.
- [ ] **Architecture and distribution coverage.** Build and run the ARM64
  bundle in a Debian ARM64 VM, and test the archive, install, and service flow
  on Ubuntu, Fedora, and Arch. Validate each distribution on one architecture
  and both architectures on Debian, expanding the matrix only for evidenced
  differences. Claim the full matrix only after these checks pass.
- [ ] **Release readiness.** Add Linux to `make check` in CI, produce AMD64
  and ARM64 archives with verified version identities and checksums, and
  confirm a downloaded archive in the VM. Update installation and development
  guides with exact commands and supported targets. Keep screenshot generation
  on macOS. Attach artifacts to a draft beta only when the user authorizes a
  release.
