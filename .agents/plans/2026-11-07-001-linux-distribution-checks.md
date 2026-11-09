# Linux distribution checks

## Goal

Run the Linux tests (`make linux-test`: `scripts/linux/test-bundle`,
`test-install`, and `test-desktop`) in Ubuntu, Fedora, and Arch containers as
well as Debian. ARM64 runs natively in Docker on the Mac for Debian, Ubuntu,
and Fedora. AMD64 runs in CI for all four; Arch publishes AMD64 images only.
When every distribution passes on both architectures where it runs, tick the
task in `agents/todo.md`.

Trial runs on the Mac (ARM64, at `b166df7`) with a draft of the image below:

| Distribution | Result | Difference found |
| --- | --- | --- |
| Ubuntu 26.04 LTS | All three tests pass | The base image has a user `ubuntu` with UID 1000, so `useradd -u 1000 tester` fails. |
| Fedora 45 | All three tests pass, screenshots render text | `pam_systemd.so` is in `systemd-pam`, a weak dependency of `systemd`; without it the test user's manager fails ("`$XDG_RUNTIME_DIR` is not set") and the container boots degraded. |
| Arch | Not run | No ARM64 image, and this Mac's Docker has no AMD64 emulation. Every package name below exists in Arch's repositories; `xorg-server-xvfb` provides `xvfb-run`, and `gtk3` brings `adwaita-fonts`. |

Both differences are in the test image. The install scripts, service unit,
desktop app, and tests needed no change on Ubuntu or Fedora: the POSIX
scripts run under Fedora's bash `/bin/sh` as under dash, `/bin/zsh` and
`/usr/bin/fish` resolve on every merged-`/usr` distribution, and the shell
configuration the tests append to `~/.bashrc` is read by the interactive login
shell the service starts, whether the distribution's `~/.bashrc` returns early
for non-interactive shells (Debian, Ubuntu, Arch) or not (Fedora).

## Related code

- `scripts/linux/Dockerfile` — the Debian 13 image: packages, Go, Bun,
  Playwright, the test user with lingering, the Electron sandbox switch.
- `scripts/linux/run` — builds the image as `hypercube-linux`, boots it, copies
  the checkout in, runs a command as `tester` under Xvfb, and copies
  `bin/linux-test/` out.
- `Makefile` — `linux-check` and `linux-test`.
- `.github/workflows/check.yml` — the `linux-test` job on `ubuntu-24.04`.
- `scripts/linux/test-apparmor` — builds the bundle through `scripts/linux/run`
  with the default distribution; unchanged.
- `agents/development.md` — "Test on Linux".

## Decisions

### One Dockerfile and a package script

The steps other than package installation are the same on every distribution:
Go and Bun tarballs, Playwright from npm, the test user, lingering, the
environment, and `/sbin/init`. Only the package manager and package names
differ. So:

- `scripts/linux/Dockerfile` starts `FROM ${BASE}` with `ARG BASE`, then
  copies and runs `scripts/linux/packages`, then the shared steps.
- `scripts/linux/packages` (POSIX sh) reads `ID` from `/etc/os-release` and
  installs that distribution's list with one `case` arm each: `debian|ubuntu`
  (apt), `fedora` (dnf), `arch` (pacman). Any other `ID` fails with a message.
  Each list is the same set of commands as today's, under that distribution's
  names.

Package lists, as tried:

- Debian and Ubuntu: today's list, unchanged, with
  `DEBIAN_FRONTEND=noninteractive`.
- Fedora, with `--setopt=install_weak_deps=False`: `systemd systemd-pam
  dbus-daemon dbus-tools ca-certificates curl git unzip xz make gcc gcc-c++
  glibc-devel binutils xorg-x11-server-Xvfb xorg-x11-xauth librsvg2-tools nodejs
  npm procps-ng sqlite gtk3 nss alsa-lib mesa-libgbm libXScrnSaver
  desktop-file-utils zsh fish iproute util-linux-core findutils openbox dunst
  ImageMagick wmctrl xdotool xprop python3-gobject-base shadow-utils`.
- Arch, with `pacman -Syu --noconfirm --needed`: `systemd dbus ca-certificates
  curl git unzip xz make gcc binutils xorg-server-xvfb xorg-xauth librsvg nodejs
  npm procps-ng sqlite gtk3 nss alsa-lib mesa libxss desktop-file-utils zsh fish
  iproute2 openbox dunst imagemagick wmctrl xdotool xorg-xprop python-gobject`.
  The Arch list is unverified until CI runs it.

Shared-step changes:

- Go and Bun choose their download from `uname -m` (`x86_64` → `amd64`/`x64`,
  `aarch64` → `arm64`/`aarch64`) instead of `dpkg --print-architecture`.
- `npm install -g --prefix /usr/local`, because Arch's npm installs globals
  under `/usr`; `NODE_PATH` stays `/usr/local/lib/node_modules`.
- Before `useradd`, delete the user that has UID 1000, if any
  (`user=$(getent passwd 1000 | cut -d: -f1); if [ -n "$user" ]; then
  userdel -r "$user"; fi`). `run` hard-codes `/run/user/1000`.

Four Dockerfiles would repeat the shared steps four times; one Dockerfile with
a `case` inside a `RUN` would be hard to read. The script keeps each
distribution's list in one place.

### Releases

`debian:trixie`, `ubuntu:26.04` (current LTS), `fedora:45` (current stable),
and `archlinux:latest` (rolling). Pinned tags keep runs reproducible; moving to
Ubuntu's next LTS or Fedora's next release is a one-line change in `run`.

### Choosing the distribution

- `scripts/linux/run` reads `LINUX_DISTRO` (`debian`, `ubuntu`, `fedora`, or
  `arch`; default `debian`) and maps it to the base image with a `case`. It
  builds and tags `hypercube-linux-$LINUX_DISTRO`, so the images coexist and
  keep their layer caches. `arch` on a machine other than `x86_64` fails at
  once: "Arch Linux publishes AMD64 images only". An unknown name fails with
  the list of names.
- Screenshots and logs go to `bin/linux-test/$LINUX_DISTRO/`, so runs of
  different distributions do not replace each other's output.
- The cache volume `hypercube-linux-cache` stays shared. It holds Go module,
  Go build, Bun, and Electron download caches, which are keyed by content and
  toolchain, and every image's `tester` is UID 1000.
- `Makefile`: `LINUX_DISTRO ?= debian`; `linux-check` and `linux-test` pass it
  to `run`. `linux-test-all` runs `linux-test` for each distribution with an
  image for the host's architecture (Debian, Ubuntu, and Fedora; Arch too on
  `x86_64`), in turn, stopping at the first failure.

### CI

The `linux-test` job becomes a matrix over `distro: [debian, ubuntu, fedora,
arch]` on `ubuntu-24.04` with `fail-fast: false`, running `make linux-test
LINUX_DISTRO=${{ matrix.distro }}` and uploading `bin/linux-test/` as
`linux-test-${{ matrix.distro }}`. Debian is one entry of the matrix rather
than a separate job: the steps are identical. The `check` jobs stay as they
are. ARM64 stays on the Mac.

Cost: the repository is public, so standard runners are free. The current
`linux-test` job took 3 minutes in the latest run; the four jobs run in
parallel, so the workflow's wall time is unchanged and runner time is about
four times that job's. `main` has no branch protection, so the job names
changing from `linux-test` to `linux-test (debian)` and so on breaks no
required check.

## Test plan

- On the Mac, `make linux-test-all` passes for Debian, Ubuntu 26.04, and Fedora
  45 on ARM64, and each `bin/linux-test/<distro>/` has the screenshots, with
  readable text.
- In CI, the four `linux-test` jobs pass on AMD64. Inspect the Arch artifact's
  screenshots. Fix any Arch difference in the owning code (package script,
  install scripts, service unit, desktop app, or tests) and rerun.
- `LINUX_DISTRO=arch scripts/linux/run true` on the Mac fails with the AMD64
  message before building; `LINUX_DISTRO=centos` fails with the list of names.
- `make linux-check` (Debian) and `scripts/linux/test-apparmor` still work.

## Implementation plan

1. Add `scripts/linux/packages` and change `scripts/linux/Dockerfile` as
   above.
2. Change `scripts/linux/run`: `LINUX_DISTRO`, the base image map, the Arch
   architecture check, the per-distribution tag and output directory.
3. Add `LINUX_DISTRO` and `linux-test-all` to `Makefile`.
4. Run `make linux-test-all` on the Mac.
5. Change `.github/workflows/check.yml` to the matrix and confirm all four jobs
   pass, Arch included.
6. Update `agents/development.md` and tick "Distribution checks" in
   `agents/todo.md`.
7. Remove the old `hypercube-linux` image from the Mac (`docker rmi
   hypercube-linux`); the per-distribution tags replace it.

## Documentation updates

- `agents/development.md`, "Test on Linux": the container is Debian 13, Ubuntu
  26.04, Fedora 45, or Arch, chosen with `LINUX_DISTRO`; `make linux-test-all`
  runs every distribution the host's architecture has; Arch runs on AMD64 only,
  so it runs in CI; output lands in `bin/linux-test/<distro>/`; CI runs all four
  on AMD64.
