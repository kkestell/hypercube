# Native macOS notifications review

## Scope and coverage

Staged changes for `agents/plans/2026-11-03-002-native-notifications.md`, reviewed against the approved behavior: agent attention requests and finished turns' replies notify while the Hypercube window lacks keyboard focus, nothing native shows while it has focus, and a click raises the window on that session. Read whole: `internal/session/alerts.go` and its test, `internal/desktop/notify.go`, `notify.m`, and the touched `app.m`, `desktop.go`, `desktop.h`, the `cmd/hypercube/main.go` wiring, `notifications.js` and its test, `main.jsx`, the `Makefile`, `cmd/hypercube/Info.plist`, `scripts/dev`, `scripts/install-service`, and the four docs. Traced against `feed.go` (subscription and publishing), `event.go` (`eventLocked`, `applyEvent`), `transcript.go` (`Requests`), `restore.go` (startup attach and replay order), `store.LastText`, `ui/snapshot.go` (`show`), `ui/socket.go` (coalescing), `window.m` (`show`, `dispatch:detail:`), and `chooseSession`.

Gaps:

- No live native run in this review: focus suppression, a request banner, and a click selecting the session in the real window remain unexercised, as the work log records. The click path was traced in code.
- Launch Services behavior with several bundles sharing an identifier (`bin/Hypercube.app` in each checkout plus the installed copy) was not tested.

## Summary

| ID | Severity | Category    | Fix       | Title                                                    |
| -- | -------- | ----------- | --------- | -------------------------------------------------------- |
| F1 | Medium   | Performance | Fixed     | Alert watcher rescans every session on each streamed chunk |
| F2 | Low      | Correctness | Not fixed | Notifications show the generic application icon         |

## F1. Alert watcher rescans every session on each streamed chunk

Medium · Performance · Fixed · `internal/session/alerts.go:35`

`WatchAlerts` ran a full read on every `Change`. Streamed text deltas publish a `Change` per harness event without a write, and each read queried `LastText` for every session in every workspace. While any agent streamed, the app process repeated a whole-database scan per chunk, and a reader that fell behind its 1024-change buffer was dropped and resubscribed.

### Evidence

A benchmark of the read's queries against a copy of the user's database (121 sessions, 7,676 transcript entries) took 6.1 ms per read; listing sessions without `LastText` took 1.3 ms. `eventLocked` returns before `save` for an event that only appends to pending items, so those changes alter neither `Requests` nor any session's activity, unseen flag, or last reply. An opened attention item always goes through `save`, which publishes an empty `Change` after commit, with `l.pending` already updated.

### Fix

The watcher skips changes that name a session (streamed items) and reads only on the empty `Change` each write publishes or after resubscribing. `LastText` runs only for idle sessions that are unseen or were busy at the previous read, which are the only sessions that can produce a reply alert.

## F2. Notifications show the generic application icon

Low · Correctness · Not fixed · `cmd/hypercube/Info.plist:1`

The bundle has no `CFBundleIconFile` or `Resources`, so macOS draws its generic application icon on Hypercube's notifications. Adding an `.icns` to the bundle would fix it; how it looks is the user's call.

## Checks run

- `go test -race -count=3 -run Alert ./internal/session` after the fix — passed.
- `make check` after the fix — passed (bundle build and signing, stale-dist check, bun tests, vet, gofmt, `go test -race ./...`).
- Read-only benchmark of the watcher's queries on a backup copy of the user's database in a temporary directory; the scratch benchmark and the copy were deleted.

## Verdict

The design meets the approved behavior: the startup baseline, the busy-to-idle check that survives the page marking a visible session seen, the focus decision on the main queue at delivery time, the click route through `show` and `hypercube-notification`, and the bundle, symlink, install, and dev-identity packaging all trace correctly. F1 is fixed. F2 is cosmetic and left for the user. Ready once the live native checks the work log lists are run.
