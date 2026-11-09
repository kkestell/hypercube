# Reduce UI response time

## Goal

Remove the measured backend delay first, then use interaction measurements to
choose any frontend changes. Preserve requests, live output, session lifecycle,
draft and view restoration, scroll following and disclosure anchors, find, and
file rendering. Follow [architecture](../../docs/dev/architecture.md) and the repository's
instructions; no saved-state or harness changes are needed.

## Evidence

The installed backend is `b16b6ee`; this checkout is `7b68a4e`. Measurements used
read-only access to the user's database without selecting or waking sessions.

| Observation | Result | Consequence |
| --- | --- | --- |
| Database contents | 179 sessions, 4 active; 12,480 transcript rows, 113,929,504 bytes of JSON; 109,797,440 bytes belong to ended sessions | Most transcript JSON is irrelevant to open offers. |
| Current `OpenOffers`, same SQLite driver, six runs | 133.4 ms first run; 89.5–92.7 ms subsequent runs; zero offers | A full transcript scan evaluates JSON before excluding ended sessions. |
| Equivalent query starting with sessions and forcing that join order | 2.6–3.2 ms; zero offers | Filtering sessions before reading transcript JSON removes the dominant measured query cost. |
| First socket state, sequential connections | 96–111 ms warm; 62,936-byte state | Initial state waits on the query. |
| Four simultaneous socket connections | 106, 196, 287, 379 ms | State reads serialize behind the runtime lock. |
| `/version` | 3 ms | The measured delay is specific to state processing. |
| Backend process sample | SQLite JSON functions beneath `Requests` → `OpenOffers` | Confirms that the running process spends time on this path. |

`Runtime.Requests` holds its lock while reading offers. Every socket currently
calls it on every event batch, even for another session's ordinary streamed
text, before sending selected transcript changes. `WatchAlerts` also calls
`Requests` after durable changes. The same query improvement therefore benefits
multiple consumers. These timings establish a query improvement, not an
end-to-end speedup already achieved.

Frontend costs have not been timed. Code inspection shows that every draft
update changes the app's `[State, Store]` context, the transcript runs layout
work after every render, and file source renders recreate elements per line.
`TextBlock` is already memoized. This checkout also persists view snapshots
with synchronous JSON serialization and `localStorage` on scroll and resize
changes. That persistence postdates the installed build and cannot explain its
current lag.

## Related code

- `internal/store/offers.go` — the measured query.
- `internal/session/transcript.go` — request collection and runtime locking.
- `internal/session/feed.go`, `event.go`, `opens.go`, and `alerts.go` — existing
  change signals, attention updates, queued file opens, and notification reads.
- `internal/ui/socket.go` and `socket_test.go` — state reads, transcript batches,
  selection, and file-open delivery.
- `internal/ui/web/store.ts`, `app/app.tsx`, and `agent/draft-storage.ts` — shared
  updates, render propagation, and draft writes.
- `internal/ui/web/transcript/transcript.tsx`, `transcript-item.tsx`,
  `files/file-view.tsx`, and `files/highlight.ts` — transcript layout, row work,
  source rendering, and synchronous highlighting.
- `internal/ui/web/editor/use-scroll-memory.ts`, `editor/editor.ts`, and
  `app/view-storage.ts` — scroll capture, unchanged-value suppression, and view
  persistence.

## Implementation plan

1. **Filter active sessions before transcript JSON. Highest confidence, lowest
   cost.** In `internal/store/offers.go`, use the measured equivalent query:
   `FROM sessions CROSS JOIN transcript ON sessions.id = transcript.session`,
   retaining `sessions.ended = 0` and both existing JSON predicates. The
   transcript's existing session key supports the lookup; no schema migration
   is needed. Add a brief comment explaining the deliberate join order.
   Repeat the read-only query timings and inspect its query plan. Recheck
   initial-state latency with one and four simultaneous sockets to quantify
   how much of the observed serialization disappears. Preserve request
   ordering and the current runtime lock ownership.

2. **Avoid state reads for ordinary transcript-only changes. High confidence,
   small cost.** In `internal/ui/socket.go`, keep initial state and selection
   behavior. Reuse the existing change signals:
   - An empty `Change.Session` makes the state stale and schedules a flush.
     These signals already cover durable writes and queued file opens.
   - An ordinary transcript change for the selected session batches its item
     and schedules a flush without making state stale.
   - An ordinary transcript change for another session does no socket work
     and does not start the coalescing timer.
   - An attention-item change schedules request refresh even when it comes
     from another session. Updates to an existing unfinished attention item
     can arrive without an empty-session change; do not lose their text.

   Separate the need to refresh requests from the existing workspace/limit
   invalidation only as needed to express these cases clearly. On a normal
   transcript-only flush, skip `Requests`, state serialization, and unrelated
   file-open reads; send the selected transcript batch. On state changes,
   preserve state-before-transcript ordering and queued file-open delivery.
   Retain the existing 50 ms streaming coalescing window. Measure query calls
   and time during selected and unselected session streaming, with one and
   several sockets. Leave the broad durable-write signal unchanged.

3. **Measure remaining local interaction costs before selecting frontend
   fixes. Lower confidence until profiled.** Use isolated development state
   and comparable short and long transcripts and a representative large file.
   Record event-to-paint time, long tasks, React commit time, layout time, and
   storage time for typing, disclosure toggles, scrolling, resizing, and
   switching sessions/tabs. Repeat with output streaming in another session
   to distinguish server delay from main-thread work. Record the tested build;
   include the checkout's newer view persistence in its baseline. Leave visual
   judgment and screenshots to the user.

   Rank frontend fixes by the measured contribution to those actions. When
   traces support them, choose the smallest useful boundary: memoize the
   context-free source renderer or stable transcript row bodies; make
   transcript layout run for actual content, visibility, expansion, and
   geometry changes; or remove repeated scroll-driven renders and synchronous
   persistence work while retaining final scroll capture and restoration.
   Preserve immediate draft
   persistence and current view save/reload behavior; delaying writes needs an
   explicit decision if it weakens either guarantee. Measure draft persistence
   separately before selecting an optimization. Confirm that a boundary
   actually avoids the expensive work; context consumers can still redraw
   through a memoized parent. Keep only changes that improve the measured
   interaction. The proposed scope is profiling and bounded fixes supported by
   it; larger changes need evidence and a separate scope decision.

## Test plan

- Reuse the existing request and offer tests in `internal/session`, including
  parked offers, answered offers, archive/unarchive, and newest-first ordering.
  Add a regression only for a missing behavior; the SQL rewrite preserves the
  result rather than introducing a new contract.
- Extend `internal/ui/socket_test.go` through the public HTTP handler: initial
  and reconnected state remains complete; selected transcript updates replace items
  in order; other sessions' text leaves that transcript alone; attention text
  updates reach requests even without a durable-write signal; attention
  completion, offer answers, and archive/unarchive refresh requests. Retain
  file-open delivery coverage for an already selected session and later
  selection. Test mixed batches as well as transcript-only batches.
- Validate reduced query work through before/after measurements, not brittle
  elapsed-time assertions or tests of private dirty flags. Repeat equivalent
  workloads and report ranges, build, database size, and socket count.
  Do not add a production counter or interface solely to count queries.
- For any measured frontend fix, extend the nearest existing mounted tests
  only for behavior at risk: independent drafts and views, final scroll on
  selection/pagehide, following live output, disclosure anchors, find, source
  selection/highlighting, or late content restoration. Compare interaction
  timings with the same fixture before and after.
- Stop after the measured backend waste is removed and remaining interaction
  measurements no longer justify additional changes. Report residual costs
  and uncertainty without turning them into extra implementation scope.
