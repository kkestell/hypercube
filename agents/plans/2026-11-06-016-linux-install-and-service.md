# Linux install and service

## Goal

On Linux, `make install` installs the bundle under the user's home, links the
CLI and plugins into `~/.local/bin`, and adds the launcher entry and icon.
`make install-service` also installs and restarts a systemd user service that
runs `hypercube ui 4280` through the user's interactive login shell, and
agents and panel shells keep running across the restart. `make uninstall`
undoes both. On Ubuntu 23.10 and later, the install adds an AppArmor profile
that lets the desktop app's sandbox create user namespaces, with one `sudo`
command, only where the kernel restricts them. No other step needs root.

The bundle ships the three installer scripts in `resources/`, so a user of the
release tarball runs the same scripts, and the desktop app task's setup button
runs `resources/install-service` as it does on macOS.

New tests in the Linux test container install into `tester`'s home, start the
service, keep a scripted agent running across `systemctl --user restart`,
check the service's environment from bash and zsh, and check that removal
leaves nothing behind. A separate script checks the AppArmor step on a real
Ubuntu 24.04 kernel in a Lima VM.

macOS packaging, installation, and service behavior do not change.

## Related code

- `Makefile` — shared variables (`BINDIR`, `APP_ID`, `PORT`, `PLUGINS`) and
  `linux-test`, which runs `scripts/linux/test-bundle` in the container.
- `make/linux.mk` — Linux `bundle` and `build`. This change adds `install`,
  `install-service`, and `uninstall`.
- `make/macos.mk`, `scripts/install-service` — the macOS install and the
  LaunchAgent installer. Unchanged; the Linux installer follows its interface
  (`APP`, `SERVICE`, `PORT`, `SHELL`) and its start command.
- `desktop/setup.js` — runs `<bundle>/Contents/Resources/install-service` with
  that interface. The desktop app task points it at the Linux path.
- `desktop/com.github.kkestell.hypercube.desktop` — the launcher entry, with
  `Exec=hypercube-desktop` for the installer to replace.
- `cmd/hypercube/main.go` (`serveUI`) — SIGTERM closes the backend and leaves
  agents running; SIGINT ends them. Both exit with status 0.
- `internal/session/restore.go` — at startup the backend reattaches holders in
  `$XDG_STATE_HOME/hypercube/run/<launch>/` (each with a `pid` file) and adopts
  panel shells.
- `plugins/scripted/main.go` — plays the scene in `HYPERCUBE_SCENE`; a turn's
  `git` steps can write a file into the session's directory, which the test
  uses to see that a turn ran.
- `internal/ui/actions.go` — `POST /workspace?action=new&path=…` and
  `POST /session?action=new…` and `?action=input`, which the test drives with
  `curl`.
- `scripts/linux/run`, `scripts/linux/Dockerfile`,
  `scripts/linux/test-bundle` — the test container and the bundle test.
- `.github/workflows/check.yml` — the `linux-test` job runs `make linux-test`.

## Decisions

### Paths and names

Everything is named by the app ID, `com.github.kkestell.hypercube`, as on
macOS, so the unit, launcher entry, icon, and AppArmor profile share one name.

| What | Path |
| --- | --- |
| Bundle | `$XDG_DATA_HOME/hypercube/` (default `~/.local/share/hypercube/`) |
| Links | `$BINDIR/hypercube`, `$BINDIR/hypercube-harness-*` (default `~/.local/bin`) |
| Launcher entry | `$XDG_DATA_HOME/applications/com.github.kkestell.hypercube.desktop` |
| Icon | `$XDG_DATA_HOME/icons/hicolor/512x512/apps/com.github.kkestell.hypercube.png` |
| Unit | `$XDG_CONFIG_HOME/systemd/user/com.github.kkestell.hypercube.service` (default `~/.config/…`) |
| Log | `$XDG_STATE_HOME/hypercube/ui.log`, as on macOS |
| AppArmor profile | `/etc/apparmor.d/com.github.kkestell.hypercube` |

The bundle directory is the installed app itself, so removal deletes one
directory. User commands are `systemctl --user restart
com.github.kkestell.hypercube`, the same label macOS's `launchctl` commands use.

### Three scripts, one per job

Linux gets its own scripts beside the macOS one, each a short POSIX `sh`
script with a header comment, and no platform branches anywhere:

- `scripts/linux-install` — installs the bundle that contains it.
- `scripts/linux-install-service` — writes the unit and starts or restarts it.
  Same interface as `scripts/install-service`: `APP`, `SERVICE`, `PORT`, and
  `SHELL` are required.
- `scripts/linux-uninstall` — removes what the other two add.

`make/linux.mk`'s `bundle` copies them to `resources/install`,
`resources/install-service`, and `resources/uninstall`. A tarball user runs
`Hypercube/resources/install`; `make install` runs `bin/Hypercube/resources/install`,
so both go through the same file. `scripts/install-service` stays where it is.

### `linux-install`

1. `src` is the bundle containing the script (`$(dirname "$0")/..`); `app` is
   `${XDG_DATA_HOME:-$HOME/.local/share}/hypercube`; `bin` is
   `${BINDIR:-$HOME/.local/bin}`.
2. Unless `src` is `app`, copy `src` to `$app.new`, remove `$app`, and rename
   `$app.new` to `$app`, so the installed directory is never half copied. A
   running backend, agents, and plugins keep running from the deleted files;
   Go's `os.Executable` on Linux drops the ` (deleted)` suffix, so a backend
   that launches an agent before its restart still finds the new plugins.
3. Link `hypercube` and the four plugins into `$bin` with `ln -sf`.
4. Write the launcher entry from `$app/resources/com.github.kkestell.hypercube.desktop`
   with `sed`, replacing the `Exec=` line with `Exec="$app/hypercube-desktop"`.
   Copy the icon with `install -D -m 644`. Freedesktop launchers watch these
   directories; no cache update is needed.
5. The AppArmor step, below.

The desktop app task adds quitting a running desktop app before the copy and
opening it after `make install-service`, as `make/macos.mk` does; on Linux the
desktop app does not run yet.

### AppArmor

The step runs only when
`/proc/sys/kernel/apparmor_restrict_unprivileged_userns` reads `1`. It writes
the profile to a temporary file and does nothing more when
`/etc/apparmor.d/com.github.kkestell.hypercube` already has the same content,
so reinstalling does not ask for a password. Otherwise it prints one line
saying the desktop app's sandbox needs an AppArmor profile, then runs one
command:

```sh
sudo sh -c 'install -m 644 "$1" "$2" && apparmor_parser -r "$2"' sh "$tmp" "$profile"
```

The profile follows the form Ubuntu 24.04 ships for applications that need
user namespaces (for example `/etc/apparmor.d/code`): unconfined, with a
`userns` rule, attached by the executable's path. The path is quoted so a home
directory with spaces works.

```
# Lets Hypercube's desktop app create the user namespaces its sandbox needs.
abi <abi/4.0>,
include <tunables/global>

profile com.github.kkestell.hypercube "/home/me/.local/share/hypercube/hypercube-desktop" flags=(unconfined) {
  userns,
}
```

`apparmor_parser -r` loads it at once; AppArmor loads it from
`/etc/apparmor.d` at every boot. The profile names the install path, so it
survives upgrades. Debian, Fedora, and Arch do not restrict user namespaces
and skip the step.

### `linux-install-service`

The unit:

```ini
[Unit]
Description=Hypercube

[Service]
Environment="HYPERCUBE_APP_EXECUTABLE=<APP>/hypercube" "HYPERCUBE_PORT=<PORT>"
ExecStart=@<SHELL> -<basename SHELL> -i -c 'exec "$${HYPERCUBE_APP_EXECUTABLE}" ui "$${HYPERCUBE_PORT}"'
KillMode=process
Restart=on-failure
StandardOutput=append:<LOG>
StandardError=append:<LOG>

[Install]
WantedBy=default.target
```

- **Login shell.** `@` sets the shell's `argv[0]` to `-bash`, `-zsh`, and so
  on, which makes it a login shell, as the LaunchAgent does; `-i` makes it
  interactive. This works in bash, zsh, fish, and tcsh, where `-l` does not
  work in tcsh. `exec` replaces the shell, so the backend is the main process.
  `$$` keeps systemd from expanding the variables, which the shell expands.
  Interactive bash without a terminal writes two job-control warnings to the
  log at each start.
- **`KillMode=process`.** Stop and restart send SIGTERM to the backend alone;
  holders stay in the unit's control group and the next backend adopts them.
  systemd logs a "left-over process" notice at the next start, which is
  expected.
- **`Restart=on-failure`** matches the LaunchAgent's `KeepAlive` with
  `SuccessfulExit=false`: a crash restarts the backend, and SIGTERM or SIGINT,
  which exit with status 0, do not.
- **Log.** The installer resolves `$XDG_STATE_HOME/hypercube/ui.log` to an
  absolute path, creates its directory, and appends the backend's output
  there, as on macOS. The waiting page and documentation keep showing one log
  path on both platforms; `journalctl` gets nothing.
- **Enabled at login.** `WantedBy=default.target` with `systemctl --user
  enable` starts the service when the user's systemd manager starts, at the
  first login. The installer does not enable lingering, so the service ends at
  logout as the LaunchAgent does.
- **Escaping.** Values written into the unit replace `%` with `%%` (systemd
  specifiers) and escape `\` and `"` inside the quoted `Environment=` values.
  `SHELL` comes from the user's account and is written as is.

Steps, mirroring `scripts/install-service`:

1. Require `APP`, `SERVICE`, `PORT`, and `SHELL`.
2. Write the unit to a temporary directory as `$SERVICE.service` and run
   `systemd-analyze --user verify` on it, refusing an invalid unit before
   touching the running one. Then move it into place.
3. `systemctl --user daemon-reload` and `enable "$SERVICE"`.
4. `systemctl --user stop "$SERVICE"`. `stop` returns after the backend exits,
   so the port is free unless another process holds it; check once with
   `ss -Hltnp "sport = :$PORT"` and, if it is held, print the holder and exit
   with an error, as the macOS installer does.
5. `systemctl --user start "$SERVICE"` and print `started $SERVICE; log: $LOG`.

### `linux-uninstall`

Removal ends the agents, since nothing remains to adopt them:

1. If the unit is active, send the backend SIGINT with `systemctl --user kill
   --kill-whom=main --signal=SIGINT`, which ends agents and shells, and wait
   until the unit is inactive.
2. `systemctl --user disable` it, delete the unit file, and
   `daemon-reload`.
3. Delete the bundle directory, the links in `$BINDIR` that point into it, the
   launcher entry, and the icon.
4. If `/etc/apparmor.d/com.github.kkestell.hypercube` exists, run one command:
   `sudo sh -c 'apparmor_parser -R "$1"; rm -f "$1"' sh "$profile"`.

Configuration and saved state stay, as they are user data.

### Make targets

`make/linux.mk`:

```make
INSTALLED := $(or $(XDG_DATA_HOME),$(HOME)/.local/share)/hypercube
SERVICE := $(APP_ID)

install: build
	BINDIR="$(BINDIR)" $(APP)/resources/install

install-service: install
	APP="$(INSTALLED)" SERVICE="$(SERVICE)" PORT="$(PORT)" "$(INSTALLED)/resources/install-service"

uninstall:
	BINDIR="$(BINDIR)" scripts/linux-uninstall
```

`bundle` adds the three `cp` lines for the scripts.

### Tests in the container

`scripts/linux/Dockerfile` adds `zsh` and `iproute2` (for `ss`).
`scripts/linux/test-install` (Bash, `set -euo pipefail`, header comment), run
as `tester` with the systemd user manager the container provides:

1. **Shell configuration.** Append to `~/.bashrc` and `~/.zshrc` an export of
   `HYPERCUBE_TEST_SHELL` (`bash` or `zsh`) and of `HYPERCUBE_SCENE`, the
   scene file the test writes. Debian's `~/.profile` reads `~/.bashrc` for
   login shells.
2. **Install.** `SHELL=/bin/bash make install-service`. Check the bundle's
   executables and `resources/` in `~/.local/share/hypercube`, that the links
   in `~/.local/bin` resolve into it, that `desktop-file-validate` accepts the
   installed launcher entry and its `Exec` names the installed
   `hypercube-desktop`, and that the icon is in place.
3. **Service.** Wait until `http://127.0.0.1:4280/version` answers. Check that
   the unit's `MainPID` has `HYPERCUBE_TEST_SHELL=bash` in
   `/proc/<pid>/environ`, and that the log file has the backend's startup line.
4. **Holder.** Build `./plugins/scripted` over the installed
   `hypercube-harness-claude`. The scene has one Claude session with two
   turns: prompt `first` writes `first.txt`, prompt `second` writes
   `second.txt`, both ending idle. Add a Git repository as a workspace, start
   a session with `first`, wait for `first.txt`, and read the holder's PID from
   the one launch directory in `~/.local/state/hypercube/run/`.
5. **Restart.** `systemctl --user restart com.github.kkestell.hypercube`. Wait
   for a new `MainPID` that answers `/version`. The holder's PID is the same
   process and still alive. Send `second` to the session and wait for
   `second.txt`: the new backend has adopted the holder and the same plugin
   process answered.
6. **zsh.** `SHELL=/bin/zsh make install-service`, which also replaces the
   installed files with a service running. Check `HYPERCUBE_TEST_SHELL=zsh` in
   the new backend's environment and that the holder is still alive.
7. **Removal.** `make uninstall`. The unit is inactive and its file gone, the
   bundle directory, links, launcher entry, and icon are gone, the holder
   process has exited, and no process runs from the old bundle path.

Each wait polls a condition with a deadline and prints the service log on
failure. `make linux-test` runs both tests in one container:

```make
linux-test:
	scripts/linux/run bash -c 'scripts/linux/test-bundle && scripts/linux/test-install'
```

CI's `linux-test` job runs it unchanged.

### AppArmor on a real Ubuntu kernel

Containers share Docker's kernel, which has no AppArmor restriction.
`scripts/linux/test-apparmor` runs on an Apple Silicon Mac with Docker and
Lima and is not part of `make linux-test`:

1. Build the ARM64 bundle in the test container:
   `scripts/linux/run bash -c 'make bundle >&2 && tar -C bin -cf - Hypercube' </dev/null >"$tmp/bundle.tar"`.
2. `limactl start --name=hypercube-apparmor --vm-type=vz --arch=aarch64 --tty=false template:ubuntu-24.04`,
   deleted with `limactl delete -f` on exit. `vz` runs it natively.
3. `limactl copy` the archive in, then in the VM: fail unless
   `kernel.apparmor_restrict_unprivileged_userns` is `1`; install `xvfb` and
   Electron's runtime libraries; unpack to `/tmp/Hypercube`.
4. **Without the profile**, `xvfb-run -a /tmp/Hypercube/hypercube-desktop`
   exits with Chromium's sandbox error.
5. Run `/tmp/Hypercube/resources/install` (Lima's user has password-free
   `sudo`). The profile is in `/etc/apparmor.d` and listed in
   `/sys/kernel/security/apparmor/profiles`. **With the profile**,
   `xvfb-run -a ~/.local/share/hypercube/hypercube-desktop` starts a zygote
   process (`--type=zygote`) whose `/proc/<pid>/ns/user` differs from the main
   process's: the sandbox has its user namespace. This happens before the
   desktop app's own code runs, so the desktop app task's Linux work is not
   needed. Stop the process.
6. Run `~/.local/share/hypercube/resources/uninstall`. The profile file is
   gone and the profile no longer loaded.

Whether GitHub's `ubuntu-24.04` runner restricts user namespaces is not
documented; checking it needs a workflow run. If it reads `1`, a CI job can run
steps 4–6 on AMD64 later.

## Test plan

- `make linux-test` passes: the bundle test and the install test above.
- `scripts/linux/test-apparmor` passes on the Mac.
- `make check` passes; `shellcheck` reports nothing for the three scripts and
  `test-install` if it is installed.
- macOS: `make -n install install-service` prints the same commands as before
  this change. Do not run `make install`, `make place`, `make quit-desktop`, or
  `make install-service` on the Mac.

## Implementation plan

1. Write `scripts/linux-install`, `scripts/linux-install-service`, and
   `scripts/linux-uninstall`.
2. Copy them into `resources/` in `make/linux.mk`'s `bundle`; add `install`,
   `install-service`, `uninstall`, `INSTALLED`, and `SERVICE`, with a comment
   on each target as `make/macos.mk` has.
3. Add `zsh` and `iproute2` to `scripts/linux/Dockerfile`.
4. Write `scripts/linux/test-install`; have `test-bundle` check that the three
   scripts are executable in `resources/`; update `linux-test` in the
   `Makefile`.
5. Write `scripts/linux/test-apparmor` and run it.
6. Update documentation.

## Documentation updates

- `agents/development.md` — "Test on Linux": `make linux-test` also runs
  `scripts/linux/test-install`; describe `scripts/linux/test-apparmor`, what it
  needs, and that it creates and deletes the `hypercube-apparmor` VM. "Code":
  list the Linux installer scripts.
- `docs/installation.md` gets its Linux sections in the release task.
