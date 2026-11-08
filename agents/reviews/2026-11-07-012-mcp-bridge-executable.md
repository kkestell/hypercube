# MCP bridge executable

## Scope and coverage

Independent review of the staged implementation of
[plan 008](../plans/2026-11-07-008-mcp-bridge-executable.md), its work log,
and the parent's
[architectural assessment](2026-11-07-011-mcp-bridge-plan.md).
Inspected the whole touched Go files, relevant discovery and restore callers,
saved command replay, plugin configuration paths, fixtures, and documentation
changes before making edits. No implementation edits were needed.

Coverage includes both commands, the extracted forwarder and retry transport,
`session.Programs`, executable symlink resolution, new and legacy start
arguments, and the Claude, Codex, OpenCode, Pi, and scripted plugins.

No live harness or Linux packaging test was run. OpenCode's installed-harness
test remains opt-in and was skipped. Packaging and development integration
belong to plan 011; this intermediate change is not ready to install alone.

## Summary

No findings.

- Compared the forwarding loop, retry transport, and closer with their
  predecessor in `HEAD`: they are identical after the planned parameter and
  header renames. Environment validation and per-call error answers remain.
- The backend command resolves its real executable once and supplies the
  plugin directory and bridge path. Discovery and session code use those
  explicit paths. The process test launches through a symlink, discovers its
  plugin, and checks the bridge path after macOS temporary-path resolution.
- Restore still resends each saved command line unchanged. `MCPCommand`
  decodes the legacy `hypercube` field, prefers `mcp` when both exist, returns
  an empty argument array for the new bridge, and rejects a missing command.
  All five plugins use the result with their existing environment handling.
- The tests exercise identity, request IDs, refusal answers, backend restart
  retry, plugin configuration, legacy decoding and Claude configuration,
  runtime start arguments, and both executable entry points. Existing
  process restart assertions continue to check agent and panel-shell survival.
- Fixture additions and the expected-path normalization are necessary
  constructor/configuration updates and a macOS path adjustment. They stay
  within the plan's behavior and do not add delivery or recovery guarantees.

## Checks run

All reviewer Go checks used
`GOCACHE=/private/tmp/hypercube-harness-boundary-go-cache`.

- `go test -race -count=1 ./internal/mcpbridge ./plugins/internal/plugin
  ./plugins/claude ./plugins/codex ./plugins/pi ./plugins/opencode` — passed.
- `go test -race -count=1 -run
  'TestAppProcess|TestSessionStartsAndDeliversInOrder|TestCreateSessionRefusals|TestCreateSessionForms|TestRequestedSessionStartsWithoutAnOffer'
  ./cmd/hypercube ./internal/session ./internal/mcp` — passed. The invocation
  also selected `TestPlugins|TestFind` in `internal/harnessclient`, which had
  no matching tests; the discovery check below covers that package.
- `go test -race -count=1 -run 'TestDescribe|TestModels$'
  ./internal/harnessclient` — passed.
- `go list -deps ./cmd/hypercube-mcp` — passed; its only Hypercube packages
  are the command and `internal/mcpbridge`. Go emitted a nonfatal module
  stat-cache write warning for the sandboxed home directory.
- Normalized source comparison against `HEAD:internal/mcp/mcp.go` — passed
  for the forwarding loop, retry transport, and closer.
- `os.Executable` search in `internal` and `plugins` — only `internal/hold`
  remains in production; other matches are test support.
- `git diff --cached --check` — passed.
- Reused the completed implementation's recorded `make check` and
  `go build ./...` results. No code changed during review, so the full checks
  were not repeated.

## Verdict

Pass. No fixes or unresolved findings. The parent can assess and commit this
plan's changes; installation waits for plan 011.
