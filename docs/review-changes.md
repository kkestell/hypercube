---
title: Review files and changes
weight: 13
---

# Review files and changes

The right panel follows the selected session's working directory. Select the
session whose output you want to inspect before opening its files.

## Inspect the agent's actions

Expand **Worked for …** in a completed turn to see reasoning, tool calls, and
messages between sessions. Expand a tool call for its input and output.
Failed and interrupted turns leave activity visible.

![An expanded turn with reasoning, tool calls, and native subagent activity](images/details-light.png)

## Read a file beside the conversation

1. Open **Files** in the right panel.
2. Select a file to preview it; double-click to keep its tab open.
3. For Markdown, HTML, or SVG, switch between rendered preview and source
   when available.

Files are read-only in this view. Right-click a tab or tree row to copy its
absolute or relative path. An agent can also open a file and highlight lines
for you; those tabs appear when you view its session, even for files outside
its working directory.

![A source file open in a separate pane beside the conversation, with the Files tree on the right](images/file-preview-light.png)

The file tab stays beside the conversation; the Files tree remains available
on the right for opening another file.

## Inspect a diff

1. Open **Changes**.
2. Select a staged, unstaged, or untracked file to read its unified diff.
3. To review committed changes, use Git or ask the agent to inspect the
   relevant commits and their diffs.

Changes covers the whole Git repository containing the session directory.
For a top-level worktree session, hover over its branch icon for merge status
and the number of commits not merged into the original checkout's branch.
This status refreshes when a session in that workspace finishes a turn.

![A build.go diff beside the agent's conversation, with added and removed lines highlighted](images/overview-light.png)

Added lines are green and removed lines are red. In this example, the diff
and conversation occupy separate panes while the file tree shows other edits.

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
line. Ignored and hidden paths are excluded in Git projects; see
[viewer and search limits](interface.md#files-and-search-limits) if an expected
file or result is missing.

## Arrange the view

Drag tabs between panes, or onto a pane edge to split the view. Drag dividers
to resize panes and panels. Footer buttons toggle the sidebar, terminal, and
right panel. Each session remembers its tabs, splits, and scroll positions.

Use **View → Zoom In**, **Zoom Out**, or **Actual Size** in the desktop app
to change the page size. Open **Settings** to change theme or terminal text
size. [Keyboard Shortcuts](interface.md#keyboard-shortcuts) lists navigation
and panel commands.

## Use a shell beside the conversation

Open the terminal panel for a shell in the selected session's directory.
Each session has its own shell. Hiding the panel or quitting the desktop app
leaves it running. Archiving, ending, or deleting its session ends it.

Commands run in the same working files the session uses. A local session's
shell and agent therefore share the original checkout.
