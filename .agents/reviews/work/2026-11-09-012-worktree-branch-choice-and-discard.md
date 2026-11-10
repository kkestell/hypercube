# Work review: 2026-11-09-012-worktree-branch-choice-and-discard

## Round 1

- Task ID: 2026-11-09-012-worktree-branch-choice-and-discard
- Mode: work
- Author: reviewer child of Hypercube session 3703b04c (Claude Code, Opus 5.5
  `claude-opus-5-5`, native conversation
  ed3d61a0-59b4-4de1-b69d-012c194ba728). I did not write this code; the
  implementer was a Sonnet 5.5 worker (34031658-bbc6-466d-adde-0f8e62ac32d4).
- Work log: `.agents/work/2026-11-09-012-worktree-branch-choice-and-discard.md`
  (commit `4073f350cb32d2ca028680a2979fb21ff70c58f5`)
- Plan: `.agents/plans/2026-11-09-012-worktree-branch-choice-and-discard.md`,
  blob `503929877370267343adfe75ee7ab9e89bad39e9`. Shared design blob
  `4e2f6244ffb183e3ccb5dd62d97621fab35f5bed`. Both blobs match the accepted
  plan review.
- Code base: `5d44211ddb094d39580a2f08b6158a7c228fd19f`
- Code head: `2e1e005e7e1bd41de48282fece8c1a275a2ccc16`. The base is an
  ancestor of the head. The checkout is at `4073f35`, which adds only the
  work log. The working tree is clean.

### Verdict

`changes-required`. Requests 1 and 2 are implemented correctly and match the
plan. Two small fixes are needed: a documentation rule violation and a
missing dialog attribute that every comparable dialog in the app sets.

### Findings

1. **Low — the same fact appears in two docs (`docs/lifecycle.md:53-54`).**
   The added sentence "Unarchive is refused while a branch with the session's
   branch name exists; rename or delete that branch and try again" repeats the
   fact also added to the Unarchive row of `docs/storage.md`.
   `docs/dev/documentation.md` says to keep each fact in one authoritative
   location. `lifecycle.md` is an explanation page and already links to
   [cleanup behavior](storage.md#session-cleanup) for what each action does.
   The "rename or delete … and try again" clause is also a how-to step inside
   an explanation page. The plan listed both files, so the duplication came
   from the plan. **Fix:** delete the two added lines from `lifecycle.md` and
   keep the `storage.md` sentence.

2. **Low — `new-branch-dialog.tsx:46` `<DialogContent>` has no description
   and does not opt out.** Radix logs a console warning ("Missing
   `Description` or `aria-describedby={undefined}`") every time the dialog
   opens. The app's other dialogs without a description pass
   `aria-describedby={undefined}` (`app/settings-models.tsx:74`,
   `app/settings-dialog.tsx:19`). Dialogs that have one use
   `DialogDescription`. **Fix:** add `aria-describedby={undefined}` to
   `DialogContent`. GitHub's **Create a branch** dialog has no description
   line, and adding text would only repeat the field labels.

### Independent assessment

- **Request 2 (pull requests, merge status, cleanup, unarchive).** The only
  change is in `create`:
  `s.Branch = cmp.Or(worktree.Branch, SessionBranch(s.ID))`. Every consumer
  reads the saved `Branch`: `pullRequest`, `MergeStatus`/`refreshMerges`,
  `Removable`/`RemoveWorktree`, archive's `BranchTip`, unarchive's
  `AddWorktree`, and `report.go`. The tests cover a custom-named branch end to
  end. `TestWorktreeSessionOnNamedBranchAtStart` checks the branch and `Base`
  at an older commit, the checked-out branch, merge status, archive deleting
  the branch, unarchive refused over a recreated branch with the session left
  archived, and unarchive succeeding after that branch is deleted.
  `github_test.go` checks that a `.hypercube/` session's custom branch gets
  its pull request.
- **`CheckNewBranch` in `AddWorktree`.** It runs before anything else in
  `AddWorktree`. It refuses only names that `worktree add -b` refused before,
  so children, unarchive, and Review behave as before, with clearer errors.
  The failed-add cleanup now deletes the branch only when it exists. Before,
  a failed add whose branch was never created also ran `branch -D` and added
  that command's failure to the returned error. The new code removes that
  noise. `TestAddWorktreeFailureLeavesNoBranch` exercises the case where Git
  creates the branch and then fails, which I confirmed with Git 2.50.1
  during plan review.
- **HTTP layer.** The "start or branch without worktree" check moved from
  the session layer to the HTTP layer (400). With `*NewWorktree`, the session
  API can no longer express that invalid combination. `/branches` reads the
  workspace, returns 409 for a non-Git workspace, and uses `fileError` for an
  unknown workspace. `Branches` skips symbolic refs, so `origin/HEAD` is
  omitted, and the test sets it.
- **Browser.** `unsentWorktree(state, worktree, branch = null)` keeps
  `branch` set only while `worktree` is true. **Existing checkout** and
  **New worktree** both clear it. The third item opens the dialog, and Cancel
  leaves the previous choice. Opening a dialog from a menu item is already
  done throughout the app (`askToConfirm` from the sidebar's dropdown-based
  context menu). The dialog disables Save while loading, on a load error,
  for an empty name, and for an existing local name. The **Use “…”** item is
  force-mounted and appears only for unlisted text. `worktreeParams` sends
  `branch` and `start` only in a worktree. `readContext` accepts drafts
  without a branch. Icons (`laptop`, `git-branch`, `check`, `chevron-down`)
  are existing Lucide names. The trigger shows only the branch name, and the
  `title` carries the source.
- **Clarity.** The touched files are easy to follow. `worktree-select.tsx`
  writes out its three items, which reads better than the old two-entry
  table with special cases would.
- **Docs.** `usage.md`, `introduction.md`, and `storage.md` match the code.
  The UI names in the docs (**New worktree on branch…**, **Create a
  branch**, **New branch name**, **Source**, **Use “…”**, **Save**) match the
  rendered labels. No screenshot shows the location menu, so none needed
  regenerating.

Informational, no action: the `NewWorktree` doc comment's word order is
awkward ("Empty Start means the checkout's HEAD, any commit or ref that
resolves in the checkout otherwise"). It is accurate.

### Verification

- **Reused:** `make check`, exit 0. Evidence: the implementer's native
  conversation 34031658-bbc6-466d-adde-0f8e62ac32d4, background task
  `bjwmq6qjp`, output
  `/private/tmp/claude-501/-Users-kyle-src-hypercube--hypercube-3703b04c/34031658-bbc6-466d-adde-0f8e62ac32d4/tasks/bjwmq6qjp.output`,
  which ends in `ok` lines for every Go package and `[exited with code 0]`.
  It ran on the working tree that became the Code head. The only later edit
  was a prose rewrap in `docs/introduction.md`, which needs no code check.
- **Reused:** the focused `go test` and `bun test` (455 pass) runs recorded
  in the log.
- **Run:** none. No open question needed a fresh run. Both findings come
  from reading the code and docs.
- **Limitation:** the dialog was tested only in happy-dom; no real-browser
  run or screenshot exists.

## Round 2

- Author: same reviewer as round 1 (Claude Code, Opus 5.5
  `claude-opus-5-5`, native conversation
  ed3d61a0-59b4-4de1-b69d-012c194ba728)
- Work log: `.agents/work/2026-11-09-012-worktree-branch-choice-and-discard.md`
  (Repair 1, commit `5997f12`)
- Code base: `5d44211ddb094d39580a2f08b6158a7c228fd19f` (unchanged)
- Code head: `10894555e725a880c0616f1e05e0ca341c4bb663`. The previously
  reviewed head `2e1e005` is an ancestor. In `2e1e005..1089455`, the only
  code or doc changes are in `docs/lifecycle.md` and
  `internal/ui/web/agent/new-branch-dialog.tsx`. Everything else is workflow
  records. The checkout HEAD (`5997f12`) adds only the work log, and the
  working tree is clean.

### Verdict

`accept`. Both round 1 findings are resolved, and the repair adds no new
findings. Round 1's assessment of the rest of the change still applies,
because none of that code changed.

### Resolution

| Finding | Status | Evidence at `1089455` |
| --- | --- | --- |
| 1. The unarchive refusal fact was duplicated in `docs/lifecycle.md` | Resolved | The two added lines are removed. The paragraph is back to its base text and still links to `storage.md#session-cleanup`, where the Unarchive row states the refusal. |
| 2. `DialogContent` had no description and no opt-out | Resolved | `new-branch-dialog.tsx:46` is now `<DialogContent aria-describedby={undefined}>`, the same as `settings-models.tsx` and `settings-dialog.tsx`. |

### Verification

- **Reused (work log, Repair 1):** `bunx tsc -p .` clean and `bun test agent`
  59 pass at `1089455`. This covers the one-attribute change. Round 1's
  reused `make check` still applies to all other code.
- **Run:** none. I inspected the two-file diff directly.
