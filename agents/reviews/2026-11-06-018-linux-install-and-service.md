# Linux install and service review

## Scope and coverage

The staged changes for
`agents/plans/2026-11-06-016-linux-install-and-service.md`:
`scripts/linux-install`, `scripts/linux-install-service`,
`scripts/linux-uninstall`, `make/linux.mk`, the `Makefile` `linux-test` target,
`scripts/linux/Dockerfile`, `scripts/linux/test-bundle`,
`scripts/linux/test-install`, `scripts/linux/test-apparmor`, and
`agents/development.md`, judged against the plan and the work log
`agents/work/2026-11-06-017-linux-install-and-service.md`. Related code read:
`scripts/install-service`, `scripts/linux/run`, `serveUI` and `app.close` in
`cmd/hypercube/main.go`, and `Runtime.Quit` in `internal/session`.

Checked: quoting of paths with spaces and other special characters, escaping
in the unit, the launcher entry, and the AppArmor profile, what the `sudo`
commands touch, what install and removal delete, the bundle replacement while
the backend runs, agent survival across restart and their end on removal, the
tests' waits, and macOS behavior.

Coverage gaps: `test-install` runs on the default XDG paths; unusual paths
were checked once in the container (see Checks run), not in a lasting test.
tcsh is not tested; its handling of the start command was checked by hand.

The work log's four departures are sound: the zygote selection matches
Electron's two zygotes, `--mount-none` keeps the Mac's home out of the VM, the
backend requires `Origin` on `POST`, and linking every
`hypercube-harness-*` in the bundle links exactly the four plugins.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | High | Correctness | Fixed | The service never starts under fish |
| F2 | Medium | Safety | Fixed | Install and removal delete whatever is at the bundle path |
| F3 | Medium | Correctness | Fixed | Removal stops waiting for the backend after 10 seconds |
| F4 | Low | Correctness | Fixed | The launcher entry's `Exec` breaks on `&`, `\|`, `\`, and `%` |
| F5 | Low | Correctness | Fixed | The bundle path is briefly missing during a reinstall |
| F6 | Low | Correctness | Fixed | Removal needs a systemd user manager without a service |
| F7 | Low | Tests | Fixed | The leftover-process check after removal is not a wait |
| F8 | Low | Correctness | Open | The AppArmor attachment path is not escaped |
| F9 | Low | Correctness | Open | The unit directory comes from the caller's `XDG_CONFIG_HOME` |
| F10 | Low | Correctness | Open | `SHELL` is written into `ExecStart` unescaped |

## F1. The service never starts under fish

High · Correctness · Fixed · `scripts/linux-install-service:42`

`ExecStart` ran `exec "${HYPERCUBE_APP_EXECUTABLE}" ui "${HYPERCUBE_PORT}"`.
fish rejects `${…}`, so for a user whose login shell is fish the shell exits
with status 127 and `Restart=on-failure` restarts it until systemd's start
limit. The comment and the plan both name fish as supported.

### Evidence

In the test image with fish and tcsh installed: `fish -c 'exec "${X}" ok'`
prints "Variables cannot be bracketed" and exits 127; `exec "$X" ok` works in
fish and tcsh.

### Resolution

The command is now `exec "$$HYPERCUBE_APP_EXECUTABLE" ui "$$HYPERCUBE_PORT"`,
the form the LaunchAgent uses. The test image has fish, and `test-install`
reinstalls the service with `SHELL=/usr/bin/fish` and checks that the backend
has the fish login environment and that the holder survives.
`agents/development.md` mentions fish.

## F2. Install and removal delete whatever is at the bundle path

Medium · Safety · Fixed · `scripts/linux-install:21`, `scripts/linux-uninstall:36`

Both scripts ran `rm -rf "$XDG_DATA_HOME/hypercube"` without checking what it
was, and install also ran `rm -rf "$app.new"`. With an unusual
`XDG_DATA_HOME`, such as a directory that holds a `hypercube` source checkout,
install or removal deletes the user's files.

### Evidence

Traced in the scripts. Nothing in Hypercube writes under `XDG_DATA_HOME`, so
the path holds only the bundle in normal use.

### Resolution

Install refuses with an error when the bundle path exists and lacks
`hypercube-desktop` and `resources/uninstall`; removal leaves such a directory
and says so. Install stages its copy in a new `mktemp -d` directory beside the
bundle and removes only that directory, so it never deletes a preexisting
`hypercube.new`. Checked in the container (see Checks run).

## F3. Removal stops waiting for the backend after 10 seconds

Medium · Correctness · Fixed · `scripts/linux-uninstall:19`

After sending SIGINT, removal polled `is-active` 100 times and then carried on
silently, disabling the unit, deleting its file, and deleting the bundle while
the backend could still be ending agents. `Runtime.Quit` waits for every
holder with no bound, so a slow agent made removal report success with agents
still running.

### Evidence

Traced through `serveUI`, `app.close`, `Runtime.Quit`, and `stopLaunches`.

### Resolution

Removal sends SIGINT, then runs `systemctl --user stop`, which waits for the
backend to exit under systemd's stop timeout. The SIGTERM that `stop` sends
reaches a backend already shutting down on SIGINT and does not change the
shutdown: the signal channel holds one signal and `close` runs once.

## F4. The launcher entry's `Exec` breaks on `&`, `|`, `\`, and `%`

Low · Correctness · Fixed · `scripts/linux-install:35`

The path went into a `sed` replacement unescaped. `&` inserts the matched text
(`/a & b` became `/a Exec=x b`), `|` ends the expression, and the desktop entry
rules for `"`, `` ` ``, `$`, `\`, and `%` were not applied.

### Evidence

Reproduced the `&` case with `sed`.

### Resolution

The installer escapes the path by the desktop entry rules and writes the line
with `awk` from the environment. `desktop-file-validate` accepts the entry for
a data directory named `odd dir & 100% $x "q" `` `b` ``.

## F5. The bundle path is briefly missing during a reinstall

Low · Correctness · Fixed · `scripts/linux-install:24`

Install deleted the old bundle, then renamed the new one into place. A backend
that launched an agent during the deletion found no plugin. The old bundle is
now renamed into the staging directory and the new one renamed into place, two
renames in a row, before the old copy is deleted.

## F6. Removal needs a systemd user manager without a service

Low · Correctness · Fixed · `scripts/linux-uninstall:28`

`systemctl --user daemon-reload` ran unconditionally under `set -e`, so after
`make install` alone, on a system without a user manager, removal failed
before deleting anything. The service steps now run only when the unit file
exists.

## F7. The leftover-process check after removal is not a wait

Low · Tests · Fixed · `scripts/linux/test-install:134`

The test checked once, right after the holder exited, that no process ran
from the removed bundle; a plugin process still exiting would fail it. It is
now a `wait_for` on the same condition.

## F8. The AppArmor attachment path is not escaped

Low · Correctness · Open · `scripts/linux-install:48`

The profile's quoted attachment path is an AppArmor glob. A data directory
containing `"`, `*`, `?`, `[`, `{`, `^`, or `\` makes the profile fail to load
(the install stops at the `sudo` command) or match more than the desktop app.
Such home or data paths are implausible, and the user runs the command
themselves. Left open.

## F9. The unit directory comes from the caller's `XDG_CONFIG_HOME`

Low · Correctness · Open · `scripts/linux-install-service:15`

The systemd user manager searches its own `XDG_CONFIG_HOME`, which differs from
the caller's when a shell configuration file sets it. `systemctl --user
enable` then fails with "Unit file … does not exist". The failure is clear and
nothing is damaged. Left open.

## F10. `SHELL` is written into `ExecStart` unescaped

Low · Correctness · Open · `scripts/linux-install-service:42`

A shell path with a space or `%` would produce a wrong `ExecStart`;
`systemd-analyze verify` rejects it before the running service is touched.
Login shell paths do not contain these characters, and the plan chose this.
Left open.

## Checks run

- `make linux-test` — passed: `test-bundle` and `test-install`, including the
  new fish step.
- A one-off container script with `XDG_DATA_HOME`, `XDG_STATE_HOME`, and
  `BINDIR` under `/home/tester/odd dir & 100% $x "q" `` `b` `` — passed:
  install refused a non-bundle directory and removal kept it; the launcher
  entry validated; the service started and answered with the unit's executable
  and log under that path; reinstalling over the running service left no
  staging directory; removal removed the bundle and links; removal with nothing
  installed succeeded.
- `scripts/linux/test-apparmor` — passed, because the install's temporary-file
  cleanup changed. `limactl list` shows no instance afterward.
- `make check` — passed.
- macOS: `make/macos.mk` and `scripts/install-service` are unchanged, and the
  `Makefile` change is the `linux-test` command only.
- `shellcheck` is not installed; not run.

## Verdict

Ready after fixes. F1 broke the service for fish users and F2 could delete
user files under an unusual `XDG_DATA_HOME`; both are fixed with F3–F7. F8–F10
are low severity and left open.
