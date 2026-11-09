# Work review: 2026-11-09-006-global-pull-requests

## Round 1

- Task ID: `2026-11-09-006-global-pull-requests`
- Mode: `work`
- Author: reviewer `review-global-prs`, Hypercube child of `04ef1b14`, Claude
  Code, native conversation `e3fb53e6-b1a8-4613-be42-9098ff5686c0`, model
  `claude-opus-5-5`. Independent of the implementer (`work-global-prs`,
  Sonnet). The same session wrote the plan review (round 1 of
  `.agents/reviews/plans/2026-11-09-006-global-pull-requests.md`).
- Work log (repository-relative): `.agents/work/2026-11-09-006-global-pull-requests.md`
  (commit `375c523a8c5cc54d2028ba1a508270987a3008a1`)
- Plan: `.agents/plans/2026-11-09-006-global-pull-requests.md`, blob
  `e13158b1d647d7805d17a3d5460d6e210e6c3860` (matches the execution decision)
- Code base: `1dd840307885152d1716294456dab2f5b8f5557f`
- Code head: `ab9673b1cddc3cd9782c08b92503382fae031e8c` (base is an ancestor)
- Checkout: HEAD `375c523` (work log only after the Code head; no code
  differences under `internal/` from the Code head); working tree clean.

### Verdict

`changes-required`. The backend change is correct and follows the plan. Two
defects block acceptance: a test that starts two watcher loops and races, and a
home view that says "Checking GitHub." indefinitely when no gh account is
signed in.

### Findings

1. **Two `Run` loops in one test race on `w.detected` (required).**
   `internal/github/watcher_test.go:576-578`, `TestPendingChoiceDoesNotFetchOrQuery`
   calls `s.run()`, then `SetPullRequests`, then `s.run()` again. Two
   goroutines run `pass` at once and both write `w.detected`
   (`watcher.go:341`), which belongs to the single `Run` goroutine. Evidence:
   `go test -race -count=20 -run TestPendingChoiceDoesNotFetchOrQuery ./internal/github`
   reported "race detected" in 1 of 20 runs, at `watcher.go:341` from both
   goroutines. The log's passing race run was luck. Correction: call
   `SetPullRequests` before a single `s.run()`.

2. **Home view without accounts is stuck on "Checking GitHub." (required).**
   `internal/ui/web/sessions/session-view.tsx:37` shows `AllPullRequests`
   whenever workspaces exist and nothing is selected, without checking
   `state.accounts`. In `all-pull-requests.tsx:17`, `checking` is
   `accounts.every(...)`, which is true for an empty list. A user without gh,
   or with no signed-in login (the watcher publishes no accounts), now sees a
   "Pull Requests" heading and "Checking GitHub." forever where the center
   used to be empty. The sidebar button already requires
   `state.accounts.length > 0`. Correction: add the same condition to the home
   branch in `SessionView`, so with no accounts the center stays as before.

3. **Duplicated warning helpers (minor).** `all-pull-requests.tsx:9-11` copies
   `sentence` and `time` from `github-header.tsx:14-16`, and line 22 copies the
   header's warning sentence shape. Correction: export the two helpers from one
   place, either `github-header.tsx` or `wording.ts`, and import them.

4. **Error merge is roundabout (minor).** `watcher.go:331-333` joins the
   account's error onto `status.Error` after `measure`, using
   `slices.DeleteFunc` over a two-element slice. Appending
   `errors.New(account.Error)` to `problems` before calling `measure` (when
   the token read succeeded) gives the same newline-joined result through the
   existing `errors.Join`. This needs the account lookup before `measure`; the
   filter can stay where it is.

### Checked against the plan

- **Classification unchanged.** `fromNode` only gains `Repo`. `turnOf` and
  `sortTurns` are untouched, and `turns.ts` only adds `yourMoves`.
- **URL identity.** `requested`, `direct`, `seen`, and the required-failure
  results are keyed by URL. The non-pull-request guard is `n.URL == ""`.
  React keys use the URL in the dashboard and `account + URL` in the global
  list, so the intended duplicates across accounts do not collide.
- **Required-check aliasing.** `requiredFailures` makes one query with an
  `rN:repository` alias per repository and a `pN` alias per number. `pN` is
  unique within a repository because URLs are deduplicated first.
  `TestRequiredFailuresAcrossRepositories` uses equal numbers in two
  repositories and checks that only the listed URLs are marked. The stand-in
  parses the aliases out of the real query text, so it checks the query
  shape.
- **Errors and keeping data on failure.**
  - A failed token read or query keeps the account's previous list and
    `Updated`, and sets `Error`. A first failure leaves an empty list and
    `Updated` 0.
  - A workspace adds the account's error only when its own token read
    succeeded (`t != ""`). `TestTokenFailureIsReported` checks that the
    error appears exactly once.
  - When gh is missing, all three maps are cleared.
  - `Changed` fires when either statuses or accounts differ.
- **Filtering and session icons.** The workspace list is the account's list
  filtered with `strings.EqualFold`. `snapshot.go` still matches branches
  against `Status.PullRequests`, unchanged. `Status.Open` is gone end to end.
- **Home view and selection.**
  - `showHome` parks the unsent session and clears the selection through
    `attach`. `homeShown` matches the plan's condition.
  - With no workspaces the invitation shows and the button is hidden.
  - `yourMoves` uses a stable sort across accounts in login order, as
    planned.
- **Proportionality.** No speculative machinery. `PullRequestRow` takes
  optional `send` and `place` props, the smallest way to share the row.
- **Docs.** `docs/interface.md` and `docs/dev/architecture.md` describe the
  new behavior plainly.

### Verification

- Reused (work log, at Code head `ab9673b`): `go test -race ./internal/github ./internal/ui`,
  `bunx tsc -p .`, `bun test` (440 pass), `make check`, and the live
  state-socket check. Code under `internal/` at the checkout is identical to
  the Code head.
- Run: `go test -race -count=20 -run TestPendingChoiceDoesNotFetchOrQuery ./internal/github`
  at the Code head. It failed (race in 1 of 20, run twice). The run was needed
  because the test visibly starts two loops (finding 1).
- Finding 2 comes from reading the code; I did not drive the browser.

### Limitations

I did not exercise the home view in the browser or check the sidebar
layout visually.
