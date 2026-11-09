# Worktree sessions review

## Scope and coverage

Reviewed commit `84e8317` against `cc1ad20`, the plan at
`agents/plans/2026-11-03-005-worktree-sessions.md`, and its work log. Read the
whole touched source and test files against AGENTS.md's Code style section,
traced creation, launch, restore, deletion, snapshots, and browser actions,
and checked the documentation changes. Generated browser assets match a
fresh build of their sources.

The four specified architecture decisions are implemented: Delete uses
`workspace.Removable`; `refreshMu` covers the complete merge refresh;
`MergeStatus` serves display only; and `report.Instructions` describes a
worktree session and its children's merge destination accurately.

Browser appearance and a live `make dev` session were not checked. This is a
findings-only review: no implementation files were edited, fixed, staged,
or committed.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Error handling | Trivial; not applied | Missing parent worktree prevents family deletion |
| F2 | Medium | Concurrency | Trivial; not applied | Base can name a commit the session never started from |
| F3 | Low | Correctness | Trivial; not applied | Untracked directories are counted as single files |

## F1. Missing parent worktree prevents family deletion

Medium · Error handling · Trivial; not applied · `internal/session/sessions.go:303`

Create a worktree session and a managed child, stop the family, and remove
the parent's worktree directory. Both worktrees are now absent, and their
fresh branches are already merged into the original checkout. Delete should
clean up the remaining Git registrations, branches, and session rows, as
it does for a missing worktree without a child. Instead, it always fails
while removing the child, leaving the family undeletable through the UI.

The original checkout still exists, so `present` is true. The child removal
uses `s.Dir` as its repository directory even though that directory is gone.
`RemoveWorktree` reaches `git worktree prune` in that nonexistent directory
and returns an error before any session row is deleted. Retrying has the
same result. The preflight at line 279 also uses the absent directory and
therefore does not inspect the surviving child branch.

### Evidence

An isolated test used the existing fixture to create a worktree session and
child, called Stop, removed `s.Dir` with `os.RemoveAll`, and called Delete.
It failed with:

```text
delete clean, merged family whose worktrees are gone: w1: git worktree prune:
```

### Recommended correction

Use the surviving original checkout for Git cleanup and branch checks when
the parent's worktree is absent, retaining the merge checks before deleting
branches.

## F2. Base can name a commit the session never started from

Medium · Concurrency · Trivial; not applied · `internal/session/sessions.go:73`

`AddWorktree` creates the session branch from checkout commit A, then runs
worktree setup. If another checkout session commits B during that interval,
`workspace.Head(w.Path)` stores B as this session's immutable Base, even
though its branch still points at A. The fresh session consequently displays
"Merged into main" and gets the merged icon tint instead of "No commits
yet", before it has made any commits. Subsequent refreshes cannot correct
the stored Base. This follows the plan's chosen read location but violates
its definition of Base as the session branch's starting commit.

### Evidence

A temporary `post-checkout` hook committed in the original checkout during
`git worktree add`, deterministically reproducing the same ordering as a
concurrent checkout session. Creating the worktree session then returned:

```text
Base=6881661d51e80e7057f7201692eb476d57263922
branch HEAD=3d098834880e37eb21747aa5fbdacef252db68ec
Merge={Unmerged:0 Committed:true Uncommitted:0}
```

### Recommended correction

Read Base from the newly created session worktree, rather than the original
checkout whose HEAD another session can move during setup.

## F3. Untracked directories are counted as single files

Low · Correctness · Trivial; not applied · `internal/workspace/workspace.go:264`

The new uncommitted-file count counts lines from `git status --porcelain`.
By default, Git collapses an entirely untracked directory to one `?? dir/`
entry. Creating three new files inside a new package directory therefore
makes the merge bar say "1 uncommitted file". This affects ordinary work
that adds a directory; it does not affect the existing dirty-worktree guard's
boolean result.

### Evidence

An isolated test created `new-package/a.go`, `b.go`, and `c.go` in a fresh
session worktree, then called `workspace.MergeStatus`. It failed with:

```text
three untracked files: Uncommitted=1
```

### Recommended correction

Have the displayed file count enumerate untracked files individually while
preserving the existing deletion guard's behavior.

## Checks run

- `go test -race ./internal/workspace ./internal/session ./internal/store ./internal/report ./internal/ui -count=1` — passed.
- `bun test web/agent/composer.test.jsx web/agent/merge-bar.test.jsx web/sidebar/sidebar.test.jsx` in `internal/ui` — 53 passed.
- Built the browser assets into a temporary directory and compared them with
  `internal/ui/dist` using `diff -rq` — matched.
- `go test -overlay /tmp/hypercube-review-overlay.json ./internal/session -run '^TestReview' -count=1 -v`
  — three isolated reproductions failed as described in F1–F3. The overlay
  supplied a temporary test file without changing checkout files.

## Verdict

Two medium findings and one low finding remain open. No fixes were applied,
as requested.
