# Work log: worktree branch choice (unit 1 of 2)

- Task ID: 2026-11-09-012-worktree-branch-choice-and-discard
- Plan (repository-relative): `.agents/plans/2026-11-09-012-worktree-branch-choice-and-discard.md`, blob `503929877370267343adfe75ee7ab9e89bad39e9`
- Shared design: `.agents/designs/2026-11-09-012-worktree-branch-choice-and-discard.md`, blob `4e2f6244ffb183e3ccb5dd62d97621fab35f5bed`
- Plan review: `.agents/reviews/plans/2026-11-09-012-worktree-branch-choice-and-discard.md` (accept, no findings)
- Author: implementer child of Hypercube session 3703b04c
- Status: implemented
- Code base: `5d44211ddb094d39580a2f08b6158a7c228fd19f`
- Code head: `2e1e005e7e1bd41de48282fece8c1a275a2ccc16`
- Implementation commits: `2e1e005e7e1bd41de48282fece8c1a275a2ccc16` (`X-Stage: work`)

## Changes

- `internal/workspace/branches.go` (new): `CheckNewBranch`, `Branches`.
  `AddWorktree` (`workspace.go`) calls `CheckNewBranch` first; the `existed`
  bookkeeping is gone, and a failed add deletes the branch when it exists.
- `internal/session/sessions.go`: `NewWorktree{Start, Branch}`;
  `CreateSubmission` takes `*NewWorktree` (nil = checkout); `Create` keeps its
  bool. `removal.go`: Unarchive wraps `AddWorktree` errors as "cannot recreate
  the session's worktree".
- `internal/ui`: `"new"` passes `start`/`branch` (400 without `worktree=1`);
  `startReview` passes `&NewWorktree{Start: commit}`; new `GET /branches`
  (`branches.go`).
- Browser: `Unsent.branch`, `unsentWorktree(state, worktree, branch)`,
  `loadBranches`, draft storage of `branch`, three-item `worktree-select.tsx`,
  new `new-branch-dialog.tsx`, `worktreeParams` in `session-actions.ts`.
- Docs: `usage.md`, `introduction.md`, `storage.md`, `lifecycle.md`.
  `teamwork.md` line 29 describes children and was left unchanged.
  `docs/workflows.md` untouched; no screenshots regenerated.
- Tests: `branches_test.go` (workspace, including the failed-add case with a
  non-empty target), `sessions_test.go` (named branch at a start, archive,
  unarchive refused/succeeds, existing name refused), `internal/ui/branches_test.go`,
  `github_test.go` (custom branch gets its pull request), `composer.test.tsx`,
  `draft-storage.test.ts`.

## Deviations

- The old create-time error "a starting point needs a worktree session" moved
  to the HTTP layer as planned; its session test was replaced.
- `Branches` returns empty (non-nil) slices so `/branches` encodes `[]`.
- `/branches` reads the workspace with `fileError` (404 for an unknown
  workspace, 409 for non-Git).

## Verification (all at Code head, macOS, Go 1.27.3)

- Focused: `go test ./internal/workspace ./internal/session` ok;
  `go test ./internal/ui` (after `make ui`) ok; `cd internal/ui && bunx tsc -p . && bun test`: 455 pass.
- `make check`: exit 0 (ui build, `go build`, tsc, bun tests for ui and
  desktop, `go vet`, gofmt, `go test -race ./...`). It started before the
  final prose-only wrap fix in `docs/introduction.md`, which is in the same
  commit; docs need no code check.

## Session provenance

| Role | Harness | Model | Native conversation ID |
| --- | --- | --- | --- |
| Implementer (this log) | Claude Code | Sonnet 5.5 (`claude-sonnet-5-5`) | `34031658-bbc6-466d-adde-0f8e62ac32d4` |

## Limitations

- The dialog is exercised in happy-dom, not a real browser; no screenshot was taken.

## Repair 1

- Findings addressed (work review `.agents/reviews/work/2026-11-09-012-worktree-branch-choice-and-discard.md`):
  1. Removed the unarchive-refusal lines from `docs/lifecycle.md`; the fact remains in the `docs/storage.md` Unarchive row.
  2. `new-branch-dialog.tsx`: `<DialogContent aria-describedby={undefined}>`.
- Code base (unchanged): `5d44211ddb094d39580a2f08b6158a7c228fd19f`
- Code head: `10894555e725a880c0616f1e05e0ca341c4bb663` (`X-Stage: fix`); previous head `2e1e005e7e1bd41de48282fece8c1a275a2ccc16`.
- Checks at this head: `cd internal/ui && bunx tsc -p .` clean; `bun test agent` 59 pass, 0 fail. The Go code is unchanged since the earlier `make check` pass.
