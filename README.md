# Hypercube

Local coding-agent teams for macOS, built with Go.

## Features

- **One interface for several harnesses.** Run Claude Code, OpenCode, Codex,
  and Pi agents side by side. Each session's profile picks its harness, model,
  and reasoning effort.
- **Agent teams.** A top-level session can create child sessions, give them
  work, exchange messages with them, and bring their results together. It can
  also message running sessions in other workspaces.
- **Worktree isolation.** In a Git project, each child works on its own branch
  in a separate worktree, seeded with the ignored files listed in
  `.worktreeinclude`. A new top-level session can run in its own worktree too,
  and its agent merges the branch when you click Merge. Hypercube removes a
  worktree and its branch once its commits are merged.
- **Live, steerable chats.** Follow reasoning, tool calls, and native subagents
  as they happen. Send follow-ups and images mid-turn, or interrupt with
  Escape. Finished turns collapse to a "Worked for …" row and the final
  response.
- **Questions and approvals.** Answer an agent's requests in its chat, in its
  parent's chat, on a notification card, or from a macOS notification. Always
  allow a tool for one session or for all of them.
- **Files, terminal, and browser per session.** Browse and preview the
  session's files, run a shell in its directory, and watch the agent drive its
  own browser in the Hypercube window.
- **Flexible layout.** Drag tabs into split panes and resize the sidebar,
  panes, and terminal.
- **Resume.** Stopping a family keeps its conversations for later, and running
  families start again when Hypercube relaunches.
- **Local.** Agents run on your machine with your own credentials, and the
  interface works in the Hypercube window or any web browser.

## Documentation

- [Introduction](docs/introduction.md): what Hypercube does, its main concepts,
  and a tour of the interface.
- [Installation](docs/installation.md): prerequisites, building, upgrades,
  and development sessions.
- [Usage](docs/usage.md): workspaces, sessions, commands, the browser view,
  resume, and cleanup.
- [Configuration](docs/configuration.md): profiles and worktree setup.
- [Architecture](docs/architecture.md): ownership, persistence, and design
  rationale, with links to the implementation.
- [Harness support](docs/harness-support.md): native integration, permissions,
  and known harness limitations.
- [Contributor instructions](AGENTS.md): scope, implementation constraints,
  and required validation.

