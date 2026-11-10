# Glossary

Project terms used in code, comments, documentation, and plans. Rules for
choosing names are in [Naming](development.md#naming); relationships and
implementation details are in [architecture](architecture.md).

## Terms

- **Hypercube**: The local application that runs coding agents, coordinates
  their work, and displays their transcripts.
- **backend**: The `hypercube ui` process that owns session state, writes the
  database, serves the browser interface, and manages agents.
- **desktop app**: The Electron client that displays the browser interface
  and provides native menus, notifications, and desktop actions independently
  of the agents' lifetimes.
- **browser interface**: The web interface served by the backend and displayed
  inside the desktop app or in a development browser view.
- **harness**: A native coding-agent runtime, such as Claude Code, OpenCode,
  or Pi, that executes an agent's work and owns its native conversation.
- **plugin**: A `hypercube-harness-<name>` executable that translates between
  Hypercube's plugin protocol and one harness's native protocol.
- **model entry**: A harness, native model ID, ordered effort levels, and default
  effort saved in `config.json` from Settings for the model picker. Sessions
  save their own settings, not a reference to an entry.
- **workspace**: A project folder registered with Hypercube that groups
  sessions; in Git, its path is the top level of the registered checkout.
- **checkout**: A Git working copy with its own working files, index, and
  checked-out branch, which several sessions can share.
- **managed worktree**: A separate Git working copy and branch that Hypercube
  creates for a session inside the workspace's `.hypercube/` directory.
- **working directory**: The folder in which a session's agent runs, whether
  an existing directory or a managed worktree.
- **session**: A saved Hypercube object with one agent, settings, a working
  directory, and a transcript, which remains when its agent process exits.
- **agent**: The coding worker a session runs through its harness and plugin.
- **top-level session**: A session with no parent, belonging directly to a
  workspace.
- **child session**: A Hypercube session created by a top-level session for
  delegated work, with its own conversation and transcript.
- **parent**: The top-level session that assigns work to a child and receives
  its results.
- **family**: A top-level session and its children, the group affected by
  archiving or deleting that top-level session.
- **native subagent**: A worker created inside a harness, without a separate
  Hypercube session; its output and questions appear as the harness integration
  supports.
- **native conversation**: The harness-owned history identified by a saved
  reference that Hypercube uses to resume an agent across launches.
- **transcript**: Hypercube's ordered record of a session's prompts, agent
  output, tool activity, requests, and state changes, including entries added
  by Hypercube separately from the native conversation.
- **launch**: One attempt to start a session's plugin and agent, with its own
  ID and output position; restarting the agent creates a new launch while a
  backend restart can reattach to a surviving one.
- **holder**: A detached `hypercube-hold` process that owns a plugin's or panel
  shell's input and output so it can survive a backend restart.
- **turn**: One period of agent work on input, ending when the harness reports
  idle, is interrupted, or fails; it can include tool calls and user questions.
- **fresh context**: A child's starting context built from its assignment and
  local instructions, without inheriting the parent's conversation.
- **fork**: A copy of a parent's saved native conversation used to start a
  child, whose Hypercube transcript begins with its own assignment.
- **parked**: A non-ended session with no running agent, retaining its saved
  state so the agent can resume when next used, whether after an idle timeout,
  a lost launch, or a failed start.
- **archived**: A top-level session's inactive state after stopping its family
  and cleaning up managed worktrees while keeping readable chats; unarchiving
  restores the top-level session, leaving its children ended.
- **ended**: A session state that prevents work and incoming agent messages,
  permanent for a child but reversible for an archived top-level session.
- **command**: A request from Hypercube to a plugin with an ID, such as
  starting the agent, supplying input, or answering a question.
- **reply**: A plugin's success or failure response carrying a command's ID;
  an input reply confirms acceptance by the harness, not completion of a turn.
- **event**: A plugin report of native conversation changes, such as work
  state, transcript items, or usage, separate from command replies.
- **item**: One identified transcript entry that can be updated in place,
  including text, reasoning, a tool call, an error, or a user question.
- **attention item**: An item asking the user for an answer or approval,
  which the agent waits for.
- **offer**: A saved question asking the user to choose **New session**,
  **Do it here**, or **Dismiss** for a new task, whose answer reaches the parent
  as a message.
- **proposal**: A saved suggestion for work initiated by an agent, including
  the child's brief and settings, which starts only when the user chooses
  **Start** and does not block the parent's turn.
- **utility model**: The harness, model, and effort chosen in Settings for
  short background prompts, such as naming sessions. A utility prompt runs
  without tools and saves no native conversation.
- **panel terminal**: A session's or workspace's on-demand interactive shell,
  owned by a holder and separate from the agent's tools.
