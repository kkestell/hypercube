# Linux release

## Goal

A beta tag drafts one prerelease that holds the macOS ZIP, the Linux AMD64 and
ARM64 tarballs, and one `SHA256SUMS` covering all three. A tarball made by
`scripts/release` reports the tag from `hypercube version`, installs, and runs
the service in the Linux test environment on every distribution. The user
guides and developer reference describe Linux installation and the supported
targets.

Nothing in this task pushes a tag, creates a GitHub release, or publishes
anything. The macOS packaging steps and screenshot generation stay as they are.

## Related code

- `.github/workflows/release.yml` — on a `v*-beta.*` tag, runs the checks, then
  one macOS job that packages and runs `gh release create --draft --prerelease`.
- `.github/workflows/check.yml` — the checks the release reuses; its Linux jobs
  already run on `ubuntu-24.04` and `ubuntu-24.04-arm`.
- `scripts/release` — packages a clean checkout at a beta tag for the host:
  the ZIP on macOS, `Hypercube-<tag>-linux-<goarch>.tar.gz` (top directory
  `Hypercube/`) on Linux. `check_identity` fails unless the packaged
  `hypercube version` prints `Hypercube <tag> (<commit>)`. It writes a
  `SHA256SUMS` for its one archive.
- `internal/buildinfo/buildinfo.go` — the version is Go's VCS stamp
  (`debug.ReadBuildInfo`), taken from a tag on a clean `HEAD`.
- `make/linux.mk` — `make bundle` needs `rsvg-convert` for the icon.
- `scripts/linux-install`, `scripts/linux-install-service`,
  `scripts/linux-uninstall` — shipped as the bundle's `resources/install`,
  `resources/install-service`, and `resources/uninstall`.
- `scripts/linux/run`, `scripts/linux/test-*`, `Makefile` `linux-test` — the
  Linux test container and its tests. The container copy has no `.git`, so its
  builds report `devel`.
- `scripts/linux/packages` — the per-distribution package lists, including
  Electron's runtime libraries.

## Decisions

- **Linux packages build on GitHub's runners, not in the test container.**
  `scripts/release` needs the Git tag to stamp the version, and the container
  copy has no `.git`. The runners match `check.yml`. Electron's binary comes
  from upstream, and Go builds on Ubuntu 24.04 need at most glibc 2.39, which
  every target exceeds. The job installs `librsvg2-bin` with `apt-get` for
  `rsvg-convert`.
- **Separate packaging jobs and one release job.** The macOS job keeps its
  checkout, setup, and `scripts/release` steps and uploads its ZIP as a
  workflow artifact instead of creating the release. A Linux matrix job
  (`ubuntu-24.04`, `ubuntu-24.04-arm`) runs the same `scripts/release` and
  uploads its tarball. A `release` job on `ubuntu-24.04` downloads the three
  archives into one directory, writes `SHA256SUMS` with
  `sha256sum Hypercube-* > SHA256SUMS`, and runs the existing
  `gh release create` with `GH_REPO` set, so it needs no checkout. Only the
  release job has `contents: write`. `scripts/release` keeps writing its own
  `SHA256SUMS` for local packaging; the jobs upload only the archives.
- **Verify the workflow without a tag through `workflow_dispatch`.** On a
  dispatched run, the packaging jobs create a local lightweight tag
  `v0.0.0-beta.1` in the runner's checkout and package at it; the tag is never pushed
  (`persist-credentials: false`). The release job runs only when
  `github.event_name == 'push'`, so a dispatched run ends with workflow
  artifacts and no release. GitHub runs `workflow_dispatch` only for workflows
  present on the default branch, so this run happens after the user pushes the
  change to `main`; it is not part of this task's work.
- **Verify the tarball in the Linux test environment with a new
  `scripts/linux/test-release`, added to `make linux-test`.** It runs the same
  `scripts/release` that the runners run, with a local tag, so every
  distribution checks tagged packaging, the archive layout, installation from
  an extracted download, and the service. CI's `linux-test` then covers all
  four distributions on AMD64 on every push. It does not install a tarball the
  workflow built; `scripts/release` and `check_identity` are the same on both.
- **The macOS checksum command gains `--ignore-missing`.** `SHA256SUMS` now
  lists three archives and a user downloads one. macOS `shasum` and GNU
  `sha256sum` both accept the option.

## Test plan

- `scripts/linux/test-release` in the container, on Debian, Ubuntu, and Fedora
  on ARM64 locally, and all four on AMD64 in CI:
  - Copy the checkout, without `bin/` and `node_modules`, into a temporary
    directory; `git init`, commit with a fixed name and email, and tag
    `v0.0.0-beta.1`. The other tests keep their checkout without `.git`.
  - `scripts/release v0.0.0-beta.1` succeeds and writes
    `Hypercube-v0.0.0-beta.1-linux-<goarch>.tar.gz` and `SHA256SUMS`;
    `sha256sum -c SHA256SUMS` passes.
  - Extract the tarball into an empty directory; it holds only `Hypercube/`.
  - `Hypercube/resources/install` succeeds; `~/.local/bin/hypercube version`
    prints `Hypercube v0.0.0-beta.1 (<commit>)`.
  - `APP=~/.local/share/hypercube SERVICE=com.github.kkestell.hypercube
    PORT=4280 ~/.local/share/hypercube/resources/install-service` starts the
    service, and `http://127.0.0.1:4280/version` reports the same identity.
  - `~/.local/share/hypercube/resources/uninstall` removes the service, the
    bundle, and the links.
- `make linux-test` still passes with `test-release` last, after
  `test-desktop` has uninstalled.
- `make check` passes.
- Run `actionlint` on `.github/workflows/release.yml` (`go run
  github.com/rhysd/actionlint/cmd/actionlint@latest`; nothing installed) and
  inspect the job graph: macOS and both Linux jobs need `check`; `release`
  needs all three and runs only on a tag push.
- Inspect the documentation diff.

## Implementation plan

1. Add `scripts/linux/test-release` as described in the test plan, following
   `test-install`'s `fail` and `wait_for` style. Append it to the `linux-test`
   command in `Makefile`.
2. Run `make linux-test` for `debian`, `ubuntu`, and `fedora` in Docker on the
   Mac. Fix differences in the owning script.
3. Change `.github/workflows/release.yml`:
   - Triggers: the tag push and `workflow_dispatch`.
   - A job-level `RELEASE_TAG`: `github.ref_name` on a tag, `v0.0.0-beta.1`
     otherwise. On `workflow_dispatch`, a step runs `git tag "$RELEASE_TAG"`
     before packaging.
   - `macos`: the current packaging steps, then `actions/upload-artifact`
     of `bin/release/*.zip`.
   - `linux`: matrix over `ubuntu-24.04` and `ubuntu-24.04-arm`; checkout with
     `fetch-depth: 0`, setup-go, setup-bun, `sudo apt-get install -y
     librsvg2-bin`, `scripts/release "$RELEASE_TAG"`, upload
     `bin/release/*.tar.gz`. Timeout 20 minutes.
   - `release`: `needs: [macos, linux]`, `if: github.event_name == 'push'`,
     `contents: write`; `actions/download-artifact` with `merge-multiple`
     into one directory, `sha256sum`, then `gh release create` with the ZIP,
     both tarballs, and `SHA256SUMS`.
   - Release notes: an Apple Silicon ZIP, ad hoc signed and not notarized,
     with the Open Anyway step; Linux AMD64 and ARM64 tarballs; the
     installation guide link; `SHA256SUMS` verifies the downloads.
4. Run `actionlint` and `make check`.
5. Update the documentation below.

## Documentation updates

Follow `agents/documentation.md`: change only text whose described behavior
changes, declaratively.

- `AGENTS.md` — "Target macOS." becomes "Target macOS and Linux."
- `README.md` — the opening line names macOS and Linux. The "macOS app" feature
  section becomes "Desktop app": menu bar and Dock access on macOS; native
  menus, zoom, and remembered window placement; background startup at login
  through launchd or a systemd user service. The External project tools bullet
  names the Files app on Linux and Terminal on macOS only. The Installation
  index entry stays.
- `docs/installation.md`:
  - Opening: Hypercube runs on macOS on Apple Silicon and on Linux on AMD64 and
    ARM64: Debian 13, Ubuntu 26.04, Fedora 45, and Arch.
  - "Download a beta" splits into macOS and Linux subsections. The macOS text
    stays, with `shasum -a 256 -c --ignore-missing SHA256SUMS`.
  - Linux subsection, with exact commands:
    - The runtime libraries the desktop app needs, libnotify included for
      notifications, from `scripts/linux/packages`:
      `sudo apt install libgtk-3-0t64 libnss3 libasound2t64 libgbm1 libxss1 libnotify4`
      (Debian, Ubuntu); `sudo dnf install gtk3 nss alsa-lib mesa-libgbm
      libXScrnSaver libnotify` (Fedora); `sudo pacman -S --needed gtk3 nss
      alsa-lib mesa libxss libnotify` (Arch).
    - Download the architecture's tarball and `SHA256SUMS`;
      `sha256sum -c --ignore-missing SHA256SUMS`;
      `tar -xzf Hypercube-<tag>-linux-amd64.tar.gz`;
      `Hypercube/resources/install`. It installs the app in
      `~/.local/share/hypercube`, links `hypercube` and the plugins into
      `~/.local/bin`, and adds the launcher entry. The extracted directory can
      then be deleted.
    - On Ubuntu, the installer asks for `sudo` once to add the AppArmor profile
      the desktop app's sandbox needs.
    - Open Hypercube from the application launcher and choose **Set Up
      Background Service**, or run
      `APP="$HOME/.local/share/hypercube" SERVICE=com.github.kkestell.hypercube PORT=4280 "$HOME/.local/share/hypercube/resources/install-service"`.
      Closing the window quits the desktop app; the service keeps running.
    - Upgrade: extract the new tarball, run its `resources/install`, run the
      service command again.
    - Removal: `~/.local/share/hypercube/resources/uninstall` ends agents and
      shells and removes the service, the app, the links, the launcher entry,
      and the AppArmor profile (with `sudo`); configuration and saved state
      stay.
  - "Build and install from a checkout": on Linux `make install` installs to
    `~/.local/share/hypercube` and needs `rsvg-convert`; `make uninstall`
    removes it.
  - "Run at login": the systemd equivalents of the launchd commands —
    `systemctl --user start com.github.kkestell.hypercube`,
    `systemctl --user kill --kill-whom=main --signal=SIGINT com.github.kkestell.hypercube`,
    and the uninstall command for removal. The notifications sentence names
    the platform's notifications.
  - "Identify a build": **Help → About Hypercube** on Linux.
  - "Files on disk": the Linux login service
    (`~/.config/systemd/user/com.github.kkestell.hypercube.service`), the
    installed app, and the AppArmor profile
    (`/etc/apparmor.d/com.github.kkestell.hypercube`).
- `agents/development.md`:
  - "Prepare an official beta": the workflow packages the macOS ZIP on
    `macos-15` and the Linux tarballs on `ubuntu-24.04` and `ubuntu-24.04-arm`,
    and the release job drafts one prerelease with all three and one
    `SHA256SUMS`. `gh workflow run release.yml` runs the packaging on a branch
    under the tag `v0.0.0-beta.1` and keeps the archives as workflow artifacts
    without creating a release. Local packaging with `make release` works on
    Apple Silicon or on Linux for the host's architecture.
  - "Test on Linux": `scripts/linux/test-release` packages a tagged copy of
    the checkout with `scripts/release`, installs from the extracted tarball,
    and checks the version, the service, and removal; `make linux-test` runs it
    last.
