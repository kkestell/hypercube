# Compact session reads

## Scope and coverage

Reviewed #47 against `agents/plans/2026-11-06-010-compact-session-read.md`
and `agents/work/2026-11-06-009-compact-session-read.md`: whole
`internal/store/store.go`, `internal/session/transcript.go`,
`internal/mcp/read_session.go`, and their three new compact-read test files.
Traced database transactions, runtime event/checkpoint ownership, pending
restoration, and existing MCP pagination coverage. Concurrent changes to the
full transcript path and unrelated issues are outside this review.

The saved query uses the existing `(session, seq)` primary key, an exclusive
sequence boundary, descending order, and a 64-row caller limit. It projects
rendering fields without returning tool output, image payloads, or full JSON.
The compact path never calls the full transcript reader or unmarshals history.

The pending copy and first SQLite read share the runtime mutex; the pinned
SQLite driver starts read-only transactions with deferred `BEGIN`. Subsequent
batches and visitor calls run outside that mutex in the same read snapshot.
Replacement IDs are retained independently of cursor filtering, and saved rows
are suppressed before rendering. Reverse merging preserves sequence order.

Formatting retains existing rune counting, separator accounting, truncation,
exclusive cursors, chronological output, and the visible-entry test for `more`.
Coverage includes multiple batches, pending replacements, filtered replacements,
replacements outside the cursor, pending-only history, concurrent completion,
early stopping, multibyte budgets, and exact formatted output.

## Summary

No findings. No implementation changes were needed.

## Checks run

- `GOCACHE=/private/tmp/hypercube-go-cache go test -race ./internal/store
  ./internal/session ./internal/mcp -run
  'TestCompact|TestTranscript|TestReadSession'` — passed.
- `git diff --check` scoped to the six implementation and test files — passed.
- The parent owns the final shared-checkout `make check`; it was not repeated
  during this focused review.

## Verdict

Approved for the compact-read scope. No unresolved findings; final integration
validation remains with the parent.
