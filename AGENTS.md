Keep this document accurate and short.

## Purpose and scope

Hypercube runs teams of local coding agents in tmux. Each agent is a native
harness (Claude Code or OpenCode) in its own pane; agents message each other
through an MCP server and receive messages through harness plugins.

- There is one user. Target macOS.
- Agents are trusted local processes running with the user's credentials.
- Configure harnesses only through per-launch arguments, environment, and
  overrides. Leave the user's global and project settings, credentials, and
  `HOME` alone.

## Code style

- Do less. Keep correct code simple, ordinary, and cheap to change.
- Write idiomatic Go. Prefer plain data and direct functions.
- Give each concern a clear boundary and each lifecycle one owner.
- Let real needs earn abstractions, dependencies, and configuration.
- Return clear errors for bad input. Test observable behavior; write comments
  that explain why.

## Edge cases

- Handle required behavior, normal use, and evidenced failures. Additional
  handling must justify its complexity. Validate external input; trust internal
  invariants and fail clearly when they break.
- Tests and reviews should address observable behavior and meaningful failures.

## Code

- `cmd/hypercube/` — the command line, workspace and session lifecycle, and
  relaunches.
- `internal/` — one package per concern: `harness`, `mcp`, `report`, `store`,
  `tmux`, `wait`, `workspace`.
- `plugins/` — the Claude Code and OpenCode harness plugins. The core names no
  harness.
- `scripts/dev` — the isolated development session (`make dev`).
- `scripts/reinstall` — stops, reinstalls, and restarts the installed
  workspaces.
- `web/` — the Hypercube web page.
- `docs/` — user documentation, indexed by `README.md`.
- `agents/` — plans, reviews, and work logs.

## Validation

- `make check` runs every check. Run it after changing code.
- Use focused `go test -run` while working.
- After changing only docs or comments, inspect the diff; run nothing.
- Tests use isolated state, their own tmux server, and harness stand-ins.
  Never stop the user's tmux server or workspaces, and never run
  `make reinstall`.
- Use `make dev` for a live smoke test, and clean up what you start.

Report any check that fails or is skipped.

## Documentation

`README.md` indexes the user documentation in `docs/`. Update a guide only
when the behavior it describes changes. `AGENTS.md` gives instructions to
agents and should rarely change.

The code describes the implementation. A fact lives in one place. Correct or
delete inaccurate text without expanding it. Do not create documents unless
asked.

## Backwards compatibility

Currently, there is none. Change the state format freely instead of adding
migrations or versions.

## Communication

- Describe things directly, clearly, and plainly.
- Put the big idea first; add detail after.
- Never use jargon, invented terms, or shorthand.
