# Work log: discard all changes in a worktree session (unit 2 of 2)

- Task ID: 2026-11-09-013-discard-worktree-work
- Plan (repository-relative): `.agents/plans/2026-11-09-013-discard-worktree-work.md`, blob `274bf1c4944c125a77282850c99f3364248845bf`
- Shared design: `.agents/designs/2026-11-09-012-worktree-branch-choice-and-discard.md`, blob `4e2f6244ffb183e3ccb5dd62d97621fab35f5bed`
- Plan review: `.agents/reviews/plans/2026-11-09-013-discard-worktree-work.md` (execution decision: proceed)
- Author: implementer child of Hypercube session 3703b04c
- Status: complete
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
| Parent (xauto coordinator) | Claude Code | Opus 5.5 (`claude-opus-5-5`) | `059787ce-5ccd-4e61-a729-78b16c865c72` |
| Planner (`planner` child) | Claude Code | Opus 5.5, effort high | `ed140e50-7ea6-40f1-84c8-faaf6329d964` |
| Plan and work reviewer (`plan-reviewer` child) | Claude Code | Opus 5.5, effort high | `ed3d61a0-59b4-4de1-b69d-012c194ba728` |
| Implementer (this log) | Claude Code | Sonnet 5.5 (`claude-sonnet-5-5`) | `34031658-bbc6-466d-adde-0f8e62ac32d4` |

## Limitations

- The confirmation and menu are exercised in happy-dom, not a real browser.

## Repair 1

- Findings addressed (work review `.agents/reviews/work/2026-11-09-013-discard-worktree-work.md`):
  1. `workspace.Discard` runs `git reset --quiet --hard` before the switch, then `git rebase --quit` when `rebase-merge` or `rebase-apply` exists under the worktree's Git path. New `TestDiscardAbandonsAnOperationInProgress` covers a conflicted merge and a conflicted rebase; each ends clean with `Removable` passing.
  2. `docs/lifecycle.md`: Discard is its own paragraph with one sentence.
  3. `docs/usage.md`: the destructive effect comes first, then the step and follow-up, linking to `storage.md#session-cleanup` for the details.
- Code base (unchanged): `90476495aaff42a549a4b7b5d7099b7a5b3c23be`
- Code head: `8b34d7e41acf75fc1d254eca914d575dd19271d7` (`X-Stage: fix`); previous head `abec2f7fdeb949bed993a080292fe378d36fff70`.
- Checks at this head: `go test ./internal/workspace -run Discard -v` pass; `go test -race ./internal/workspace ./internal/session` pass. The change touches Go in `internal/workspace` and docs only, so the full `make check` was not rerun.

## Completion

- Final reviewed Code head: `8b34d7e41acf75fc1d254eca914d575dd19271d7`; later commits change workflow records only.
- Work review: `.agents/reviews/work/2026-11-09-013-discard-worktree-work.md`, Round 1 `changes-required` (one medium, two low findings), Round 2 `accept`.
- Plan review: `.agents/reviews/plans/2026-11-09-013-discard-worktree-work.md`.
- Completed by the parent (xauto coordinator).
