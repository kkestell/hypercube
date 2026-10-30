# Relaunch in the wrapper

## Scope and coverage

Reviewed the staged changes for `agents/plans/2026-10-30-009-relaunch-in-wrapper.md`: `runAgent` and `markExited` in `cmd/hypercube/main.go`, the removed `exited` command, the `openWorkspace` reset, the removed `Launched` and `QuickExits` fields, the removed `pane-died` hook, `Restart`, and `Dead` in `internal/tmux/tmux.go`, the `Launch` comment, and the new integration checks. Checked the code that depends on them: every remaining reference to the removed names, the `pane-died` hook, `respawn-pane`, `store.Exited` readers in `internal/mcp`, the hook writes to `Activity` in `internal/hook`, how panes are killed (`KillPane`, `KillWindow`, `KillSession`, `Respawn -k`), and the user guides that describe relaunches (`docs/usage.md` stays accurate).

Gaps:

- Did not rerun the `make dev` smoke test; the implementer ran it and no fix changed runtime behavior.

## Summary

No findings.

The change does what the plan asks and no more. The relaunch policy moved into one loop with a local counter, the wrapper catches SIGINT without ignoring it, SIGHUP keeps its default so killing a pane, window, or session still ends both processes, and the dead pane keeps showing the harness's status. No speculative or defensive code was added.

## Checks run

- `make check` — passes.
- `grep` for `QuickExits`, `Launched`, `quick_exits`, `pane-died`, `hypercube exited`, `tmux.Restart`, `tmux.Dead`, and `respawn` across `cmd`, `internal`, `scripts`, `docs`, and `README.md` — no stale references.

## Verdict

No findings. The change is ready to commit.
