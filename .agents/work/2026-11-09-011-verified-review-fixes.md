# Work log: verified review fixes

- Task ID: 2026-11-09-011-verified-review-fixes
- Status: complete
- Plan (repository-relative): `.agents/plans/2026-11-09-011-verified-review-fixes.md`,
  accepted blob `793b9abfb0fee2f660aff4afd765109d7dce5568`
- Plan review and execution decision:
  `.agents/reviews/plans/2026-11-09-011-verified-review-fixes.md`
  (execution decision after round 1: proceed)
- Guidance blobs, checked unchanged before editing: `AGENTS.md`
  `da2afb1eac69ad0b8f99faf9d986246965fcc051`; `docs/dev/development.md`
  `58c66cdf38859ff4e8b3e558f3d65d144dbc95c3`; `docs/dev/documentation.md`
  `91d766f9a831875a497bb8e0b9472778130df738`
- Author: implementer, Hypercube child session fix-verified-issues-opus
  (Claude Code, claude-opus-5-5)
- Checkout: `/Users/kyle/src/hypercube/.hypercube/0987244a`, branch
  `hypercube/session-0987244a`
- Code base: `0040f146a84bf0be9776ebfad6c9f8023c98bf44`
- Code head: `71cbb6b78b522f303c4f555f9dbbebbfa76b969e`
- Implementation commits: `71cbb6b78b522f303c4f555f9dbbebbfa76b969e`
  (X-Stage: work)

## Changes

The design is in the plan; this lists where each fix landed.

1. Submodule commits survive removal: `Removable` calls the new
   `unpushedSubmodule` in `internal/workspace/workspace.go`, which lists
   initialized submodules recursively by `$displaypath` and runs
   `rev-list --count HEAD --branches --not --remotes` in each.
   `RemoveWorktree`'s comment says `--force` follows that check.
   `docs/storage.md` states the submodule rule and limits the ignore-settings
   sentence to the uncommitted-files check.
2. Merge target: `Runtime.Merge` in `internal/session/merges.go` refuses an
   ended session or one with an empty `Base`, then compares the live branch
   with the target. On a mismatch it refreshes merge status and returns
   `the checkout is now on X, not Y`; otherwise it sends the merge prompt.
   `internal/ui/actions.go` adds the `merge` action. `requestMerge` posts
   `action=merge&target=…` with no body, and `MergeButton` no longer reads the
   workspace path.
3. Clone authentication: `workspace.Clone` takes the token and, when set,
   prepends `-c credential.helper=` and `-c credential.helper=<token helper>`
   and puts `GH_TOKEN` in the command's environment. `github.Clone` passes
   the token.
4. Comment after a push: `fromNode` counts an own pull request's threads only
   if their last comment is newer than the last commit, when changes were
   requested and a push answered them. `afterLastCommit` is shared with
   `unansweredConversation`. `ownTurn` checks unanswered comments before
   "Waiting for another review.". `docs/interface.md` states the rule.
5. All review threads: `threadFragment` is shared by the search query and the
   follow-up; the `pr` fragment reads `id` and `pageInfo`.
   `accountPullRequests` dedupes nodes, calls `remainingThreads`, then maps
   them. `remainingThreads` sends one aliased query per round for every pull
   request with more threads, using ID and cursor variables.
   `githubtest.API` answers those queries from `SetThreadPage`, records the
   requested IDs in `ThreadReads`, and leaves them out of `Requests()`.
6. Collapse: `Split` takes `onCollapse` and calls it, without `onResize`,
   when a user interaction leaves the fixed child at 0. `hideRegion` in
   `app/workbench.ts` hides a region (`closePanel` for the terminal panel).
   `app.tsx` passes it to the three region splits.

Changed paths (23): `docs/interface.md`, `docs/storage.md`,
`internal/github/{clone.go,githubtest/githubtest.go,pulls.go,watcher_test.go}`,
`internal/session/{merges.go,merges_test.go}`, `internal/ui/actions.go`,
`internal/ui/web/app/{app.tsx,view-storage.test.ts,workbench.ts}`,
`internal/ui/web/changes/{git-panel.test.tsx,merge-button.tsx}`,
`internal/ui/web/github/{turns.ts,turns.test.ts}`,
`internal/ui/web/sessions/session-actions.ts`,
`internal/ui/web/shared/{split.tsx,split.test.tsx}`,
`internal/workspace/{remote.go,remote_test.go,workspace.go,workspace_test.go}`.

## Deviations

None in design. Local details:

- A refused merge of a session without its own worktree reports
  `only a top-level worktree session can be merged`; an ended session reports
  `the session has ended`.
- `remainingThreads` returns an error when GitHub answers a follow-up with no
  pull request for an ID, so a missing node cannot loop forever. Like any
  query error, it fails the pass.
- The #5 regression test pages twice (cursors `c1`, `c2`) to cover the loop
  and asserts by recorded node IDs rather than request counts, so a second
  watcher pass cannot make it flaky. The workspace configures its account,
  so `Requests()` must stay 0 while paging.
- The #4 backend cases sit in the existing table in
  `TestPullRequestsDescribeTheAccountsPart` (commit at minute 10, old thread
  at minute 5, newer comment or reply at minute 12).

## Verification

Environment: macOS 26 (Darwin 25.5.0, arm64), Go 1.27.3, Bun 1.4.2,
Git 2.50.1 (Apple Git-155). All tests use temporary directories and isolated
Git configuration; no user agents or services were started or stopped.

Focused checks while developing (native tool results in this session):

- `go test ./internal/workspace -run 'TestRemoveWorktreeKeepsUnpushedSubmoduleCommits|TestWorktreeSetUp'`: ok.
  The new test fails when the submodule check is disabled.
- `go test ./internal/workspace -run TestClone`: ok. The token test fails
  (`the credential store was written`) when `-c credential.helper=` is
  removed.
- `go test ./internal/session -run TestMerge -race`: ok.
- `go test ./internal/github/... -race`: ok. The three #4 table cases fail
  when the after-push thread rule is disabled; the paging test fails
  (`unanswered = map[1:0 2:0]`) when `remainingThreads` is skipped.
- `bun test` for `git-panel.test.tsx`, `turns.test.ts`, `split.test.tsx`,
  `view-storage.test.ts`: pass. The split test fails (`collapses` 0) without
  the `onCollapse` branch.

Required check, `make check` (ui build, `go build`, `bunx tsc -p .` and
`bun test` in `internal/ui`, desktop `bun test`, `go vet`, gofmt, and
`go test -race ./...`):

- Run 1 on the uncommitted diff: failed at the gofmt step on
  `internal/github/watcher_test.go` (all earlier steps passed). Log:
  `/tmp/2026-11-09-011-make-check-1-gofmt-fail.log`. Fixed by adding a
  `pushedPast` helper in the test table and running gofmt.
- Run 2 on the final tree, exit 0: UI 451 pass / 0 fail, desktop 28 pass /
  0 fail, every Go package `ok` under `-race` (unchanged packages reported
  from Go's test cache). Log: `/tmp/2026-11-09-011-make-check.log`. That
  tree was committed unchanged as Code head
  `71cbb6b78b522f303c4f555f9dbbebbfa76b969e`; no hooks modified it.

No screenshots, per the plan. Not run: Linux container checks
(`scripts/linux/run`), which the plan does not require.

## Session provenance

| Stage | Role / coordinator name | Harness | Model / effort | Native conversation ID | Context |
| --- | --- | --- | --- | --- | --- |
| Parent | coordinator 0987244a | OpenCode | openai/gpt-6.1-sol, xhigh | `ses_e3d29b976ffehNoBOsD5RvO3Xv` | this checkout |
| Plan | planner plan-verified-fixes-opus | Claude Code | claude-opus-5-5, effort unavailable (not exposed) | unavailable (not exposed) | this checkout, 2026-11-09 |
| Plan review | reviewer review-fix-plan-opus | Claude Code | claude-opus-5-5, effort unavailable (not exposed) | unavailable (not exposed) | this checkout, 2026-11-09 |
| Work | implementer fix-verified-issues-opus | Claude Code | claude-opus-5-5, effort unavailable (not exposed) | unavailable (not exposed to this session) | this checkout, 2026-11-09 |
| Work review | reviewer review-verified-fixes-opus | Claude Code | claude-opus-5-5, effort unavailable (not exposed) | unavailable (not exposed to this session) | this checkout, 2026-11-09 |

## Reviews

- Plan review: `.agents/reviews/plans/2026-11-09-011-verified-review-fixes.md`
- Work review: `.agents/reviews/work/2026-11-09-011-verified-review-fixes.md`,
  round 1 **accept**, committed as
  `61dc25c6a28267b47f8fe48a9dcdfb197f5f9be1`.

## Completion assessment

The parent inspected the implementation diff and the successful `make check`
output, verified the recorded Git anchors, and checked the independent Opus
review against the six requested outcomes. No blocking findings remain.
After Code head, only this work log and the named work-review record changed;
the accepted and tested code is unchanged. No repeat of the checks was needed
for those record-only commits. All task-owned code and workflow records are
committed on `hypercube/session-0987244a` before the requested merge into main.
The unrelated untracked `web/` directory in the original checkout is excluded
from the task and will be preserved during the merge. No push or installation
is part of this task.

## Limitations

- The GitHub GraphQL follow-up query was checked against the fake API, not
  the live API.
- The `make check` logs are in `/tmp` and are not committed.
