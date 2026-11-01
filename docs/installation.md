# Installation

Hypercube needs macOS, Git, tmux, and the harness executables selected in
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
Hypercube** stops the icon and view until the next login, while agents keep
running. Its log is `~/Library/Logs/hypercube-ui.log`. Remove the service by
unloading it and deleting its property list:

```sh
launchctl bootout gui/$(id -u)/com.github.kkestell.hypercube
rm ~/Library/LaunchAgents/com.github.kkestell.hypercube.plist
```

## Upgrades

Run `make install-service` to install the build and restart the menu bar icon
and browser view without stopping agents. Running agents use the new build for
new hook and tool calls at once, and for launch arguments when they next
launch. Their existing MCP server and waiter processes keep running until
their next launch.

## Development

`make dev` builds this checkout and opens its browser view at
http://localhost:4281/ with isolated state, configuration, tmux server, and a
scratch repository under `~/projects/hypercube-dev`, which you add as a
workspace in the view. Set `HYPERCUBE_DEV_DIR` to use another location or
`HYPERCUBE_DEV_PORT` to use another port.
It stays in the foreground, rebuilds after source changes, and reloads the
browser. Failed builds leave the previous view running. Ctrl-C stops the view
and its dev sessions; rebuilding keeps those sessions running.
`make dev-stop` can also stop it; `make dev-clean` removes its disposable
resources, the dev agents' OpenCode conversations, and their Claude Code
transcripts. Details belong to [scripts/dev](../scripts/dev).

Automated tests use local harness stand-ins. Live model behavior requires a
separate smoke test; the latest recorded result is in
[Harness support](harness-support.md#live-verification).
