# Testing

This reference covers running the checks, keeping tests isolated and fast,
writing tests, and the Linux test environment. Project conventions are in
[development and releases](development.md).

## Run the checks

- `make check` runs every check: it builds the browser view and the programs,
  type-checks the browser view, tests the browser view and the desktop app with
  Bun, runs
  `go vet` and `gofmt`, and runs `go test -race ./...`. Run it before
  committing code.
- While working, run focused checks: `go test -run` on the packages you
  changed, or `bunx tsc -p .` and `bun test` in `internal/ui`.
- Run `make ui` before raw `go build`, `go vet`, or `go test`; Go embeds the
  generated browser view.
- After changing only docs or comments, inspect the diff; run nothing.
- Check a change to the browser view with tests and the build, without
  screenshots. The user judges how it looks.

## Speed

A `make check` with an empty test cache takes a little over a minute on an
Apple Silicon Mac. Under the race detector, `internal/session` takes 40 to 60
seconds and every other Go package 30 seconds or less. A package that runs well
past its usual time has a fault in the tests or their environment. Find the
cause before continuing:

1. List each test's duration:

   ```sh
   go test -count=1 -v ./internal/workspace 2>&1 | grep -E '^\s*--- '
   ```

   Parallel tests count the time they wait for each other, so a long duration
   does not by itself mark the slow test.
2. Run a slow test alone with `-run '^TestName$'`. A test that is fast alone
   and slow in its package waits on something the tests share.
3. While the package runs, list its child processes with `ps` and sample a
   long-lived one with `sample <pid> 1` on macOS. The stack names the wait,
   such as a network lookup or a lock.

## Isolation

Tests never read or change the user's configuration, state, or workspaces.

- Every Go test package that runs Git, directly or through the code it tests,
  calls `gittest.Isolate()` (`internal/workspace/gittest`) first in its
  `TestMain`. It keeps the user's and the system's Git configuration out,
  sets a fixed author and committer, and allows local file transports for
  submodule fixtures.
  Without a configured identity, Git derives one from the machine's host name
  through a network lookup that can take seconds.
- Tests take their state, configuration, and log folders from `t.TempDir`,
  and never stop the user's workspaces.
- Tests that start agents use the stand-in plugin in
  `internal/harness/harnesstest`, and processes held for agents and shells use
  `internal/hold/holdtest`. Tests run in their own shells.
- Tests use no network: GitHub tests use `internal/github/githubtest`, and
  remote repositories are local bare repositories.

## Write tests

- Go tests cover observable behavior through public entry points. Use tables
  for repeated cases, `t.Helper`, `t.TempDir`, and `t.Cleanup`; mark
  independent tests `t.Parallel()`; wait on conditions, never fixed sleeps;
  and pass under `-race`.
- Choose test responsibilities by consequence and maintenance cost. Existing
  assertions are not requirements: remove low-value tests and their machinery
  without automatically replacing their coverage. Prefer one suitable boundary
  over repeating behavior at every layer. Stand-ins must earn their cost through
  the behavior they protect. Assert observable results; specify styling, markup,
  or private state only when that detail is the behavior being protected.
- Browser tests check what users see and do.

## Test on Linux

`scripts/linux/run [command]` runs the command in a new container that boots
systemd, then removes the container. `LINUX_DISTRO` chooses the distribution:
`debian` (Debian 13, the default), `ubuntu` (26.04), `fedora` (45), or `arch`.
Arch publishes AMD64 images only, so it runs in CI, not on Apple Silicon. The
default command is `make check`; `make linux-check` runs it. `make linux-test`
runs the Linux tests: `scripts/linux/test-bundle` builds the bundle and checks
its layout, its executables, and the backend started from it, and
`scripts/linux/test-install` installs the bundle into the test user's home,
runs the service under bash, zsh, and fish, keeps a scripted agent running
across a service restart, and checks `make uninstall`.
`scripts/linux/test-desktop` drives the installed desktop app against the
installed service with Playwright: the window frame and menus, each waiting
page, setting up the service, a notification and the launcher badge, the open
actions, the shortcuts and Ctrl labels, the panel terminal's keys (pressed with
`xdotool`), closing the window, and reinstalling and uninstalling while the app
runs. It starts openbox, the dunst notification server, a stand-in file
manager, and stand-ins for `code` and `xdg-open`. `scripts/linux/test-release`
runs last: it packages a tagged copy of the checkout with `scripts/release`,
installs from the extracted tarball, and checks the version, the service, and
removal. Screenshots and logs land in `bin/linux-test/<distro>/`, copied out of
the container when the command ends.
`make linux-test-all` runs the Linux tests for every distribution that has an
image for the host's architecture, and CI runs them for all four on AMD64. The
container runs natively on Apple Silicon and needs Docker.

The command runs as the user `tester`, which has a systemd user manager, under
Xvfb. The image has Go and Bun at the versions the repository declares,
a C compiler, `rsvg-convert`, SQLite, the Electron runtime libraries,
Node, and Playwright. It has no harness CLIs, so tests that start sessions use
the scripted plugin.

The container gets a copy of the tracked and untracked files that Git does not
ignore, without `.git`, so builds report version `devel` and nothing in the
checkout changes. The volume `hypercube-linux-cache` keeps Go and Bun
downloads and builds across runs; `docker volume rm hypercube-linux-cache`
resets it. The first run builds the image and downloads dependencies, and later
runs take seconds. Each distribution has its own image,
`hypercube-linux-<distro>`, built from `scripts/linux/Dockerfile`;
`scripts/linux/packages` installs that distribution's packages. New Linux
tests go in `scripts/linux/`.

`scripts/linux/test-apparmor` checks the installer's AppArmor profile on a real
Ubuntu 24.04 kernel, which restricts user namespaces; the container's kernel
does not. It builds the bundle in the container, then boots the Lima VM
`hypercube-apparmor` natively (`vmType: vz`) and checks that the desktop app's
sandbox fails without the profile, works with it, and that uninstalling removes
it. It needs an Apple Silicon Mac with Docker and Lima, downloads an Ubuntu image
on its first run, and is not part of `make linux-test`. It deletes the VM when it
ends.
