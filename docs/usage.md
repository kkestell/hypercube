# Usage

After creating your configuration, run `hypercube ui` and open
http://localhost:4280/. The browser view is Hypercube's interface.

A workspace is a folder that holds sessions. In Git, its folder is the
checkout's top level; outside Git, it is the folder itself. A session is a lead
and its workers, with their chats displayed in tabs.
You create, start, stop, rename, and delete sessions; a stopped session has no
running agents and stays listed until you start it.

Each session has a random ID, such as `k3f9a2c1`, and a title. A session is
titled with the first line of the prompt that creates it, cut to 200 characters
("Image" for a prompt of only images). You can change the title at any time;
later prompts leave it alone.
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

The sidebar lists each workspace's sessions by last activity, newest first,
with stopped sessions below running ones. It shows five under each workspace;
click the workspace row to collapse or expand them, and click Show more to
list five more. Click `+` at the top of the sidebar to choose a folder and add
it as a workspace. Hover over a workspace's row and click New Session to list
its lead profiles; choose one to open a new session with it. Sending its first
prompt creates the session; selecting another session first discards it. Click More Actions,
or right-click the row, to delete the workspace.

Click a session to show it, and double-click it to change its title.
Right-click a session to start or stop it, or to delete it. A stopped session's
row is dimmed; clicking it starts it. A session shows a spinner at the right of
its row while any of its agents is busy, an amber dot while any
of its agents awaits your approval, and a blue dot after its lead finishes a
reply you have not seen. A worker's card shows a spinner while it is busy, an
amber dot while it awaits you, and a red dot after it exits.

Notification cards stack in the top right for unseen lead replies and for
questions and approval requests outside the session you are viewing. Each card
shows the workspace, the session, and the worker when a worker asks, with the
reply's opening lines or the request with its choices and text field, so you
can answer from the card. The eye icon opens the session. Closing a reply's
card marks the reply seen; closing a request's card only hides it. A reply's
card closes by itself after 15 seconds, or when its lead starts working again.
A request's card stays until the request is answered or the card is closed.

The lead's chat stays in its own column. The right panel's Workers tab shows
cards with each worker's latest report and its age. Click a card's Open
Transcript icon to open that worker in the tab area to the right of the lead.
That area appears when the first tab opens and disappears when the last tab
closes. Drag the divider to resize the lead's column. New worker tabs use the
first pane, ordered from left to right and top to bottom. Worker tabs show
their agent's activity indicator; the lead's appears beside its harness and
model. Closing a worker's tab leaves it running.

A chat shows the agent's messages, reasoning, and tool calls in order, with a
box below it for your next message. Press Enter to send it and Shift+Enter to
start a new line. A message goes to the agent at once, even while it works.
Attach PNG, JPEG, GIF, or WebP images with the paperclip button or by dropping
files anywhere on the chat; they total at most 10 MiB, appear as thumbnails with
a remove button, and can be sent without text. Images from the agent or its
tools appear in the transcript. The stop button next to the send button appears
while the agent works and interrupts its current turn; the turn stays in the
chat. Consecutive tool calls share one expandable item. It shows the latest
call until the next non-tool item arrives, then shows the elapsed time as
"Worked for 32s". Expand it to see each call's input, output, and native
subagent calls. Failures are marked on the individual calls. Thinking shows
its latest six lines by default; click Thinking to expand the full text or
return to the preview. When an agent asks you a question or for approval, its
chat shows the request with its choices and a field for typed text; answer
there. An answered request shows its question and the answer beside a check.
Each agent's unsent text stays in its
box when you switch sessions or messages arrive.

Below each message box, the harness, model, and effort appear on the left,
with context use and cost on the right. Codex reports no cost. Drag a tab to
another pane's tab bar to move it there, or to a pane's left, right, top, or
bottom edge to split it. These moves and splits stay within the worker tab
area; the lead stays in its column. Drag dividers to resize panes and the sidebar.

The right panel button in the header shows or hides the right panel. Its icon
tabs select Workers, Files, and Changes; Changes is blank for now. Files
shows the session's folder as a read-only tree. Click a file to preview it in
the tab area, and double-click it to keep its tab open.

The panel button in the header shows or hides a terminal below the agents. It
runs your shell in the workspace's folder, one per session, and keeps running
while the panel is hidden or the browser is closed, until the shell exits or the
session stops. Drag the divider above it to change its height.

## Resume and cleanup

Sessions stop automatically after ten minutes without a prompt or agent
activity, once none of their agents is working. Sessions selected in an open
browser page are exempt. Closing the browser leaves
agents running until that timeout. Quitting Hypercube ends them, and
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
