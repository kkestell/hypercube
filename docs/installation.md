# Installation

Hypercube is a Go command for macOS. It needs Go 1.27.1 or later, Git, tmux,
and the harnesses selected in `config.json`: `claude` for Claude Code and
`opencode` for OpenCode 2.x. Each harness uses its own configured credentials.

## Build and check

```sh
make
make check
```

The executable is `bin/hypercube`. The build uses the Go standard library,
with no external Go modules. `make check` builds, runs `go vet`, checks Go
formatting, and runs the tests with the race detector. The integration tests
use an isolated tmux server and local harness stand-ins.

## Configure and run

Create `~/.config/hypercube/config.json`, or
`$XDG_CONFIG_HOME/hypercube/config.json`, following
[Configuration](configuration.md). Then run from your checkout:

```sh
/path/to/hypercube/bin/hypercube
```

Hypercube creates one tmux session per workspace and one pane per agent.
Detach with Ctrl-b d; run `hypercube` in the checkout to attach again.
`hypercube start <dir>` starts a team without attaching. `stop [dir]` ends
its panes but keeps conversation IDs and queued messages; `delete [dir]`
also removes its state and merged, clean worker worktrees.

## Install

```sh
make install
```

This installs to `~/.local/bin/hypercube`; `BINDIR=<dir>` selects another
destination. Put that directory on PATH.

To replace an installed build while retaining its saved sessions:

```sh
make reinstall
```

Run this outside Hypercube. It uses the currently installed binary to stop
running workspaces, installs the new binary, and starts those workspaces
again detached. A running turn ends when its workspace stops. Run
`hypercube` in its checkout to attach afterward. Existing state with multiple
teams or scheduled tasks is refused on startup and left unchanged.

Each agent uses the executable path that launched it for MCP, hooks, and
wake-ups. Stop and restart a workspace to put all of its agents on a new
build; replacing the binary alone does not update their launch arguments.

## Development

`make dev` builds this checkout and opens a team in Ghostty with isolated
state, configuration, tmux server, and a scratch repository under
`~/projects/hypercube-dev` (`HYPERCUBE_DEV_DIR` overrides it).
`make dev-stop` stops it; `make dev-clean` removes its disposable state,
repository, and harness transcripts. The installed workspaces are separate.
