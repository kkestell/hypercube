# Review: worktree sessions start anywhere and are removed when no work would be lost

- Task ID: 2026-11-09-008-worktree-start-and-safe-removal
- Work log (repository-relative): `.agents/work/2026-11-09-008-worktree-start-and-safe-removal.md`
- Plan (repository-relative): `.agents/plans/2026-11-09-008-worktree-start-and-safe-removal.md`, blob `3513c30fa4431709cbd4742ecd24e967f8064e14` (matches the log)

## Round 1

- Author: xreview worker p1-review (Hypercube child session of 777ef702), Claude Code, claude-opus-5-5, native conversation ID `a58716f7-b64e-461b-a3e3-37cb246f7a4c`
- Code base: 1273b36947232e3b9a436111a7f9d08cb35c6c2a
- Reviewed Code head: 8bb30c99d1d440ed3c1c8f5347bb8b96ff1e84c6 (single implementation commit; base is its parent)
- Checkout: `/Users/kyle/projects/hypercube/.hypercube/777ef702` at 85ba5808 (Code head plus the work-log commit only), clean status, no untracked files
- Verdict: **changes-required**. One correctness finding (F1) in the unarchive rollback; everything else in the request and plan is implemented and tested as specified.
- Independence: I did not write or plan this change. I read the full diff, the touched files, and their callers.

### Findings

**F1 (required, low frequency): a failed unarchive that restored a tip no kept ref reaches cannot be retried.** `internal/session/removal.go:89-91` (`Unarchive`, the rollback after `r.save` fails).

When `restore` is true, the branch is recreated at `s.Tip`, and the rollback calls `RemoveWorktree(checkout, s.Dir, s.Branch, base)` with `base = s.Base`. If `s.Tip` is still in the object database but no longer on `HEAD`, `Base`, or a remote-tracking ref, `Removable` refuses. That happens when the pushed branch was deleted and pruned after archiving, for example after a squash-merged pull request. The worktree and branch stay, the row stays archived with its `Tip`, and every retry fails in `AddWorktree`, because `git worktree add -b` refuses an existing branch. The user has to remove the branch by hand. The existing contract, which `TestFailedUnarchiveCanBeRetried` names, is that a failed unarchive leaves the session retryable.

The refusal protects nothing. Before the unarchive, the archived row's `Tip` was the only reference to that commit. Removing the just-created branch returns to exactly that state, and the row still records `Tip` because the save failed.

Smallest correction: treat the restored tip as the kept commit for this rollback, so `Removable` still checks the worktree for uncommitted files, e.g. `workspace.RemoveWorktree(checkout, s.Dir, s.Branch, cmp.Or(start, base))` (`start` is `s.Tip` when restoring and `""` otherwise). Then move `start` out of the `if base != ""` block, or derive it again there. Update the comment, and drop the work log's first limitation. A regression test is optional: injecting a save failure needs a seam the fixture lacks, and the change is mechanical.

### Limitations noted in the work log

1. Unarchive rollback: a real defect. See F1.
2. `internal/report/report.go:136` ("A child session with a worktree ends once its branch is merged into HEAD"): not a defect. Merging into the parent's branch is still sufficient for `EndChild`, and the instructions direct children's work through merges. Pushed child branches are an extra allowance agents need not be told about. Phase 2 should revisit the top-level wording at `report.go:134` ("made from the original checkout", "reaches the original checkout's branch when the user asks you to merge it") if review sessions are not meant to merge.

### Notes (non-blocking)

- `workspace.unkeptCommits` (`internal/workspace/workspace.go:404`) lists "the session's starting commit" for children too, which have none. `EndChild` refusals therefore name an option that does not apply. The message is harmless, but a child-specific wording would read more accurately.
- For phase 2: `MergeStatus` counts `HEAD..branch`. A session started at a pull request's head will show the pull request's commits as unmerged, and the Git panel will offer **Merge into …** right away. This phase does not expose `start` in the browser, so nothing shows it yet.

### Assessment of the difficult invariants

- Reachability rule: `unkept` runs `rev-list --count <tip> --not HEAD [<base>] --remotes` in the right root: the checkout for top-level sessions, the parent's worktree (or the checkout when that worktree is gone) for children. It checks both `refs/heads/<branch>` and the worktree's `HEAD`, so detached commits are covered, and `RemoveWorktree`'s `branch -D` stays safe. Callers pass `Base` only for top-level sessions; children carry an empty `Base`.
- Start point: `AddWorktree` resolves `start^{commit}` with `--end-of-options` before touching excludes or branches, and passes the resolved ID to `worktree add`. `Base` comes from the new worktree's `HEAD`, so it is always a commit ID. `start` without `worktree` is refused before any side effect.
- Archive tip: read after the final `checkWorktrees`, with writers stopped, and before any removal. It is saved in the same transaction that ends the row. Pre-change archived rows and checkouts that are gone get an empty `Tip`, and unarchive then falls back to `HEAD` and resets `Base`, as the plan specifies.
- Schema: the column is in `schema`, every insert, update and select, and an idempotent `pragma_table_info` migration in the `Open` transaction. `TestOpenAddsTipToSavedSessions` uses `testdata/current-schema.sql`, which lacks `tip`, so it exercises the migration. Every `SaveSession` caller reads the row in its transaction, so no write clears `Tip`.
- Docs (`storage.md`, `lifecycle.md`, `usage.md`) match the implemented rule and are declarative.

### Verification

- Reused: `make check` at Code head 8bb30c9, macOS, git 2.56.0, exit 0 (`/tmp/p1-work-make-check-head.log`, no `FAIL` lines; Go package results are cached, and the cache is keyed on source content, so they apply to this tree). The tests cover every plan verification item: `TestAddWorktreeAtStart`, `TestRemovableKeepsStartAndRemoteCommits`, `TestWorktreeSessionAtStart`, `TestArchiveRestoresPushedWork`, the missing-tip fallback in `TestUnarchive`, and `TestOpenAddsTipToSavedSessions`. I read their assertions, not just their names.
- Run: none. F1 follows directly from `Removable`'s rule and `git worktree add -b` refusing an existing branch, so a fresh run would add no evidence.

### Limitations

- F1 has no automated reproduction. It rests on code reading.
