# Development and releases

Start with [AGENTS.md](../AGENTS.md) for project scope and document routing.
For source setup, local builds, and installing a checkout, use
[Contributing](../CONTRIBUTING.md). This reference covers project conventions,
workflow, testing, and releases. App users should start with the
[user documentation](../README.md#start-here).
Documentation procedures are in [documentation](documentation.md).

## Code

- `cmd/hypercube/` — the command line and the backend process.
- `desktop/` — the Electron desktop app, a client of the backend.
- `internal/` — one package per concern: `desktop`, `harness`, `hold`, `mcp`,
  `report`, `session`, `store`, `terminal`, `ui`, `workspace`.
- `plugins/` — the Claude Code, OpenCode, Codex, and Pi harness plugins. The
  core names no harness. `plugins/scripted` plays documentation screenshot
  scenes and never ships.
- `make/` — the macOS and Linux packaging rules the `Makefile` includes.
- `scripts/dev` — the isolated development session (`make dev`).
- `scripts/screenshots` — the documentation screenshots (`make screenshots`).
- `scripts/linux-install`, `scripts/linux-install-service`,
  `scripts/linux-uninstall` — the Linux installer scripts, shipped in the
  bundle's `resources/` as `install`, `install-service`, and `uninstall` and run
  by `make install`, `make install-service`, and `make uninstall`.
- `scripts/linux/` — the Linux test container (`scripts/linux/run`).
- `web/` — the Taikonaut website, a Hugo site built and served by `make web-build`
  and `make web-serve`.
- `docs/` — user tutorials, how-to guides, reference, and explanations, indexed
  by `README.md` and the website's documentation landing page.
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

After the required checks and review are complete, the parent commits the
task's changes unless the user asks otherwise. Merge the task branch into
`main` only when the user asks. Do not delegate the commit or merge to the
three agents.

## Validation

- `make check` runs every check. Run it before committing code.
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
- Test Linux behavior with `scripts/linux/run`.
- `make install-service` loads a merged change into the installed app. Run it
  from the original checkout. Offer to run it after finishing a change, and run
  it whenever the user asks.
- Agents and panel shells keep running across `make install-service` with the
  code they started with. A change to the harness plugins, `hypercube hold`,
  `hypercube mcp`, the plugin protocol, or how agents and shells launch reaches
  an active session only after that session restarts. When a requested change
  needs that, warn the user clearly before starting the work and again when
  offering to install it.

## Test on Linux

`scripts/linux/run [command]` runs the command in a new container that boots
systemd, then removes the container. `LINUX_DISTRO` chooses the distribution:
`debian` (Debian 13, the default), `ubuntu` (26.04), `fedora` (45), or `arch`.
Arch publishes AMD64 images only, so it runs in CI, not on Apple Silicon. The
default command is `make check`; `make linux-check` runs it. `make linux-test`
runs the Linux tests: `scripts/linux/test-bundle` builds the bundle and checks
its layout, its executables, and the backend started from it, and
`scripts/linux/test-install` installs the bundle into the test user's home,
runs the service under bash, zsh, and fish, keeps a scripted agent running
across a service restart, and checks `make uninstall`.
`scripts/linux/test-desktop` drives the installed desktop app against the
installed service with Playwright: the window frame and menus, each waiting
page, setting up the service, a notification and the launcher badge, the open
actions, the shortcuts and Ctrl labels, the panel terminal's keys (pressed with
`xdotool`), closing the window, and reinstalling and uninstalling while the app
runs. It starts openbox, the dunst notification server, a stand-in file
manager, and stand-ins for `code` and `xdg-open`. `scripts/linux/test-release`
runs last: it packages a tagged copy of the checkout with `scripts/release`,
installs from the extracted tarball, and checks the version, the service, and
removal. Screenshots and logs land in `bin/linux-test/<distro>/`, copied out of
the container when the command ends.
`make linux-test-all` runs the Linux tests for every distribution that has an
image for the host's architecture, and CI runs them for all four on AMD64. The
container runs natively on Apple Silicon and needs Docker.

The command runs as the user `tester`, which has a systemd user manager, under
Xvfb. The image has Go and Bun at the versions the repository declares,
a C compiler, `rsvg-convert`, SQLite, the Electron runtime libraries,
Node, and Playwright. It has no harness CLIs, so tests that start sessions use
the scripted plugin as `make screenshots` does.

The container gets a copy of the tracked and untracked files that Git does not
ignore, without `.git`, so builds report version `devel` and nothing in the
checkout changes. The volume `hypercube-linux-cache` keeps Go and Bun
downloads and builds across runs; `docker volume rm hypercube-linux-cache`
resets it. The first run builds the image and downloads dependencies, and later
runs take seconds. Each distribution has its own image,
`hypercube-linux-<distro>`, built from `scripts/linux/Dockerfile`;
`scripts/linux/packages` installs that distribution's packages. New Linux
tests go in `scripts/linux/`.

`scripts/linux/test-apparmor` checks the installer's AppArmor profile on a real
Ubuntu 24.04 kernel, which restricts user namespaces; the container's kernel
does not. It builds the bundle in the container, then boots the Lima VM
`hypercube-apparmor` natively (`vmType: vz`) and checks that the desktop app's
sandbox fails without the profile, works with it, and that uninstalling removes
it. It needs an Apple Silicon Mac with Docker and Lima, downloads an Ubuntu image
on its first run, and is not part of `make linux-test`. It deletes the VM when it
ends.

## Backwards compatibility

The user's state carries forward from each version to the next. A change to
the database schema or other saved state includes a migration that upgrades
the previous version's state when the app starts. Delete obsolete files when
you find them, including leftovers from earlier builds and installs.

## Develop without replacing the installed app

Follow [Run a development copy](../CONTRIBUTING.md#run-a-development-copy).
Additional controls:

- `make dev-clean` also removes disposable resources and native development
  conversations, including holders left by a crash.
- `HYPERCUBE_DEV_DIR` and `HYPERCUBE_DEV_PORT` change the directory and port.

See [scripts/dev](../scripts/dev) for cleanup details.

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
contain the version's three-number core. See [build identification](../docs/storage.md#build-identities)
for the user-visible checks.
