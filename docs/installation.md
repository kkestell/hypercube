# Installation and use

Hypercube needs macOS, Git, tmux, and the harness executables selected in
[Configuration](configuration.md): `claude` or `opencode`. Configure
credentials through each harness before starting. See
[Harness support](harness-support.md) for compatibility and limitations.

Use the Go version required by [go.mod](../go.mod). Build and install with:

```sh
make install
```

The executable goes to `~/.local/bin/hypercube`; add that directory to PATH.
Set `BINDIR=<dir>` to install elsewhere. For a local build without installing,
run `make` and use `bin/hypercube`. Build and validation targets are defined
in the [Makefile](../Makefile).

## Workspaces and sessions

After creating your configuration, run `hypercube` from the directory you
want to work in. In Git, the workspace root is the checkout's top level;
outside Git, it is the current directory.

A workspace owns one tmux session. Each Hypercube session is a team in its own
window, with a lead on the left and workers stacked on the right.
Agents are addressed as `session/name`, such as `review/lead`; a bare name
means an agent in the sender's own team. Workers message only their own team,
and leads can also message other teams' leads. Worker lifecycle is scoped to the
team.
Leads share the original directory, including its files, Git index,
and branch. Use worker worktrees for independent edits.

| Command | Use |
| --- | --- |
| `hypercube` | Start missing saved windows and attach; a new workspace starts with `default`. |
| `hypercube new [name]` | Create and select another team. Omit the name for the first unused `session-N`. |
| `hypercube new -d [name]` | Create a team without attaching or changing the selected window. |
| `hypercube start <dir>` | Start saved teams detached. |
| `hypercube delete-session <name>` | Delete one team and its resources from the current workspace. |
| `hypercube stop [dir]` | End all windows in a workspace, keeping conversations and queued messages. |
| `hypercube delete [dir]` | Delete a workspace and its managed resources. |
| `hypercube status` | Show known workspaces and their status. |
| `hypercube running` | Print running workspace paths for scripts. |

Session names start with a lowercase letter or digit and contain only
lowercase letters, digits, and hyphens. Stop and delete default to the current
workspace; `--all` lists targets and asks for confirmation. They can also
use a disappeared checkout's saved path.

### Keyboard controls

With tmux's default prefix, press Ctrl-b, then the listed key:

| Key | Action |
| --- | --- |
| `w` | Choose a window. |
| `n` / `p` or a window number | Switch windows. |
| `Shift-c` | Create and select a Hypercube session. |
| `Shift-x` | Confirm deletion of the current Hypercube session. |
| Arrow key | Select a pane. |
| `z` | Zoom the current pane. |
| `d` | Detach. |
| `s` | Choose a tmux session. |

Hypercube installs the uppercase `C` and `X` bindings in tmux's shared prefix
table; they act only on windows marked as Hypercube teams. Other controls
come from tmux and follow your tmux configuration.

Pane titles identify the agent and its settings. A green dot means busy,
gray means idle, and red requests approval attention. An exited pane shows
its exit status. See [harness limitations](harness-support.md#known-limitations)
when interpreting activity.

### Resume and cleanup

Detaching or closing the terminal leaves agents running. Reattaching keeps
the selected window and does not restart agents in existing windows. Starting
recreates closed saved windows with their conversations and queues.

A harness that exits is relaunched in its pane with its saved conversation.
Repeated quick exits stop automatic retries, and launch errors stay visible.
A plain attach does not retry an exited agent in an existing window; stop
and start the workspace to retry after correcting the error.

Deleting a team or workspace removes its workers' managed worktrees and
branches only after checking for uncommitted work and commits not merged into
the original checkout's HEAD. Existing directories supplied to workers are
left alone. Failed cleanup stays recorded so deletion can be retried.

The cleanup check currently follows Git's display settings: hiding untracked
files or ignoring submodule changes can make a dirty worktree pass. Keep
those settings at their normal visibility and inspect worker work before
removal. The check is in
[workspace.go](../internal/workspace/workspace.go).

## Upgrades

Run `make reinstall` **outside Hypercube**. It stops running workspaces
using the installed executable, installs the build, and starts them detached.
Stopping ends any running turns. Attach afterward from each workspace.

Each agent's hooks, MCP server, and wake-ups use the executable path that
launched it. Replacing a binary alone does not update those launch arguments;
restart workspaces when switching executable paths.

## Development

`make dev` builds this checkout and opens Ghostty with isolated state,
configuration, tmux server, and a scratch repository under
`~/projects/hypercube-dev`. Set `HYPERCUBE_DEV_DIR` to use another location.
`make dev-stop` stops it; `make dev-clean` removes its disposable resources
and harness transcripts. Details belong to [scripts/dev](../scripts/dev).

Automated tests use local harness stand-ins. Live model behavior requires a
separate smoke test; the latest recorded result is in
[Harness support](harness-support.md#live-verification).
