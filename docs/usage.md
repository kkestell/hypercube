# Usage

After creating your configuration, run `hypercube ui` and open
http://localhost:4280/. The browser view is Hypercube's interface.

A workspace is a folder that holds sessions. In Git, its folder is the
checkout's top level; outside Git, it is the folder itself. Each session has
one agent and its own chat. A top-level session's agent can create child sessions
one level deep to work in parallel. Together they form a family. Starting or stopping a top-level session
starts or stops its children too.

Each session has a random ID, such as `a3f9a2c1`, and a title. A top-level
session is titled with the first line of its first prompt, cut to 200 characters
("Image" for a prompt of only images). A child's title starts as its name. You
can rename any title; later prompts leave it alone. A child's name stays fixed
and cannot be reused among that parent's children, including ended children.

A parent addresses its children by name, and other top-level sessions in the
workspace by ID. A child can message only its parent, by ID. `list_sessions`
lists the sessions each agent can message. Top-level sessions share the
workspace's files, Git index, and branch; use child worktrees for independent
edits.

| Command | Use |
| --- | --- |
| `hypercube ui [port] [--menu]` | Serve the browser view at http://localhost:4280/, or on the given port. `--menu` adds a menu bar icon; the service from `make install-service` uses it. |

## Browser view

The sidebar lists top-level sessions by last activity, newest first, with
stopped sessions below running ones. It shows five under each workspace;
Show more lists five more. Children appear under their parent in creation
order and do not count toward this limit. Click a workspace's name or caret to
collapse or expand it. A parent's caret collapses or expands its children;
clicking its title selects the session without changing its children's
expansion. A collapsed parent still lists the children waiting on you, then a
row counting the rest and how many have unseen replies; click it to expand the
parent. Click Add Workspace at the bottom of the sidebar to add a workspace. Click `+`
beside a workspace's name to choose a lead profile for a new session. Sending
its first prompt creates the session; selecting another session first discards
it. More Actions, shown while hovering a workspace, or right-clicking a
workspace, offers Delete Workspace.

Click a session to show its chat, and double-click it to change its title.
Hovering a row shows More Actions in place of its dot, which opens the same
menu as right-clicking it. A top-level session's menu offers Start/Stop and
Delete. Stopped rows are dimmed; clicking a child of a stopped parent starts
the family. Each row's dot shows only its own session: amber means it awaits
you, and blue marks a reply you have not seen. The composer
shows the agent's spinner, waiting dot, or red dot after an unexpected exit.

A parent ends children with `end_session`. Ending removes a managed worktree
and branch only when Git permits it, and stops the child for good. The ended
row stays dimmed and opens its saved chat with sending disabled. Its menu offers
Delete Session, which deletes its chat. An active child has no menu.

Notification cards stack in the top right for unseen replies and open
questions or approval requests from any session. A card names the workspace,
parent, and child, and a request card offers its answers. A card is hidden
while you view its own session. The eye opens the requesting session.
Closing a reply card marks it seen; closing a request card only hides it.
Reply cards close after 15 seconds or when the agent works again. Request
cards stay until answered or closed.

The center shows one session's chat beside its open files. Drag the divider
to resize them. The file area appears only while files are open. A child's
requests also appear in its parent's transcript, where you can answer them;
answering in either place updates both. Child status updates appear there too,
with the child's title and time. These copies are for the user; the parent's
agent does not receive them.

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
with context use and cost on the right. Codex reports no cost. Drag a file tab to
another pane's tab bar to move it there, or to a pane's left, right, top, or
bottom edge to split it. These moves and splits stay within the file
area; the chat stays in its column. Drag dividers to resize panes and the sidebar.

The right panel button in the header shows or hides the right panel. Its icon
tabs select Files and Changes; Changes is blank for now. Files
shows the session's directory as a read-only tree, including a child's worktree.
After an ended child's worktree is removed, it says the folder no longer exists. Click a file to preview it in
the tab area, and double-click it to keep its tab open.

The panel button in the header shows or hides a terminal below the chat. It
runs your shell in the session's directory, one per session, and keeps running
while the panel is hidden or the browser is closed, until the shell exits or the
session stops. Drag the divider above it to change its height.

## Resume and cleanup

A family stops automatically after ten minutes without a prompt or agent
activity, once no member is working. Viewing any family member in a browser
page keeps the family running. Closing the browser leaves agents running until
that timeout. Quitting Hypercube ends them, and families that were running
start again at the next launch. Stopping keeps conversations, chats, and queued
messages. Starting resumes the top-level agent and children that have not
ended. It starts no turn and delivers no queued message until you prompt any
family member.

A harness that exits after starting is relaunched with its saved conversation.
Repeated quick exits stop automatic relaunches. A harness that fails to start
stays exited, with the error in its chat. Stop and start the family to retry.

Deleting a top-level session deletes its children and their chats, managed
worktrees, and branches. Git first checks for uncommitted work and commits not
merged into the original checkout's HEAD. Existing directories supplied to
children are left alone. Failed cleanup stays recorded so deletion can be retried.
Ended children can also be deleted individually.

The cleanup check currently follows Git's display settings: hiding untracked
files or ignoring submodule changes can make a dirty worktree pass. Keep those
settings at their normal visibility and inspect child work before ending it.
The check is in [workspace.go](../internal/workspace/workspace.go).
