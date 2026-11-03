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
("Image" for a prompt of only images) until its agent sets a title. An agent
gives each child a title when it creates the child. You can rename any title;
later prompts leave it alone. A child's name stays fixed and cannot be reused
among that parent's children, including ended children.

A parent addresses its children by name, and any other running top-level
session across all workspaces in the app by ID. A child can message only its
parent, by ID. Stopped and ended recipients are refused. `list_sessions` lists
only running addressable recipients, with their workspace paths and parent IDs.
Top-level sessions in the same workspace share its files, Git index, and branch;
use child worktrees for independent edits.

| Command | Use |
| --- | --- |
| `hypercube ui [port] [--menu]` | Serve the browser view at http://localhost:4280/, or on the given port. `--menu` adds a menu bar icon and the Hypercube window, which shows the view and the sessions' browsers; the service from `make install-service` uses it. |

## Browser view

The sidebar lists top-level sessions newest first, in the order they were
created, so a session keeps its place. Each workspace shows five sessions;
Show more reveals five more. If the selected session is outside that group,
it takes the fifth place.
Children appear under their parent in creation order. Running children are always visible;
Show Stopped Sessions in the parent's menu lists stopped children too, and
Hide Stopped Sessions hides them again. Click Add Workspace at the bottom of
the sidebar to add a workspace. Click `+` beside a workspace's name to open a
new session, with the animated hypercube above its composer.
Its composer chooses the profile until the first prompt is sent,
and sending that prompt creates the session;
selecting another session first discards it. More Actions, shown while hovering a workspace, or
right-clicking a workspace, offers Delete Workspace.

Click a session to show its chat, and double-click it to change its title.
Hovering a row shows More Actions in place of its dot, which opens the same
menu as right-clicking it. A top-level session's menu offers Start/Stop,
Show/Hide Stopped Sessions when it has children, and Delete. Stopped child rows
are dimmed; clicking a child of a stopped parent starts the family. Each row's
dot shows only its own session: amber means it awaits you, and blue marks a reply
you have not seen. The composer shows the agent's spinner, waiting dot, or red
dot after an unexpected exit.

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

The center is one tabbed area holding a session's chat and its open files. The
chat's tab carries the session's title and cannot be closed. A file opens
beside the chat. A child's requests also appear in its parent's transcript, where you can answer them;
answering in either place updates both. These copies are for the user; the
parent's agent does not receive them.

A chat shows the agent's messages, reasoning, and tool calls in order, with a
box below it for your next message. Press Enter to send it and Shift+Enter to
start a new line. A message goes to the agent at once, even while it works.
Attach PNG, JPEG, GIF, or WebP images with the paperclip button or by dropping
files anywhere on the chat; they total at most 10 MiB, appear as thumbnails with
a remove button, and can be sent without text. Images from the agent or its
tools appear in the transcript. Press Escape to interrupt the shown agent's
current turn; the turn stays in the chat. Thinking and tool calls share one
expandable working block across messages
to the agent. Its header counts the time as "Working for 2m 10s…" until the agent
responds or stops working and every tool in the block finishes, then reads
"Worked for 2m 10s". Expand it to see the
thinking and each call's input, output, and native subagent calls. Failures
are marked on the individual calls. Inside the block, thinking counts its time
("Thinking for 5s…", then "Thought for 3m 10s"); click it to show a five-line
box of the newest text, which scrolls back through the rest. Messages between sessions show their sender or recipient
and first line; expand one to read the full message. When an agent asks you a question or for approval, its
chat shows the request with its choices and a field for typed text; answer
there. An answered request shows its question and the answer beside a check.
Each agent's unsent text stays in its
box when you switch sessions or messages arrive.

Below each message box, the profile selector shows the harness, model, and
effort on the left. It is disabled after the first message is sent. Context use
and cost appear on the right. Codex reports no cost. Drag a tab,
including the chat's, to another pane's tab bar to move it there, or to a pane's
left, right, top, or bottom edge to split it. Drag dividers to resize panes and
the sidebar.

The buttons at the right of the top-right tab bar show or hide the sidebar, the
terminal panel, and the right panel. The right panel's icon
tabs select Files and Changes; Changes is blank for now. Files
shows the session's directory as a read-only tree, including a child's worktree.
After an ended child's worktree is removed, it says the folder no longer exists. Click a file to preview it in
the tab area, and double-click it to keep its tab open.

In the Hypercube window, each session's agent has a browser, which it loads
pages in and runs scripts on with its `browser_navigate` and `browser_eval`
tools. The session's Browser tab shows the page, with back, forward, reload,
an address field, and a button that opens the page in your default browser.
The tab opens when the agent first loads a page and comes forward whenever the
agent navigates. Closing it keeps the page; browsers last until Hypercube
quits and share your logins.

The panel button shows or hides a terminal below the tabbed area. It
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
