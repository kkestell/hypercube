# Development and releases

Start with [AGENTS.md](../../AGENTS.md) for project scope and document routing.
For source setup, local builds, and installing a checkout, use
[Contributing](../../CONTRIBUTING.md). This reference covers project conventions,
workflow, and releases. App users should start with the
[user documentation](../../README.md#start-here).
Documentation procedures are in [documentation](documentation.md), and running
and writing tests in [testing](testing.md).

## Code

- `cmd/hypercube/` — the command line and the backend process.
- `cmd/hypercube-mcp/` — the MCP bridge each agent's harness runs.
- `cmd/hypercube-hold/` — the holder that runs agents' plugins and panel shells.
- `desktop/` — the Electron desktop app, a client of the backend.
- `internal/` — one package per concern: `buildinfo`, `config`, `desktop`,
  `github`, `harness`, `harnessclient`, `hold`, `mcp`, `mcpbridge`, `report`,
  `session`, `store`, `terminal`, `ui`, `workspace`.
- `plugins/` — the Claude Code, OpenCode, and Pi harness plugins. The
  core names no harness. `plugins/scripted` plays scripted sessions for
  the Linux tests and never ships; `plugins/internal/plugin` holds shared plugin code.
- `make/` — the macOS and Linux packaging rules the `Makefile` includes.
- `scripts/dev` — the isolated development session (`make dev`).
- `scripts/screenshots` — the documentation screenshots (`make screenshots`).
- `scripts/linux-install`, `scripts/linux-install-service`,
  `scripts/linux-uninstall` — the Linux installer scripts, shipped in the
  bundle's `resources/` as `install`, `install-service`, and `uninstall` and run
  by `make install`, `make install-service`, and `make uninstall`.
- `scripts/linux/` — the Linux test container (`scripts/linux/run`).
- `docs/*.md` — user tutorials, how-to guides, reference, and explanations, indexed
  by `README.md`.
- `docs/dev/` — developer references and the architecture diagram.
- `.agents/` — research, plans, design records, reviews, work logs, and scratch work.

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
- React components are small functions of props and the store. Derive what
  can be computed. State has one owner and changes through pure functions.
  No mutable module state outside the store except component-owned handles;
  no prop drilling for values the store provides.
- Effects serve real side effects, with correct dependencies and cleanup.
  Prefer platform APIs: native dialogs, AbortController, URL, fetch, and CSS
  container queries where they fit. No polyfills, manual DOM patching, or
  timers to wait for rendering. HTML holds markup only.
- Use stock shadcn components and theme styles, Lucide icons, and Tailwind
  utilities on the Tailwind scale. Component styling lives in the shadcn
  source in `internal/ui/web/components/ui/`; class names passed to a shadcn
  component set only its position, spacing, size, and layout. Build on the
  shared pieces in `internal/ui/web/shared/`: `IconButton` for icon-only
  buttons, `Message` for empty, loading, and error lines, `useRequest` and
  `errorText` for requests a component makes, and `tones.ts` for status
  colors. Reserve red for errors and removed lines in diffs; repeated status
  indicators use icons with accessible names and hover text. Write interface
  text as plain phrases and sentences, with facts joined by words or given
  their own lines rather than separator characters.
- Animate every change a person can see happen. Controls that appear on hover
  or focus fade with Tailwind's default transition on opacity and visibility
  and show while their row has keyboard focus; a control in a group with
  nothing else to focus fades on opacity alone, so the keyboard still reaches
  it. `CollapsibleContent` slides open and closed, with children that do not
  depend on whether it is open; menus and dialogs keep shadcn's animations.
  `internal/ui/web/base.css` holds the timings and turns CSS motion off for a
  reduced-motion setting; animation written in JavaScript checks
  `useReducedMotion`.
- Provide semantic elements, labels, correct focus order and returned focus.
  Add accessibility handling where native semantics fall short. Each module
  owns one concern, named for its behavior; remove duplication and split files
  that are hard to follow.
- Keep generated files apart from their source, never edit them by hand, and
  have `make check` fail when they are stale.

## Naming

Use the [glossary](glossary.md) for project terms in code, comments,
documentation, and plans. Add a naming rule when a term is ambiguous, gains a
competing synonym, or collides with a library or protocol term; keep its
definition in the glossary.

- Use **session** for the saved Hypercube object and **agent** for the worker
  it runs. A session can exist without a running agent.
- Use **child session** for a Hypercube child and **native subagent** for one
  created inside a harness. Do not use them interchangeably.
- Use **harness** for the native coding-agent runtime and **plugin** for
  Hypercube's executable adapter to it.
- Use **native conversation** for the harness-owned history and **transcript**
  for Hypercube's displayed record. A harness may call its conversation a
  session or thread; keep those names when discussing its API types or fields.
- Distinguish **workspace**, **checkout**, and **managed worktree**. Use
  **working directory** for the directory in which an agent runs; it need not
  be a workspace root or a separate worktree.
- Say **backend**, **desktop app**, or **browser interface** when an
  unqualified **app** would obscure process ownership or lifetime.
- In the plugin protocol, use **command**, **reply**, **event**, and **item**
  for their defined roles. Qualify **reply** as a plugin reply or agent reply
  when the surrounding text does not identify which one it means.
- Use **offer** for the task placement question and **proposal** for saved
  agent-suggested work. User guides call a proposal a **suggestion**; retain
  `proposal` in code and protocol references.

## Edge cases

- Handle required behavior, normal use, and evidenced failures. Additional
  handling must justify its complexity. Validate external input; trust internal
  invariants and fail clearly when they break.

## Development workflow

Write a plan for changes to architecture, system boundaries, state or lifecycle
ownership, new subsystems or major features, and work spanning concerns that
need coordinated design. Localized fixes, copy, styling, documentation, and
small refactors may be done directly. Reassess if a contained change grows.
A change to the browser interface's styling or markup, including one done
directly, gets an independent review before it is committed. The review covers
the touched files whole: an existing component or hook the change repeats,
which component owns each piece of state and each effect, status colors from
`tones.ts`, motion, and tests that check what users see and do.

Before planning, inspect related code for existing behavior, duplicate paths,
and the right owner for the change. Revise a flawed boundary when needed, and
delete obsolete paths so the result is one coherent design.

Run planned work with the `xauto` skill from the
[xskills](https://github.com/kkestell/xskills) suite. It coordinates `xplan`,
`xwork`, and `xreview` and follows the suite's workflow contract for task IDs,
`.agents/` records, commit stages, and trailers.

1. Start `xauto` in a top-level worktree session. That worktree and its branch
   are the task checkout; `main` and other sessions' edits stay untouched.
2. The parent creates each stage worker as a fresh child in the task checkout
   (`dir: "."`), one at a time: a planner running `xplan`, an implementer
   running `xwork`, and a reviewer running `xreview` that did not write the
   code. The reviewer is never a fork. The parent reuses the implementer for
   fixes and the reviewer for re-review.
3. The implementer runs `make check`, then commits the code and its work log.
   The parent commits plan and review records and the final `complete` records.
4. Only independent units of a split plan run in children with their own
   worktrees, and each is merged into the task branch before its child ends.

Merge the task branch into `main` only when the user asks, using **Merge into
main** in the Git panel.

## Validation

Running checks, test isolation and speed, writing tests, and the Linux test
environment are in [testing](testing.md).

- App packaging is separate from checks. When a missing or changed icon needs
  `sips` SVG conversion, request execution outside the Codex sandbox directly:
  that conversion fails inside it with error 13. Do not retry it there.
- Use `make dev` for a live smoke test, and clean up what you start.
- One `make dev` server serves the checkout and rebuilds on changes. Reuse it;
  start another only on a separate port and state folder when two checkouts
  must be compared. It serves the browser view without the desktop app; open
  its URL in the user's browser with `open`. Use `make dev-app` only to test
  desktop app behavior.
- `make install-service` loads a merged change into the installed app. Run it
  from the original checkout. Offer to run it after finishing a change, and run
  it whenever the user asks.
- Agents and panel shells keep running across `make install-service` with the
  code they started with. A change to the harness plugins, `hypercube-hold`,
  `hypercube-mcp`, the plugin protocol, or how agents and shells launch reaches
  an active session only after that session restarts. When a requested change
  needs that, warn the user clearly before starting the work and again when
  offering to install it.

## Backwards compatibility

The current database and configuration are the supported baseline. Startup
creates missing tables and indexes. A change to the database schema or other
saved state includes a migration that preserves the user's state. Delete
obsolete files when you find them, including leftovers from earlier builds and
installs.

## Develop without replacing the installed app

Follow [Run a development copy](../../CONTRIBUTING.md#run-a-development-copy).
Additional controls:

- `make dev-clean` also removes disposable state, holders left by a crash, and
  Claude Code and OpenCode development conversations.
- `HYPERCUBE_DEV_DIR` and `HYPERCUBE_DEV_PORT` change the directory and port.

See [scripts/dev](../../scripts/dev) for cleanup details.

## Prepare an official beta

GitHub Actions runs `make check` on main-branch pushes and pull requests, using
the Go and Bun versions declared in the repository. Beta tags run those same
checks before packaging a signed Apple Silicon app and Linux tarballs.

Once the intended changes are committed and checked, create and push an
annotated tag such as `v0.1.0-beta.1`. The **Beta release** workflow packages
the macOS ZIP on `macos-15` and the Linux tarballs on `ubuntu-24.04` and
`ubuntu-24.04-arm`. A release job then creates one **draft prerelease** with
all three, one `SHA256SUMS`, and generated notes. Inspect the notes, test the
downloaded apps, and publish the draft when ready. GitHub Actions must be
enabled and permitted to create releases in the repository.

The macOS app is signed with the self-signed "Hypercube Local Signing"
certificate, so installed updates keep their macOS privacy permissions. The
workflow imports it from the repository secrets `MACOS_SIGNING_P12` (the
base64-encoded `.p12`) and `MACOS_SIGNING_PASSWORD`. Local builds use the same
certificate when it is in the login keychain and are signed ad hoc otherwise;
`scripts/release` requires it.

`gh workflow run release.yml` packages a branch under the tag `v0.0.0-beta.1`
and keeps the archives as workflow artifacts without creating a release.

To check and package locally on Apple Silicon or on Linux, for the host's
architecture:

```sh
make check
make release
```

Only release packaging requires a clean checkout at a beta tag. Its outputs
are under `bin/release/`; it does not install the app or restart the service.

Go embeds the release tag or commit-based version, adding `+dirty` for
uncommitted changes. Builds without Git metadata report `devel`. The bundle's
`CFBundleGetInfoString` records the full identity; its numeric version fields
contain the version's three-number core. See [build identification](../storage.md#build-identities)
for the user-visible checks.
