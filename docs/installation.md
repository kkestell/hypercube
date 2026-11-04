# Installation

Hypercube builds from source on macOS. You need Git, the Go version in
[go.mod](../go.mod), and at least one installed, authenticated coding harness:
`claude`, `opencode`, `codex`, or `pi`. Check
[harness requirements](harness-support.md) before configuring profiles.

## Install and launch

From this checkout:

```sh
make install
```

This installs `~/Applications/Hypercube.app` and links `hypercube` and its
plugins into `~/.local/bin`. Put that directory on your PATH. Use
`APPDIR=<dir>` and `BINDIR=<dir>` to change the destinations.

Create [your configuration](configuration.md), then run:

```sh
hypercube ui --menu
```

This starts the menu bar icon and native window. Choose **Show Hypercube** from
the icon if the window is closed. The first launch asks for notification
permission.

The same interface is available at http://localhost:4280/. Omit `--menu` to run
without the native window, or pass a port to use another address:

```sh
hypercube ui 4282
```

The native window is required for agents' Browser tabs and macOS notifications.

## Run at login

```sh
make install-service
```

This installs the build and loads a launchd user agent. It runs through your
interactive login shell, giving agents your login environment.

Closing the window leaves Hypercube running. **Quit Hypercube** ends the app,
agents, and terminal shells until the next login. Saved conversations remain;
[resume behavior](usage.md#resume-and-cleanup) explains when work starts again.

To remove the service, choose **Quit Hypercube**, then run:

```sh
launchctl bootout gui/$(id -u)/com.github.kkestell.hypercube
rm ~/Library/LaunchAgents/com.github.kkestell.hypercube.plist
```

## Upgrade

Run `make install-service` again. It replaces the installed build and restarts
the window and menu bar icon, while agents and terminal shells keep running.

Those agents still use the plugins they launched with. Restart their sessions
to use a new plugin build. A holder-directory or plugin-protocol change requires
quitting Hypercube before installation.

State migrates on startup. Upgrading from the earlier leads/workers version
also restarts agents and panel shells once.

## Files on disk

| File | Default location |
| --- | --- |
| Configuration | `~/.config/hypercube/config.json` |
| Database | `~/.local/state/hypercube/hypercube.db` |
| Service log | `~/.local/state/hypercube/ui.log` |
| Login service | `~/Library/LaunchAgents/com.github.kkestell.hypercube.plist` |

Configuration follows `XDG_CONFIG_HOME`; saved state follows `XDG_STATE_HOME`.

## Develop without replacing the installed app

```sh
make dev
```

Open http://localhost:4281/. This uses separate configuration and state, with a
scratch repository under `~/projects/hypercube-dev/repo`. Its stopped preview
session has sample files and starts no model. Use `make dev-app` when testing
native-window behavior.

Source changes rebuild the development app and reload the interface. A failed
build leaves the previous build running. Successful rebuilds preserve agents
and terminal shells.

- **Ctrl-C** or `make dev-stop` ends the development app and its processes.
- `make dev-clean` also removes disposable resources and native development
  conversations, including holders left by a crash.
- `HYPERCUBE_DEV_DIR` and `HYPERCUBE_DEV_PORT` change the directory and port.

Sessions remain marked running for the next development launch. See
[scripts/dev](../scripts/dev) for cleanup details.

For a build alone, run `make` and use `bin/hypercube` or
`bin/Hypercube.app`. Code changes require `make check`, which also needs Bun.
Tests use local harness stand-ins; live model behavior needs a smoke test.
