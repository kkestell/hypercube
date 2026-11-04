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
Top-level sessions that run in the workspace's checkout share its files, Git
index, and branch; use worktree sessions or child worktrees for independent
edits.

| Command | Use |
| --- | --- |
| `hypercube ui [port] [--menu]` | Serve the browser view at http://localhost:4280/, or on the given port. `--menu` adds a menu bar icon and the Hypercube window, which shows the view and the sessions' browsers; the service from `make install-service` uses it. |

## Browser view

The interface follows the system’s light or dark appearance.

The sidebar lays out sessions as VS Code's Explorer does: each workspace is a
collapsible section, with running sessions ahead of stopped sessions, keeping
creation order within each group. A parent with a running child stays in the
running group. Children appear under their parent in creation order. Stopped and ended sessions are
dimmed, and a long list scrolls. Click a section header to collapse or expand
it, and click a parent's chevron or double-click the parent to collapse or
expand its children. Hovering or focusing a section header shows New Session
(`+`) and More Actions (`…`), which offers Delete Workspace; right-clicking the
section also opens its menu. With the focus on a section header, Enter or Space
toggles it, Left collapses it, and Right expands it.

Workspace and session menus also offer Open in VS Code, Reveal in Finder, and
Open in Terminal. These open the workspace folder or the session’s directory,
including its worktree when it has one.

A new session shows the animated hypercube above its composer. Add Workspace
in the sidebar header adds a project and opens a new session in it.
The composer chooses the profile until the first prompt is sent, and sending
that prompt creates the session. In a Git workspace, the Worktree switch beside
the profile, off by default, runs the session in its own worktree under
`.hypercube/` on a new branch from the checkout's HEAD; the branch checked out
in the checkout is its merge target. Selecting another session first discards
the unsent session.

Search in the sidebar header or Command+K searches session titles and saved
conversation text across all workspaces, including stopped, ended, and archived sessions.
Choose a result to show its session.

Choose Help > Keyboard Shortcuts or press Command+/ to see the keyboard
shortcuts. In the native app, the Session and View menus also show shortcuts;
View > Zoom In, Zoom Out, and Actual Size change the window's page size. The
terminal keeps its own keys except Command+P and Command+comma, and text fields keep their editing
shortcuts.

The status bar along the bottom of the window shows each harness's
subscription usage once an agent of that harness has reported it: for each
usage window, its length, a meter of the percent used, and the time left
until it resets, such as `2d 4h`. A mark on the meter shows how much of the
window has elapsed, the pace that would use exactly all of it; the meter turns
amber when usage is ahead of that pace. Claude Code and Codex report after
each turn. A window past its reset time keeps its last reported percent,
dimmed, until the harness reports again.

Click a session to show its chat. Up, Down, Home, and End move through the
tree; Right expands a parent or moves to its first child, Left collapses it or
moves from a child to its parent, and Enter or Space shows the session. Press F2,
or choose Rename from a row's menu, to change a session's title. Right-click a
row, or press Shift+F10 on it, to open its menu. A top-level session's menu
offers Start/Stop, Rename, Archive, and Delete. Stopped rows are
dimmed; clicking a child of a stopped parent starts the family. The right edge
of each row shows only its own session: a spinner while it works, an amber dot
while it awaits you, a blue dot for a reply you have not seen, and a red dot
after an unexpected exit. An idle session shows nothing there.
The composer shows the agent's spinner, waiting dot, or red
dot after an unexpected exit.

A worktree session's row shows a branch icon just left of its status. The icon is
purple while the branch has commits not merged into the checkout's branch or
uncommitted files, and dimmed once its commits are merged. Its composer shows the branch
beside the profile. While the branch has unmerged commits, a Merge button
follows it, which asks the session's agent to commit any remaining work and
merge the branch into the checkout's branch, resolving conflicts. The merge
state updates whenever a session in the workspace finishes a turn.

A parent ends children with `end_session`. Ending removes a managed worktree
and branch only when Git permits it, and stops the child for good. The ended
row stays dimmed and opens its saved chat with sending disabled. Its menu offers
Rename and Delete Session, which deletes its chat. An active child's menu
offers Rename.

Notification cards stack in the top right for unseen replies and open
questions or approval requests from any session. A card names the workspace,
parent, and child, and a request card offers its answers. A card is hidden
while you view its own session. The eye opens the requesting session.
Closing a reply card marks it seen; closing a request card only hides it.
Reply cards close after 15 seconds or when the agent works again. Request
cards stay until answered or closed.

While the Hypercube window lacks keyboard focus, whether another app is active
or the window is hidden or minimized, macOS shows a notification for each new
request and reply instead. Clicking one brings the window forward and opens its
session. A window with focus shows only the cards.

The center is one tabbed area holding a session's chat and its open files. The
chat's tab carries the session's title and cannot be closed. A file opens
beside the chat. A child's requests also appear in its parent's transcript, where you can answer them;
answering in either place updates both. These copies are for the user; the
parent's agent does not receive them. Completed requests disappear from the
parent's transcript and remain in the child's chat.

A chat shows everything the agent does in order, with a
box below it for your next message. Press Enter to send it and Shift+Enter to
start a new line. A message goes to the agent at once, even while it works.
Attach PNG, JPEG, GIF, or WebP images with the paperclip button, by pasting into the composer, or by dropping
files anywhere on the chat; they total at most 10 MiB, appear as thumbnails with
a remove button, and can be sent without text. Images from the agent or its
tools appear in the transcript. Press Escape to interrupt the shown agent's
current turn, or click Stop in an empty composer while it works; the turn stays in the chat. Scroll up to read earlier output and click the down arrow above the composer to return to the newest output.

A message you send, or one from another session, while the agent is idle begins a
turn. While the agent works, the chat shows its reasoning in full, each tool
call collapsed to its title and status, failures marked on the
individual calls, its progress text, messages to and from other sessions, and
messages you send during the turn, all in order. Expand a tool call to see its
input, output, images, and native subagent calls. When the turn ends successfully, the message that began it
stays, followed by one closed "Worked for 2m 10s" row and the agent's whole final
response. The time runs from the beginning message until the agent is idle. Click the row to see
everything else the turn showed, in the same order. A message that began the
turn from another session shows its sender and first line; expand it to read
the full message. A turn you interrupt, a turn that fails, a turn whose final
response the harness cannot identify, and chats saved by earlier versions show every row. When an agent asks you a question or for approval, its
chat pins the request above the message box, with its choices and a field for
typed text. It stays there as output arrives and you scroll through the chat;
answer there. Supported tool approvals also offer Always allow in this session and
Always allow in all sessions. Saved choices belong to that harness and survive
restarts. Hypercube then allows that tool's later approvals without asking,
and the tool's call still appears in the chat. Remove saved choices in Settings, opened from the sidebar or with Command-comma. An answered
request returns to its place in the chat, showing its question and the answer
beside a check.
Unsent text, including typed answers to requests and a new session's first
prompt, is saved in the browser and restored after reloads and app restarts.
Image attachments stay in memory and do not survive a reload.

Below each message box, the profile selector shows the harness, model, and
effort on the left. It is disabled after the first message is sent. Context use
and cost appear on the right. Codex reports no cost. Drag a tab,
including the chat's, to another pane's tab bar to move it there, or to a pane's
left, right, top, or bottom edge to split it. Drag dividers to resize panes and
the sidebar.

The buttons at the right of the header at the top of the window show or hide
the sidebar, the terminal panel, and the right panel. The right panel's icon
tabs select Files and Changes. Changes shows the branch and any commits not
merged into the checkout's branch, followed by the repository's uncommitted
files with their status and added and removed line counts. It covers the whole
repository containing the session's folder. Click a file to open its unified
diff in a preview tab; double-click the tab to keep it open. Files
shows the session's directory as a read-only tree, including a child's worktree.
In a file or diff, Command+F opens find; Enter and Shift+Enter move between matches, and Escape closes it. Toggle word wrap for all files and diffs with the tab bar button or Option+Z. In a diff, the arrow buttons or Option+F5 and Shift+Option+F5 move between changes.

After an ended child's worktree is removed, it says the folder no longer exists. Click a file to preview it in
the tab area, and double-click it to keep its tab open. Right-click a tree row
or file tab to copy its absolute or relative path. A Markdown file opens in its
preview; the button at the right of its tab bar switches to its source and
back. An agent can use `open_file` to open any file you can read, in or
outside its directory, and highlight lines; the tab opens when you view its session.

In the Hypercube window, each session's agent has a browser, which it loads
pages in and runs scripts on with its `browser_navigate` and `browser_eval`
tools and closes with `browser_close`. The session's Browser tab shows the page, with back, forward, reload,
an address field, and a button that opens the page in your default browser.
The tab opens when the agent first loads a page and comes forward whenever the
agent navigates. Closing the tab keeps the page; a browser lasts until its
agent closes it or Hypercube quits, and browsers share your logins.

The panel button shows or hides a terminal below the tabbed area. It
runs your shell in the session's directory, one per session, and keeps running
while the panel is hidden or the browser is closed, until the shell exits or the
session stops. Drag the divider above it to change its height.

## Resume and cleanup

A family keeps running until you stop it. Stopping a worktree session keeps
its worktree and branch. Closing the browser leaves agents
running. Quitting Hypercube ends them, and families that were running
start again at the next launch. Stopping keeps conversations, chats, and queued
messages. Starting resumes the top-level agent and children that have not
ended. It starts no turn and delivers no queued message until you prompt any
family member.

A harness that exits after starting is relaunched with its saved conversation.
Repeated quick exits stop automatic relaunches. A harness that fails to start
stays exited, with the error in its chat. Stop and start the family to retry.

Archiving a top-level session runs the same Git checks as deleting it, stops its
family, and removes its managed worktrees and branches. It keeps the family’s
chats read-only and removes the family from the sidebar. Search can open an
archived chat. Settings, opened from the sidebar or with Command-comma, lists
archived sessions with their workspaces. Open one to see its read-only chat
and its children in the sidebar, or delete it and its family's chats from
Settings. The family leaves the sidebar again when you select another session.

Deleting a top-level session deletes its children and their chats, managed
worktrees, and branches, and a worktree session's own worktree and branch. Git
first checks for uncommitted work and commits not merged: a child's into its
parent's checkout or worktree, and a worktree session's into the original
checkout's HEAD. Delete refuses, changing nothing, until they pass. Existing directories supplied to
children are left alone. Failed cleanup stays recorded so deletion can be retried.
Ended children can also be deleted individually.

The cleanup check follows Git's submodule settings: ignoring submodule changes
can make a dirty worktree pass. Inspect child work before ending it.
The check is in [workspace.go](../internal/workspace/workspace.go).
