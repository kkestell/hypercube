# Work review: verified review fixes

## Round 1

- Task ID: 2026-11-09-011-verified-review-fixes
- Mode: work
- Author: reviewer, Hypercube child session review-verified-fixes-opus
  (Claude Code, claude-opus-5-5, effort not exposed); native conversation ID
  unavailable (not exposed to this session). Parent: coordinator 0987244a
  (OpenCode, openai/gpt-6.1-sol, xhigh), native conversation
  `ses_e3d29b976ffehNoBOsD5RvO3Xv`.
- Work log (repository-relative):
  `.agents/work/2026-11-09-011-verified-review-fixes.md`
- Plan: `.agents/plans/2026-11-09-011-verified-review-fixes.md`, blob
  `793b9abfb0fee2f660aff4afd765109d7dce5568` (verified with
  `git hash-object`); execution decision in
  `.agents/reviews/plans/2026-11-09-011-verified-review-fixes.md`
- Code base: `0040f146a84bf0be9776ebfad6c9f8023c98bf44`
- Code head: `71cbb6b78b522f303c4f555f9dbbebbfa76b969e` (base is an ancestor)
- Checkout: `/Users/kyle/src/hypercube/.hypercube/0987244a`, branch
  `hypercube/session-0987244a`, HEAD
  `b23078dfca6ccf2fb297c8c349f9ef3df831a2ef`, clean. The only change after
  Code head is the work log (commit `b23078d`, record only).

### Verdict: accept

The full `Code base..Code head` range (23 files) delivers the six outcomes
without scope growth. I did not write the plan or the implementation.

### Findings

None blocking.

### Assessment of the difficult points

- **#1 submodules.** `unpushedSubmodule` (`internal/workspace/workspace.go`)
  runs after the uncommitted-files check, only when the worktree exists, so
  every caller of `Removable`/`RemoveWorktree` (archive, delete, child removal,
  start-failure cleanup in `sessions.go` and `children.go`) gets the rule.
  `setUp` initializes submodules from their upstream, so a freshly set-up
  submodule sits on a remote-tracking commit and plain removal still works
  (`TestWorktreeSetUp` keeps passing). `$displaypath` paths and the
  `printf "%s\0"` format work under both bash and dash. The test covers the
  merged-gitlink case, the pushed case, and a commit left only on a submodule
  branch.
- **#5b merge target.** `Runtime.Merge` uses the same eligibility as
  `refreshMerges` (`Base != ""`, not ended), refuses before reading the branch
  or sending input, and on a mismatch refreshes the published target.
  `MergeStatus`, `Unmerged`, and the button's visibility rule are unchanged, so
  the pull-request-base counting is preserved. `r.input` is what
  `InputSubmission` called before, minus the submission ID.
- **#6 clone.** Git applies credential config with every matching entry in
  order (`select_all` in `credential.c`), and command-line `-c` entries come
  last, so `-c credential.helper=` also clears URL-scoped helpers such as
  `credential.https://github.com.helper`. Git ignores the helper's nonzero
  exit for `store`/`erase`. Nothing is written into the clone; the test
  confirms a configured `store` helper writes no file and the clone's config
  holds neither the token nor a helper.
- **#10 comment after a push.** `answered` limits only threads, only for the
  author's pull request in the responded changes-requested state, using the
  same `afterLastCommit` rule as conversation comments. `ownTurn` checks
  `unanswered` after unanswered change requests and before waiting. The three
  table cases and two turn cases cover the new comment, the old thread, and
  the newer reply.
- **#12 thread paging.** Paging runs before `fromNode`, so both `Unanswered`
  and `Responded` see every thread. Follow-up queries are made only for pull
  requests with `hasNextPage`, batched in one aliased query per round with
  typed variables (`ID!`, `String!` into the nullable `after` argument is valid
  GraphQL). The cost stays near two points per paged pull request per round.
  A missing node returns an error instead of looping.
- **#14 collapse.** A zero layout value for the fixed child now goes to
  `onCollapse` and never to `onResize`, including in the session editor split,
  which passes no `onCollapse`. `kept` keeps the last positive size, so showing
  the region restores it. `hideRegion` reuses `closePanel` for the terminal
  panel.

### Observation (not blocking)

- With the checkout on a detached HEAD, `Merge` returns
  `the checkout is now on , not main`. The refusal is correct; only the text
  is awkward. The button label for a detached checkout already shows an empty
  target from before this change.

### Verification

Reused (no fresh runs; nothing in the review raised a concrete doubt that a
new run would settle):

- `make check`, exit 0, log `/tmp/2026-11-09-011-make-check.log` (finished
  21:38; Code head committed 21:38:47 with the tree the log says was
  committed unchanged). UI build, `go build`, `bunx tsc -p .`, UI `bun test`
  451 pass / 0 fail, desktop `bun test` 28 pass / 0 fail, `go vet`, gofmt,
  `go test -race ./...` all `ok`. The changed packages (`internal/github`,
  `internal/workspace`, `internal/session`, `internal/ui`) ran fresh, not from
  cache.
- First `make check` failure at gofmt, `/tmp/2026-11-09-011-make-check-1-gofmt-fail.log`,
  fixed before the passing run.
- The focused regression and mutation checks recorded in the work log were
  read as claims; I inspected each new test's assertions and fixtures and
  confirmed they would fail without the corresponding fix.

### Limitations

- The GraphQL follow-up was checked against the fake API and the GraphQL
  rules, not the live GitHub API.
- Linux checks were not run; the plan does not require them.
- No browser screenshots; the collapse fix is covered by component and
  storage tests.
