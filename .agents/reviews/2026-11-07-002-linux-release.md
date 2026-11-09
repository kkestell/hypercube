# Linux release

## Scope and coverage

The staged changes for `agents/plans/2026-11-07-002-linux-release.md`:
`.github/workflows/release.yml`, `scripts/release`,
`scripts/linux/test-release`, `Makefile`, `AGENTS.md`, `README.md`,
`docs/installation.md`, and `agents/development.md`, read as whole files.

The workflow was checked by reading it against `check.yml`, the previous
`release.yml`, and the `actions/download-artifact@v8` inputs. `actionlint` did
not run, and the workflow itself has not run; a dispatched run after the change
reaches `main` is the first execution. The Arch package command was compared
with `scripts/linux/packages` but not run, since Arch has no ARM64 image.

Workflow points confirmed by reading:

- `macos` and `linux` need `check`; `release` needs both and runs only when
  `github.event_name == 'push'`, with the implicit success condition.
- Only `release` has `contents: write`; the workflow default is
  `contents: read`, which the reused `check.yml` also gets.
- A dispatched run tags only the runner's checkout, checks out with
  `persist-credentials: false`, has no push step, and skips `release`, so it
  cannot create a release or push a tag.
- The macOS job keeps the old checkout, setup, and `scripts/release` steps;
  `RELEASE_TAG` moved to the job and equals `github.ref_name` on a tag push.
- `SHA256SUMS` lists the ZIP and both tarballs in `sha256sum` format, which
  `shasum -a 256 -c --ignore-missing` and `sha256sum -c --ignore-missing` both
  read.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Correctness | Fixed | Linux packaging installs a package without refreshing the package index |
| F2 | Medium | Documentation | Fixed | "Run at login" gives macOS behavior and commands for Linux |
| F3 | Low | Correctness | Fixed | The release job downloads every artifact in the run |
| F4 | Low | Documentation | Fixed | The README's project tools sentence is hard to read |

## F1. Linux packaging installs a package without refreshing the package index

Medium · Correctness · Fixed · `.github/workflows/release.yml:68`

The `linux` job ran `sudo apt-get install -y librsvg2-bin` on the runner's
image index. When the index is older than the archive, the install fails with
a missing package file and no tarball is built.

### Evidence

GitHub's runner documentation directs workflows to run `sudo apt-get update`
before `apt-get install` because the image's index can be stale.

### Resolution

The step runs `sudo apt-get update` before the install.

## F2. "Run at login" gives macOS behavior and commands for Linux

Medium · Documentation · Fixed · `docs/installation.md:146`

The section applies to `make install-service` on both platforms, but:

- it said closing the window leaves Hypercube running; on Linux it quits the
  desktop app;
- running without a service said to open `~/Applications/Hypercube.app`, and
  the other-port example started only the macOS app;
- removing the service on Linux pointed to "the uninstall command above", the
  beta section's full uninstall, while the macOS text removes only the
  service.

### Evidence

`desktop/window.js` hides the window on close only on macOS;
`make/linux.mk` installs to `~/.local/share/hypercube`; `desktop/main.js` reads
`--port=` on both platforms.

### Resolution

The section states the window behavior per platform, opens the app from the
launcher on Linux, gives
`~/.local/share/hypercube/hypercube-desktop --port=4282`, and removes only the
service with `systemctl --user disable --now`, removing the unit file, and
`systemctl --user daemon-reload`. These commands ran in the Debian and Fedora
containers.

## F3. The release job downloads every artifact in the run

Low · Correctness · Fixed · `.github/workflows/release.yml:92`

`download-artifact` without `pattern` downloads all artifacts, including the
`linux-test-<distro>` screenshots and logs that `check.yml` uploads, and merges
them into `archives/`. `sha256sum Hypercube-*` and `gh release create` name
only the archives, so the release was unaffected, but the job fetched
unrelated files.

### Resolution

The packaging artifacts are named `release-macos` and
`release-linux-<runner>`, and the release job downloads `pattern: release-*`.

## F4. The README's project tools sentence is hard to read

Low · Documentation · Fixed · `README.md:66`

"Visual Studio Code, Finder or the Files app, or Terminal on macOS" now reads
"Visual Studio Code, Finder on macOS or Files on Linux, and Terminal on macOS",
matching the app's labels.

## Checks run

- Every Linux command in `docs/installation.md` for a beta download and the
  login service, run as written in the Debian and Fedora containers on ARM64
  against a tarball from `scripts/release`: the runtime package names are the
  installed ones, `sha256sum -c --ignore-missing` with a `SHA256SUMS` that also
  lists a missing ZIP, extract, install, deleting the extracted directory,
  `install-service`, SIGINT with `systemctl --user kill`, `systemctl --user
  start`, service-only removal, and `resources/uninstall` — passed.
- `shasum -a 256 -c --ignore-missing` on macOS with a missing entry — passed.
- `make check` — passed.
- `make linux-test LINUX_DISTRO=debian` and `LINUX_DISTRO=fedora` — passed;
  `test-bundle`, `test-install`, `test-desktop`, and `test-release` each
  passed.

## Verdict

The workflow's job graph, permissions, and dispatch path meet the plan, and
the macOS packaging is unchanged. F1 and F2 are fixed. `actionlint` and the
first dispatched run remain to be done.
