# What keeps running when you leave

Closing Hypercube's window is not a stop command for agents. Work continues
in the background, and conversations remain available when you return.

## The window and the work are separate

Hypercube's background service runs agents, delivers their messages, and saves
conversations. The desktop app shows that work and lets you interact with it.

| Action | Effect on agents and terminal shells |
| --- | --- |
| Close the macOS window | Keep running; the desktop app stays in the menu bar and Dock. |
| Close the Linux window | Keep running; the desktop app quits. |
| Quit the desktop app | Keep running in the background. |
| Stop a turn in chat | Interrupt that turn, not the rest of the team. |
| Restart Agent | Interrupt the turn and resume the saved conversation in a new agent process. |

This separation lets an agent continue a task while the interface is closed,
and lets normal app upgrades happen without ending that task.
It does not mean every process survives a machine shutdown.

## A saved session is not always a running process

Hypercube starts a session's agent when you open it, prompt it, or a message
reaches it, including a child's report. After 15 minutes idle while nobody
views that session, the agent ends. Its conversation, queued messages, branch,
and worktree remain. This is a **parked** session, shown dimmed in the sidebar,
not an ended session. Each session follows this lifecycle independently.

When used again, the agent resumes its saved native conversation. If a
harness cannot resume that history, Hypercube reports the failure rather than
silently opening a new conversation.

After a background-service restart, surviving agents reconnect. Other sessions
start when next used. A harness that exits unexpectedly is relaunched with its
saved conversation, but repeated quick failures stop automatic relaunches. A
failed start stays visible in chat; opening the session or sending a prompt
retries it.

## Restarting is different from finishing

**Restart Agent** replaces a session's agent process and loads its installed
plugin, keeping the saved conversation. It interrupts any current turn.

**Archive** finishes a family's active use while preserving readable chats.
**End child** permanently ends that child. These actions can also remove
managed worktrees, so commit and merge work first. **Unarchive** makes an
archived session usable again. If it had a managed worktree, that worktree is
recreated; existing project folders and assigned directories are kept.

See [cleanup behavior](storage.md#session-cleanup) for what each action keeps.
