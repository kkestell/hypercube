# Review of Go runtime fixes

## Scope and coverage

Reviewed `git diff main...eae4d9f`, covering commits `748adde`, `1bc6c79`,
and `eae4d9f`. Read all ten touched files in full and traced their callers,
process lifetimes, runtime locking, transcript writes, and regression tests.
The review covered blocked plugin shutdown, starting a session during deletion,
and the published position of a replaced transcript item. No additional
correctness, race, leak, or readability findings were confirmed.

No live application smoke test was run, as requested by the lead. Plugin
behavior was checked through isolated stand-ins.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Testing | Fixed | Replacement test assumes event ordering |

## F1. Replacement test assumes event ordering

Medium · Testing · Fixed · `internal/session/feed_test.go:173`

The test sent `!finish` immediately after `!work`, then required an exact
transcript with the working item's position before the finish input. The
stand-in replies before emitting its working event. `Input` can therefore
return and reserve the finish input's position before the runtime processes
that event. The resulting transcript is valid, but the test times out before
it checks the replacement behavior.

### Evidence

The failure occurred in the harness/session/store race suite and repeatedly
in ten focused runs. `internal/session/input.go:52` reserves the input's
position before sending it, while
`internal/harness/harnesstest/harnesstest.go:240` replies to `!work` before
emitting its item.

### Fix

As directed by the lead, wait until the transcript contains the unfinished
working item before sending `!finish`. This establishes its position before
the next input and preserves the regression test's comparison between the
published replacement position and the saved one.

## Checks run

- Before the fix, `go test -race ./internal/harness ./internal/session
  ./internal/store`: harness and store passed; session failed on F1.
- Before the fix, `go test -race ./internal/session -run
  'Test(ReplacedItemKeepsItsPublishedPosition|DeletingSessionCannotStart)$'
  -count=10`: reproduced F1 repeatedly.
- After the fix, the same focused command with `-count=20`: passed.
- `make check`: passed with a temporary `GOCACHE` under `/tmp`. The first
  attempt was blocked by sandbox access to the shared Go build cache. The
  successful run covered builds, generated UI consistency, 113 browser unit
  tests, vet, formatting, and all Go tests with the race detector.
- `git diff --check main...HEAD`: passed during review.

## Verdict

F1 is fixed. The three production fixes satisfy the reviewed behavior, and
the requested validation passes.
