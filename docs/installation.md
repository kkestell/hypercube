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

## Upgrades

Run `make reinstall` **outside Hypercube**. It stops running workspaces
using the installed executable, installs the build, and starts them again.
Stopping ends any running turns.

Each agent's hooks, MCP server, and wake-ups use the executable path that
launched it. Replacing a binary alone does not update those launch arguments;
restart workspaces when switching executable paths.

## Development

`make dev` builds this checkout and opens its browser view at
http://localhost:4281/ with isolated state, configuration, tmux server, and a
scratch repository under `~/projects/hypercube-dev`. Set `HYPERCUBE_DEV_DIR` to
use another location.
`make dev-stop` stops it; `make dev-clean` removes its disposable resources
and harness transcripts. Details belong to [scripts/dev](../scripts/dev).

Automated tests use local harness stand-ins. Live model behavior requires a
separate smoke test; the latest recorded result is in
[Harness support](harness-support.md#live-verification).
