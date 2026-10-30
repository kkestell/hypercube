# Review of mux exits and `list`

## Scope and coverage

I reviewed the staged changes for task 2 of milestone 1 in `agents/mux-design.md`: exit recording in `mux/internal/server/pane.go`, `spawn` into an existing session or window and the `list` handler in `mux/internal/server/server.go`, the list types in `mux/client/client.go`, `mux spawn` options and `mux ls` in `mux/cmd/mux/main.go`, and the new tests. I read them against the plan `agents/plans/2026-10-30-002-mux-exits-and-list.md`, the work log, the design, and `AGENTS.md`, and traced the attach path to confirm an exited pane can still be attached, typed into, and detached from.

Gaps:

- Later milestone 1 tasks are out of scope. Nothing in this change blocks them.

## Summary

| ID | Severity | Category | Fix   | Title                                                  |
| -- | -------- | -------- | ----- | ------------------------------------------------------ |
| F1 | Medium   | Testing  | Fixed | New `TestErrors` spawn cases fail for the wrong reason |

## F1. New `TestErrors` spawn cases fail for the wrong reason

Medium · Testing · Fixed · `mux/cmd/mux/main_test.go:378`

The three new spawn cases in `TestErrors` sent no `Env`. With no `PATH`, `startPane` fails with `sleep: not found in PATH`, so each case returned an error even when spawn ignored its IDs. The test could not catch a broken unknown-session, unknown-window, or wrong-session check.

### Evidence

Replacing the wrong-session check in `spawn` with `if false` left `TestErrors` passing.

### Fix

The three cases now pass `PATH=/usr/bin:/bin`, with a comment that only their IDs can fail them. With the same mutation, `TestErrors` now fails on "spawn into another session's window".

## Checks run

- Mutation of the wrong-session check in `spawn`: `TestErrors` passed before the fix and fails after it. The mutation was reverted.
- Scratch tests, removed afterward: attaching to a pane whose process exited succeeds, and typing into it and then pressing Ctrl-b d detaches without blocking.
- `make check` from the repository root: passes.

## Verdict

The change meets the plan. Exits, `list`, `mux ls`, and spawning into a session or window are correct for normal use. F1 left the new ID checks untested and is fixed.
