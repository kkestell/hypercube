# Extract the MCP bridge and its executable contract

## Goal and prerequisites

Give stdio-to-HTTP MCP forwarding its own package and `hypercube-mcp`
executable. Backend tool registration stays in `internal/mcp`. New plugin
starts receive the actual MCP executable path, while old recorded starts and
already running native harnesses keep working through narrow compatibility
paths.

Prerequisites: [application configuration](2026-11-07-006-application-config.md)
and [harness protocol boundary](2026-11-07-007-harness-protocol-boundary.md).
This source change addresses review findings F2 and F3 and supplies explicit
program configuration for the holder change. Packaging and development
recipes are owned by
[runtime integration](2026-11-07-011-runtime-integration.md); intermediate
bundles do not yet contain the helper. Build needed executables directly in
test fixtures without adding a production fallback.

## Related code

- `internal/mcp/mcp.go` mixes `Bridge`, identity-header transport/retry, and
  environment lookup with database-backed tool registration and `Handler`.
  `mcp_test.go` mixes bridge coverage and tool clients using that transport.
- `cmd/hypercube/main.go` dispatches `mcp` and starts the backend.
- `internal/harness/protocol.go:StartArgs` sends `Hypercube` and `Env`.
  `internal/session/launch.go:startArgs` derives that path from its caller's
  executable. `commands.go` persists and replays complete command lines.
- `internal/session/runtime.go` exposes mutable `PluginDir`; discovery in
  `internal/harnessclient` still derives a default directory.
- All four native plugins build MCP settings in their launch/configuration
  code. `plugins/scripted/main.go:createSessions` independently executes
  `start.Hypercube` with `mcp`; it must use the same command selection.
- `cmd/hypercube/integration_test.go`, session fixtures, backend MCP tests,
  and UI fixtures construct runtimes and plugin directories.

## Decisions

`internal/mcpbridge.Run(ctx, baseURL, sessionID, stdin, stdout)` owns forwarding
and the existing connection-refused retry policy. It imports the MCP SDK and
standard library, with no local backend-package dependencies. The new narrow
command reads `HYPERCUBE_URL` and `HYPERCUBE_SESSION_ID`, reports missing
inputs, supplies context and stdio, and reports errors under its own name.
No helper opens SQLite or discovers backend state.

Keep `internal/mcp` as the backend tools and HTTP adapter. Use the stable
`X-Hypercube-Session` header literal at both ends and document that wire
contract. Do not create a shared package for one constant. Backend tool test
clients add the header themselves and do not import the bridge for its
retrying transport.

Introduce `session.Programs{PluginDir, MCP string}` as concrete construction
data. Pass it to `session.New`, store it privately, remove mutable
`Runtime.PluginDir`, and adapt the existing private `startApp` constructor.
At the backend command boundary, get `os.Executable()`, resolve symlinks with
`filepath.EvalSymlinks`, and use its real parent directory for plugins and
`hypercube-mcp`. Keep this small location/exec code in a focused command file;
do not introduce `internal/backend` or a helper registry. Discovery now
requires its explicit directory and loses its executable fallback. The
native harness CLIs still use the user's `PATH` as before.

The next plan covers the holder path and launch signatures. Holder launch
still uses `os.Executable()` plus `hold`, production still dispatches `hold.Main`, and
existing test-binary holder dispatch remains necessary until the next plan.
This plan does not claim to remove those dependencies.

## Persisted and live-process compatibility

Add plain `StartArgs.MCP string` with JSON name `mcp`. New commands contain the
absolute helper path and current MCP environment; they omit `Hypercube`.
Retain `StartArgs.Hypercube` with `omitempty` solely for old saved starts.
Do not change recorded command bytes, IDs, replay order, or acceptance/deletion.
A new plugin can receive an old saved start when a missing launch directory
forces current plugin code to run; an old surviving plugin must still receive
its original line.

Add one shared `plugins/internal/plugin.MCPCommand` adapter. Prefer nonempty
`MCP`, returning that executable with no arguments; otherwise return the old
`Hypercube` plus `mcp` command. Return a clear error when neither is usable.
Keep this argv construction inside plugin helpers rather than broadening the
new wire contract into a command vector. Each native integration renders the
result into its existing configuration format and preserves `Env`.

Replace `hypercube mcp` with `syscall.Exec` of the canonical sibling helper,
preserving environment and streams without another supervised process.
Already running native harnesses can start their saved command after binary
replacement. The backend command must not import `mcpbridge` once this move
is complete. No database or native-settings migration is needed: native
launch/resume already supplies MCP configuration every time. Keep global and
project settings, credentials, and `HOME` unchanged.

## Implementation plan

1. Move forwarding/retry into `internal/mcpbridge` and move bridge tests to an
   independent HTTP MCP fixture without backend, store, or session imports.
   Keep tool schemas, scopes, and handlers in `mcp`; remove the old bridge
   implementation and adjust tool test clients.
2. Add `cmd/hypercube-mcp`, canonical sibling discovery in `cmd/hypercube`,
   and the `mcp` compatibility exec. Do not temporarily route production
   dispatch through the bridge library. Direct test builds provide helpers
   until the final build-integration task.
3. Add explicit `Programs` construction and update every runtime/`startApp`
   caller and fixture. Remove library executable discovery for plugin paths
   and MCP starts. Tests that invoke callbacks build the real helper in their
   existing process-fixture setup; tests whose stand-ins never execute MCP
   still supply an explicit fixture path rather than relying on production
   discovery or overrides.
4. Add the `MCP` field and shared command adapter. Update Claude Code, Codex,
   OpenCode, and Pi native configuration builders and their stand-in tests.
   Update scripted `createSessions` to use the shared adapter and existing
   environment, despite its separate serve loop. Inspect all fixtures that
   construct `StartArgs`; retain deliberate legacy inputs.
5. Add narrow compatibility and callback regressions at the existing source
   test boundaries. Remove obsolete imports/comments and old forwarding
   aliases. Keep the runtime's transactions and restart behavior unchanged.

## Acceptance and verification

- Bridge tests cover a request and matching response, initialization and
  notifications, session header, EOF, backend errors, and delayed-backend
  retry. Preserve current retry limits and message ordering. Command tests
  cover missing environment. Backend MCP schema/scope/tool coverage stays at
  its current owner.
- Shared adapter tests cover new path precedence, legacy field fallback, and
  missing command. Native stand-ins observe the direct path without arguments
  and unchanged environment in each integration's own format. No model calls
  or user-settings edits are needed.
- Extend the existing backend process test to build its sibling MCP helper
  directly and exercise both `hypercube-mcp` and `hypercube mcp` against a
  known session. Preserve the same backend address across restart so a
  surviving process's configured callback remains valid; exercise the legacy
  command after replacement as well. Keep existing agent/shell continuity
  assertions. Complete bundle/symlink and scripted-scene checks belong to the
  final integration plan.
- A narrow old-start regression preserves serialized bytes and ID during
  replay to an old-compatible stand-in. The shared adapter separately accepts
  that legacy start when new plugin code consumes it. No old application
  binary or version matrix is required.
- Inspect `go list -deps ./cmd/hypercube-mcp`: the only local package must be
  `internal/mcpbridge`. Inspect the backend's imports for removal of bridge
  implementation; search for legacy field uses and library `os.Executable()`
  to distinguish retained holder behavior from removed path discovery.
- Run focused bridge, command, plugin, and affected session/MCP/UI fixture
  tests while working. Run `make check` before a code commit, following the
  development rules. Record results and any omitted checks in the work log;
  stage this source task for fresh code review. Do not run packaging or
  installation checks here merely to make the intermediate bundle releasable.

Before implementation, explain that already running sessions retain their old
plugin/native command until restarted; compatibility keeps them supported.
Current architecture, build-layout, and operational documentation changes are
consolidated with the final integration plan.

Next plan: [Holder executable](2026-11-07-009-holder-executable.md).
