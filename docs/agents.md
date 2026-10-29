# Agents, sessions and where they work

How workspaces, sessions and agents map onto repos, directories and processes. This is the v1 design for bringing agent processes back; nothing here is implemented on `bun` yet. Process management as built before lives on branch `process-management`.

Background research: `agents/research/acp/` (the protocol and which harnesses support it) and `agents/research/harnesses/` (each harness's own integration points).

## Concepts

**Workspace.** A GitHub repo (`owner/name` or URL). It is the project; it has no local directory of its own.

**Session.** A team working on the workspace in its own clone of the repo. Exactly one lead, plus any number of workers. A session can be about code, or just a place to ask questions, read logs or poke at infrastructure with the project's context; it doesn't have to produce changes.

**Environment.** Where a session's clone lives and its agents run, chosen when the session is created:

- **Local:** the repo is cloned to `~/.local/share/hypercube/sessions/<session-id>/` and agents run on the host.
- **Docker:** the repo is cloned inside a container created for the session, and agents run in the container.

Everything in a session runs in its environment. An environment can start a process with piped stdin and stdout (command, directory, environment variables), kill it with everything it owns, run one-off commands, and be set up and torn down.

**Agent.** An ACP agent process running in a directory inside its session's clone, with Hypercube as its ACP client. Its role is lead or worker.

**Profile.** A named combination of harness, model and effort, with a description of when to use it, as in the Rust Hypercube's `profiles` (`~/projects/hypercube/docs/configuration.md`). When the lead starts a worker it picks a profile, and the worker runs with exactly those settings.

## Harnesses: ACP

Hypercube standardizes on the [Agent Client Protocol](https://agentclientprotocol.com) (ACP). Any harness that can act as an ACP agent, natively or through an adapter, works with Hypercube. Hypercube is the ACP client.

- **One process per agent.** Hypercube starts each agent's ACP process in its environment and talks JSON-RPC over its stdio (`docker exec -i` for Docker). It creates one ACP session in it. One process per agent keeps agents' MCP identities apart; OpenCode, for one, shares injected MCP servers across sessions in the same process.
- **Hypercube renders the agent.** There is no terminal. The UI shows the conversation from ACP updates: messages, thoughts, tool calls, plans, diffs, permission requests and usage. The user talks to an agent by sending it prompts.
- **What's per harness** is small: the command that starts its ACP agent, and how a profile's model and effort map onto the agent's session config options (ACP leaves their names and values to each agent).
- **Status** comes from Hypercube's own requests: busy while a prompt is in flight, waiting while a permission request is open, idle otherwise.
- **Resuming.** Hypercube records each agent's ACP session id. After the agent's process stops (session stopped, Hypercube restarted), Hypercube starts a new process and resumes or loads that session. A turn that was running when the process stopped is lost.
- **Transcripts.** Hypercube stores what it renders in its database, so history shows even for agents that can resume a session but not replay it.

**Initial build: OpenCode** (`opencode acp`, native ACP) with the user's ChatGPT subscription, already logged in locally. OpenCode supports resume and load, MCP servers over stdio and HTTP with headers, and model and effort as session options.

**Claude Code: later.** It has no native ACP mode, and its ACP adapter is built on the Claude Agent SDK, which can't use a Claude subscription. How Claude Code fits is undecided.

## Working in the clone

Hypercube creates the session's clone and deletes it with the session. What happens inside it is the lead's business:

- The lead works in the clone. It may create branches and git worktrees inside the session's directory.
- When the lead starts a worker, it picks the worker's profile and the directory it runs in: the clone itself or a worktree the lead made.
- Hypercube never merges, rebases or pushes anything.

Your own local checkout is not involved. A session's work reaches you through GitHub: fetch its branch or look at its pull request.

## Landing work

The lead lands work by opening a pull request and merging it into `main`. The pull request is the gate (CI, and review if the repo requires it) and GitHub is the conflict resolution mechanism: if another lead merged first and the branches conflict, the lead resolves the conflicts before it can merge.

## Agent operations

Agents talk to Hypercube through Hypercube's MCP server, which Hypercube passes to every agent in `mcpServers` when it creates the agent's ACP session. Each agent connects with its own identity (e.g. a per-agent token in an HTTP header), so Hypercube knows who is asking. v1 operations:

- **Start and stop workers.** Lead only. Starting takes a profile and a directory in the session's clone.
- **Send messages**, within the scopes below.
- **List agents and profiles:** teammates and their status, and the profiles a lead can choose from.

## Messaging

- Within a session, the lead and its workers message each other.
- Within a workspace, leads message each other, for example to avoid overlapping work.
- Workers don't message outside their session; anything cross-session goes through their lead.

A message reaches an agent as an ACP prompt that says who sent it. ACP allows one prompt at a time per session, so Hypercube queues messages, from agents and from the user, and sends the next when the agent's turn ends.

## State

Hypercube keeps its model (workspaces, sessions, agents with their ACP session ids, each session's environment and clone location, and transcripts) in `~/.local/share/hypercube/hypercube.db`, a SQLite database. Clones and containers outlive the server process, so the model must too.

## Deleting a session

Deleting a session stops its agents and removes its clone (and container, for Docker). If the clone has uncommitted changes or commits not pushed to GitHub, Hypercube shows them and asks before deleting.

## Docker

Hypercube maintains a base image that contains the harnesses. A workspace can name its own image, built `FROM` the base image, to add the project's toolchain; otherwise sessions use the base image. At container start, Hypercube injects a GitHub token (e.g. from `gh auth token`) and the harnesses' credentials as environment variables. Nothing from the host's home directory is mounted.

## Decisions

| Question | Decision |
|---|---|
| What a workspace is | A GitHub repo |
| Where a session works | Its own clone, local or in a Docker container, chosen at creation |
| Local clone location | `~/.local/share/hypercube/sessions/<session-id>/` |
| Interface to harnesses | ACP; Hypercube is the client, one ACP process per agent |
| How agents are shown | Hypercube renders the conversation from ACP updates; no terminals |
| First harness | OpenCode with the user's ChatGPT subscription |
| Claude Code | Later; its ACP adapter can't use a Claude subscription |
| Worktrees and branches | Up to the lead, inside the session's clone |
| What a worker runs | The lead picks a profile and a directory in the clone |
| How work lands | The lead opens a PR and merges it into `main`, resolving conflicts first |
| Who merges | The lead; Hypercube never touches git history |
| How agents ask Hypercube for things | Hypercube's MCP server, passed in `mcpServers`, one identity per agent |
| Agent operations in v1 | Start/stop workers (lead only), send messages, list agents and profiles |
| Message delivery | ACP prompts, queued while the agent's turn runs |
| Persistence | SQLite at `~/.local/share/hypercube/hypercube.db`, including ACP session ids and transcripts |
| Deleting a session with unpushed or uncommitted work | Ask |
| Large repos | A full clone per session for now; a per-workspace mirror later if needed |
| Non-GitHub or non-git workspaces | Not in v1 |
| Docker image | A Hypercube base image with the harnesses; a workspace can supply its own image built `FROM` it to add the project's toolchain |
| Docker credentials | GitHub token and harness credentials injected as environment variables at container start; nothing from the home directory is mounted |
