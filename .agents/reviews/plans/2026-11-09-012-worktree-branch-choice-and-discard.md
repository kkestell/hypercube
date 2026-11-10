# Plan review: 2026-11-09-012-worktree-branch-choice-and-discard

## Round 1

- Task ID: 2026-11-09-012-worktree-branch-choice-and-discard
- Mode: plan
- Author: plan-reviewer child of Hypercube session 3703b04c (Claude Code,
  Opus 5.5 `claude-opus-5-5`, native conversation
  ed3d61a0-59b4-4de1-b69d-012c194ba728). Independent of the planner
  (ed140e50-7ea6-40f1-84c8-faaf6329d964).
- Plan: `.agents/plans/2026-11-09-012-worktree-branch-choice-and-discard.md`,
  blob `503929877370267343adfe75ee7ab9e89bad39e9`
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

Work size: right-sized. Detail: appropriate. Readiness: ready. Action:
`accept`.

### Evidence

Choices checked against the code:

- **Request item 2 needs no new mechanism.** `pullRequest`
  (`internal/ui/snapshot.go:82`) matches a `.hypercube/` session by
  `s.Branch`; `MergeStatus`, `Removable`/`RemoveWorktree`, archive's
  `BranchTip`, `Unarchive`'s `AddWorktree`, and `report.go:134` all read the
  saved `Branch`. Only `create` (`internal/session/sessions.go:108`) derives
  the name. `MergeStatus` excludes commits on the way to `Base`, so a session
  started from another branch counts only its own commits.
- **`CheckNewBranch` inside `AddWorktree` is safe for every caller.**
  `git worktree add -b` already refuses an existing branch, so the check
  changes only the error text for children (`children.go:114`), unarchive
  (`removal.go:72`), and Review. All callers pass names that are new by
  construction or must be new. Dropping the `existed` bookkeeping is correct
  once the check runs first.
- **Git behavior verified with Git 2.50.1:** `check-ref-format --branch`
  refuses `HEAD`, `-x`, `a b`, `a..b`, and `x.lock`, accepts `feature/login`,
  and expands `@{-1}` to the previous branch, which the plan's "prints
  anything other than name" rule catches. A `worktree add -b` into an
  existing non-empty directory fails *after* creating the branch, so the
  cleanup is needed. That case also gives the test "a failed add of a new
  name leaves no branch" a concrete setup.
- **Unarchive:** a user-created branch with the session's name already makes
  `worktree add -b` fail today. Refusing with a clear wrapped error and
  keeping the session archived preserves behavior. Reusing the user's branch
  would let archive delete it later, so refusing is the right choice.
- **Browser:** unsent sessions are kept per workspace (`openUnsent`,
  `unsentDrafts` keyed by `unsentKey`), so a chosen source never carries
  over to another repository. A failed creation returns no session and keeps
  the unsent session, so correcting the branch and retrying works as
  described. `readContext` accepts drafts saved before this change.
- **UI pattern:** a radio menu with an "Other…"-style item that opens a
  dialog is the standard macOS form. It keeps the one-click **New worktree**
  path unchanged. GitHub's **Create a branch** dialog (name + source picker)
  is a sound model. Using **Save** instead of GitHub's **Create new branch**
  is accurate because nothing is created until the prompt is sent. The
  **Use “…”** item is the usual way a command palette accepts typed text,
  and it covers the request's "or commit". Icons are Lucide names the app
  already uses.
- **Tests:** they cover name validation, existence, failed-add cleanup,
  remote listing without `origin/HEAD`, the full create → archive → unarchive
  path with a custom name, the unarchive conflict, `pullRequest` matching,
  the 400/409 routes, and the composer and draft round trips. This covers the
  risky Git behavior.

No findings require correction.

Advisory (no action required): for the failed-add test, put a non-empty
directory at the target path. Git then creates the branch and fails, which
exercises the cleanup.

### Normalization

Target: the default 600–1,500 words (no curated reference set). Measured
1,569 words (101 in fenced code) with `plan_metrics.py`, 69 words above the
band. Compared with the reference "one operator family through existing
layers": one mechanism (a named branch at a chosen start) runs through the
Git layer, session creation, two routes, and one control with a dialog.
Splitting the backend from the browser would leave neither part usable. The
extra length comes from the dialog's behavior contract, which the user's UI
rules make necessary. I found no removable block of substantial size.
Confidence: moderate.

### Handoff

Proceed with the plan as written.
