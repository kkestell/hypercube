# Bound compact session reads

## Goal

Fix #47: read only the saved transcript rows needed for a compact `read_session` page, without decoding tool output or images or holding the runtime mutex throughout the read. Keep the full `Transcript` path unchanged.

## Decisions and implementation

- Add a concrete reverse-batch read in `internal/store/store.go`, using the existing `(session, seq)` primary key, an exclusive sequence boundary and a row limit. Project only the fields used by `entryLine` with SQLite JSON functions, including whether `final` is nonempty; do not return full JSON or add a schema, index, cache, or general query framework. Preserve item ID for replacement matching. Do not filter saved rows in SQL in a way that hides a pending replacement.
- In `internal/session/transcript.go`, provide a compact read path for this consumer. Under `r.mu`, copy the target's pending items and their positions, and establish a consistent SQLite read snapshot with a lightweight read before releasing the mutex. Read subsequent batches in that transaction outside the mutex. This makes pending and saved state correspond even if a streaming item completes during the read. Copy only fields needed for compact rendering; do not retain mutable pending data.
- Merge the reverse-ordered saved rows and pending snapshot by sequence. Suppress saved rows whose item IDs are pending, even when the pending version is filtered out or lies outside the requested cursor. Apply the exclusive cursor to the merged positions; continue through batches until the consumer has enough visible entries to decide the page and whether another visible entry remains, or history is exhausted. Avoid materializing the whole saved history. Keep formatting and page-budget decisions in `internal/mcp/read_session.go`, with a narrowly scoped way to stop the read after that decision.
- Preserve exact current output: count formatted Unicode runes, include the existing two-character separator accounting, truncate an oversized first entry to 40,000 runes plus ellipsis, and set `more` only when another nonempty formatted entry exceeds the budget. Return the oldest included sequence as cursor, retain chronological output order, and preserve the behavior of nonpositive cursors and filtered entries.

## Validation

- Extend the MCP behavior tests in `internal/mcp/mcp_test.go` for multi-batch history, cursor boundaries, filtered entries around page boundaries, multibyte text, an oversized first entry, and the no-more-visible-entries case. Check actual text and cursors across pages.
- Exercise pending entries interleaved with saved rows, including a pending replacement of a saved item and a filtered replacement, and completion concurrent with a read; verify no duplicate or missing replacement and that unrelated runtime activity is not blocked for the full read. Use an appropriate existing test seam and race testing.
- Verify the storage projection omits large tool output and image data while preserving every field used in rendering, including idle markers with and without final responses. Run focused Go tests, then `make check` as required by `agents/development.md`.
