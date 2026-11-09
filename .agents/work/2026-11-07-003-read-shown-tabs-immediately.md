# Read shown tabs immediately

## Plan

`agents/plans/2026-11-07-003-read-shown-tabs-immediately.md`

## Summary

Shown unread file and diff views initiate their own reads. Explicit opens of
already-loaded content retain their immediate refresh behavior. Readiness treats
empty content, errors, previews, and deleted files as loaded.

## Checks run

- `bunx tsc -p .` and `bun test` in `internal/ui` — passed, 359 tests.
- `GOCACHE=/tmp/hypercube-shown-tabs-go-cache make check` — passed.

## Session-switch timing

Measured in the installed desktop app (Electron) on macOS, connected by remote
debugging. Each run used an isolated state folder holding a copy of the user's
database with every session ended, so no agent launched. It also used the
user's saved desktop view state: the Docs session (`1207f1d2`) with
`agents/development.md` active and the Files panel open, in a 1577×800 window.
A real mouse click switched from another session to Docs, 10 times per case,
with a random 0.7–1.3 s pause between switches. The time is from pointer down
to the frame showing the final content.

- Before: the installed backend at 601b07a, port 4291.
- After: this worktree's staged changes, built into a separate binary, port 4292.

| Case | Before (ms) | After (ms) |
| --- | --- | --- |
| Docs, file tab active | 153 386 395 520 647 770 871 986 997 1018 (mean 674, median 770) | 39 39 43 43 43 44 44 44 46 54 (mean 44, median 44) |
| Docs, transcript tab active (99 rows, 3 images loaded) | 85–95 (mean 92) | 86–103 (mean 94) |
| Restored inactive file tab, selected after switching | not measured | 28 33 32 37 34 |

Request timing on the file tab, relative to the click:

- Before: the first `/file` started at 300–1000 ms. It always followed a poll's
  `/files/tree` and `/changes`, which take 40–55 ms.
- After: `/file` started at 11–17 ms, alongside the Files panel's
  `/files/tree` and `/changes`, and finished 1–2 ms later.

The polls that follow still run their tree, changes, and file requests in
order, as before.

Limitations:

- A restored diff tab could not be checked by hand, because the Docs checkout
  had no changes. The DOM tests cover it.
- The runs were on the user's machine with their live agents running, so
  background load varied between runs.
- The transcript-tab time includes about 42 ms of backend transcript loading,
  which this change does not address.
