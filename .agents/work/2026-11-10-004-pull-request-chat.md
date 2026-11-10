# Work log: Chat about a pull request from its row

- Task ID: 2026-11-10-004-pull-request-chat
- Plan: [.agents/plans/2026-11-10-004-pull-request-chat.md](../plans/2026-11-10-004-pull-request-chat.md),
  blob `89d7a44eb3910849d383acbd847cd29e501bdc3e` as executed
- Guidance blobs: none (no linked shared design)
- Author: implementer child of Hypercube session 32a6c484 (Claude Code, Opus 5.5)
- Status: implemented
- Code base: `42cc82bd4f5936e077612f2ce20caa0552c3f0be`
- Code head: `d1d33e8e4211fbd3a2a07f53ae804b14ce8c2ae3`
- Implementation commits: `26c8ebbf3dfae19234be869baf6909b454cbb382` (work),
  `d1d33e8e4211fbd3a2a07f53ae804b14ce8c2ae3` (fix, round 1)

## Changes

Implemented as planned; see the plan for the design.

- Server: `workspace.FreeBranch` (`internal/workspace/branches.go`);
  `Watcher.PullRequestHead` and `CheckRepo` (`internal/github/chat.go`); the
  `pull-request-branch` action and the `repo` check on `new`
  (`internal/ui/actions.go`).
- Browser: `github/row-action.ts` (`useRowAction`, replacing `useReview`),
  `github/chat.ts` (`startChat`), the row's `onChat` button, both lists, the
  **Add {repo}** dialog with **Use Existing Folder…** and `onAdded`,
  `api.ts` `requestWorkspace` (replacing `cloneWorkspace`), and removal of the
  pending workspace-dashboard selection with its test.
- Docs: `docs/interface.md` and `docs/workflows.md`; regenerated
  `pull-requests` and `dashboard` captures.

## Deviations

- The Go integration test creates `refs/pull/7/head` by pushing from the
  checkout rather than `git update-ref` in the bare repository; same result.
  It adds the workspace after the watcher's first pass, which also exercises
  Chat before the workspace's status is published; adding it first made the
  pass slow enough under full-package `-race` load to exceed the 5-second wait.
- `selectSession` with a missing workspace returns the parked-unsent state, as
  before the clone task added the pending path; page load with a
  `#workspace:` hash relies on `initialState` already holding it.
- `make screenshots` also rewrote twelve unrelated captures (approval,
  attention, hero, new-branch, overview, suggestion, usage) with rendering
  noise; they were restored, and only the four planned captures committed.

## Verification (all at Code head `26c8ebb`, macOS, Go and Bun from the repo)

- `go test -race ./internal/workspace ./internal/github ./internal/ui` — ok
  (an earlier run failed `TestPullRequestBranchAndExistingFolder` on the
  5-second accounts wait under load; fixed as above, then
  `go test -race -count=2 ./internal/ui` — ok).
- `cd internal/ui && bunx tsc -p .` — no errors; `bun test` — 480 pass, 0 fail.
- `make check` — exit 0.
- `make screenshots` — completed; `pull-requests` and `dashboard` light/dark
  inspected: Chat at each row's right edge, Review to its left on requested
  reviews, disabled Review where no workspace holds the repository.
- Manual `make dev` check: not run. It needs driving the browser view against
  a real GitHub pull request, which this session cannot do; composer focus
  rests on the existing mount focus, and the fetched-head start is covered by
  the Go integration test against a local bare repository.

## Repair round 1

Review: [.agents/reviews/2026-11-10-004-pull-request-chat.md](../reviews/2026-11-10-004-pull-request-chat.md)
(committed at `b742dcf004827fb2c439d71f4ee341d40ba3df01`). N1 needed no
change.

- N2 fixed in `d1d33e8e4211fbd3a2a07f53ae804b14ce8c2ae3`: `useRowAction` now
  returns `{run, running}`, and an untracked row's Chat is disabled while
  `running(rowKey)` holds, which covers the chat that follows an add before
  the workspace is listed. Deviation from the suggested fix: wrapping the
  open-dialog handler in the running key disabled the opener while the dialog
  opened, so focus could not return to it (the existing focus test failed);
  only the post-add chat is tracked. New bun test: the untracked row's Chat is
  disabled while the post-add `pull-request-branch` request is pending, and
  enabled after; it fails without the fix.
- Verification at `d1d33e8`: `cd internal/ui && bunx tsc -p .` — no errors;
  `bun test` — 481 pass, 0 fail; `make check` — exit 0. No rendering change,
  so no screenshots regenerated.

## Session provenance

| Stage | Coordinator ID | Harness | Native conversation ID | Model / effort |
| --- | --- | --- | --- | --- |
| work, fix round 1 | implementer (child of 32a6c484) | Claude Code | `1e99122f-a1ce-416a-bf0b-0630ff77488e` | claude-opus-5-5 / default |

## Outstanding

- Manual `make dev` check above.
