# Hypercube

A macOS app for running Claude Code, OpenCode, Codex, and Pi sessions together.
A session can ask another agent to review its changes, delegate implementation
on a separate branch, or coordinate with an agent in another repository.
Agents run locally with your existing credentials.

## Setup

[Install Hypercube](docs/installation.md), then
[configure at least one agent profile](docs/configuration.md). Profiles are the
harness, model, and effort choices available to you and to agents creating
children.

## Documentation

- [Sessions, context, and working copies](docs/introduction.md) — what agents
  share, what they know about each other, and where their edits go.
- [Using Hypercube](docs/usage.md) — starting sessions, reviewing work, answering
  requests, and returning to a task later.
- [Configuration](docs/configuration.md) — profiles and local files needed by
  worktrees.
- [Harness differences](docs/harness-support.md) — follow-ups, permissions,
  model changes, and version requirements.
- [Installation](docs/installation.md) — beta downloads, checkout builds, the
  login service, upgrades, and recovery.

Developers: start with [AGENTS.md](AGENTS.md) for instructions, architecture,
and development and release references.
