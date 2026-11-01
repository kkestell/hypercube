# Usage

After creating your configuration, run `hypercube ui` and open
http://localhost:4280/. The browser view is Hypercube's interface.

A workspace is a folder that holds sessions. In Git, its folder is the
checkout's top level; outside Git, it is the folder itself. A session is a lead
and its workers, with their chats displayed in tabs.
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
click `+` to list its lead profiles; choose one to create a session with it.
Right-click a workspace to delete it.

Click a session to show it, and double-click it to change its title.
Right-click a session to start or stop it, or to delete it. A stopped session's
row is dimmed; clicking it starts it. A workspace's folder icon becomes a
spinner while any of its agents is busy, and a session shows one in the same
place. A session shows an amber dot while any
of its agents awaits your approval, and a blue dot after its lead finishes a
reply you have not seen. A worker's card shows a spinner while it is busy, an
amber dot while it awaits you, and a red dot after it exits.

The lead's chat opens in a tab that cannot be closed. Click Workers in its
header to see preview cards with each worker's latest report and its age.
Click a card's Open Transcript icon to open that worker in a tab. The first
worker opens in a new pane to the right; later workers reuse that pane. With
other splits, new workers use the first pane without the lead, ordered from
left to right and top to bottom. Closing a worker's tab leaves it running.

A chat shows the agent's messages, reasoning, and tool calls in order, with a
box below it for your next message. Press Enter to send it and Shift+Enter to
start a new line. A message goes to the agent at once, even while it works.
Attach PNG, JPEG, GIF, or WebP images with the paperclip button or by dropping
files anywhere on the chat; they total at most 10 MiB, appear as thumbnails with
a remove button, and can be sent without text. Images from the agent or its
tools appear in the transcript. The stop button next to the send button appears
while the agent works and interrupts its current turn; the turn stays in the
chat. A tool call shows its name and its main argument; click it to see its
input, its output, and the calls a native subagent made inside it. When an agent
asks you a question or for approval, its chat shows the request with its choices
and a field for typed text; answer there. Each agent's unsent text stays in its
box when you switch sessions or messages arrive.

Below each message box, the harness, model, and effort appear on the left,
with context use and cost on the right. Codex reports no cost. Drag a tab to
another pane's tab bar to move it there, or to a pane's left, right, top, or
bottom edge to split it. Drag dividers to resize panes and the sidebar.

The panel button in the header shows or hides a terminal below the agents. It
runs your shell in the workspace's folder, one per session, and keeps running
while the panel is hidden or the browser is closed, until the shell exits or the
session stops. Drag the divider above it to change its height.

## Resume and cleanup

Closing the browser leaves agents running. Quitting Hypercube ends them, and
the sessions that were running start again at the next start. Stopping a session
keeps its conversations, chats, and queued messages. Starting it resumes each agent's
conversation, and at login Hypercube starts the sessions that were running when
it last exited. A started session starts no turn and delivers no queued
message until you type a prompt into any of its agents.

A harness that exits is relaunched with its saved conversation. Repeated quick
exits stop automatic retries, and launch errors stay in the agent's chat. Click
the session, or stop and start it, to retry after correcting the error.

Deleting a session or workspace removes its workers' managed worktrees and
branches only after checking for uncommitted work and commits not merged into
the original checkout's HEAD. Existing directories supplied to workers are
left alone. Failed cleanup stays recorded so deletion can be retried.

The cleanup check currently follows Git's display settings: hiding untracked
files or ignoring submodule changes can make a dirty worktree pass. Keep
those settings at their normal visibility and inspect worker work before
removal. The check is in
[workspace.go](../internal/workspace/workspace.go).
