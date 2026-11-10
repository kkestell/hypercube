# Plan review: 2026-11-09-013-discard-worktree-work

## Round 1

- Task ID: 2026-11-09-013-discard-worktree-work
- Mode: plan
- Author: plan-reviewer child of Hypercube session 3703b04c (Claude Code,
  Opus 5.5 `claude-opus-5-5`, native conversation
  ed3d61a0-59b4-4de1-b69d-012c194ba728). Independent of the planner
  (ed140e50-7ea6-40f1-84c8-faaf6329d964).
- Plan: `.agents/plans/2026-11-09-013-discard-worktree-work.md`, blob
  `41d554f586a632b68266c8e400e0287a6ca19791`
- Guidance blobs:
  - `.agents/designs/2026-11-09-012-worktree-branch-choice-and-discard.md`
    `4e2f6244ffb183e3ccb5dd62d97621fab35f5bed`
  - `AGENTS.md` `da2afb1eac69ad0b8f99faf9d986246965fcc051`
  - `docs/dev/development.md` `58c66cdf38859ff4e8b3e558f3d65d144dbc95c3`
  - `docs/dev/architecture.md` `6b837af568056e61d0ffe85ed985ac11f1d45139`
  - `docs/dev/documentation.md` `80e857b3690cbf19b896edaa2231d70e2f4c4513`
- Source HEAD: `8e3abf8868511cfddcc639e053834c52dcab89d3` (code unchanged
  since `aadeae94d694eafe119f44800afa6932983b8742`)

### Verdict

Work size: right-sized. Detail: appropriate, but two settled Git decisions
are wrong. Readiness: not ready. Action: `resolve-decisions`.

### Findings

1. **Required: `Discard` can reset the user's main checkout.** Plan step 1
   runs `git reset --hard` and `git clean` "in `dir`", and the plan says a
   missing folder "fails with Git's error". The risky case is different: a
   directory at `.hypercube/<id>` that exists but is not a worktree root.
   This happens when the worktree's `.git` file is gone or the worktree was
   pruned while its folder stayed. Git then finds the enclosing checkout.
   `reset --hard <Base>` moves the checkout's branch to `Base` and deletes
   its uncommitted changes. The existing code guards removal for this reason
   (`isWorktree`, `internal/workspace/workspace.go:583`, used by `Removable`
   and `RemoveWorktree`). **Correction:** `Discard` refuses unless
   `isWorktree(dir)`, with an error naming the folder. Add a test: a plain
   directory under `.hypercube/` inside a checkout that has a commit and an
   uncommitted file. `Discard` returns an error, and the checkout's branch
   and file are unchanged.

2. **Required: reset the session's branch, not whatever `HEAD` is on.**
   `git reset --hard <Base>` moves the branch checked out in the worktree.
   If the agent ran `git checkout -b fix` or detached `HEAD`, discard rewinds
   `fix` and loses its commits (only the reflog keeps them). The session's
   own branch keeps its commits, so `Removable` still refuses and Archive or
   Delete still fails. That defeats request item 3. The plan's docstring says
   "its branch moves there", but the command moves a different branch.
   **Correction:** `Discard(dir, branch, commit)` runs `git switch --quiet
   --discard-changes --recurse-submodules --force-create <branch> <commit>`,
   then the cleans. I verified with Git 2.50.1 that in a worktree switched to
   another branch with a staged file, this checks out `<branch>` at
   `<commit>`, drops the staged change, and leaves `fix` and its commit
   intact. Untracked files remain for `clean`. Callers pass
   `tree.session.Branch`. Add a session or workspace test: commit, switch to
   another branch in the worktree, commit again, then `Discard`. The session
   branch and `HEAD` equal `Base`, the other branch keeps its commit, and
   `Removable` passes.

### Choices checked and agreed

- **Reset target is `Base` across the family.** A child's kept refs are the
  parent worktree's `HEAD` and remote-tracking branches
  (`Removable`/`keptRefs`). Once the parent is at `Base`, a child at `Base`
  has nothing unkept. Commits already merged into the checkout or pushed
  stay reachable there, so only work the user asked to discard is lost.
- **Stopping agents** reuses `removeFamily`'s
  `claimRemoval`/`waitChildCreations`/`stopLaunches`/`releaseRemoval`
  sequence. `Restart` and `park` already stop launches without ending the
  session, so leaving the family active matches existing behavior. Leaving
  panel shells running is a fair boundary. Renaming `errDeleting`'s text is
  needed: during the claim, `wake` returns it to viewers, and no code or test
  matches the old text.
- **Ignored files stay.** They never block removal, and they include
  `.worktreeinclude` copies and `/.hypercube/`. `clean -ffd` removes nested
  repositories, which `uncommitted` counts. Child worktrees live under the
  workspace's `.hypercube/`, not inside the session worktree, so the clean
  does not reach them.
- **Submodule local commits are kept**, and `Removable` still names that
  submodule. This is a reasonable limit: deleting branches in another
  repository would go beyond the request.
- **UI:** **Discard All Changes** is VS Code's Source Control label, and
  Lucide `Undo2` matches its discard arrow. Placing it in the
  Archive/Delete group with a destructive style and an `askToConfirm`
  confirmation follows `deleteSession`'s existing form. Offering it on every
  active top-level worktree session is justified because children's changes
  are not in the snapshot.
- **Docs targets exist:** the `docs/lifecycle.md` action table and
  `docs/troubleshooting.md` "Archive or delete is refused".

### Normalization

Target: the default 600–1,500 words (no curated reference set). Measured
1,201 words (66 in fenced code) with `plan_metrics.py`, inside the band.
Comparable to the reference "restored file loading plus refresh ownership":
one runtime action reusing an existing lifecycle, one Git function, one
route, and one menu item. It is one coherent mechanism and needs no split.
The two corrections replace one command and add one guard, so they will not
change the size much. Confidence: high.

### Handoff

To proceed, revise step 1, step 2, and Verification:

- `workspace.Discard(dir, branch, commit string) error` refuses when
  `!isWorktree(dir)`. It then runs `git switch --quiet --discard-changes
  --recurse-submodules --force-create <branch> <commit>`, `git clean -ffdq`,
  and the submodule clean.
- `Runtime.Discard` passes `tree.session.Branch` and `s.Base`.
- Add the two tests from findings 1 and 2.

## Execution decision

- Author: parent, Hypercube session 3703b04c (Claude Code, Opus 5.5, native
  conversation 059787ce-5ccd-4e61-a729-78b16c865c72)
- Finding 1 (refuse a non-worktree dir): resolved. Plan step 1 and Decisions
  now require `isWorktree(dir)` before any Git command, with the
  plain-directory test that checks the main checkout's branch and uncommitted
  file are unchanged.
- Finding 2 (reset the session's branch): resolved. `Discard(dir, branch,
  commit)` runs `git switch --quiet --discard-changes --recurse-submodules
  --force-create <branch> <commit>` before the cleans; `Runtime.Discard`
  passes `tree.session.Branch` and `s.Base`; the switched-branch test is
  added. The commands are the reviewer's verified correction verbatim.
- Final plan blob: `274bf1c4944c125a77282850c99f3364248845bf`. Guidance blobs unchanged: design `4e2f6244ffb183e3ccb5dd62d97621fab35f5bed`,
  `AGENTS.md`, `docs/dev/*.md` as recorded in Round 1.
- Decision: proceed.
