# Usage

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

## Keyboard controls

Each Hypercube session is a tmux window, listed by number and name in the
status line. With tmux's default prefix, press Ctrl-b, then the listed key:

| Key | Action |
| --- | --- |
| `c` | Create and select a Hypercube session. |
| `n` / `p` | Switch to the next or previous Hypercube session. |
| `0`–`9` | Switch to the Hypercube session with that number. |
| `w` | Choose a Hypercube session from a list. |
| `Shift-x` | Confirm deletion of the current Hypercube session. |
| Arrow key | Select a pane. |
| `z` | Zoom the current pane. |
| `d` | Detach. |
| `s` | Choose a workspace's tmux session. |

Clicking a Hypercube session's name in the status line also switches to it.

Hypercube installs the `c` and uppercase `X` bindings in tmux's shared prefix
table; they act only on windows marked as Hypercube teams. Elsewhere, `c`
opens a plain tmux window and `X` does nothing. Other controls
come from tmux and follow your tmux configuration.

Pane titles identify the agent and its settings. A green dot means busy,
gray means idle, and red requests approval attention. The status line also
puts a red dot after a session's name for each of its agents awaiting
approval. An exited pane shows its exit status. See [harness limitations](harness-support.md#known-limitations)
when interpreting activity.

## Resume and cleanup

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
