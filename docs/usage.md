# Using Hypercube

## Start a session

Choose **Add Workspace** to select a project folder, or **New Session** for a
workspace already in the sidebar. Choose a [profile](configuration.md) below
the message box and, in a Git workspace, **Local** to work in the checkout or
**Worktree** to work on a new branch in its own folder above it. The choice
starts at the default location from **Settings**. Send the first prompt to
create the session.

Once you type or attach something, the new session stays in the sidebar as a
**New Session** row, keeping its prompt, profile, and worktree choice while you
switch sessions. Choose **Discard Draft** from its context menu to remove it.
Typed text also survives reloading the app; image attachments stay available
until the app reloads.

Leave Worktree off to use the original checkout, including its uncommitted
changes. Turn it on for a separate checkout and branch. That branch starts at
HEAD, so it excludes uncommitted work. See
[Sessions, context, and working copies](introduction.md) before putting agents
to work on the same project.

A session keeps the harness it started with. The model and effort can change
later through the profile selector, including during work. The selection
survives restarts; editing or deleting the profile does not alter existing
sessions automatically. [Harness timing](harness-support.md#messages-and-model-changes)
determines when a new selection takes effect.

## Send a correction or stop a turn

Send another message while the agent works. Claude Code, Codex, and Pi can use
it during that turn; OpenCode queues it for the next turn. **Escape** in chat,
or **Stop** in an empty message box, interrupts the current turn without
removing its output.

**Enter** sends; **Shift+Enter** adds a line. Attach images with the paperclip,
by pasting, or by dropping them onto chat. Supported formats are PNG, JPEG,
GIF, and WebP, totaling at most 10 MiB per message. You can send images alone.
Draft text survives reloads, including typed answers and a new session's first
prompt. Image attachments do not.

## Inspect what happened

Successful turns collapse their intermediate activity into **Worked for …**.
Expand that row for the reasoning, tool calls, and messages between sessions.
Expand an individual tool call for its input and output. Failed or interrupted
turns keep their activity visible.

### Files and diffs

The right panel follows the selected session's working directory:

| View | Use |
| --- | --- |
| **Files** | Open a read-only file preview. Double-click to keep the tab open. Markdown, HTML, and SVG have preview and source views. |
| **Changes** | Inspect uncommitted files and unified diffs, plus commits not merged into the original checkout's branch. |
| **Search** | Find case-insensitive plain text and open a matching line. |

Images, audio, video, and PDFs use the browser's built-in viewers without the
text viewer's 256 KiB limit. HTML previews load relative resources with scripts
disabled. Media support depends on the browser's formats and codecs. Source
view is available for HTML and SVG with supported text encodings within the
text size limit.

Changes covers the whole repository containing the session directory. Search
uses the same file list as Quick Open: in Git, tracked and untracked files that
are not ignored, excluding hidden paths. It skips files over 256 KiB and stops
at 1,000 matching lines.

| Shortcut | Action |
| --- | --- |
| **Command+P** | Find a session file by name. |
| **Command+Shift+F** | Search session files. |
| **Command+F** | Find within the open file or diff. Enter/Shift+Enter moves between matches. |
| **Option+F5** / **Shift+Option+F5** | Move between changes in a diff. |
| **Option+Z** | Toggle word wrap for files and diffs. |

Right-click a file tab or tree row to copy its absolute or relative path. An
agent can also open files and highlight lines for you, including files outside
its working directory; those tabs appear when you view its session.

### Terminal and browser

The terminal panel is your shell in the selected session's directory. Each
session has its own shell. Hiding the panel, closing the interface, or quitting
the desktop app leaves it running; archiving or deleting the session, or ending
the backend, ends it.

With the desktop app open, an agent's page navigation opens its own browser
window. It accepts HTTP, HTTPS, and `about:blank` addresses. The toolbar above
the page has back, forward, reload, an address field, and a button that opens
the page in your default browser. The **Window** menu
lists open browser windows. Closing the window or the agent's `browser_close`
ends that browser; quitting the desktop app closes them all and fails their
pending calls. With the app closed, an agent's browser calls fail with a
message asking you to open it. Session browsers share website logins.

Quitting the desktop app, from ⌘Q, the menu bar icon, or the Dock, ends only
the app. Agents, shells, and messaging keep running in the background service.
Ending every agent at once is a service command:
`launchctl kill SIGINT gui/$(id -u)/com.github.kkestell.hypercube`, or Ctrl-C
for a `hypercube ui` in a terminal.

### Merge a worktree session

For a top-level session in its own worktree, **Merge** appears beside the branch
below the message box while it has unmerged commits. It asks the agent to
commit remaining work and merge into the original checkout's branch, including
resolving conflicts. This is an instruction to the agent, not an automatic Git
operation.

Hover over a session's branch icon for its merge status. The status refreshes
when a session in that workspace finishes a turn. For child branches, ask the
parent to merge the work.

## Context and subscription use

Below the message box, a ring shows the session's context use; hover over it
for token counts and cost. Beside it, sessions whose harness reports
subscription usage show the percent used and, in a wide enough pane, the time
until reset. It shows the shortest reported window normally and the longest on
hover or keyboard focus. When the harness reports only one window, that window stays
visible. The meter's mark shows how much of the time window has elapsed.
After a window resets, its last reading stays dimmed and marked **Waiting**
until the harness reports again. See
[usage reports](harness-support.md#usage-reports) for what each harness supplies.

## Answer a request

Questions and approvals stay above the message box while you scroll. A child's
request also appears in its parent's chat, where you can answer without
switching sessions. The parent's agent does not receive that copy. Once
answered, it disappears there and remains in the child's history.

Questions that allow several choices show checkboxes. Select the choices you
want, then choose **Submit**; you can also include an **Other response** when
the question offers it.

Approvals may offer **Always allow in this session** or **Always allow in all
sessions**. These choices belong to the harness and survive restarts. Remove
saved choices in **Settings** (**Command+,**). Available approval choices vary
[by harness](harness-support.md#permissions).

For requests from other sessions, answer the notification card or use its eye
button to open the chat. Closing a request card hides it without answering.
Closing a reply card hides it without marking the reply seen; reply cards also
close after 15 seconds or when the agent works again. Their expiry pauses while
the pointer is over the card or keyboard focus is inside it.

While the desktop app is open and its window lacks focus, macOS notifications
announce requests and replies, with a sound. Clicking one opens its session.
**Settings → Notifications** turns request and reply alerts off, both as cards
and as macOS notifications, and turns off macOS notifications or their sound. Requests and
replies that arrive while the app is closed are not announced later; the badge
and the page show the current requests when it opens.

## Find a conversation

**Command+K** searches saved titles and conversation text across workspaces,
including ended and archived sessions. **F2** or **Rename** in a
session's menu changes its title. Right-click a row for its menu; workspace
and session menus can also open their directories in VS Code, Finder, or
Terminal.

Children appear under their parent. A breathing dot indicates work, an amber
dot means an answer is needed, and a blue dot marks an unseen reply. Ended
children are dimmed and carry no dots.

When you explicitly ask for another session or delegation, such as an
independent review by another agent, the agent starts it without asking again.
For a separate new task whose placement you have not chosen, a question above
the message box names the task and offers **New session** and **Do it here**. The
agent ends its turn and your answer reaches it as a message, whenever you give
it; the question stays open across reloads and restarts. New session lets the
agent start one child for that task with a brief it writes then; Do it here
keeps the work in the session. Unanswered offers also appear in request
notifications and waiting indicators, even while the parent is parked.
Corrections and follow-ups need no question;
follow-ups on a child's work go back to that child while its context helps.

Work the agent suggests on its own appears as cards showing the brief,
profile, context, and folder or new worktree.
**Start** launches it; **Dismiss** closes the card. Handled cards collapse to a
single line; expand one to read its brief again. Cards survive reloads and
do not block the parent. An unsuccessful launch shows an error and leaves the
card or accepted offer available to retry when its child was cleaned up.

## Layout and shortcuts

Drag tabs between panes, or onto a pane's edge to split it. Drag dividers to
resize panes and panels. Buttons at the right of the footer toggle the
sidebar, terminal, and right panel. Each session remembers its tabs, editor
splits, open files, expanded items, and scroll positions across switches and
restarts. Overall panel sizes, sidebar choices, and the last selected session
are remembered in this browser; the desktop app keeps its own view choices.
Use **View → Zoom In**, **Zoom Out**, or
**Actual Size** in the desktop app to change page size. The theme follows macOS
appearance unless **Settings** sets it to Light or Dark; Settings also sets
the terminal's text size. In the desktop app, the menus own the shortcuts they show, so ⌘N,
⌘B, ⌘J, ⌘⌥B, ⌘⌥↑/↓, ⌘,, ⌘/, and zoom work wherever focus is, including text
fields, terminals, and dialogs.

**Help → Keyboard Shortcuts** (**Command+/**) lists all shortcuts, including
sidebar navigation. Text fields retain their editing keys. The terminal keeps
its own keys except **Command+P**, **Command+Shift+F**, and **Command+,**.

## Resume and cleanup

Hypercube starts a session's agent, resuming its conversation, when you open
the session, send it a prompt, or a message reaches it, such as a child's
report. It ends the agent after 15 idle minutes while nobody views the
session; the conversation, queued messages, branch, and worktree stay. Each
session starts and stops on its own, children included.

After a backend restart, Hypercube reconnects to agents that survived. Other
sessions start when next used. A harness that exits unexpectedly is relaunched
with its conversation; repeated quick failures stop relaunches. For a failed
start, the error stays in chat. Open the session or send it a prompt to retry.

Choose a cleanup action according to what you want to keep:

| Action | Conversation history | Managed worktrees and branches |
| --- | --- | --- |
| **Archive** | Kept read-only and searchable; family leaves the sidebar. | Removed after Git checks. |
| **Delete** | Deleted with the family's chats. | Removed after Git checks. |
| **End child** | Kept read-only; that child cannot resume. | Removed after Git checks. |
| **Unarchive** | Returns to the session list. | A worktree session gets its worktree back, on its branch from the checkout's HEAD. |

The parent ends children with its `end_session` tool. Ended children can later
be deleted individually. Existing directories assigned to children are left
alone by cleanup.

Before removing a managed worktree, Git checks for uncommitted files and
unmerged commits. Child commits must be merged into the parent; a top-level
worktree's commits must be merged into the original checkout's HEAD. Deletion
refuses without changing anything until its checks pass. The checks honor Git's
submodule settings, including settings that ignore submodule changes.

Open archived chats through Search or the sidebar’s **Archived** tab, which lists
archived families by workspace and also lets you unarchive or delete them.
