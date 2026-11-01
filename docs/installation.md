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
browser view. It runs as a launchd user agent through your interactive login
shell, so it and the agents it launches get your login environment. It starts
the sessions that were running when it last exited; each starts no turn and
delivers no queued message until you type a prompt into it. Hypercube keeps its
data in `~/.local/state/hypercube/hypercube.db`. **Quit
Hypercube** ends the agents, the panel shells, the icon, and the view until the
next login; the sessions stay marked running and start again then. Its log is
`~/Library/Logs/hypercube-ui.log`. Remove the service by unloading it and
deleting its property list:

```sh
launchctl bootout gui/$(id -u)/com.github.kkestell.hypercube
rm ~/Library/LaunchAgents/com.github.kkestell.hypercube.plist
```

## Upgrades

Run `make install-service` to install the build and restart the menu bar icon
and browser view. The restart ends the agents and panel shells, then starts the
sessions that were running, resuming each agent's conversation. Each session
delivers no queued message until you type a prompt into it.

## Development

`make dev` builds this checkout and opens its browser view at
http://localhost:4281/ with isolated state, configuration, and a scratch
repository under `~/projects/hypercube-dev`, which you add as a workspace in the
view. Set `HYPERCUBE_DEV_DIR` to use another location or `HYPERCUBE_DEV_PORT`
to use another port.
It stays in the foreground, rebuilds after source changes, and reloads the
browser. A failed build leaves the running app alone. A successful build and
Ctrl-C both stop the app through its shutdown; the dev sessions stay marked
running, and the restarted app resumes them.
`make dev-stop` can also stop it; `make dev-clean` removes its disposable
resources, the dev agents' OpenCode conversations, and their Claude Code
transcripts. Details belong to [scripts/dev](../scripts/dev).

Automated tests use local harness stand-ins and the Bun test of the
transcript renderer, which `make check` runs; Bun must be installed. Live model
behavior requires a separate smoke test.
