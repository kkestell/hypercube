# Introduction to Hypercube

Hypercube is a macOS app for working with teams of local coding agents. You
give an agent a task, follow its work in a chat, and steer it as it reads and
changes files, runs commands, and checks results. For work that benefits from
parallel effort, that agent can bring in other agents and coordinate their
contributions.

Hypercube runs existing coding tools—Claude Code, OpenCode, Codex, and Pi—and
adds a shared interface for their conversations, coordination, files, and
terminals. The agents run locally with your credentials and access to your
files. You can use the interface in the Hypercube window or in your web browser;
the agents' Browser tabs require the Hypercube window.

## Workspaces, sessions, and agents

A **workspace** is a project folder and the sessions associated with it. For a
Git project, the workspace folder is the top level of the checkout. A folder
outside Git can also be a workspace. The sidebar groups sessions by workspace,
so you can move between projects without mixing their conversations.

A **session** holds one agent and its conversation. It has its own chat history,
working directory, and title. You can keep separate sessions for different
tasks in the same project and return to their conversations later.

An **agent** is the coding assistant doing the work in a session. Its
**harness** is the tool running it, such as Claude Code or Codex. A **profile**
selects the harness, model, and reasoning effort. When starting a session, you
choose a profile beneath the message box before sending the first prompt.

Sessions you create with the workspace's **+** button are **top-level
sessions**. Each works in the workspace's original folder. Separate chats do
not give these sessions separate copies of the project: they share files,
the Git index (staged changes), and the checked-out branch. Changes made by
one are visible to the others.

## Parents and children

A top-level session can create **child sessions** to handle parts of a task.
It becomes their **parent**: it assigns work, exchanges messages, and brings
the results together. A parent and its children form a **family**. Children
appear beneath their parent in the sidebar, and each has its own agent,
profile, conversation, and working directory. Children cannot create further
child sessions.

For example, you might ask a parent to add a feature. It could ask one child
to implement a self-contained part and another to review the result. You can
follow the overall task in the parent's chat, or select a child to inspect its
work and speak to it directly.

### Worktrees keep file changes separate

In a Git workspace, a child normally gets a **worktree**: another checkout of
the same repository in a separate folder, on its own branch. This lets it
change files without changing the parent's working files. The new branch
starts from the parent's current commit; it does not include the parent's
uncommitted changes. The child commits its work, and the parent merges the
branch into the original checkout.

A child can instead work in an existing directory. This is useful for tasks
such as reading code or reviewing changes in the parent's checkout. Children
given the same directory share its files and Git state. Outside Git, children
must use existing directories because Hypercube cannot create worktrees there.

Worktrees separate working files; agents still have access to the local
filesystem. The parent coordinates which agent edits which files.

### Messaging connects separate conversations

Agents do not automatically see each other's chats. They use explicit
messages to share instructions, findings, and results. A parent can message
its children and other running top-level sessions, including those in other
workspaces. A child can message only its parent. Stopped or ended sessions
cannot receive agent messages.

Messages between agents appear in the transcript with their sender or
recipient. A message that starts a turn has an expandable preview; messages
received during work appear in full. The child sends its parent a message to
report results or ask for help.

## Finding your way around the UI

The interface follows the selected session. Switching sessions changes the
chat, file tree, and terminal to that session's work.

| Area | What it shows and does |
| --- | --- |
| Left sidebar | Workspaces and their sessions, with children nested under parents. Use **Add Workspace** to add a project and **+** beside a workspace to begin a session. Select a row to open its chat; double-click its title to rename it. |
| Central tab area | The session's chat alongside any open files. Drag tabs to arrange them in split panes, and drag dividers to resize them. |
| Chat and message box | Your conversation with the agent. Follow reasoning and tool calls live; after a successful turn, expand its “Worked for …” row to inspect the activity. Send instructions or follow-ups from the box below the transcript, and attach images with the paperclip or by dropping them onto the chat. |
| Right panel | **Files** shows a read-only tree of the session's directory, including a child's worktree. Click a file to preview it; double-click to keep its tab open. **Changes** is currently empty. |
| Bottom terminal panel | Your shell in the selected session's directory. Each session has its own shell, which keeps running when you hide the panel. |
| Browser tab | In the Hypercube window, a page the session's agent can navigate and interact with. The tab appears when the agent first loads a page. You can also navigate it yourself or open the page in your default browser. |

The layout buttons in the header at the top of the window show or hide the sidebar,
terminal panel, and right panel.

### Steering work and answering requests

Press **Enter** in the message box to send, or **Shift+Enter** for a new line.
You can send a follow-up while the agent is working. Press **Escape** in the
chat to interrupt its current turn; the conversation remains available.

A spinning indicator means the session is working, an amber dot means it is
waiting on you, and a blue dot marks an unseen reply. Notification cards at
the top right bring replies, questions, and approval requests from other
sessions to your attention. You can answer requests in their chat or on a
notification card. A child's requests also appear in the parent's chat, so
you can answer them while following the overall task. When Hypercube is not
the app you are using, macOS notifications bring the same requests and replies
to you, and clicking one opens its session.

## Pausing and finishing work

Stopping a top-level session stops its family and keeps the conversations so
you can resume later. Use its sidebar menu to start or stop it. Closing the
interface leaves agents running; inactive families eventually stop
automatically. See [Resume and cleanup](usage.md#resume-and-cleanup) for the
details.

Ending a child is permanent. The parent ends it when its work is done;
Hypercube checks that a managed worktree is clean and its commits are merged
before removing the worktree and branch. The ended child's saved chat remains
readable. Deleting a session removes its chat; deleting a parent also removes
its children.

For detailed controls and session behavior, continue with the
[usage guide](usage.md).
