# Review: Review starts a worktree session at the pull request's head

- Task ID: 2026-11-09-009-review-starts-a-worktree-session
- Work log (repository-relative): `.agents/work/2026-11-09-009-review-starts-a-worktree-session.md`
- Plan (repository-relative): `.agents/plans/2026-11-09-009-review-starts-a-worktree-session.md`, blob `4840e94fedae422d8f4b8537074ec28b67b1f21c` (matches the log)

## Round 1

- Author: xreview worker p2-review (Hypercube child session of 777ef702), Claude Code, claude-opus-5-5, native conversation ID `6fe12acf-189c-4b15-87d7-0d6d3985a211`
- Code base: 72ccc1dcd099890f4aa759210ff2f39f99a5a34e
- Reviewed Code head: cbe99753bc5cc2e10a0dcbd815f68b745eaba512 (single implementation commit; base is its parent)
- Checkout: `/Users/kyle/projects/hypercube/.hypercube/777ef702` at 175e011e39fc115f33e0e3f89b4863521a835b49 (Code head plus the work-log commit only), clean status, no untracked files
- Verdict: **changes-required**. One finding (F1): when the session is created but its first input fails, the page loses the session. The rest of the request and plan is implemented and tested as specified.
- Independence: I did not write or plan this change. I read the full diff, the touched files, and their callers (`Runtime.create`/`input`, `refreshMerges`, `unkept`/`Removable`, `api.ts` `post`/`submitPrompt`, `deliverSubmission`, the Git panel and sidebar merge consumers).

### Findings

**F1 (required): a review session created before its first input fails is not opened, and each retry creates another one.** `internal/ui/actions.go:216-220` (`startReview`) and `internal/ui/web/github/review.ts:16-17` (`startReview`).

`Runtime.create` persists the session and its worktree, then returns `s, r.input(...)` (`internal/session/sessions.go:126`). `input` fails when the agent cannot be woken or does not start (`wake`, `awaitStart`). `startReview` passes that error to `refused`, which writes a plain-text 409 without the ID. In the browser, `post` shows the error and returns null, so nothing is selected. The user sees a toast, and an idle session with no transcript and a worktree at the pull request's head appears in the sidebar. Clicking **Review** again repeats the fetch and the creation, adding another orphaned worktree session on each attempt while the harness keeps failing.

The `"new"` action already handles this case. It responds 409 with JSON `{session, error}` (`actions.go:118-126`), and the page adopts the session (`api.ts:54-55`: "Creation can fail after persisting a session, in which case its ID still belongs to the page"; `deliverSubmission` selects `result.session`). Plan step 5 ("A session created before its first input failed still appears in the sidebar through the normal snapshot") chose this behavior, so this is a plan defect that the code follows. It still contradicts the request's "opens the session" and the existing contract.

Smallest correction: when `created.ID != ""` and `err != nil`, return the ID along with the error, in the same JSON shape `"new"` uses. In `review.ts`, select the session whenever an ID comes back, and show the error when there is one. Add a `pull-requests.test.tsx` case for a 409 that carries a session: the session is selected and the error is shown. The existing refused-review test then covers a 409 without a session.

### Areas the parent asked about

- **Fetch's temporary ref and error paths** (`internal/workspace/remote.go:598-609`): sound. The ref name `refs/hypercube/fetch/<rand.Text()>` is valid and unique, the `+refs/pull/N/head` source does not overlap the remote's configured refspec, so no tracking refs move, and `update-ref -d` runs after both a successful and a failed `rev-parse`. A failed fetch creates no ref. The test checks for leftover refs after both a successful fetch and a missing pull request. The only path that could leave a ref is a request context cancelled after Git updates the ref but before it exits. That window is negligible, and the ref would be harmless.
- **Merge status for ordinary worktree sessions** (`internal/workspace/workspace.go:683-687`): unchanged in practice. `refreshMerges` measures only sessions with a `Base`, and the browser never sends `start` for `"new"`, so every ordinary top-level worktree session starts at the checkout's `HEAD`, and its base's history is already excluded. The count differs only when the checkout's `HEAD` later stops containing the base (the main checkout switched to another branch or reset behind the base). Commits before the base no longer count then, which matches the requested rule. The `base != ""` branch mirrors `unkept` and is dead at the only call site, which is harmless. Removal is unaffected: `Removable` uses `unkept` with the session's base, so an untouched review session is removable.
- **HTTP error after creation**: see F1. Errors before creation (bad number 400; unknown workspace, no GitHub status, config, model rule, fetch, `AddWorktree` all 409) are correct and leave nothing behind.
- **Browser in-flight state** (`review.ts` `useReview`): correct. `post` never throws (`request` catches), so the key is always cleared. The key is per workspace and number, so one request cannot disable another row. Discrete clicks re-render before the next click, so a double click cannot start two reviews. State is per component, so the dashboard and the Pull Requests view each track their own in-flight requests. That is acceptable.

### Note (informational)

With the requested counting rule, a review session that commits once shows "1 commit not merged into main", but **Merge into …** merges the pull request's commits along with it. This follows the request ("counts only commits the session made") and the documentation states it. It is recorded here so phase 3 or the user can decide whether the summary should say so.

### Verification

- Reused: `make check` from the repository root at Code head cbe99753…, exit 0, `/tmp/p2-work-make-check.log` (Go tests incl. `TestFetchPullRequest`, `TestMergeStatus*`, `TestReviewSettings`, `TestReviewNeedsANumber`; `bunx tsc`; `bun test` incl. `pull-requests.test.tsx`, `review.test.ts`). The log's tree was the committed tree (status clean after commit, no hooks), so it applies to the reviewed head.
- Run: none. F1 follows from reading `create`/`input` and the handler, and no other open question needed a check.
- Not run (left for the user, per the work log): manual `make dev` with a real requested review.

### Limitations

- No end-to-end run against GitHub; the fetch against a real `refs/pull/<n>/head` (including forks) is covered only by the bare-remote test; the plan's claim that GitHub serves that ref for fork pull requests was not checked here.
