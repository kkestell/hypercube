# Work review: 2026-11-09-013-discard-worktree-work

## Round 1

- Task ID: 2026-11-09-013-discard-worktree-work
- Mode: work
- Author: reviewer child of Hypercube session 3703b04c (Claude Code, Opus 5.5
  `claude-opus-5-5`, native conversation
  ed3d61a0-59b4-4de1-b69d-012c194ba728). I did not write this code; the
  implementer was a Sonnet 5.5 worker (34031658-bbc6-466d-adde-0f8e62ac32d4).
  I also wrote this unit's plan review, so finding 1 is a gap in my own plan
  review as well as in the plan.
- Work log: `.agents/work/2026-11-09-013-discard-worktree-work.md` (commit
  `d921630`)
- Plan: `.agents/plans/2026-11-09-013-discard-worktree-work.md`, blob
  `274bf1c4944c125a77282850c99f3364248845bf` (matches the execution
  decision). Shared design blob `4e2f6244ffb183e3ccb5dd62d97621fab35f5bed`.
- Code base: `90476495aaff42a549a4b7b5d7099b7a5b3c23be`
- Code head: `abec2f7fdeb949bed993a080292fe378d36fff70`. The base is an
  ancestor of the head. The checkout is at `d921630`, which adds only the
  work log. The working tree is clean.

### Verdict

`changes-required`. Both plan-review corrections are implemented and tested,
and the code is easy to follow. However, Discard fails in the state an
interrupted agent most often leaves behind (a conflicted merge or rebase),
and two doc passages break the documentation rules.

### Findings

1. **Medium — Discard fails while a merge or rebase is in progress
   (`internal/workspace/workspace.go`, `Discard`).** `git switch
   --discard-changes` refuses with "cannot switch branch while merging" or
   "cannot switch branch while rebasing". I reproduced both with Git 2.50.1.
   Agents merge inside these worktrees as part of normal work: the agent
   instructions (`report.go`) tell a parent that its children's branches
   merge into its branch, and **Merge into …** asks the agent to merge. Discard
   stops the family's agents first, so an agent stopped mid-conflict leaves
   `MERGE_HEAD` or a `rebase-merge` directory behind. Discard then fails, and
   the conflicted files keep Archive and Delete refused, so the user has no
   way in the app to throw the work away. The plan did not cover this, and my
   plan review recommended `git switch` without checking it. **Fix (verified
   with Git 2.50.1):** before the switch, run `git reset --quiet --hard` in
   `dir`. That clears merge, cherry-pick, and revert state. Then, when `git
   rev-parse --git-path rebase-merge` or `--git-path rebase-apply` exists, run
   `git rebase --quit`. After those steps the existing switch and cleans
   succeed and the worktree is clean. Add workspace tests for a conflicted
   merge and a stopped rebase, each ending with `Removable` passing.

2. **Low — `docs/lifecycle.md:46-49`: the Discard sentences are joined to the
   Archive/End child paragraph.** There is no blank line before
   `**Archive Session**`, so the paragraph's "These actions can also remove
   managed worktrees" now includes Discard, which removes nothing. The
   passage also repeats the exact effect list (starting commit, uncommitted
   and untracked files, later commits) that the new `docs/storage.md` row
   states. `documentation.md` says to keep each fact in one place, and this
   page already links to `storage.md#session-cleanup`. **Fix:** make Discard
   its own paragraph with one sentence of explanation, for example "**Discard
   All Changes** throws away a worktree session family's work and keeps its
   sessions active." Leave the details to `storage.md`.

3. **Low — `docs/usage.md:139-144` repeats the same effect list and puts the
   destructive effect after the command.** `documentation.md` says "Put
   destructive effects before the command" and to keep each fact in one
   place. **Fix:** say first that it deletes the session's and its children's
   uncommitted files and commits since the session started. Then give the
   step and the follow-up (Archive or Delete then pass), and link to
   [cleanup behavior](storage.md#session-cleanup) for the exact list. The
   `storage.md` row and the `troubleshooting.md` step are correct and need no
   change.

### Independent assessment

- **Plan-review corrections.** `workspace.Discard` refuses unless
  `isWorktree(dir)`. `TestDiscardRefusesAFolderThatIsNotAWorktree` puts a
  plain folder under `.hypercube/` and checks that the checkout's `HEAD` and
  an uncommitted file are unchanged. `Discard` takes the branch and uses
  `switch --force-create`. `TestDiscardReturnsToTheSessionBranch` switches to
  another branch and commits, then checks that the session branch is checked
  out at `Base`, the other branch keeps its commit, and `Removable` passes.
- **`removal.go` change (not listed in the plan).** It extracts `treeName`
  from `checkWorktrees` so that `Discard`'s error names a tree the same way,
  as the plan's Failure decision requires. The behavior is identical, and
  sharing the helper is the right way to meet that requirement.
- **Runtime.** `Runtime.Discard` follows `removeFamily`'s sequence:
  `claimRemoval`, the three refusals, `waitChildCreations`, re-reading the
  children, stopping launches with `l.root == id`, `familyWorktrees`, then
  `refreshMerges`. `releaseRemoval` is deferred, so it runs on every return.
  Every tree resets to the session's `Base`. Children in the parent's folder
  have no branch and are excluded, as decided. The session test covers a
  parent and a worktree child, each with a commit and an untracked file:
  Archive refuses before the discard and succeeds after it, agents are not
  running, and no session is ended.
- **Git function.** `TestDiscard` covers a commit, a tracked change, an
  untracked folder, an untracked nested repository, an ignored file that
  survives, and a submodule with a new commit and an untracked file. The
  superproject's status ends empty and `Removable` passes.
- **Browser.** The **Discard All Changes** label is VS Code's. The `discard`
  icon is Lucide `Undo2`, added to the existing icon map. The item is
  destructive and placed before **Archive Session** only for active
  top-level worktree sessions. The confirmation uses `askToConfirm` with the
  branch clause only when `merge` is present. `sidebar.test.tsx` checks the
  menu for worktree, checkout, child, and archived sessions, and that Cancel
  posts nothing and confirming posts `{action: 'discard', id}`.
- **`errDeleting` text.** It changed to "another change to the session is in
  progress". Callers use `errors.Is`.
- **Clarity.** `discard.go` and `Discard` are short and read in order. The
  touched files remain easy to follow.

### Verification

- **Reused:** `make check`, exit 0 at `abec2f7`, from `/tmp/check13.log`
  (every Go package `ok`, no `FAIL`). Also reused: the log's focused
  `go test -run Discard` runs for `internal/workspace` and
  `internal/session`, and `bunx tsc -p .` plus `bun test sidebar`.
- **Run:** shell experiments in temporary repositories (Git 2.50.1), because
  Discard's Git behavior is destructive and was not tested for in-progress
  operations:
  - `git switch --quiet --discard-changes --recurse-submodules
    --force-create sess <base>` during a conflicted merge exits 128 ("cannot
    switch branch while merging"). During a stopped rebase it exits 128
    ("cannot switch branch while rebasing").
  - `git reset -q --hard` followed by the switch succeeds after a conflicted
    merge and after a conflicted cherry-pick.
  - `git rebase --quit` followed by the switch succeeds after a stopped
    rebase.
- **Limitation:** the menu and confirmation were tested only in happy-dom.

## Round 2

- Author: same reviewer as round 1 (Claude Code, Opus 5.5
  `claude-opus-5-5`, native conversation
  ed3d61a0-59b4-4de1-b69d-012c194ba728)
- Work log: `.agents/work/2026-11-09-013-discard-worktree-work.md` (Repair 1,
  commit `442f273`)
- Code base: `90476495aaff42a549a4b7b5d7099b7a5b3c23be` (unchanged)
- Code head: `8b34d7e41acf75fc1d254eca914d575dd19271d7`. The previously
  reviewed head `abec2f7` is an ancestor. In `abec2f7..8b34d7e`, the only
  code or doc changes are in `internal/workspace/workspace.go`,
  `internal/workspace/discard_test.go`, `docs/lifecycle.md`, and
  `docs/usage.md`. Everything else is workflow records. The checkout HEAD's
  `internal/` tree is identical to `8b34d7e`, and the working tree is clean.

### Verdict

`accept`. All three round 1 findings are resolved, and the repair adds no new
findings.

### Resolution

| Finding | Status | Evidence at `8b34d7e` |
| --- | --- | --- |
| 1. Discard failed during a merge or rebase | Resolved | Before the switch, `Discard` now runs `git reset --quiet --hard`, then `git rebase --quit` when `--git-path rebase-merge` or `rebase-apply` exists. This is the sequence I verified in round 1. Relative `--git-path` output is joined to `dir`. The doc comment states that an operation in progress is abandoned. `TestDiscardAbandonsAnOperationInProgress` creates a real conflict for both `merge` and `rebase`, requires that the operation actually conflicted, then checks `HEAD == base`, empty porcelain status, no "in progress" or "rebase" in `git status`, and `Removable` passing. |
| 2. `lifecycle.md` Discard text joined to the Archive paragraph and repeating storage's list | Resolved | Discard is now its own one-sentence paragraph ("throws away a worktree session family's work and keeps its sessions active"). "These actions can also remove managed worktrees" again covers only Archive and End child. |
| 3. `usage.md` repeated the effect list and put the destructive effect after the command | Resolved | The paragraph now starts with what is permanently deleted, then gives the step and the follow-up, and links to `storage.md#session-cleanup` for the exact list. The `storage.md` row stays the one detailed source. |

The rest of round 1's assessment still applies: the runtime, browser, and
`removal.go` code did not change.

### Verification

- **Run:** `go test ./internal/workspace -run Discard -count=1` at the
  checkout. Its `internal/` tree is identical to `8b34d7e`. Result: `ok`.
  I ran it because it is cheap and directly exercises the repaired
  destructive path.
- **Reused (work log, Repair 1):** `go test -race ./internal/workspace
  ./internal/session` pass at `8b34d7e`. Round 1's reused `make check` still
  covers the unchanged packages and browser code. No other package depends
  on the change except through `Discard`'s unchanged signature.
