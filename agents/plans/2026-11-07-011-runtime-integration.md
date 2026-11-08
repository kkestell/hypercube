# Integrate the separate executables into development and distribution

## Goal and prerequisites

Make normal builds, development, screenshots, signed bundles, installers, and
release archives supply and find the three runtime executables. Validate their
combined behavior and update current references to match the finished design.

Prerequisites: implement and review the five preceding source plans in the
[sequence overview](2026-11-07-005-runtime-boundaries.md), ending with
[backend composition](2026-11-07-010-backend-composition.md). Those plans
own package/API changes and the direct helper builds needed by source tests.
This plan owns distribution and operational integration; intermediate bundles
were not required to be releases. It is not permission to leave dead source
or unresolved source-review findings for a final cleanup pass.

## Related code

- `Makefile:binaries` builds the backend and native plugins side by side.
  The screenshot target separately builds a temporary backend and scripted
  plugin layout; it does not use the binaries target.
- `make/macos.mk` packages executables in `Contents/MacOS`, signs plugins and
  backend, and creates existing public backend/plugin symlinks. The backend's
  signing identifier carries the installed app's privacy identity.
- `make/linux.mk` builds the Linux bundle through `binaries`; installers
  operate on the whole bundle and create backend/plugin links.
- `scripts/dev` rebuilds normal binaries/bundles and recognizes orphan holders
  by old command text for PIDs under its isolated state directory.
- `scripts/linux/test-{bundle,install,release}` check executable layout,
  service replacement, surviving scripted agents, archive installation, and
  removal. `scripts/release` packages the normal bundle.
- Command integration already builds helpers directly and tests restart and
  compatibility. `plugins/scripted/main.go:createSessions` now uses the
  shared MCP command adapter; its actual callback needs an integration scene.

## Decisions

Build and ship `hypercube`, `hypercube-mcp`, `hypercube-hold`, and the existing
native plugins together. Helpers are private siblings inside the bundle, not
new public CLI links. Backend canonical executable discovery from the MCP
plan must resolve them when launched through `bin/hypercube` or an installed
CLI symlink. No helper-selection environment variables or `PATH` fallbacks
are needed.

Sign both helpers explicitly like the native plugin executables. Preserve
the existing backend application identifier and outer-bundle signing order.
Do not introduce independent helper application identities or alter privacy
permissions. Signature verification alone does not establish macOS privacy
attribution for the newly separate holder; report that limit if an installed
bundle check cannot establish it.

Keep services, ports, `hypercube ui`, build identity, backend logs, login-shell
environment, replacement ordering, and public install/uninstall behavior.
Existing holders and native processes survive replacement. Exec compatibility
handles released callers, not an invented release between these plans.
Do not add update transactions, version negotiation, public helper links, or
new recovery guarantees.

## Implementation plan

1. Add both helper commands to `Makefile:binaries` and the independent
   screenshot build recipe. Update build comments and the screenshot driver's
   layout comment. Keep scripted plugins out of release bundles. Inspect
   release construction to confirm it inherits both helpers from the normal
   bundle rather than adding another build path.
2. Include/sign helpers in macOS packaging and update Linux bundle/layout
   expectations. Preserve existing backend/plugin symlink and uninstall
   lists; helpers leave with their private bundle. Inspect desktop service
   setup and installers for assumptions invalidated by the new layout;
   change only actual mismatches.
3. Update `scripts/dev` crash cleanup to recognize old `hypercube hold` and
   new `hypercube-hold` command forms only for PIDs read from that development
   state's run directory. Do not expand cleanup into a global process sweep.
   Normal dev builds now obtain helpers from their existing binaries/bundle
   targets.
4. Extend the existing command process fixture to launch the backend through
   an external symlink into a sibling executable directory containing spaces.
   Reuse its built helpers and restart/compatibility coverage instead of
   creating a second process framework. Exercise direct and legacy MCP after
   restart at the same address and retain agent/panel-shell continuity.
5. Extend Linux bundle/install/release checks for private helper presence and
   executable layout, symlink-backed backend discovery, and removal. Keep
   the installed scripted agent alive over service restart/reinstall.
   Exercise scripted `createSessions` with an offer in the existing install
   scene and assert backend state, without rendering screenshots or adding
   another child process to its one-agent PID assertions.
6. Update `agents/architecture.md` process descriptions, launch/protocol
   links, and backend route ownership; `agents/development.md` code layout
   and restart guidance; the glossary's holder definition; and contributor
   build/layout text where it would otherwise misdescribe the result. Follow
   `agents/documentation.md`. Keep helper implementation detail out of normal
   product flows and leave historical documents unchanged. The architecture
   image already depicts the process roles; regenerate it only if its labels
   become inaccurate. No screenshot regeneration is expected.

## Acceptance and verification

- Normal binaries, macOS and Linux bundles, screenshot temporary layouts, and
  extracted releases contain both executable helpers beside the backend.
  Existing public links and service commands work; no helper link is added.
- A backend invoked through a symlink into a path containing spaces finds
  native plugins and helpers. Direct/legacy MCP and legacy holder exec paths
  preserve environment, streams, arguments, and required inherited files.
  Existing source tests own detailed transport and holder behavior; this
  task verifies layout and integration rather than duplicating them.
- The scripted scene's actual callback succeeds, and backend replacement
  preserves the existing agent/holder and continued work. Dev cleanup remains
  limited to its disposable state. SIGTERM/SIGINT behavior remains unchanged.
- Inspect final `go list -deps` for both helpers and the four native plugins:
  MCP helper has only `mcpbridge` as a local dependency; holder helper has
  only `hold`; native plugins use shared harness data rather than backend
  process clients, config, store, or UI. Inspect final searches for
  `os.Executable`, old command forms, legacy `Hypercube`, and replaced entry
  points; remaining uses must have their current command/test/compatibility
  purpose. No import-policy test framework is needed.
- Run `make check` for the integrated tree. Build the macOS bundle using
  existing icon outputs when available; check helpers are executable and
  independently signed, then `codesign --verify --deep --strict`. Smoke one
  isolated `make dev` startup and rebuild with the complete layout and clean
  up what it starts. Normal `scripts/dev` strips external scene variables;
  do not assume exporting `HYPERCUBE_SCENE` supplies a scripted agent there.
- Run `scripts/linux/run scripts/linux/test-bundle`,
  `scripts/linux/run scripts/linux/test-install`, and
  `scripts/linux/run scripts/linux/test-release`. Use the existing distro
  matrix in CI rather than repeating every distro locally without a concrete
  failure. Run desktop integration locally only if service/desktop behavior
  changes beyond executable contents; existing desktop coverage stays in CI.
- Inspect the screenshot recipe's complete executable layout without
  regenerating images. Do not install into the user's running app or publish
  a release for verification. If a platform check is unavailable, record its
  cause and the unverified behavior in the work log/review instead of claiming
  source inspection established it.

Before this integration work and when offering installation, explain that
surviving agents/shells retain their current code; loading the new launch code
requires restarting affected sessions or shells. No such restart is needed
merely to review these plans.

Stage implementation, tests, documentation, and work log for the required
fresh review. Stop when the complete layout and operational behavior pass the
checks above and the whole touched files are easy to follow.
