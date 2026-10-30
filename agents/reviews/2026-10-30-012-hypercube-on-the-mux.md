# Hypercube on the mux

## Scope and coverage

The staged change for `agents/plans/2026-10-30-011-hypercube-on-the-mux.md`, with its work log: the new `internal/mux` package, every caller that moved off `internal/tmux` (`cmd/hypercube`, `harness`, `hook`, `mcp`, `statusline`, `wait`), the tests, `Makefile`, `go.mod`, `scripts/dev`, `scripts/reinstall`, and the documentation. Reviewed against the plan and the later decision recorded in the work log: Hypercube sets identity labels (agent name, team name, workspace base name) through `label` and attention through `attention`, via `mux/client`, and draws no status of its own. The mux server's `kill`, `list`, `ls`, key, and click behavior was read where the change depends on it.

Gaps:

- The `make dev` smoke test was not rerun; neither fix changes normal runtime behavior.

## Summary

| ID | Severity | Category       | Fix   | Title                                                      |
| -- | -------- | -------------- | ----- | ---------------------------------------------------------- |
| F1 | Low      | Error handling | Fixed | A lead whose launch fails after spawning leaves its window |
| F2 | Low      | Documentation  | Fixed | Usage guide ties pane titles to Hypercube's activity limit |

The labels and attention follow the decision: pane, window, and session labels are single plain spans with the agent name, team name, and `filepath.Base(root)`; attention goes through `attention`; nothing in Hypercube draws progress, bars, or colors in the mux (the ANSI colors in `internal/statusline` are Claude Code's own status line). No tmux leftovers remain outside `mux/` and `agents/`. The tmux mention in `docs/harness-support.md` is accurate: it records the earlier live-model smoke test, which ran on a tmux server.

## F1. A lead whose launch fails after spawning leaves its window

Low · Error handling · Fixed · `cmd/hypercube/main.go:480`

`harness.Launch` returns the spawn reply even when saving `Target` or labeling the pane fails afterward, so the caller can close what it started; `createAgent` does this. `openWorkspace` recorded a lead's window only when `Launch` succeeded, so such a failure left `windows[a.Team]` empty, the cleanup killed nothing, and the lead's wrapper kept running in an untracked window. The next `hypercube` would see the team as not running and start a second lead. Both failures are unlikely.

### Fix

`openWorkspace` records the lead's window from the reply before checking the error, so the existing cleanup closes it. A comment says why.

## F2. Usage guide ties pane titles to Hypercube's activity limit

Low · Documentation · Fixed · `docs/usage.md:49`

The controls section pointed readers to the harness limitations "when interpreting activity". Hypercube no longer shows its busy or idle state; the progress in a pane title is what the harness itself reports to the mux, so the interrupted-turn limitation does not describe what the title shows.

### Fix

Removed the sentence.

## Checks run

- `make check` — passes after the fixes.

## Verdict

The switch to the mux is complete and follows the plan and the labels decision. Two low findings, F1 and F2, are fixed.
