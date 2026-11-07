# Development and releases

Start with [AGENTS.md](../AGENTS.md) for project scope and document routing.
Source builds need Git, the Go version in [go.mod](../go.mod), and Bun.
Packaging downloads Electron on first use. User installation, service management,
and recovery are in [the installation guide](../docs/installation.md).
Documentation procedures are in [documentation](documentation.md).

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
- `agents/` — developer references, research, plans, work logs, and reviews.

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

## Development workflow

Write a plan for changes to architecture, system boundaries, state or lifecycle
ownership, new subsystems or major features, and work spanning concerns that
need coordinated design. Localized fixes, copy, styling, documentation, and
small refactors may be done directly. Reassess if a contained change grows.

Before planning, inspect related code for existing behavior, duplicate paths,
and the right owner for the change. Revise a flawed boundary when needed, and
delete obsolete paths so the result is one coherent design.

The parent session owns a task branch and gives all three agents the same
checkout. Use a new agent for each step, in order, so the next agent sees the
staged files from the previous one. Give each agent the user's requirements
and the relevant artifact path. The parent assesses each result and keeps the
task within the user's scope.

1. Have a fresh agent run the local [kplan](../.claude/skills/kplan/SKILL.md)
   skill. It writes and stages a plan in `agents/plans/`. The parent checks the
   plan. For a new subsystem or architectural change, the parent independently
   reviews it against the whole system and a simpler baseline, records any
   findings in `agents/reviews/`, and resolves justified concerns before work.
2. Have a different fresh agent run [kwork](../.claude/skills/kwork/SKILL.md)
   with the plan path. It implements the plan, validates the change, writes a
   work log in `agents/work/`, and stages the related files.
3. Have another fresh agent run [kreview](../.claude/skills/kreview/SKILL.md)
   on the staged changes and plan. It reviews the whole touched files, fixes
   confirmed medium and high severity issues itself, validates the fixes,
   writes a review in `agents/reviews/`, and stages it with the fixes. The
   parent checks the review and has the reviewer resolve any medium or high
   finding still open before committing.

After the required checks and review are complete, the parent makes the final
commit and merges the task branch into `main` when the user has authorized
delivery. Do not delegate the commit or merge to the three agents.

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

## Backwards compatibility

The user's state carries forward from each version to the next. A change to
the database schema or other saved state includes a migration that upgrades
the previous version's state when the app starts. Delete obsolete files when
you find them, including leftovers from earlier builds and installs.

## Develop without replacing the installed app

```sh
make dev
```

Open http://localhost:4281/. This uses separate configuration and state, with a
scratch repository of sample files under `~/projects/hypercube-dev/repo`. Use
`make dev-app` when testing desktop app behavior; it also runs a development
desktop app connected to the development backend.

Source changes rebuild the development app and reload the interface. A failed
build leaves the previous build running. Successful rebuilds preserve agents
and terminal shells.

- **Ctrl-C** or `make dev-stop` ends the development backend, desktop app, and
  their processes.
- `make dev-clean` also removes disposable resources and native development
  conversations, including holders left by a crash.
- `HYPERCUBE_DEV_DIR` and `HYPERCUBE_DEV_PORT` change the directory and port.

See [scripts/dev](../scripts/dev) for cleanup details.

For a build alone, run `make` and use `bin/hypercube` or `bin/Hypercube.app`.
`make install-service` builds the checkout, including uncommitted changes,
without requiring a release tag or running the full checks.

## Prepare an official beta

GitHub Actions runs `make check` on main-branch pushes and pull requests, using
the Go and Bun versions declared in the repository. Beta tags run those same
checks before packaging an ad hoc signed Apple Silicon app.

Once the intended changes are committed and checked, create and push an
annotated tag such as `v0.1.0-beta.1`. The **Beta release** workflow builds the
ZIP and `SHA256SUMS`, then creates a **draft prerelease** with generated notes.
Inspect the notes, test the downloaded app, and publish the draft when ready.
No Apple account or signing secrets are required. GitHub Actions must be enabled
and permitted to create releases in the repository.

To check and package locally on Apple Silicon:

```sh
make check
make release
```

Only release packaging requires a clean checkout at a beta tag. Its outputs
are under `bin/release/`; it does not install the app or restart the service.

Go embeds the release tag or commit-based version, adding `+dirty` for
uncommitted changes. Builds without Git metadata report `devel`. The bundle's
`CFBundleGetInfoString` records the full identity; its numeric version fields
contain the version's three-number core. See [build identification](../docs/installation.md#identify-a-build)
for the user-visible checks.
