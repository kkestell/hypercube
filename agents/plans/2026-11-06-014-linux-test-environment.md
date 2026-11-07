# Linux test environment

## Goal

Give Linux work a place to build and test the checkout on Linux from this Mac
and in CI. A Debian 13 container image boots systemd with a user manager for
a test user and has Xvfb, Electron's runtime libraries, Git, Go at the
`go.mod` version, Bun at the `internal/ui/package.json` version,
`rsvg-convert`, Node, and Playwright. It runs natively on ARM64 in Docker on
the Mac and on AMD64 in CI.

One command copies the checkout into a fresh container, runs a command there
as the test user on a virtual display, and removes the container:

```sh
scripts/linux/run              # make check
scripts/linux/run make dev     # the dev backend
```

This task proves the environment by running `make check` and `make dev` in it. The later tasks (bundle, install and
service, desktop app, keys) add their tests to `scripts/linux/` and run them
through the same command.

## Related code

- `Makefile` — `check`, `dev`, and `screenshots`, which installs the scripted
  harness plugin under each harness name in a temporary folder.
- `scripts/dev` — `make dev`; serves the backend on 127.0.0.1:4281 and passes
  `SHELL` through a clean environment.
- `web/Makefile` — `make check` builds Hugo extended from source with
  `go install`, which needs a C compiler; Go's build cache makes later builds
  quick.
- `.github/workflows/check.yml` — `make check` on macOS, `ubuntu-24.04`, and
  `ubuntu-24.04-arm`.
- `agents/research/linux.md` — "Test environments on this Mac" records the
  systemd and Electron container flags this plan uses.

## Decisions

### Files and names

- `scripts/linux/Dockerfile` — the image.
- `scripts/linux/run` — the command. It follows `scripts/dev`: Bash,
  `set -euo pipefail`, a header comment with usage.
- `Makefile` gets `linux-check`, which runs `scripts/linux/run`, beside
  `dev` and the other script targets.

The image is tagged `hypercube-linux`. The build context is `scripts/linux`,
which holds no source, so no `.dockerignore` is needed.

### The image

Based on `debian:trixie`. Packages: `systemd systemd-sysv dbus-user-session
ca-certificates curl git unzip xz-utils make gcc g++ libc6-dev xvfb xauth
librsvg2-bin nodejs npm procps sqlite3 libgtk-3-0t64 libnss3
libasound2t64 libgbm1 libxss1`. GTK brings DejaVu fonts, so screenshots show
text. `gcc` is needed by `go test -race` and Hugo extended.

- Go: `ARG GO_VERSION`; the `go.dev/dl` tarball for `dpkg --print-architecture`
  into `/usr/local/go`, with `go` and `gofmt` linked into `/usr/local/bin` so
  login shells find them.
- Bun: `ARG BUN_VERSION`; the GitHub release zip
  (`bun-linux-x64.zip` or `bun-linux-aarch64.zip` from `uname -m`) into
  `/usr/local/bin`, with `bunx` linked to `bun`.
- Playwright: `ARG PLAYWRIGHT_VERSION=1.65.0`; `npm install -g
  playwright@$PLAYWRIGHT_VERSION`, with `NODE_PATH=/usr/local/lib/node_modules`
  so test scripts `require('playwright')`. Its Electron support needs no
  browser download; tests launch the Electron in `desktop/node_modules`.
- The test user `tester` (UID 1000, Bash) lingers through
  `/var/lib/systemd/linger/tester`, so systemd starts its user manager at
  boot. `/home/tester/.cache` exists and belongs to `tester`, so a new cache
  volume mounted there takes that owner.
- `systemd-modules-load.service` is masked; it fails in a container and would
  leave the system `degraded`.
- `ENV GOMODCACHE=/home/tester/.cache/go-mod
  BUN_INSTALL_CACHE_DIR=/home/tester/.cache/bun ELECTRON_DISABLE_SANDBOX=1`.
  Chromium's sandbox does not work in a container.
- `STOPSIGNAL SIGRTMIN+3` and `CMD ["/sbin/init"]`.

`run` passes `GO_VERSION` from the `go` line of `go.mod` and `BUN_VERSION`
from `packageManager` in `internal/ui/package.json`, the same sources CI uses,
so the image follows version changes. It runs `docker build -q` every time;
a cached build takes about a second.

### Getting the checkout in

`run` streams the tracked and untracked, unignored files into the container:

```sh
git ls-files -z -co --exclude-standard |
	while IFS= read -r -d '' f; do
		if [ -e "$f" ] || [ -L "$f" ]; then printf '%s\0' "$f"; fi
	done |
	COPYFILE_DISABLE=1 tar --no-xattrs --no-mac-metadata --null -T - -cf - |
	docker exec -i -u tester "$container" sh -c 'mkdir ~/hypercube && tar -xf - -C ~/hypercube'
```

The loop drops tracked files deleted from the working tree. Ignored paths
(`bin/`, `node_modules/`, `internal/ui/dist/`, `web/bin/`) and `.hypercube/`
(ignored through `.git/info/exclude` in the main checkout) are left out. The
copy has no `.git`, so Go builds report version `devel`. Nothing is mounted
from the checkout, and builds write only inside the container. The copy of
this checkout (about 700 files, 12 MB) took 0.4 seconds.

### Caches

One named volume, `hypercube-linux-cache`, mounted at `/home/tester/.cache`,
holds Go's build cache (`~/.cache/go-build`, the default), Go modules, Bun's
install cache, and Electron downloads (`~/.cache/electron`). Each container
starts from a fresh copy and fresh `node_modules`, which Bun fills from its
cache. Concurrent runs share the volume; Go's caches are safe for that.
`docker volume rm hypercube-linux-cache` resets it.

### The container and the command

Each run starts its own container, so runs from several worktrees do not
collide:

```sh
docker run -d --rm --privileged --cgroupns=host \
	-v /sys/fs/cgroup:/sys/fs/cgroup:rw -v hypercube-linux-cache:/home/tester/.cache \
	hypercube-linux
```

A trap removes it with `docker rm -f` on exit, including Ctrl-C.
`docker stop` waits for systemd's shutdown, which is slower.

`run` waits with `systemctl is-system-running --wait`, which returns once boot
jobs finish, including the test user's manager, and fails unless the result
is `running`. It then copies the checkout and runs the command (default
`make check`) with:

```sh
docker exec $tty -u tester -w /home/tester/hypercube \
	-e SHELL=/bin/bash -e XDG_RUNTIME_DIR=/run/user/1000 \
	-e DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus \
	"$container" bash -lc 'exec xvfb-run -a -s "-screen 0 1920x1080x24" "$@"' bash "$@"
```

`$tty` is `-it` when standard input is a terminal and `-i` otherwise, so CI
works and Ctrl-C reaches `make dev`. The command's exit status is `run`'s.
The `XDG_RUNTIME_DIR` and D-Bus address let `systemctl --user` reach the user
manager for the service tests.

### Sessions

The image has no harness CLIs. Tests that start sessions build the scripted
plugin and link it under each harness name in their own folder, as
`make screenshots` does, with a scene file. This task starts no sessions:
`make dev` in the container serves the view, and creating a session there
fails because the real plugins find no harness.

### CI

No CI job in this task. The `ubuntu-24.04` and `ubuntu-24.04-arm` jobs already
run `make check` on Linux. The first task that adds Linux desktop tests to
`scripts/linux/` adds a job that runs them through `scripts/linux/run` on
`ubuntu-24.04`.

## Test plan

- `scripts/linux/run` on the Mac with no image or cache volume builds the
  image and passes `make check`. Measured with a trial image: image build
  about 1 minute, cold `make check` about 2 minutes (Go modules, race builds,
  Hugo, Bun installs).
- A second `scripts/linux/run` passes in about 10 seconds: container start
  under a second, copy 0.4 seconds, cached Go builds and tests.
- `scripts/linux/run make dev` starts the dev backend; from another
  terminal, `docker exec` into the container and `curl
  http://localhost:4281/` returns the page. Ctrl-C ends the run and leaves no
  container (`docker ps -a --filter ancestor=hypercube-linux`).
- `git status --ignored` in the checkout is unchanged after both runs.
- A one-off Playwright script, run through `scripts/linux/run` and not
  committed, launches Electron from `desktop/node_modules` (after
  `node node_modules/electron/install.js`) with `_electron.launch`, reads
  `process.platform` as `linux`, and saves a page screenshot that shows text.
  The trial image did this in 3 seconds.
- `scripts/linux/run sh -c 'systemctl --user is-system-running'` prints
  `running`.

## Implementation plan

1. Write `scripts/linux/Dockerfile`.
2. Write `scripts/linux/run` and make it executable.
3. Add `linux-check` to the `Makefile` and its `.PHONY` list.
4. Update `agents/development.md`.
5. Run the test plan and remove the trial containers. Leave the
   `hypercube-linux` image and cache volume; they are the cache.

## Documentation updates

- `agents/development.md`:
  - Code: `scripts/linux/` — the Linux test container
    (`scripts/linux/run`).
  - Validation: Linux behavior is tested with `scripts/linux/run`.
  - A "Test on Linux" section: what the image contains; `scripts/linux/run
    [command]` (default `make check`, also `make linux-check`) copies the
    checkout, without ignored files, into a new container and runs the command
    as `tester` under Xvfb; the cache volume and how to reset it; tests that start sessions
    use the scripted plugin as `make screenshots` does; later Linux tests live
    in `scripts/linux/`.
