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

- Keep the codebase small enough to read once and understand. Prefer deleting
  code. No speculative handling, defensive layers for unevidenced failures,
  tests of things that cannot break, or abstractions with one caller. Choose
  the simplest complete fix for correctness and readability, even when it adds
  code. Judge the responsibilities a reader must understand, not line counts.
- Judge whole touched files. Hard-to-follow code blocks completion without
  needing a reproduction: split concerns, move code where it belongs, and
  delete unused code before continuing.
- Go uses plain data and direct functions, one named concern per package. No
  `util` or `common` packages, internal reach-through, or workarounds for import
  cycles. Prefer the standard library to dependencies.
- Define interfaces at their use, with one or two methods, only for two real
  implementations or a necessary test seam. Use useful zero values; avoid
  default-only constructors, getters, setters, and options for one or two settings.
- Give each goroutine one owner who starts it and knows when it ends. Locks
  guard named fields, stay brief, and never pass from caller to callee. Prefer
  channels or one owner over shared state. Pass context first for work that
  can block or be cancelled; never store it for later.
- Return errors instead of logging and ignoring them. Wrap once with context;
  compare with `errors.Is` or `errors.As`, never text. No normal-path panics.
  Use short names in short scopes and comments that explain why. Delete dead
  code and just-in-case flags; use no fixed sleeps or timing guesses.
- Go tests cover observable behavior through public entry points. Use tables
  for repeated cases, `t.Helper`, `t.TempDir`, and `t.Cleanup`; wait on conditions
  and pass under `-race`.
- Choose test responsibilities by consequence and maintenance cost. Existing
  assertions are not requirements: remove low-value tests and their machinery
  without automatically replacing their coverage. Prefer one suitable boundary
  over repeating behavior at every layer. Stand-ins must earn their cost through
  the behavior they protect. Assert observable results; specify styling, markup,
  or private state only when that detail is the behavior being protected.
- React components are small functions of props and the store. Derive what
  can be computed. State has one owner and changes through pure functions.
  No mutable module state outside the store except component-owned handles;
  no prop drilling for values the store provides.
- Effects serve real side effects, with correct dependencies and cleanup.
  Prefer platform APIs: native dialogs, AbortController, URL, fetch, and CSS
  container queries where they fit. No polyfills, manual DOM patching, or
  timers to wait for rendering. HTML holds markup only.
- Use stock shadcn components and theme styles, Lucide icons, and Tailwind
  utilities for layout. Keep component styling in the upstream shadcn source;
  do not add custom component CSS or theme overrides. Reserve red for errors;
  repeated status indicators use icons with accessible names and hover text.
- Provide semantic elements, labels, correct focus order and returned focus.
  Add accessibility handling where native semantics fall short. Each module
  owns one concern, named for its behavior; remove duplication and split files
  that are hard to follow. Browser tests check what users see and do.
- Keep generated files apart from their source, never edit them by hand, and
  have `make check` fail when they are stale.

## Edge cases

- Handle required behavior, normal use, and evidenced failures. Additional
  handling must justify its complexity. Validate external input; trust internal
  invariants and fail clearly when they break.

## Code

- `cmd/hypercube/` — the command line and the backend process.
- `desktop/` — the Electron desktop app, a client of the backend.
- `internal/` — one package per concern: `desktop`, `harness`, `hold`, `mcp`,
  `report`, `session`, `store`, `terminal`, `ui`, `workspace`.
- `plugins/` — the Claude Code, OpenCode, Codex, and Pi harness plugins. The
  core names no harness. `plugins/scripted` plays documentation screenshot
  scenes and never ships.
- `scripts/dev` — the isolated development session (`make dev`).
- `scripts/screenshots` — the documentation screenshots (`make screenshots`).
- `web/` — the Taikonaut website, a Hugo site built and served by `make web-build`
  and `make web-serve`.
- `docs/` — user documentation, indexed by `README.md`.
- `agents/` — developer references, research, and active task records.

## Validation

- `make check` runs every check. Run it after changing code.
- App packaging is separate from checks. When a missing or changed icon needs
  `sips` SVG conversion, request execution outside the Codex sandbox directly:
  that conversion fails inside it with error 13. Do not retry it there.
- Use focused `go test -run`, or `bunx tsc -p .` and `bun test` in
  `internal/ui`, while working.
- After changing only docs or comments, inspect the diff; run nothing.
- Tests use isolated state, harness stand-ins, and their own shells. Never stop
  the user's workspaces.
- Use `make dev` for a live smoke test, and clean up what you start.
- One `make dev` server serves the checkout and rebuilds on changes. Reuse it;
  start another only on a separate port and state folder when two checkouts
  must be compared. It serves the browser view without the desktop app; open
  its URL in the user's browser with `open`. Use `make dev-app` only to test
  desktop app behavior.
- Check a change to the browser view with tests and the build, without
  screenshots. The user judges how it looks.
- `make install-service` loads a merged change into the installed app. Run it
  from the original checkout. Offer to run it after finishing a change, and run
  it whenever the user asks.
- Agents and panel shells keep running across `make install-service` with the
  code they started with. A change to the harness plugins, `hypercube hold`,
  `hypercube mcp`, the plugin protocol, or how agents and shells launch reaches
  an active session only after that session restarts. When a requested change
  needs that, warn the user clearly before starting the work and again when
  offering to install it.

## Documentation

The example in `docs/configuration.md` is the user's
`~/.config/hypercube/config.json`. Change both together so they stay the same.

`README.md` indexes the user documentation in `docs/`. Update a guide only
when the behavior it describes changes. `AGENTS.md` gives instructions to
agents and should rarely change. Developer reference lives in
[architecture](agents/architecture.md) and
[development and releases](agents/development.md). Outstanding work belongs in
[GitHub Issues](https://github.com/kkestell/hypercube/issues).

Use the [hypercube skill](.agents/skills/hypercube/SKILL.md) for nontrivial
changes, including architecture changes, new concepts, subsystems, and major
features, even without an explicit invocation. Simple, contained changes may be
made directly; the skill defines the boundary. Keep it up to date as the
workflow evolves.

The code describes the implementation. A fact lives in one place. Correct or
delete inaccurate text without expanding it. Do not create documents unless
asked.

## Backwards compatibility

The user's state carries forward from each version to the next. A change to
the database schema or other saved state includes a migration that upgrades
the previous version's state when the app starts. Delete obsolete files when
you find them, including leftovers from earlier builds and installs.

## Communication

- Describe things directly, clearly, and plainly.
- Put the big idea first; add detail after.
- Never use jargon, invented terms, or shorthand.
