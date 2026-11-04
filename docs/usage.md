# Using Hypercube

## Start a session

Choose **Add Workspace** to select a project folder, or **New Session** for a
workspace already in the sidebar. Below the message box, choose a
[profile](configuration.md) and decide whether to enable **Worktree**. Send the
first prompt to create the session.

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
| **Files** | Open a read-only file preview. Double-click to keep the tab open. Markdown has preview and source views. |
| **Changes** | Inspect uncommitted files and unified diffs, plus commits not merged into the original checkout's branch. |
| **Search** | Find case-insensitive plain text and open a matching line. |

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
session has its own shell. Hiding the panel or closing the interface leaves it
running; stopping the session or quitting Hypercube ends it.

In the native window, an agent's first page navigation opens its **Browser**
tab. You can navigate that page yourself or open it in your default browser.
Closing the tab keeps the page alive; the agent can close it, or quitting
Hypercube ends it. Session browsers share website logins.

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

Context use and cost appear below the message box. The bottom status bar shows
subscription usage reported by the harness: percent used and time until reset.
The meter's mark shows how much of the time window has elapsed; amber means
usage is ahead of that pace. A reading stays dimmed after reset until the
harness reports again. See [usage reports](harness-support.md#usage-reports)
for what each harness supplies.

## Answer a request

Questions and approvals stay above the message box while you scroll. A child's
request also appears in its parent's chat, where you can answer without
switching sessions. The parent's agent does not receive that copy. Once
answered, it disappears there and remains in the child's history.

Approvals may offer **Always allow in this session** or **Always allow in all
sessions**. These choices belong to the harness and survive restarts. Remove
saved choices in **Settings** (**Command+,**). Available approval choices vary
[by harness](harness-support.md#permissions).

For requests from other sessions, answer the notification card or use its eye
button to open the chat. Closing a request card hides it without answering.
Closing a reply card marks the reply seen; reply cards also close after
15 seconds or when the agent works again.

When the native window lacks focus, macOS notifications announce requests and
replies. Clicking one opens its session.

## Find a conversation

**Command+K** searches saved titles and conversation text across workspaces,
including stopped, ended, and archived sessions. **F2** or **Rename** in a
session's menu changes its title. Right-click a row for its menu; workspace
and session menus can also open their directories in VS Code, Finder, or
Terminal.

Children appear under their parent. A spinner indicates work, a question icon
means an answer is needed, a dot marks an unseen reply, and a stop icon marks
an unexpected exit. Stopped and ended sessions are dimmed.

## Layout and shortcuts

Drag tabs between panes, or onto a pane's edge to split it. Drag dividers to
resize panes and panels. Header buttons toggle the sidebar, terminal, and
right panel. Use **View → Zoom In**, **Zoom Out**, or **Actual Size** in the
native window to change page size. The theme follows macOS appearance.

**Help → Keyboard Shortcuts** (**Command+/**) lists all shortcuts, including
sidebar navigation. Text fields retain their editing keys. The terminal keeps
its own keys except **Command+P**, **Command+Shift+F**, and **Command+,**.

## Resume and cleanup

Closing the interface leaves agents running. To pause work, use **Stop** in the
top-level session's menu. It stops that session and its children—the family—
while keeping their conversations, queued messages, branches, and worktrees.
**Start** resumes the family, but work and queued-message delivery wait until
you send a prompt to a family member. Clicking a child of a stopped parent also
starts the family.

After an app restart, Hypercube reconnects to agents that survived. If every
agent in a running family was lost, or you quit Hypercube or rebooted, the
family starts awaiting your prompt. A harness that exits unexpectedly is
relaunched with its conversation; repeated quick failures stop relaunches.
For a failed start, the error stays in chat. Stop and start the family to retry.

Choose a cleanup action according to what you want to keep:

| Action | Conversation history | Managed worktrees and branches |
| --- | --- | --- |
| **Stop** | Kept; work can resume. | Kept. |
| **Archive** | Kept read-only and searchable; family leaves the sidebar. | Removed after Git checks. |
| **Delete** | Deleted with the family's chats. | Removed after Git checks. |
| **End child** | Kept read-only; that child cannot resume. | Removed after Git checks. |

The parent ends children with its `end_session` tool. Ended children can later
be deleted individually. Existing directories assigned to children are left
alone by cleanup.

Before removing a managed worktree, Git checks for uncommitted files and
unmerged commits. Child commits must be merged into the parent; a top-level
worktree's commits must be merged into the original checkout's HEAD. Deletion
refuses without changing anything until its checks pass. The checks honor Git's
submodule settings, including settings that ignore submodule changes.

Open archived chats through Search or **Settings**, which lists archived
families by workspace and also lets you delete them. An archived family appears
in the sidebar while selected, then leaves it when you choose another session.
