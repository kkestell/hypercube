# Work log: discard all changes in a worktree session (unit 2 of 2)

- Task ID: 2026-11-09-013-discard-worktree-work
- Plan (repository-relative): `.agents/plans/2026-11-09-013-discard-worktree-work.md`, blob `274bf1c4944c125a77282850c99f3364248845bf`
- Shared design: `.agents/designs/2026-11-09-012-worktree-branch-choice-and-discard.md`, blob `4e2f6244ffb183e3ccb5dd62d97621fab35f5bed`
- Plan review: `.agents/reviews/plans/2026-11-09-013-discard-worktree-work.md` (execution decision: proceed)
- Author: implementer child of Hypercube session 3703b04c
- Status: implemented
- Code base: `90476495aaff42a549a4b7b5d7099b7a5b3c23be`
- Code head: `abec2f7fdeb949bed993a080292fe378d36fff70`
- Implementation commits: `abec2f7fdeb949bed993a080292fe378d36fff70` (`X-Stage: work`)

## Changes

- `internal/workspace/workspace.go`: `Discard(dir, branch, commit)` refuses unless `isWorktree(dir)`, then runs `git switch --quiet --discard-changes --recurse-submodules --force-create`, `git clean -ffdq`, and the submodule clean when `.gitmodules` exists.
- `internal/session/discard.go` (new): `Runtime.Discard` claims the family, refuses a child, an archived session, and a checkout session, stops the family's launches, discards each `familyWorktrees` tree at the session's `Base`, and refreshes merges. `removal.go`: `treeName` shared by `checkWorktrees` and `Discard`. `runtime.go`: `errDeleting` text is now "another change to the session is in progress".
- `internal/ui/actions.go`: `discard` action.
- Browser: `discard` icon (Lucide `Undo2`), `discardSession` confirmation, **Discard All Changes** menu item before Archive for worktree sessions.
- Docs: `lifecycle.md`, `usage.md`, `troubleshooting.md`, `storage.md`. `docs/workflows.md` untouched; no screenshots.
- Tests: `internal/workspace/discard_test.go` (full discard with commit, tracked change, untracked file, nested repository, ignored file, submodule; switched branch; non-worktree folder), `internal/session/discard_test.go` (family with a child; refusals), `sidebar.test.tsx`.

## Deviations

None. The Go test for a missing checkout was not added; the plan lists none.

## Verification (at Code head, macOS, Go 1.27.3)

- Focused: `go test ./internal/workspace -run Discard`, `go test ./internal/session -run Discard`, `cd internal/ui && bunx tsc -p . && bun test sidebar`: pass.
- `make check`: exit 0 (output in `/tmp/check13.log`, not kept in Git).

## Session provenance

| Role | Harness | Model | Native conversation ID |
| --- | --- | --- | --- |
| Implementer (this log) | Claude Code | Sonnet 5.5 (`claude-sonnet-5-5`) | `34031658-bbc6-466d-adde-0f8e62ac32d4` |

## Limitations

- The confirmation and menu are exercised in happy-dom, not a real browser.
