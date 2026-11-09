# Work log: worktree sessions start anywhere and are removed when no work would be lost

- Task ID: 2026-11-09-008-worktree-start-and-safe-removal
- Plan (repository-relative): `.agents/plans/2026-11-09-008-worktree-start-and-safe-removal.md`, blob `3513c30fa4431709cbd4742ecd24e967f8064e14`; no guidance blobs
- Author: xwork worker p1-work (Hypercube child session of 777ef702), Claude Code, claude-opus-5-5
- Status: complete
- Code base: 1273b36947232e3b9a436111a7f9d08cb35c6c2a
- Code head: 1f8799f470635cb964e43b9499ad9b808b842cd9
- Implementation commits: 8bb30c99d1d440ed3c1c8f5347bb8b96ff1e84c6 (work), 1f8799f470635cb964e43b9499ad9b808b842cd9 (fix)

## Changes

Responsibilities as in the plan.

- `internal/workspace/workspace.go`: `AddWorktree(root, dir, branch, start)` resolves `start` (empty means `HEAD`) with `rev-parse --verify --end-of-options <start>^{commit}` before creating anything. `Removable`/`RemoveWorktree` take `base`; the private `unkept` runs `rev-list --count <tip> --not HEAD [<base>] --remotes` for the branch and the worktree's `HEAD`. New `BranchTip` (empty when the branch is gone) and `HasCommit` (`cat-file -e`).
- `internal/store/store.go`: `Session.Tip`, `tip` column in the schema and every session insert, update and select; `addTip` migration in `Open` via `pragma_table_info`.
- `internal/session/sessions.go`: `CreateSubmission`/`create` take `start`; a start without a worktree is refused.
- `internal/session/removal.go`: removal passes each session's `Base` (children pass `""`); archive reads the branch tip after the final check and saves it in `recordRemovedFamily`; unarchive restores the branch at `Tip` when `HasCommit` finds it, keeping `Base`, and otherwise starts from `HEAD` and resets `Base`; `Tip` is cleared on unarchive.
- `internal/session/children.go`, `internal/ui/actions.go` (`start` query parameter): pass-through.
- Docs: `docs/storage.md` (Unarchive row, removal paragraph), `docs/lifecycle.md`, `docs/usage.md`.
- Tests: `workspace_test.go` (`TestAddWorktreeAtStart`, `TestRemovableKeepsStartAndRemoteCommits`), `sessions_test.go` (`TestWorktreeSessionAtStart`), `lifecycle_test.go` (`TestArchiveRestoresPushedWork`; `TestUnarchive` now covers the missing-tip fallback), `reopen_test.go` (`TestOpenAddsTipToSavedSessions`).

## Deviations

- `BranchTip` is a named helper for the plan's "read the branch tip; save `""` if the branch is gone".
- In `create`, the rollback after a failed `Head` read passes `start` as `base`, so a worktree started off `HEAD` can still be rolled back.
- Existing tests adjusted: `TestWorktreeSession` matches the new refusal text; `TestFailedUnarchiveCanBeRetried` clears the recorded tip, because only the `HEAD` fallback reads the new worktree's `HEAD`, which is the failure it injects.

## Verification (all at Code head 8bb30c9…, macOS, git 2.56.0)

- `go test ./internal/workspace/`, `go test ./internal/store/`, `go test ./internal/session/ -run 'Unarchive|Archive|WorktreeSession|Delete|Child'`: pass (run during work).
- `make check` from the repository root on the committed tree with a clean status: exit 0 (output in `/tmp/p1-work-make-check-head.log`; an earlier run before the commit, `/tmp/p1-work-make-check.log`, also exited 0).

## Session provenance

| Stage | Name / ID | Harness | Native conversation ID | Model |
| --- | --- | --- | --- | --- |
| parent | 777ef702 (Hypercube session) | Claude Code | 209b0949-92b0-42bc-8596-4341a4bf0785 | Opus 5.5 |
| work | p1-work (child of 777ef702) | Claude Code | 24081c1b-5d43-4bab-8800-82f2facb6744 (from the harness's task-output path for this session) | claude-opus-5-5 |
| review | p1-review (child of 777ef702) | Claude Code | a58716f7-b64e-461b-a3e3-37cb246f7a4c | claude-opus-5-5 |

## Limitations

- The agent instructions in `internal/report/report.go` still say a child's worktree ends once its branch is merged into `HEAD`; that remains true but omits pushed branches. Outside the plan's scope.
- Awaiting independent review of the repair.

## Repair 1

Review: `.agents/reviews/2026-11-09-008-worktree-start-and-safe-removal.md`, round 1 (`changes-required`, commit 0706039a0b74f323fc5c1809aa14f4fd0e056f28).

- F1: `Unarchive`'s rollback after a failed save passes `cmp.Or(start, base)` to `RemoveWorktree`, so a branch restored at the recorded tip is removed with that tip as its kept commit; the uncommitted-file check still applies (`internal/session/removal.go`). No regression test: a failed save has no clean test seam.
- Note: refusals name the session's starting commit only when a base is given (`keptRefs` in `internal/workspace/workspace.go`).

Verification at Code head 1f8799f…, macOS, git 2.56.0:

- `go test ./internal/workspace/` and `go test ./internal/session/ -run 'Unarchive|Archive|WorktreeSession'`: pass.
- `make check` from the repository root on the committed tree with a clean status: exit 0 (output in `/tmp/p1-work-make-check-fix.log`).

## Completion

- Reviews: `.agents/reviews/2026-11-09-008-worktree-start-and-safe-removal.md`, round 1 `changes-required`, round 2 `accept` at Code head 1f8799f470635cb964e43b9499ad9b808b842cd9.
- Final reviewed range: 1273b36947232e3b9a436111a7f9d08cb35c6c2a..1f8799f470635cb964e43b9499ad9b808b842cd9; later commits change workflow records only.
- Checks: `make check` exit 0 at the final Code head (reused, `/tmp/p1-work-make-check-fix.log`).
