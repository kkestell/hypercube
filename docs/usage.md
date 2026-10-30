# Usage

After creating your configuration, run `hypercube` from the directory you
want to work in. In Git, the workspace root is the checkout's top level;
outside Git, it is the current directory.

A workspace owns one mux session. Each Hypercube session is a team in its own
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

## Controls

Run `hypercube` from a shell outside the mux. The status bar lists the
workspace's teams on the left and the workspaces on the right; click one to
switch to it, or click a pane to focus it. Press Ctrl-b, then the listed key:

| Key | Action |
| --- | --- |
| `d` | Detach. |
| `n` / `p` | Switch to the next or previous team. |
| `0`–`9` | Switch to the team with that number. |
| `o` | Focus the next pane. |
| Ctrl-b | Send Ctrl-b to the focused pane. |

A pane's title is the agent's name, with the progress its harness reports. A
bell shows on a pane awaiting approval, and on its team and workspace in the
status bar, until it is answered, and rings the terminal's bell. An exited
pane shows its exit status.

When a pane's program does not use the mouse, the wheel scrolls its scrollback
and dragging selects text within that pane, copying it to the clipboard
through the terminal on release. Click a worker's title to collapse or open
it, and drag the divider to resize the lead's side. The icons need a Nerd Font
in the terminal.

One terminal attaches to a workspace at a time; attaching from another
detaches the first. Panes that do not fit the terminal are not drawn until it
grows.

## Resume and cleanup

Detaching or closing the terminal leaves agents running. Reattaching keeps
the selected team and does not restart agents in existing windows. Starting
recreates closed saved windows with their conversations and queues.

A harness that exits is relaunched in its pane with its saved conversation.
Repeated quick exits stop automatic retries, and launch errors stay visible.
A plain attach does not retry an exited agent in an existing window; stop
and start the workspace to retry after correcting the error.

Deleting a team or workspace removes its workers' managed worktrees and
branches only after checking for uncommitted work and commits not merged into
the original checkout's HEAD. Existing directories supplied to workers are
left alone. Failed cleanup stays recorded so deletion can be retried.

The cleanup check follows Git's display settings: hiding untracked
files or ignoring submodule changes can make a dirty worktree pass. Keep
those settings at their normal visibility and inspect worker work before
removal.
