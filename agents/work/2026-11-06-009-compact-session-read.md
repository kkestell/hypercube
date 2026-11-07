# Bound compact session reads

## Plan

`agents/plans/2026-11-06-010-compact-session-read.md`

## Summary

Implemented #47 using reverse batches of 64 compact projections, a consistent
saved/pending snapshot, and a visitor that stops after the page budget decision.
The full `Transcript` path is unchanged. No schema, index, or cache was added.

## Decisions

- Existing `DB.Read` supplies the read-only transaction. `LastSeq` establishes
  its snapshot while the runtime mutex guards the pending copy; batch reads,
  sorting, and formatting run after unlocking.
- Added read-specific MCP tests in `read_session_test.go` to avoid another
  worker's concurrent edits to `mcp_test.go`.
- The completion test pauses the visitor while another goroutine acquires the
  runtime mutex, saves pending replacements, and adds a row in a later batch.
  The page retains its original pending fields and excludes the later row.
- An older row with an invalid projected boolean makes the early-stop test
  fail if the reader fetches history beyond the needed batch.

## Checks run

- Focused `go test ./internal/store ./internal/session ./internal/mcp -run
  'TestCompact|TestTranscript|TestReadSession'` — passed with a temporary Go cache.
- `GOCACHE=/private/tmp/hypercube-go-cache go test -race ./internal/store
  ./internal/session ./internal/mcp -run
  'TestCompact|TestTranscript|TestReadSession'` — passed, including snapshot
  completion, pending-only history, formatting, rune budgets, cursor boundaries,
  reverse projection, and multiple batches.
- `git diff --check` for the owned implementation and tests — passed.
- The default Go cache was outside the writable sandbox; reruns used
  `/private/tmp/hypercube-go-cache`.

## Follow-up work

- Parent will run the shared checkout's final `make check` after concurrent work
  and arrange independent review. No commit, merge, installation, or issue edit
  was performed.
