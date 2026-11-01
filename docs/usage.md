# Usage

After creating your configuration, run `hypercube ui` and open
http://localhost:4280/. The browser view is Hypercube's interface.

A workspace is a folder that holds sessions. In Git, its folder is the
checkout's top level; outside Git, it is the folder itself. A session is a lead
and its workers, with the lead on the left and workers stacked on the right.
You create, start, stop, rename, and delete sessions; a stopped session has no
running agents and stays listed until you start it.

Each session has a random ID, such as `k3f9a2c1`, and a title. A new session
shows as "New Session" until the first prompt you type to its lead titles it
with that prompt's first line, cut to 200 characters. You can change the title
at any time; later prompts leave it alone.
Agents are addressed as `id/name`, such as `k3f9a2c1/lead`; a bare name
means an agent in the sender's own session. Agents see each session's title
beside its ID. Workers message only their own session, and leads can also
message the leads of the workspace's other sessions. Worker lifecycle is scoped
to the session.
Leads share the workspace's folder, including its files, Git index, and branch.
Use worker worktrees for independent edits.

| Command | Use |
| --- | --- |
| `hypercube ui [port] [--menu]` | Serve the browser view at http://localhost:4280/, or on the given port. `--menu` adds a menu bar icon; the service from `make install-service` uses it. |

## Browser view

The sidebar lists every workspace's sessions under a row for the workspace;
click the row to collapse or expand them. Click `+` at the top of the sidebar
to choose a folder and add it as a workspace. Hover over a workspace's row and
click `+` to list its lead profiles, each with its harness, model, effort, and
description; choose one to create a session with it.
Right-click a workspace to delete it.

Click a session to show it, and double-click it to change its title.
Right-click a session to start or stop it, or to delete it. A stopped session's
row is dimmed; clicking it starts it. A workspace's folder icon becomes a
spinner while any of its agents is busy, and a session shows one in the same
place. A session shows an amber dot while any
of its agents awaits your approval, and a blue dot after its lead finishes a
reply you have not seen. A worker's card shows a spinner while it is busy, an
amber dot while it awaits approval, and a red dot after it exits. See
[harness limitations](harness-support.md#known-limitations) when interpreting
activity.

The lead's terminal fills the left side, and each worker is a card on the
right. Click a card, or its terminal button, to show
the worker's terminal in the card; click the button again to return to the
summary. The summary shows the worker's latest report of what it is doing and
how long ago it made it, its context use, and its cost so far. Codex reports no
cost. Drag the dividers to resize the sidebar and the lead's side.

The panel button in the header shows or hides a terminal below the agents. It
runs your shell in the workspace's folder, one per session, and keeps running
while the panel is hidden or the browser is closed, until the shell exits or the
session stops. Drag the divider above it to change its height.

## Resume and cleanup

Closing the browser leaves agents running. Stopping a session keeps its
conversations and queued messages. Starting it resumes each agent's
conversation, and at login Hypercube starts the sessions that were running when
it last exited. A started session starts no turn and delivers no queued
message until you type a prompt into any of its agents.

A harness that exits is relaunched in its pane with its saved conversation.
Repeated quick exits stop automatic retries, and launch errors stay visible.
Stop and start the session to retry after correcting the error.

Deleting a session or workspace removes its workers' managed worktrees and
branches only after checking for uncommitted work and commits not merged into
the original checkout's HEAD. Existing directories supplied to workers are
left alone. Failed cleanup stays recorded so deletion can be retried.

The cleanup check currently follows Git's display settings: hiding untracked
files or ignoring submodule changes can make a dirty worktree pass. Keep
those settings at their normal visibility and inspect worker work before
removal. The check is in
[workspace.go](../internal/workspace/workspace.go).
