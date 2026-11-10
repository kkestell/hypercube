# Inject application paths and parallelize Go tests

- Task: `2026-11-10-001-injected-state-paths`
- Status: ready; design approved by coordinator `b9d8c3e5` in chat.
- Source HEAD: `848727d65518d2c2be4dbdb5708f3b3186dd252b`
- Author: Hypercube worker `go-test-speed`, OpenCode, GPT-6.1 Sol.
- Native conversation ID: unavailable; current tools expose no native ID.
- Dependencies: none. Paths below are repository-relative.

## Outcome and scope

Reduce the uncached `go test -race ./...` wall time well below 30 seconds,
without changing application behavior. The preceding test-only work reduced
102.97 seconds to 87.77 seconds; serial session integration tests still dominate.
Do not change JavaScript tests, installed applications, global settings, or
running workspaces. Stop if this requires lifecycle redesign rather than path
injection and isolated test setup.

## Design

- Represent a preferences file as `config.File`, with `Load` and `Save` methods.
  Resolve the production default from XDG/HOME only at the application boundary.
  Remove implicit global configuration reads/writes from services.
- Make `store.Open` receive the state directory and preferences file explicitly.
  Keep the state directory on the opened database, and pass the preferences file
  to its existing model-entry migration. Database layout, permissions, schema,
  migration order, and configuration defaults remain unchanged.
- Pass `config.File` to the session runtime and browser handler at construction.
  Holder paths, terminal paths, restored launch paths, and plugin state use the
  database's injected state directory. Alert configuration uses the same file.
- Production startup resolves the existing default paths once and supplies them
  to all consumers. Tests construct unique state/config locations directly.
- Reuse the runtime's existing per-workspace environment callback for stand-in
  logs, barriers, and shell markers. Do not mutate the test process environment
  for ordinary fixture setup. Keep tests that intentionally replace Git on PATH
  serial unless measurement shows they prevent the target.

## Execution

1. Change path/configuration APIs and update every production and test caller.
2. Isolate fixture environment and parallelize independent session, GitHub, MCP,
   UI Go, store, and configuration tests. Keep explicit global-environment cases
   sequential, with their environment restored before parallel tests execute.
3. Measure the full uncached race suite and package/test timings. Address actual
   remaining bottlenecks; prune duplicated or implementation-detail assertions
   without automatically replacing their coverage.
4. Run the full suite several times uncached, run `make check`, inspect the diff,
   and commit the completed changes. Report before/after timings and limits.

## Verification

- Existing migration tests verify that moving paths does not lose preferences
  or database content.
- Concurrent fixtures verify that session state, transcripts, holder directories,
  configuration changes, and cleanup remain isolated under `-race`.
- Preserve consequential public lifecycle, recovery, authorization, and worktree
  data-loss tests. Do not introduce fixed sleeps or product timing changes.

## Handoff assessment

Ready. One constructor-injection change plus mechanical caller/fixture updates
and measurement-driven test improvements; no new subsystem or lifecycle. This
is comparable to passing an existing dependency through several service layers,
with a substantive race-test verification burden. The deliberately short plan
settles its consequential interfaces and compatibility constraints; additional
prose would enumerate repetitive wiring rather than resolve another decision.
The coordinator requested implementation in this worker and independent review
after the work log, rather than separate stage workers.
