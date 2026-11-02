Keep this document accurate and short.

## Purpose and scope

Hypercube runs teams of local coding agents. Each agent is a native harness
(Claude Code, OpenCode, Codex, or Pi) that the app drives through an
executable plugin; agents message each other through an MCP server, and the
browser shows each agent's transcript.

- There is one user. Target macOS.
- Agents are trusted local processes running with the user's credentials.
- Configure harnesses only through per-launch arguments, environment, and
  overrides. Leave the user's global and project settings, credentials, and
  `HOME` alone.

## Code style

- Do less. Keep correct code simple, ordinary, and cheap to change. Measure
  simplicity by what a reader must understand, not by the size of a diff.
- Leave the code a change touches easy to follow. When a change would make a
  file, module, or function harder to follow, restructure it as part of the
  change: split it by concern, move code to where it belongs, delete what is
  unused.
- Write idiomatic Go. Prefer plain data and direct functions.
- Give each concern a clear boundary and each lifecycle one owner. This holds
  for browser code too: one ES module per concern, and HTML files hold markup
  only.
- Keep generated files apart from their source, never edit them by hand, and
  have `make check` fail when they are stale.
- Let real needs earn abstractions, dependencies, and configuration.
- Return clear errors for bad input. Test observable behavior; write comments
  that explain why.

## Edge cases

- Handle required behavior, normal use, and evidenced failures. Additional
  handling must justify its complexity. Validate external input; trust internal
  invariants and fail clearly when they break.
- Tests and reviews should address observable behavior and meaningful failures.

## Code

- `cmd/hypercube/` — the command line and the app process.
- `internal/` — one package per concern: `harness`, `mcp`, `menu`, `report`,
  `session`, `store`, `terminal`, `ui`, `workspace`.
- `plugins/` — the Claude Code, OpenCode, Codex, and Pi harness plugins. The
  core names no harness.
- `scripts/dev` — the isolated development session (`make dev`).
- `docs/` — user documentation, indexed by `README.md`.
- `agents/` — plans, reviews, and work logs.

## Validation

- `make check` runs every check. Run it after changing code.
- Use focused `go test -run`, or `bun test` in `internal/ui`, while working.
- After changing only docs or comments, inspect the diff; run nothing.
- Tests use isolated state, harness stand-ins, and their own shells. Never stop
  the user's workspaces, and never run `make install-service`.
- Use `make dev` for a live smoke test, and clean up what you start.
- The user judges how the browser view looks. Leave rendering, screenshots,
  and visual checks of it to them.
- After a change is merged, the user runs `make install-service` from the
  original checkout to load it.

## Documentation

The example in `docs/configuration.md` is the user's
`~/.config/hypercube/config.json`. Change both together so they stay the same.

`README.md` indexes the user documentation in `docs/`. Update a guide only
when the behavior it describes changes. `AGENTS.md` gives instructions to
agents and should rarely change.

Leads must use the [hypercube skill](.agents/skills/hypercube/SKILL.md) for new
features and behavior changes. Keep that skill up to date as the workflow evolves.

The code describes the implementation. A fact lives in one place. Correct or
delete inaccurate text without expanding it. Do not create documents unless
asked.

## Backwards compatibility

Currently, there is none. Change the state format freely instead of adding
migrations or versions. Delete obsolete files when you find them, including
leftovers from earlier builds and installs.

## Communication

- Describe things directly, clearly, and plainly.
- Put the big idea first; add detail after.
- Never use jargon, invented terms, or shorthand.
