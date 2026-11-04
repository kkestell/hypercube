# Archive sessions

## Plan

`agents/plans/2026-11-03-009-archive-sessions.md`

## Summary

Archiving stops a family, removes its managed Git resources, and keeps its chats read-only and searchable. The sidebar hides archived families.

## Departures from the plan

- Per the approved cuts, Settings integration and the broader retry and edge-case tests are deferred; cleanup uses Delete's existing behavior.

## Checks run

- `make ui` — regenerated the browser assets.
- `make check` outside the sandbox — passed, including race-enabled Go tests.
- A focused sandboxed `go test` could not access the Go build cache; the full check outside the sandbox passed.

## Manual verification

- `HYPERCUBE_DEV_DIR=/tmp/hypercube-archive-dev HYPERCUBE_DEV_PORT=4298 make dev` — serving http://localhost:4298/ for the user's visual check. No visual inspection was performed.

## Follow-up work

- Add the Archived Sessions section after the Settings branch merges.
