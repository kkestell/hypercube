# Explicit stop and a running-first session list

## Goal

Sessions run until the user stops them or quits Hypercube; nothing stops a
family for inactivity. The sidebar lists every running top-level session in
each workspace, hides stopped ones until Show more reveals them five at a
time, and its workspace and session rows, like the Files tree, sit slightly
closer together.

## Related code

- `internal/session/inactivity.go` — `recordActivity`, `ViewSession`,
  `StopIdleSessions`, `stopIdleSessions`. All of it exists only for the idle
  stop.
- `internal/session/runtime.go` — `idleTimeout` and the `viewers` map.
- `internal/session/input.go` — `Input`, `Interrupt`, and `Answer` call
  `recordActivity`.
- `internal/session/commands.go:148`, `internal/session/event.go:131` —
  `tx.TouchSession` when an item or event is recorded.
- `internal/session/sessions.go:122` — `Start` sets `s.LastActive`.
- `cmd/hypercube/main.go:96` — `go rt.StopIdleSessions(ctx)`. `ctx` stays for
  `WatchAlerts`.
- `internal/ui/socket.go` — `dataSocket.releaseView` and the `ViewSession`
  call in `selectSession`.
- `internal/store/store.go` — `Session.LastActive`, `TouchSession`,
  `AddSession`, `SaveSession`, `sessionColumns`, `scanSession`, and the
  `last_active` column in `schema`. Nothing else reads `LastActive`; the
  sidebar orders sessions by `created`.
- `internal/ui/web/sidebar/sidebar.jsx` — `pageSize`, `limits`, the slice and
  selected-session swap per workspace, and Show more.
- `internal/ui/web/base.css:12` — `--size-tree-row-height`, shared by the
  sidebar rows, the workspace header, Show more, and the Files tree
  (`internal/ui/web/files/files-panel.css`).
- `internal/ui/web/sidebar/sidebar.css` — the gap between workspaces and the
  space under each workspace header.

## Decisions

- **The `last_active` column stays in the database; the Go code forgets it.**
  Dropping it needs a migration for no behavior. `AddSession` writes `created`
  into `last_active` to satisfy `NOT NULL`; `SaveSession`, `sessionColumns`,
  and `scanSession` omit it. A short comment on the column in `schema` says it
  is unused and kept so existing databases open unchanged. `convertLegacy`
  still fills it and stays as it is.
- **Which top-level sessions show.** Per workspace, in the existing newest-first
  order: every session with `running` true, the first `stopped` stopped
  sessions, and the selected session (or the parent of the selected child)
  wherever it sits. `limits` becomes the number of stopped sessions shown,
  starting at 0; Show more adds `pageSize` (5). Show more appears while some
  session is still hidden, using the existing
  `sessions.length < workspace.sessions.length` test. The selected session is
  added in its own place instead of replacing the fifth row, since running
  sessions no longer have a fixed page.
- A session stopped while not selected leaves the list until Show more
  reveals it again. Child rows and the parent menu's Show Stopped Sessions are
  unchanged.
- **Compression.** `--size-tree-row-height` goes from 26px to 24px, which
  compresses the session rows, workspace headers, Show more, and the Files
  tree together. The gap between workspaces goes from `--size-space * 4` to
  `* 3`, and the margin under a workspace header from `* 2` to `* 1`.
- No active session needs a restart: the idle stop runs in the app process,
  and no plugin, holder, MCP server, or launch changes.

## Implementation plan

1. Delete `internal/session/inactivity.go` and
   `internal/session/inactivity_test.go`.
2. `internal/session/runtime.go`: remove `idleTimeout`, the `viewers` field,
   and its initialization in `New`.
3. `internal/session/input.go`: remove the three `recordActivity` calls and
   their error branches.
4. `internal/session/commands.go`, `internal/session/event.go`: remove the
   `TouchSession` calls. `internal/session/sessions.go`: remove the
   `s.LastActive` assignment and the `time` import if unused.
5. `internal/session/restart_test.go`: in
   `TestDeliveryTimeoutHoldsLaterMessagesUntilAcceptance`, remove the
   `stopIdleSessions` call and its check.
6. `cmd/hypercube/main.go`: remove `go rt.StopIdleSessions(ctx)`.
7. `internal/ui/socket.go`: remove `releaseView` (field, initialization, the
   deferred call) and the `ViewSession` call; `selectSession` sets `selected`
   and its doc comment says it sends the whole transcript.
8. `internal/store/store.go`: remove `Session.LastActive` and `TouchSession`;
   update `AddSession`, `SaveSession`, `sessionColumns`, and `scanSession` as
   decided; comment the `last_active` column.
9. Store tests: in `internal/store/store_test.go`, drop the `TouchSession`
   and `LastActive` parts of `TestOpenCreatesSchema`,
   `TestSaveSessionWritesMutableFields` (keep the title check), and the "child
   did not touch root" check; in `internal/store/migrate_test.go`, drop
   `LastActive` from the expected sessions.
10. `internal/ui/ui_test.go` `TestSnapshotOrder`: remove the `TouchSession`
    calls; keep the title and running changes and the created-order
    assertion.
11. `internal/ui/web/sidebar/sidebar.jsx`: per workspace, compute the stopped
    sessions, keep the first `limit` of them (`limits[workspace.id] ?? 0`), and
    filter `workspace.sessions` to running, kept stopped, and selected
    sessions; delete the fifth-row swap. Show more adds `pageSize`. Update the
    file's opening comment to describe this.
12. `internal/ui/web/base.css`: `--size-tree-row-height: 24px`.
    `internal/ui/web/sidebar/sidebar.css`: the workspace gap and header margin
    as decided.

## Test plan

- `internal/ui/web/sidebar/sidebar.test.jsx`:
  - Replace the five-per-page test: with running and stopped sessions
    interleaved, every running session shows in order and no stopped one does;
    each Show more adds the next five stopped sessions in place; Show more
    disappears when all are shown.
  - Replace the fifth-row test: a selected stopped session shows in its place
    with no stopped sessions revealed, and Show more still reveals the rest.
  - A workspace with only stopped sessions shows Show more and no rows.
  - Tests that use stopped top-level sessions (`clicking a session row chooses
    it, and a stopped one starts`, `a session's menu stops a running session
    and starts a stopped or failed one`) click Show more first.
- Go: the session, store, and UI packages pass with the idle tests removed;
  nothing else in them refers to idle stops.

## Documentation updates

- `docs/usage.md` Browser view: the sidebar shows every running session in a
  workspace and Show more reveals stopped sessions five at a time; the
  selected session always shows.
- `docs/usage.md` Resume and cleanup: a family runs until it is stopped or
  Hypercube quits; closing the browser leaves agents running. Remove the
  ten-minute timeout and viewing sentences.
- `docs/introduction.md` Pausing and finishing work: closing the interface
  leaves agents running until you stop them; remove "inactive families
  eventually stop automatically".
