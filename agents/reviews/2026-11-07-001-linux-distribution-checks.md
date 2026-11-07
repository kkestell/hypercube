# Linux distribution checks

## Scope and coverage

The staged changes for
`agents/plans/2026-11-07-001-linux-distribution-checks.md`:
`scripts/linux/Dockerfile`, `scripts/linux/packages`, `scripts/linux/run`,
`Makefile`, `.github/workflows/check.yml`, and `agents/development.md`, read
against the plan, the work log, and the callers of `scripts/linux/run`
(`make linux-check`, `make linux-test`, `scripts/linux/test-apparmor`, CI).

The Arch package list was checked against Arch's package database
(archlinux.org) but not run: Arch has no ARM64 image. The CI matrix runs it.

Points checked with no finding:

- Debian default. `LINUX_DISTRO` defaults to `debian` in both `run` and the
  `Makefile`; Debian installs the same package list as before, and
  `npm install -g --prefix /usr/local` matches Debian's default prefix. The
  only Debian differences are the planned ones: the image tag
  `hypercube-linux-debian` and the output directory `bin/linux-test/debian/`.
  Nothing else reads `bin/linux-test/` or the old tag. `test-apparmor` builds
  through `run` with the default and only reads its standard output.
- UID 1000. The `userdel` runs in a Dockerfile `RUN`, so it changes only the
  image. Debian, Fedora, and Arch have no user at UID 1000; Ubuntu's `ubuntu`
  user is removed. `getent`'s exit status for a missing UID is masked by the
  pipeline into `cut`, so `set -e` does not stop the build.
- Cache volume. It holds the Go module cache (content-addressed), the Go build
  cache (keyed by toolchain and C compiler identity), Bun's install cache
  (content-addressed), and Electron downloads (keyed by version and
  architecture). `tester` is UID 1000 in every image. `linux-test-all` runs
  the distributions one after another; CI jobs have separate volumes.
- CI. The matrix entries match `run`'s names, `fail-fast: false` keeps one
  failure from cancelling the others, and the artifact names
  `linux-test-<distro>` are unique per job. Each artifact contains
  `<distro>/`, because `run` copies into that subdirectory.
- `make linux-test-all LINUX_DISTRO=x`: the sub-make's own `LINUX_DISTRO=$d`
  overrides the inherited command-line value (checked with a stand-in
  Makefile).
- Arch packages. Every name exists in `core` or `extra`. `mesa` ships
  `libgbm.so.1`; `gtk3` depends on `adwaita-fonts` and `libcups`;
  `imagemagick` ships `/usr/bin/import`, which `test-desktop` uses; `/sbin/init`
  comes from `systemd-sysvcompat` in the `base` image.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Low | Correctness | Fixed | `pacman -Scc --noconfirm` keeps the package cache |
| F2 | Low | Clarity | Fixed | "Test on Linux" paragraph lost its structure |
| F3 | Low | Efficiency | Fixed | `LINUX_DISTROS` runs `uname` on every `make` |

## F1. `pacman -Scc --noconfirm` keeps the package cache

Low · Correctness · Fixed · `scripts/linux/packages:34`

The second `-c` makes pacman ask "Do you want to remove ALL files from
cache? [y/N]", and `--noconfirm` takes the default, No. The step meant to keep
the Arch image small removed nothing.

### Evidence

pacman's `sync_cleancache` asks that question with a default of No at
`-cc`; `--noconfirm` answers every question with its default.

### Resolution

The Arch arm removes `/var/cache/pacman/pkg/*` directly.

## F2. "Test on Linux" paragraph lost its structure

Low · Clarity · Fixed · `agents/development.md:161`

The change split "`make linux-test` runs the Linux tests:" from the list of
the tests it runs by inserting `make linux-test-all` between them, and left a
line of 130 columns.

### Resolution

The paragraph again introduces the three tests after `make linux-test`, and
mentions `make linux-test-all` and CI after the output directory. Rewrapped at
80 columns.

## F3. `LINUX_DISTROS` runs `uname` on every `make`

Low · Efficiency · Fixed · `Makefile:87`

`:=` with `$(shell uname -m)` ran `uname` whenever the Makefile was read,
including `make check`.

### Resolution

`LINUX_DISTROS` is now a recursive variable, expanded only by
`linux-test-all`.

## Checks run

- `sh -n scripts/linux/packages` — Passed.
- `make -n linux-test-all` on the Mac — Runs Debian, Ubuntu, and Fedora.
- `make check` — Passed.
- `make linux-test` (Debian, ARM64) — Passed `test-bundle`, `test-install`, and `test-desktop`.
- `make linux-test LINUX_DISTRO=ubuntu` (ARM64) — Passed all three tests.
- Arch (AMD64) and the CI matrix — Not run; the parent runs CI.

## Verdict

Ready for CI. No medium or high findings; three low findings fixed (F1–F3).
The Arch list and the AMD64 runs remain unverified until the CI matrix runs.
