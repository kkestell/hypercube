---
title: Manage sessions
weight: 11
---

# Manage sessions

A session is one agent's conversation and working directory. For a guided
first use, follow [Your first session](getting-started.md).

## Start a session

1. Choose **Add Workspace** for a new project folder, or **New Session** in an
   existing workspace.
2. Below the message box, choose a model and an effort level
   with the slider. Models come from **Settings › Agents › Models**.
3. In a Git workspace, choose where edits will go:
   - **Existing checkout** uses the project folder, including uncommitted
     changes.
   - **New worktree** creates a separate folder and branch from the current
     commit. It does **not** include uncommitted changes.
4. Send the first prompt to create the session.

New sessions initially use the workspace's newest session settings, or your
[configured default](configuration.md#session-defaults) if there are no sessions.
The initial location choice comes from Settings.

Unsent sessions remain as **New Session** rows once you type or attach
something. Their text, settings, and location choice survive switching
sessions. Text survives reloads too; image attachments do not. Use **Discard
Draft** in the row's context menu to remove a draft.

## Correct or interrupt work

- **Send a correction:** type another message while the agent works. It can
  use the correction during that turn. OpenCode waits for running tools to
  finish before reading it at the next step; other
  [harness timing](harness-support.md#messages-and-model-changes) varies.
- **Interrupt:** press **Escape** in chat, or choose **Stop** in an empty
  message box. The turn ends, but its output remains.
- **Attach an image:** use the paperclip, paste, or drop it onto chat. You can
  send images without text. See [attachment limits](interface.md#prompts-and-attachments).

An interruption is not an undo. Inspect changes before asking the agent to
continue or reverse work.

## Change the model or effort

Use the selector below the message box to choose a model and adjust its effort
with the slider. Models come from **Settings › Agents › Models**. A session
keeps its original harness, but its model and effort can change, including
during work. The selection
survives agent restarts. A child with different settings from its parent is
marked in its composer.

The change takes effect at different times
[depending on the harness](harness-support.md#messages-and-model-changes).

## Answer a question or approval

Requests stay above the message box while you scroll. Select an answer; for
multiple-choice questions with checkboxes, select your choices and choose
**Submit**. Use **Other response** when offered.

A child's request also appears in its parent's chat. You can answer there
without switching sessions; the parent agent does not receive that copy.
Afterward, the answer remains in the child's history.

Read approvals before allowing them. Some offer **Always allow in this
session** or **Always allow in all sessions**. Review or remove saved choices
in **Settings**. [Permission behavior](harness-support.md#permissions) varies
by harness.

![A question and command approval from other sessions, shown as notification cards](images/attention-light.png)

## Find and resume earlier work

Use **Command+K** on macOS or **Ctrl+K** on Linux to search titles and
conversation text across workspaces, including archived and ended sessions.
Select an active session and send a prompt to continue its conversation.

Use **Rename** in a session's context menu, or **F2**, to change its title.
Right-click a row to open its menu.

For a failed start, open the session or send a prompt to retry. To reload an
installed plugin, choose **Restart Agent** in the session's context menu.
This cuts short a turn in progress and resumes the saved conversation.
See [session lifecycle](lifecycle.md) if you need to know what restarts.

## Archive or delete completed work

1. Commit and merge any work you want to keep from managed worktrees.
2. Choose **Archive** in the session's menu to keep searchable, read-only
   conversations while removing the family from the active list.
3. Find it later through Search or the sidebar's **Archived** tab. Choose
   **Unarchive** to make it active again.

**Delete** removes the family's saved conversations. It is not a substitute
for Archive. Managed worktrees and branches are removed only after Git checks
for uncommitted files and unmerged commits; the original project directory is
not deleted. See the [cleanup reference](storage.md#session-cleanup) before
removing a team.
