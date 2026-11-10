# Review files and changes

The right panel follows the selected session's working directory, or the
workspace's main checkout when its dashboard is selected. Select the view
whose output you want to inspect before opening its files.

## Inspect the agent's actions

Expand **Worked for …** in a completed turn to see reasoning, tool calls, and
messages between sessions. Short reasoning shows in full. Longer reasoning
shows its last three lines under **Thinking…**, or **Thought** once it finishes;
expand it to read the whole trace, even while it streams. Expand a tool call for its input and output.
Failed and interrupted turns leave activity visible.

While a turn runs, a **Working…** row ends the transcript when nothing new has
arrived for a few seconds, so a pause in output does not look like a stopped
agent. A running tool shows its own status instead, and the row stays hidden
while a question or approval waits on you.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/details-dark.png">
  <img alt="An expanded turn with reasoning, tool calls, and native subagent activity" src="images/details-light.png">
</picture>

## Read a file beside the conversation

1. Open **Files** in the right panel.
2. Select a file to preview it; double-click to keep its tab open.
3. For Markdown, HTML, or SVG, switch between rendered preview and source
   when available.

Files are read-only in this view. Right-click a tab or tree row to copy its
absolute or relative path. An agent can also open a file and highlight lines
for you; those tabs appear when you view its session, even for files outside
its working directory.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/file-preview-dark.png">
  <img alt="A source file open in a separate pane beside the conversation, with the Files tree on the right" src="images/file-preview-light.png">
</picture>

When the conversation is the only tab, the first file opens in a pane beside
it if the view is at least 800 pixels wide, and in a tab on top of it
otherwise. Later files join the pane that already holds files. The Files tree
remains available on the right for opening another file.

## Inspect a diff

1. Open **Git** in the right panel.
2. Choose the comparison:
   - **Uncommitted** shows staged, unstaged, and untracked changes against
     the current commit.
   - **Committed**, available for top-level managed worktree sessions, shows the
     branch's committed changes since the common ancestor of the session's
     branch and the original checkout's current branch. Above the file list,
     it names that branch and counts the commits ahead of it.
3. Select a changed file or its right-aligned added and removed line counts
   to read its unified diff.

To inspect individual commits or other comparisons, use Git or ask the agent.

Git lists changes within the current view's directory, not the whole
containing repository. Paths are relative to that directory. Status
colors also remain visible in **Files**; line counts appear only in **Git**
and are omitted when unavailable, such as for binary files.

For a top-level worktree session, hover over its branch icon for merge status
and the number of commits not merged into the original checkout's branch.
This status refreshes when a session in that workspace finishes a turn.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/overview-dark.png">
  <img alt="A build.go diff beside the agent's conversation, with added and removed lines highlighted" src="images/overview-light.png">
</picture>

Added lines are green and removed lines are red. In this example, the diff
and conversation occupy separate panes.

The bar above chat shows the working directory and current Git branch.
Click the branch name to open the Git panel. For top-level worktree sessions
with unmerged commits, **Merge into …** at the bottom of the Git panel asks
the agent to integrate the work; it does not run a merge automatically.

Use **Command+F** on macOS or **Ctrl+F** on Linux to find text in an open
file or diff. Move between diff changes with **Option+F5** /
**Shift+Option+F5** on macOS, or **F7** / **Shift+F7** on Linux.

## Find a file or a line of text

| Goal | macOS | Linux |
| --- | --- | --- |
| Find a file by name or path | Command+P | Ctrl+P |
| Search project file contents | Command+Shift+F | Ctrl+Shift+F |
| Find in the open file or diff | Command+F | Ctrl+F |

Project search is case-insensitive plain text. Select a result to open its
line. Git-ignored files and version-control folders are excluded; see
[viewer and search limits](interface.md#files-and-search-limits) if an expected
file or result is missing.

## Arrange the view

The conversation, or a workspace's dashboard, is a tab like any file, except
that it never closes. Drag tabs, that one included, between panes, or onto a
pane edge to split the view. Drag dividers to resize panes and panels. Toolbar buttons toggle the sidebar, terminal, and
right panel. Each session remembers its tabs, splits, and scroll positions.

Use **View → Zoom In**, **Zoom Out**, or **Actual Size** in the desktop app
to change the page size. Use **Settings › General › Appearance** to change
theme or terminal text size.
[Keyboard Shortcuts](interface.md#keyboard-shortcuts) lists navigation and
panel commands.

## Use a shell beside the conversation

Open the terminal panel for a shell in the selected session's directory.
Each session has its own shell. Hiding the panel or quitting the desktop app
leaves it running. Archiving, ending, or deleting its session ends it.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/terminal-dark.png">
  <img alt="The terminal panel below a session's conversation, showing git status --short in the session's checkout" src="images/terminal-light.png">
</picture>

Commands run in the same working files the session uses. A local session's
shell and agent therefore share the original checkout.
