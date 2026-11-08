# Separate shared harness data from backend process operations

## Goal and prerequisite

Keep `internal/harness` as shared data and validation. Move the backend's
holder-backed plugin client and executable discovery operations into
`internal/harnessclient`. A production plugin or storage package that uses
harness data must no longer depend on holder execution or application
preferences.

Prerequisite: implement and review
[application configuration](2026-11-07-006-application-config.md). This is the
second unit in the [sequence](2026-11-07-005-runtime-boundaries.md), addressing
[review finding F4](../reviews/2026-11-07-007-runtime-boundaries.md#f4-shared-protocol-data-and-backend-operations-have-the-same-owner).

## Related code

- `internal/harness/protocol.go` owns commands, replies, events, transcript
  items, `Record`, and the private record decoder and event checks.
- `internal/harness/discovery.go` mixes `Settings`, descriptions, requests,
  catalog data and validation, and the `Harness` result with executable
  lookup, subprocess execution, timeouts, and cancellation.
- `internal/harness/harness.go` owns `Client`, holder launch/attachment, output
  reading, and connection lifetime. `harness_test.go` tests record validation,
  so it belongs with the protocol rather than the moved client.
- `internal/session/{runtime,launch,selection,sessions}.go` use the process
  operations and shared data. `internal/harness/discovery_test.go` exercises
  executable discovery, catalog replies, and process-group cancellation.
- Native plugins, `internal/store`, `internal/report`, and browser handlers
  use shared data. `internal/harness/harnesstest` is the intentional plugin
  stand-in used by process tests.

## Decisions

Keep `Settings.Check`, `Catalog.Check`, discovery request/reply types,
`Harness`, commands, records, image validation, and event validation in
`harness`. Move `Client`, `Launch`, `Attach`, `Plugins`, `Find`, `Describe`,
`Models`, and their executable implementation helpers into `harnessclient`.
The client imports `harness` and `hold`; shared data imports neither client
nor holder. `config` may keep importing pure `harness.Settings`.

Expose the current decoder as `harness.DecodeRecord`. Its record and private
event validation remain one implementation. The moved client's `Read`
consumes it; do not duplicate validation or export the event checks.

Preserve execution semantics, lookup behavior, cancellation grace, diagnostic
formatting, output offsets, and holder lifetime. In particular, discovery's
empty-directory fallback still resolves `os.Executable()` in this task, and
the holder still self-executes the caller's `hold` dispatch. The MCP plan
replaces discovery defaults with explicit programs; the holder plan removes
self-execution. Keeping them for now avoids coupling this package extraction
to an executable migration.

No wire fields, stored commands, configuration, executable paths, build
targets, or fixtures' holder-dispatch setup change. Keep coordinated runtime
transactions and locking together. Do not add storage adapters, parallel data
types, or new interfaces to make an import diagram look uniform.

## Implementation plan

1. Split discovery's data and validation from its executable operations.
   Retain a focused shared discovery-data file in `harness`; put execution in
   `harnessclient/discovery.go`. Update package comments to describe their
   actual owners, including per-launch native configuration at the plugins.
2. Move the holder-backed client into `harnessclient`, qualifying shared
   records and using `harness.DecodeRecord`. Rename the existing decoder and
   its record-validation tests; keep these tests in `harness`, in a file named
   for the protocol. Move discovery execution tests with their implementation
   and qualify the shared data they construct.
3. Update backend session operations to use `harnessclient`, retaining
   `harness` imports where they still use shared types. Search all Go callers
   of the moved functions and `Client`; remove old declarations instead of
   preserving aliases. Keep `Runtime.PluginDir` and existing constructors for
   this task.
4. Inspect stand-ins, storage, report, UI, and all four production plugins for
   accidental process-client imports. Keep their shared data usage. The
   scripted plugin's existing backend/report use is separate from production
   plugins and does not justify expanding this task.
5. Update current package ownership and discovery links in
   `agents/development.md` and `agents/architecture.md`, following
   `agents/documentation.md`. Avoid rewriting historical artifacts.

## Acceptance and verification

- Shared record and image validation tests pass unchanged in behavior.
  Discovery tests still observe valid/malformed catalogs, missing or
  nonexecutable plugins, and native-child cleanup on cancellation.
- Existing session process, delivery, restore, and selection tests use the
  moved client without changing observable results. Keep their current
  `holdtest.Main` dispatch; it remains necessary at this stage.
- Inspect `go list -deps` for `internal/harness`, `internal/config`,
  `internal/store`, and each of `plugins/claude`, `plugins/codex`,
  `plugins/opencode`, and `plugins/pi`. Shared harness data and production
  plugins must not acquire `hold`, `harnessclient`, `config`, `session`,
  `store`, `terminal`, or UI through protocol imports. Storage and config
  may themselves use their own existing dependencies, but their harness data
  dependency must no longer bring process execution with it.
- Inspect the moved whole files and search for old execution entry points in
  `harness`. `harnessclient` must not import the session runtime or config.
  No automated dependency-test framework is needed.
- Run focused tests for `internal/harness`, `internal/harnessclient`, and the
  affected session behavior, then `make check` before this code commit. This
  package-only extraction needs no bundle or install test and no native model
  calls. Record results or omissions in the work log and stage the complete
  task for fresh code review.

Next plan: [MCP bridge executable](2026-11-07-008-mcp-bridge-executable.md).
