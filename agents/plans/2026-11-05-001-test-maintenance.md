# Reduce test maintenance

## Goal

Keep tests that protect distinct product behavior while removing duplicated
scenarios, assertions about markup details, and machinery that tests itself.
Production behavior stays unchanged.

## Decisions

The baseline is deletion and simpler use of existing helpers. Keep separate
session, UI, and app fixtures: their owners and entry points differ. Keep
recorded harness fixtures, migration and restart tests, and the session idle
barrier needed to queue messages deterministically. Add no testing framework,
production interface, or shared fixture subsystem.

## Implementation plan

1. In `internal/harness/client_test.go`, replace the test-only request
   coordinator with explicit command IDs and an ordered record channel.
   Preserve events before replies, rejected commands, process-group shutdown,
   malformed output, and exit diagnostics. Remove tests of the stand-in's
   deduplication and the helper's own unknown-reply rejection; remove unused
   stand-in commands in `internal/harness/harnesstest/harnesstest.go`.
2. In `cmd/hypercube/integration_test.go`, remove the repeated session routing
   matrix already exercised by MCP and session tests. Keep end-to-end child
   management, deletion, restart, and real app-process wiring. Delete helpers
   made unnecessary by those removals.
3. In UI transcript, sidebar, and find tests, remove scenarios covered at the
   behavior's owning function or by another component integration test.
   Retain component wiring, focus, pending requests, and streaming coverage.
   Replace CSS class and direct-child-shape assertions with visible behavior.
   Reuse `stubFetch` and keep transcript sequence state local to each setup.
   Remove the file-icon test that only checks upstream Lucide behavior.
   Remove nested React `act` from the store subscription and make tests own
   the `act` scope of their mounted updates and asynchronous interactions.
4. Consolidate the test guidance in `AGENTS.md`: protect distinct consequential
   behavior at the cheapest suitable boundary; keep assertions independent of
   styling and markup; justify stand-ins and remove redundant setup or tests.

## Test plan

Verify each removed scenario has retained behavior coverage. Run focused Go
and UI tests after their changes and the repository checks after integration.
Review all touched files for clarity and accidental coverage loss.
