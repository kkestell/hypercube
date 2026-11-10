# Discard all changes in a worktree session

- Task ID: 2026-11-09-013-discard-worktree-work
- Status: ready
- Source HEAD: aadeae94d694eafe119f44800afa6932983b8742 (run after
  2026-11-09-012 in the same checkout; no interface dependency)
- Author: planner child of Hypercube session 3703b04c (Claude Code, Opus 5.5,
  native conversation ed140e50-7ea6-40f1-84c8-faaf6329d964)
- Shared design: [.agents/designs/2026-11-09-012-worktree-branch-choice-and-discard.md](../designs/2026-11-09-012-worktree-branch-choice-and-discard.md)
  (unit 2 of 2)
- Request item: 3 of the shared design.

## Outcome and boundaries

An active top-level worktree session's menu gains **Discard All Changes**.
After confirmation, Hypercube stops the family's agents and returns the
session's worktree, and each active child's managed worktree, to the commit
the session started from (`Base`). Uncommitted tracked changes, untracked
files and folders (including nested repositories), and the branches' commits
since `Base` are gone. Ignored files stay. The family stays active; its
agents park and wake when next used. **Archive Session** and **Delete
Session** then pass their Git checks, because every tree is at `Base`.

Non-goals: discard for checkout sessions, children on their own, or archived
sessions; removing commits from submodule repositories; telling the agents
what happened (their conversations are unchanged); a combined discard and
archive action.

## Current system

- `Removable`/`checkWorktrees` (`internal/workspace/workspace.go`,
  `internal/session/removal.go`) refuse on uncommitted files, commits not
  reachable from the kept refs, and unpushed submodule commits. A child
  worktree's kept refs are its parent worktree's `HEAD` and remote branches,
  so resetting only the parent would make merged child work unremovable.
- `removeFamily` shows the sequence to reuse: `claimRemoval`,
  `waitChildCreations`, re-read children, `familyWorktrees`,
  `selectLaunches(l.root == id)` + `stopLaunches`, and `releaseRemoval`
  (which re-settles and delivers queued messages).
- Sidebar menu: `sessionItems` (`internal/ui/web/sidebar/sidebar-menus.ts`);
  confirmation: `askToConfirm` (`shared/confirm.ts`); actions:
  `sessions/session-actions.ts`; icons: `shared/icon.tsx` (Lucide).

## Decisions

- **Reset target: the session's `Base`, for every tree in the family.** `Base`
  is a kept ref for the top-level worktree; after the parent is at `Base`, a
  child at `Base` has nothing outside its parent's `HEAD`. Resetting to "the
  last kept commit" instead would need a per-tree search and leaves merged
  history that the user asked to throw away.
- **What is deleted:** `git reset --hard` plus `git clean -ffd` (the second
  `-f` removes untracked nested repositories, which `status` reports and
  `Removable` counts). Ignored files are left: they never block removal, and
  they include `.worktreeinclude` copies such as `.env` and dependency
  folders the session still needs.
- **Submodules:** `--recurse-submodules` on the reset returns submodule
  working trees to the recorded commits, and `git submodule foreach --quiet
  --recursive git clean -ffdq` removes their untracked files, so the
  superproject reports no changes. Commits on a submodule's local branches
  are kept; `Removable` still names that submodule, as today.
- **Agents:** stop the family's launches first, as removal does, so no agent
  writes during the reset. Panel shells keep running; they belong to the
  user. The claim blocks wakes and new shells until the reset ends.
- **Children:** every active child with a managed worktree is reset with the
  family (`familyWorktrees` already lists them). Children in the parent's
  folder share its reset. Ended children have no worktree.
- **Availability:** the item appears for every active top-level worktree
  session (`!parent && !ended && worktree`), not only when its own merge
  status shows changes, because a child's changes are not in the snapshot.
- **Failure:** stop at the first tree that fails and return
  `"<name>: <error>"` (title for the top-level session, `Name` for a child,
  as `checkWorktrees` names them). Trees already reset stay reset; running
  the action again finishes the rest. A missing worktree folder fails with
  Git's error.
- **Claim message:** `errDeleting` now also guards discard, so change its
  text (`internal/session/runtime.go`) to "another change to the session is
  in progress". Callers compare with `errors.Is`; no test matches the text.

## Implementation sequence

1. `internal/workspace/workspace.go`, beside `Removable`:

   ```go
   // Discard returns the worktree in dir to commit: its branch moves there,
   // and its uncommitted changes and untracked files, including those of its
   // submodules, are deleted. Ignored files stay.
   func Discard(dir, commit string) error
   ```

   Runs in `dir`: `git reset --quiet --hard --recurse-submodules
   --end-of-options <commit>` (accepted by Git 2.50), `git clean -ffdq`, and,
   when `.gitmodules` exists, the submodule clean.

2. New `internal/session/discard.go`:

   ```go
   // Discard stops a top-level worktree session's family and returns its
   // worktree and its children's worktrees to the commit it started from.
   func (r *Runtime) Discard(id string) error
   ```

   `claimRemoval(id)` → `defer releaseRemoval(id)`; refuse a child ("only a
   top-level session's changes can be discarded"), an ended session ("the
   session is archived"), and `Base == ""` ("only a worktree session's
   changes can be discarded"); `waitChildCreations`, re-read children; stop
   launches with `l.root == id`; for each `familyWorktrees` tree,
   `workspace.Discard(tree.session.Dir, s.Base)`; then
   `r.refreshMerges(s.Workspace)`. The root-missing case of `removeFamily`
   does not apply: a missing checkout fails in Git.

3. `internal/ui/actions.go`: `case "discard": err = s.rt.Discard(id)`.

4. Browser:
   - `shared/icon.tsx`: `discard: Undo2` (Lucide's undo arrow; VS Code's
     discard icon is the same shape).
   - `sessions/session-actions.ts`: `discardSession(store, session)` asks
     with `askToConfirm`: message `Are you sure you want to discard all
     changes in '<title>'?`, detail `Its agents stop, and its worktree and its
     children's worktrees return to the commit it started from. Their
     uncommitted and untracked files and the commits on branch <branch> are
     deleted. Ignored files stay.` (omit the branch clause when
     `session.merge` is absent), primary **Discard All Changes**; accept posts
     `{action: 'discard', id}`.
   - `sidebar/sidebar-menus.ts`: in the active top-level group with Archive
     and Delete, add `{label: 'Discard All Changes', icon: 'discard',
     destructive: true}` before **Archive Session** when `session.worktree`.

## Verification

Go:

- `internal/workspace`: a worktree at `base` with one commit, a modified
  tracked file, an untracked file, an untracked nested repository, and an
  ignored file. After `Discard(dir, base)`: branch tip and `HEAD` equal
  `base`, `git status --porcelain` is empty, the ignored file remains, and
  `Removable(root, dir, branch, base)` passes. A worktree with an initialized
  submodule whose pointer changed and that has an untracked file: after
  `Discard`, the superproject's status is empty (follow the existing
  submodule test setup in `workspace_test.go`).
- `internal/session` (fixture): a worktree session with a worktree child;
  commit in both and leave an untracked file in each; `Archive` refuses.
  `Discard` succeeds; both worktrees are at the parent's `Base` and clean;
  the family's agents are not running and the sessions are not ended;
  `Archive` then succeeds. Discard refuses a checkout session, a child, and
  an archived session with the messages above.

Browser (`bun test`, `sidebar/sidebar.test.tsx`):

- The menu of an active top-level worktree session lists **Discard All
  Changes** before **Archive Session**; checkout, child, and archived
  sessions do not.
- Choosing it shows the confirmation; confirming posts `action=discard` with
  the session ID; cancelling posts nothing.

Commands: focused tests while working, then `make check`.

## Docs

`docs/lifecycle.md` (action table: a **Discard All Changes** row, and a
sentence beside Archive), `docs/usage.md` (Archive or delete completed work:
discarding work you do not want to keep), `docs/troubleshooting.md` (Archive
or delete is refused: Discard All Changes as the fix for work you do not want),
and `docs/storage.md` (session cleanup: discard returns trees to the starting
commit). Leave `docs/workflows.md` to the parent.

## Handoff

Ready. One runtime action reusing the removal claim and stop sequence, one Git
function, one route, one menu item with confirmation, and Git-backed plus
browser tests: below the review-session phase in size, but it changes
lifecycle and destructive Git behavior across a family, so it warrants a plan.
No open engineering decisions.
