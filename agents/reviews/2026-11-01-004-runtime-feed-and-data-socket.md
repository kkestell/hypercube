# Review of the runtime change feed and data socket

## Scope and coverage

This review covers commit 18ab91a, "Unify runtime writes and stream browser data
changes". It is the Go half of
`agents/plans/2026-11-01-003-browser-ui-rewrite.md` and step 1 of
`agents/reviews/go-audit.md`. Every file the commit touches was read in full as
it stands after the change: `internal/session`, `internal/report`,
`internal/store`, `internal/mcp`, `internal/ui`, and `cmd/hypercube`. The review
checks whether every write is published, the order of each agent's changes, the
subscribe-then-reset sequence, dropping of slow subscribers, the 50 ms batching,
and whether the socket messages match the plan's protocol.

Gaps:

- The plan puts these out of scope: launch exit paths, splitting `runtime.go`,
  the test fixture, and the plugins.
- No live browser check was run. The page in this tree still uses the old
  protocol.

## Summary

| ID | Severity | Category    | Fix   | Title                                                   |
| -- | -------- | ----------- | ----- | ------------------------------------------------------- |
| F1 | Medium   | Readability | Fixed | The data socket is one function that does five things   |
| F2 | Low      | Simplicity  | Fixed | `ended` repeats `markEnded`                             |
| F3 | Low      | Comments    | Fixed | Nothing in the code says how each agent's order is kept |

## F1. The data socket is one function that does five things

Medium · Readability · Fixed · `internal/ui/socket.go:36`

`state` was a 110-line function. It set up the connection and held a
`sendState` closure over a `workspaces` variable that the loop reassigned. It
also did the reset on `select`, the batching of changes, and the timer flush.
To follow any one step, a reader had to keep all the others in mind.

### Fix

A `dataSocket` type now holds each connection's state. Each step is one method:
`sendState`, `selectSession`, `add`, `flush`, and `sendTranscript`. `state`
keeps only accepting the connection, subscribing, and the select loop. Behavior
is unchanged.

## F2. `ended` repeats `markEnded`

Low · Simplicity · Fixed · `internal/session/runtime.go:318`

`ended` read the agent, set it to exited with attention cleared, and saved it.
`markEnded` does the same thing.

### Fix

`ended` calls `markEnded`, then reads the session by `l.session` for the
relaunch check. `markEnded` doesn't use the runtime, so it is now a plain
function.

## F3. Nothing in the code says how each agent's order is kept

Low · Comments · Fixed · `internal/session/feed.go:64`

Each agent's changes are published in order only because every caller that
saves items holds `r.mu` around `save`, and `event` publishes streaming items
under `r.mu`. No comment said so, so a later change could break the order
without anyone noticing.

### Fix

A comment on `save` says what it publishes and when, and gives this reason.

## Checks run

- `go test -race ./...` on the commit passed.
- `gofmt -l` and `go vet ./...` were clean.
- `db.Tx(` appears only in `save`. `PutEntry` appears only in `putItem` and in
  `internal/store`.
- After the fixes, `make check` passed: the browser tests, vet, formatting, and
  all Go tests with the race detector.

## Verdict

The feed and the data socket are correct and follow the protocol. Every write
is published after it commits. Each agent's changes are published in order.
Subscribing before the reset loses no change. F1 through F3 are fixed.
`scripts/dev` still sets `HYPERCUBE_DEV_RELOAD`, which nothing reads now. That
file belongs to the browser half.
