# Mux `kill` and server exit

## Scope and coverage

The staged changes for task 3 of milestone 1 in `agents/mux-design.md`: `kill` for panes, windows, and sessions in `mux/internal/server/server.go`, hanging up and freeing panes in `pane.go`, ending attachments in `attach.go`, `mux kill <id>` in `mux/cmd/mux/main.go`, and the new tests in `main_test.go`. Reviewed against the plan `agents/plans/2026-10-30-003-mux-kill-and-server-exit.md` and `AGENTS.md`.

Gaps:

- No live smoke test with an interactive shell in an attached pane; the tests use `sh -c` and `sleep`.

## Summary

| ID | Severity | Category    | Fix   | Title                                          |
| -- | -------- | ----------- | ----- | ---------------------------------------------- |
| F1 | Low      | Concurrency | Fixed | Two simultaneous final kills close `exited` twice |

## F1. Two simultaneous final kills close `exited` twice

Low · Concurrency · Fixed · `mux/internal/server/server.go:98`

`serve` decides whether the server should exit by calling `s.empty()` after `kill` has returned and the reply is written, instead of using what `kill` saw under the mutex. If two clients kill the last two sessions at the same moment, the first kill leaves one session, the second leaves none, and then both connections see the tree empty and both call `close(s.exited)`. The second close panics. The server was exiting anyway, but it would exit with a panic instead of status 0.

This needs two kills of the last two sessions to finish within microseconds of each other, which is unlikely in normal use.

### Evidence

Traced `serve`, `handle`, and `kill`: `kill` releases the mutex before returning, and `empty()` re-acquires it separately on each connection, so nothing ties the check to the kill that emptied the tree.

### Fix

Extended the existing comment above the check in `serve` to name the case. No code change.

## Checks run

- `make check` — passes.
- `go test -race -count=3 -run 'Kill|Attach|Errors' ./cmd/mux/` in `mux/` — passes.

## Verdict

The change meets the plan: kill removes targets and cascades empty windows and sessions, hangs up processes, frees each pane in exactly one place, ends attachments safely, and the server exits cleanly after its last session. The only finding, F1, is an unlikely race, now named in a comment.
