# Usage

After creating your configuration, run `hypercube ui`, open
http://localhost:4280/, and choose the directory you want to work in, or run
`hypercube start <dir>`. In Git, the workspace root is the checkout's top
level; outside Git, it is the directory itself.

A workspace owns one tmux session. Each Hypercube session is a team in its own
window, with a lead on the left and workers stacked on the right.
Each session has a random ID, such as `k3f9a2c1`, and a title. A new session
shows as "New Session" until the first prompt you type to its lead titles it
with that prompt's first line, cut to 200 characters. You can change the title
at any time; later prompts leave it alone.
Agents are addressed as `id/name`, such as `k3f9a2c1/lead`; a bare name
means an agent in the sender's own team. Agents see each session's title beside
its ID. Workers message only their own team,
and leads can also message other teams' leads. Worker lifecycle is scoped to the
team.
Leads share the original directory, including its files, Git index,
and branch. Use worker worktrees for independent edits.

| Command | Use |
| --- | --- |
| `hypercube ui [port]` | Serve the browser view at http://localhost:4280/, or on the given port. The service from `make install-service` runs it. |
| `hypercube start <dir>` | Start a workspace's saved sessions, or a new workspace with one new session. |
| `hypercube stop [dir]` | End all windows in a workspace, keeping conversations and queued messages. |
| `hypercube delete [dir]` | Delete a workspace and its managed resources. |
| `hypercube status` | Show known workspaces and their status. |
| `hypercube running` | Print running workspace paths for scripts. |

Stop and delete default to the current
workspace; `--all` lists targets and asks for confirmation. They can also
use a disappeared checkout's saved path.

## Browser view

The browser view is Hypercube's interface. It shows the running workspaces
and talks to their tmux sessions.

The sidebar lists every running workspace's sessions under a row for the
workspace; click the row to collapse or expand them. Click `+` at the top of the
sidebar to choose a folder and start a workspace in it. Hover over a
workspace's row and click `+` to create a session in it, or click the trash icon
to delete the workspace. Click a session to switch to it, double-click it
to change its title, or hover over one and click the trash icon to delete it. A
workspace's folder icon becomes a spinner while any of its agents is busy, and
a session shows one in the same place. A session
shows an amber dot while any of its agents awaits your approval, and a blue dot
after its lead finishes a reply you have not seen. A worker's card shows a spinner while it is busy, an amber dot while
it awaits approval, and a red dot after it exits. See
[harness limitations](harness-support.md#known-limitations) when interpreting
activity.

The lead's terminal fills the left side, and each worker is a card on the
right. Click a card, or its terminal button, to show
the worker's terminal in the card; click the button again to return to the
summary. The summary shows the worker's latest report of what it is doing and
how long ago it made it, its context use, and its cost so far. Codex reports no
cost. Drag the dividers to resize the sidebar and the lead's side.

The panel button in the header shows or hides a terminal below the agents. It
runs your shell in the workspace's folder, one per workspace, and keeps running
while the panel is hidden or the browser is closed, until the shell exits or the
workspace stops. Drag the divider above it to change its height.

## Resume and cleanup

Closing the browser leaves agents running. Starting a workspace recreates its
closed saved windows with their conversations and queues, and does not restart
agents in existing windows.

A harness that exits is relaunched in its pane with its saved conversation.
Repeated quick exits stop automatic retries, and launch errors stay visible.
Starting a workspace does not retry an exited agent in an existing window; stop
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
