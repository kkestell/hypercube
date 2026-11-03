# Installation

Hypercube needs macOS, Git, and the harness executables selected in
[Configuration](configuration.md): `claude`, `opencode`, `codex`, or `pi`.
Configure credentials through each harness before starting. See
[Harness support](harness-support.md) for compatibility and limitations.

Use the Go version required by [go.mod](../go.mod). Build and install with:

```sh
make install
```

`hypercube` and its harness plugins go to `~/.local/bin`; add that directory
to PATH. Set `BINDIR=<dir>` to install elsewhere. For a local build without
installing, run `make` and use `bin/hypercube`, which uses the plugins beside
it. Build and validation targets are defined in the [Makefile](../Makefile).
See [Usage](usage.md) to start working.

To start Hypercube at login, run:

```sh
make install-service
```

Hypercube starts at login with a menu bar icon. **Show Hypercube** opens the
Hypercube window, which shows the browser view; closing the window leaves
Hypercube running in the menu bar. It runs as a launchd user agent through your interactive login
shell, so it and the agents it launches get your login environment. It starts
the sessions that were running when it last exited, attaching to surviving
agents. When every agent of a session was lost, or after Quit, the session
starts no turn and delivers no queued message until you type a prompt. Hypercube keeps its
data in `~/.local/state/hypercube/hypercube.db`. **Quit
Hypercube** ends the agents, the panel shells, the icon, and the window until the
next login; the sessions stay marked running and start again then. Its log is
`~/.local/state/hypercube/ui.log`. To remove the service, choose **Quit
Hypercube** first, then unload it and delete its property list:

```sh
launchctl bootout gui/$(id -u)/com.github.kkestell.hypercube
rm ~/Library/LaunchAgents/com.github.kkestell.hypercube.plist
```

## Upgrades

Run `make install-service` to install the build and restart the menu bar icon
and window. Agents and panel shells keep running across the restart.
A new plugin build applies at each agent's next launch. A change to the holder
directory or plugin protocol requires quitting Hypercube before installing.

Upgrading from the earlier leads/workers version restarts agents and panel
shells once. Saved sessions, conversations, worktrees, transcripts, and queued
messages migrate automatically.

## Development

`make dev` builds this checkout and runs it with its own menu bar icon and
Hypercube window, serving its browser view at http://localhost:4281/, with
isolated state, configuration, and a scratch repository under
`~/projects/hypercube-dev/repo`. It adds the workspace and
opens a stopped preview session, with sample source files ready in Files,
without starting a model. Set `HYPERCUBE_DEV_DIR` to use another location or
`HYPERCUBE_DEV_PORT` to use another port.
It stays in the foreground, rebuilds after source changes, and reloads the
browser. A failed build leaves the running app alone. Successful rebuilds keep
dev agents and panel shells running. Ctrl-C and `make dev-stop` end them;
sessions stay marked running for the next launch. `make dev-clean` also ends
holders left by a crashed dev app before removing its disposable
resources, the dev agents' OpenCode conversations, and their Claude Code
transcripts. Details belong to [scripts/dev](../scripts/dev).

Automated tests use local harness stand-ins and the Bun test of the
transcript renderer, which `make check` runs; Bun must be installed. Live model
behavior requires a separate smoke test.
