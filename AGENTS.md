Keep this document accurate and short.

## Purpose and scope

Hypercube runs teams of local coding agents. Each agent is a native harness
(Claude Code, OpenCode, or Pi) that the app drives through an
executable plugin; agents message each other through an MCP server, and the
browser shows each agent's transcript.

- There is one user. Target macOS and Linux.
- Agents are trusted local processes running with the user's credentials.
- Configure harnesses only through per-launch arguments, environment, and
  overrides. Leave the user's global and project settings, credentials, and
  `HOME` alone.

## References

For code changes, plans, reviews, and validation, read
[development and releases](docs/dev/development.md) for the code layout, code
style, workflow, compatibility, and release rules. Before changing guides or
screenshots, read
[documentation](docs/dev/documentation.md). The system design is in
[architecture](docs/dev/architecture.md), and [README.md](README.md) indexes user
guides. Use the [glossary](docs/dev/glossary.md) for project terms.

Commit completed, validated work unless the user asks otherwise. Include only
changes belonging to the task. Merge only when the user asks.

## Communication

- Describe things directly, clearly, and plainly.
- Put the big idea first; add detail after.
- Never use jargon, invented terms, or shorthand.
