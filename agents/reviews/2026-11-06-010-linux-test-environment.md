# Linux test environment review

## Scope and coverage

The staged changes for
`agents/plans/2026-11-06-014-linux-test-environment.md`: `scripts/linux/Dockerfile`,
`scripts/linux/run`, the `linux-check` target in `Makefile`, and the "Test on
Linux" section of `agents/development.md`, with the work log
`agents/work/2026-11-06-013-linux-test-environment.md`.

`scripts/linux/run` was run on this Mac (Docker 29.1.3, ARM64). It was not run
on a Linux host; the host side of the file copy was checked by running the same
`tar` command with GNU tar in the image. The cold first run (no image, no cache
volume) was not repeated.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Portability | Fixed | The file copy fails on a Linux host |
| F2 | Medium | Reliability | Fixed | The boot wait hides errors behind a 10 second guess |
| F3 | Low | Reliability | Fixed | The container shares the host's cgroup tree |
| F4 | Low | Correctness | Fixed | The run can start a different image than it built |
| F5 | Low | Correctness | Fixed | A listed directory is copied with its ignored contents |

## F1. The file copy fails on a Linux host

Medium · Portability · Fixed · `scripts/linux/run:45`

The plan runs this command in CI on `ubuntu-24.04`. The host `tar` there is
GNU tar, which has no `--no-mac-metadata` option, so `run` stopped at the copy
on any Linux host.

### Evidence

`tar --no-xattrs --no-mac-metadata ...` with GNU tar 1.35 in the image printed
`unrecognized option '--no-mac-metadata'` and exited 64. Without
`--no-xattrs`, macOS `tar` writes extended attributes such as
`com.apple.provenance` into the archive and GNU tar prints `Ignoring unknown
extended header keyword` for each file.

### Resolution

`run` uses `COPYFILE_DISABLE=1 tar --no-xattrs --no-recursion --null -T -`,
which both macOS `tar` and GNU tar accept. `COPYFILE_DISABLE` keeps out
AppleDouble `._` files and `--no-xattrs` keeps out extended attributes, so
extraction in the container is silent.

## F2. The boot wait hides errors behind a 10 second guess

Medium · Reliability · Fixed · `scripts/linux/run:31`

The work log's departure retried `systemctl is-system-running --wait` 100 times
at 0.1 second intervals with `2>/dev/null || true`. The development rules ask
for waits on conditions, not timing guesses. The loop was a condition wait
with a guessed 10 second limit, and it discarded every error: a container that
had exited, a Docker failure, and a slow start all ended as
`booted to state '', not running` after 10 seconds.

### Evidence

Read the loop; `docker exec` errors and `systemctl` errors went to
`/dev/null`, and an empty state was treated as "not ready yet".

### Resolution

One `docker exec` waits in the container for the exact condition `systemctl`
needs, systemd's private socket `/run/systemd/private`, and then runs
`systemctl is-system-running --wait`. There is no time limit: PID 1 creates the
socket at startup, and if the container exits the `docker exec` fails. Errors
are shown, and a boot that ends in any state but `running` prints the state
and `systemctl --failed`.

## F3. The container shares the host's cgroup tree

Low · Reliability · Fixed · `scripts/linux/run:23`

`--cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:rw` gave the container's
systemd write access to the whole cgroup tree of the Docker host, which in CI
is the runner's own systemd tree. A private cgroup namespace, Docker's default
on cgroup v2, is enough for systemd in a privileged container.

### Evidence

With only `--privileged`, systemd reached `running`, the test user's manager
reported `running`, `/proc/self/cgroup` showed `0::/`, and `make check` passed.

### Resolution

`run` starts the container with `--privileged` and the cache volume only.

## F4. The run can start a different image than it built

Low · Correctness · Fixed · `scripts/linux/run:19`

`run` built the image, then started it by tag. Another worktree building with a
different `go.mod` or `package.json` version in between would retag
`hypercube-linux`, and this run would use the other worktree's toolchain.

### Evidence

Traced the build and run commands; the tag is shared by all worktrees.

### Resolution

`run` keeps the image ID that `docker build -q` prints and starts that image.

## F5. A listed directory is copied with its ignored contents

Low · Correctness · Fixed · `scripts/linux/run:45`

`git ls-files` can list a directory, such as a submodule or a tracked file
replaced by a directory, and `tar` then copied everything under it, ignored
files included.

### Evidence

Traced `tar -T -` behavior: listed directories are archived recursively by
default.

### Resolution

`--no-recursion` makes `tar` copy exactly the listed paths. Extraction creates
their parent folders.

## Checks run

- `scripts/linux/run` before the fixes — `make check` passed in 11 s.
- `scripts/linux/run` after the fixes — `make check` passed in 12 s, with no
  `tar` warnings.
- `scripts/linux/run sh -c '... exit 5'` — exit status 5; `scripts/linux/run
  sh -c 'exit 7'` before the fixes gave 7.
- File copy with a file named with a space, one named with a newline, a
  symlink, and a deleted tracked file (`docs/configuration.md`) — the first
  three arrived intact (the symlink as a symlink), the deleted file was absent.
- In the same run, `systemctl --user is-system-running` printed `running`.
- Ctrl-C through a pseudo-terminal and SIGINT to the process group without a
  terminal, during `scripts/linux/run sleep 60` — both exited 130 within 0.3 s
  and left no container.
- GNU tar 1.35 in the image with the new flags archived a file name with a
  space and a symlink.
- `git status --short --ignored` before and after the runs — unchanged apart
  from the edit to `scripts/linux/run`.

## Verdict

The environment works on this Mac: `make check` passes, exit statuses
propagate, cleanup works on success, failure, and Ctrl-C, nothing is written
into the checkout, and the image follows the `go.mod` and `package.json`
versions. F1 would have broken the planned CI job, and F2 replaced a guessed
time limit and hidden errors with a wait on the exact condition. Both are
fixed, as are F3 to F5. Running `scripts/linux/run` on a Linux host remains
unverified until the CI job exists.
