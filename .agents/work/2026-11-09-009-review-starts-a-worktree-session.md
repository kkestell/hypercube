# Work log: Review starts a worktree session at the pull request's head

- Task ID: 2026-11-09-009-review-starts-a-worktree-session
- Plan (repository-relative): `.agents/plans/2026-11-09-009-review-starts-a-worktree-session.md`, blob `4840e94fedae422d8f4b8537074ec28b67b1f21c`; no guidance blobs. Dependency read: `.agents/work/2026-11-09-008-worktree-start-and-safe-removal.md`.
- Author: xwork worker p2-work (Hypercube child session of 777ef702), Claude Code, claude-opus-5-5
- Status: implemented
- Code base: 72ccc1dcd099890f4aa759210ff2f39f99a5a34e
- Code head: 78e76b8e7fa485d1c091e63d011c657b18731866
- Implementation commits: cbe99753bc5cc2e10a0dcbd815f68b745eaba512 (work), 78e76b8e7fa485d1c091e63d011c657b18731866 (fix)

## Changes

Responsibilities as in the plan.

- `internal/workspace/remote.go`: `FetchPullRequest` fetches `+refs/pull/<n>/head` into `refs/hypercube/fetch/<rand.Text()>`, resolves it, and always deletes the ref. `Fetch` and it share a private `fetch` that runs `git fetch --quiet --no-write-fetch-head` with the environment and `GIT_TERMINAL_PROMPT=0`.
- `internal/workspace/workspace.go`: `MergeStatus` counts `rev-list --count <branch> --not HEAD <base>` (base omitted when empty, as in `unkept`). The signature already carried `base`.
- `internal/github/review.go`: `Watcher.FetchPullRequest`.
- `internal/ui/actions.go`: `sessionAction` case `"review"` calls `startReview`; `reviewSettings` holds the model rule; the prompt is the plan's fixed text.
- Browser: `internal/ui/web/github/review.ts` has `canReview`, `startReview`, and `useReview`, the callers' in-flight state (a row's `onReview` is undefined while its review starts, which disables the button). `pull-requests.tsx` and `all-pull-requests.tsx` use it; `send-dialog.tsx`, `instructions.ts`, and `instructions.test.ts` are deleted.
- `docs/interface.md`: the Review paragraph.
- Tests: `TestFetchPullRequest` (the commit exists only in the bare remote at `refs/pull/7/head`; a missing pull request errors; no `refs/hypercube/` refs remain), `TestMergeStatusCountsOnlyCommitsAfterStart` (the existing `TestMergeStatus` covers a session started at `HEAD` with one commit reporting 1), `internal/ui/review_test.go` (`reviewSettings` rule; bad `number` values give 400), `pull-requests.test.tsx` (posts the review action with workspace and number, button disabled in flight, returned session selected; a 409 shows its text and keeps the selection), `review.test.ts` (`canReview`, moved from `instructions.test.ts`). The Pull Requests view's disabled-rule test is unchanged and passes.

## Deviations

- `startReview` reads the settings before fetching, so a missing model fails without a network fetch. The plan lists the fetch first; the outcome is the same.
- The in-flight state lives in a `useReview` hook in `review.ts`, shared by both callers, rather than repeated in each.

## Verification (at Code head cbe99753…, macOS, git 2.56.0)

- During work: `go test ./internal/workspace/`, `go test ./internal/ui/ -run Review`, `bunx tsc -p .` and `bun test web/github` in `internal/ui`: pass.
- `make check` from the repository root on the tree that was then committed unchanged (no hooks): exit 0, output in `/tmp/p2-work-make-check.log`. Status was clean after the commit.
- Not run: the plan's manual `make dev` check, which needs a real requested review. Left for the user.

## Session provenance

| Stage | Name / ID | Harness | Native conversation ID | Model |
| --- | --- | --- | --- | --- |
| parent | 777ef702 (Hypercube session) | Claude Code | 209b0949-92b0-42bc-8596-4341a4bf0785 | Opus 5.5 |
| work | p2-work (child of 777ef702) | Claude Code | ae939d1a-7dfa-45f6-ac1e-8496ca8a03e8 (`CLAUDE_CODE_SESSION_ID`) | claude-opus-5-5 |

## Limitations

- Manual `make dev` check not run (above).
- Awaiting independent review of the repair.

## Repair 1

Review: `.agents/reviews/2026-11-09-009-review-starts-a-worktree-session.md`, round 1 (`changes-required`, commit d701c71).

- F1: plan step 5 is replaced, at the parent's direction: once the session exists, the review action answers like `"new"`, with JSON `{"session": id}`, or 409 JSON `{"session": id, "error": msg}` when its first input fails. Failures before creation stay plain-text 400 or 409. `writeCreated` in `internal/ui/actions.go` writes that answer for both actions. `review.ts` reads it with `submitPrompt`, selects any returned session, and shows any error. New `pull-requests.test.tsx` case: a 409 carrying a session selects it and shows the error. The refused-review test still covers a 409 without one.

Verification at Code head 78e76b8e…, macOS, git 2.56.0:

- `bunx tsc -p .` and `bun test web/github` in `internal/ui`: pass.
- `make check` from the repository root on the tree that was then committed unchanged: exit 0, output in `/tmp/p2-work-make-check-fix.log`.
