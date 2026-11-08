# MCP bridge executable

## Plan

[MCP bridge executable](../plans/2026-11-07-008-mcp-bridge-executable.md)

## Summary

The plan's goal is met. The stdio forwarder lives in `internal/mcpbridge`
and runs through `hypercube-mcp`; `hypercube mcp` calls the same code for
existing harness configurations. Backend startup resolves its executable
before supplying plugin and helper paths through `session.Programs`. All
five plugins use one resolver for new MCP paths and saved legacy starts.
The forwarding loop, retry transport, and closer match the prior bridge
apart from parameter and header names.

## Departures from the plan

- Updated the alert and compact-transcript runtime fixtures and the plugin
  configure/connect/discovery fixtures omitted from the original inventory.
  They need the new constructor argument or a valid MCP path.
- The process test resolves the expected helper path because macOS exposes
  temporary directories through the `/var` symlink. Resolving the whole
  fixture directory instead exceeded the existing holder socket path limit;
  only the expected executable path is resolved. The plan records both fixture
  adjustments.

## Checks run

- `go build ./...` — passed.
- `make check` — passed after correcting the process fixture: browser and
  website builds, TypeScript, browser and desktop tests, Go build, vet,
  formatting, and all Go tests with the race detector.
- `go test -race -run TestAppProcess -count=1 ./cmd/hypercube` — passed.
  The backend starts through a symlink, finds the stand-in plugin and helper
  beside its real executable, records the helper path, serves tool calls
  through both commands, and preserves agents and the panel shell across
  backend restarts.
- Bridge tests — passed: session identity, notification forwarding, unknown
  methods, an error answer for every refused call, backend restart retry, and
  missing environment. Plugin tests cover empty argument arrays, legacy
  saved-start decoding, new configurations, and Claude's legacy configuration.
- `go list -deps ./cmd/hypercube-mcp` — its only Hypercube packages are the
  command and `internal/mcpbridge`.
- Production `os.Executable` search in `internal` and `plugins` — only
  `internal/hold/hold.go` remains; other matches are test support.
- Compared the extracted forwarding code with its predecessor — unchanged
  after the planned renames. `git diff --check` — passed.

Checks used a temporary writable Go build cache. Build and dependency listing
also emitted a nonfatal module stat-cache write warning for the sandboxed
home directory; both completed successfully.

## Manual verification

No installed app, live harness, or Linux packaging test was run. The isolated
process test covers the command boundary and existing restart behavior.

## Follow-up work

Packaging and development build integration remain in plan 011. This
intermediate change is not ready to install by itself. Nothing was installed
or committed.
