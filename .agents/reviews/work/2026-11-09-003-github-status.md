# Work review: 2026-11-09-003-github-status

## Round 1

- Task ID: `2026-11-09-003-github-status`
- Mode: `work`
- Author: reviewer `plan-reviewer`, Hypercube child of `1771c79b`, Claude Code,
  native conversation `7989b8d0-1436-4b4f-8ad3-10b45e003508`, model
  `claude-opus-5-5`. Independent of the implementer
  (`59409197-d670-4900-b15a-31e269b91831`). This reviewer wrote the plan review,
  not the plan or code.
- Work log (repository-relative): `.agents/work/2026-11-09-003-github-status.md`
  (committed in `9156ee3acabd1adaf0f3530f6ce774d1d566feef`)
- Plan: `.agents/plans/2026-11-09-003-github-status.md`, blob
  `6a839caf26b790cc2707c6398aed2e6529358e27` (matches the execution decision)
- Guidance: `.agents/designs/2026-11-09-001-github-integration.md`, blob
  `1be166e4fffb44898b53d6c18ad9f4fb89bd4152` (matches)
- Code base: `bfa7c7b5c20f19da12c3ab707ab90afaa531ae07` (ancestor of head, verified)
- Code head: `8717353c8035eb4154039c703c05c7801d873634`
- Checkout: HEAD `9156ee3acabd1adaf0f3530f6ce774d1d566feef`; the only change
  after Code head is the work log. Working tree clean.

### Verdict

`accept`. No findings. Both plan-review corrections are implemented as
decided, and the background fetch, schema, and snapshot shapes hold up.

### Independent checks of the difficult invariants

- **Session branch (plan-review finding 1).** `internal/ui/snapshot.go`
  `pullRequest` uses `s.Branch` under `<path>/.hypercube/`, the status branch
  elsewhere in the checkout, and no branch outside the checkout. Fork pull
  requests and ended sessions never match. Children go through the same
  function. Session and worktree directories derive from `w.Path`
  (`internal/session/sessions.go:98`, `workspace.WorktreeDir`), so the
  string-prefix test compares like with like. `TestSnapshotShowsGitHubStatusAndPullRequests`
  gives the checkout session a stale saved branch with its own pull request
  (#4) and asserts the live branch's pull request (#5) instead.
- **Token read failure (plan-review finding 2).** `pass` adds the token error
  to `problems`, and `measure` joins it into `Error`. A pending choice has no
  token and no error. `TestTokenFailureIsReported` and
  `TestPendingChoiceDoesNotFetchOrQuery` cover both.
- **Background fetch leaves the user's checkout alone.**
  `workspace.Fetch` runs `git fetch --quiet --no-write-fetch-head REMOTE` with
  the default refspec. It updates only remote-tracking refs and never the
  index, working tree, or `HEAD`. `Branch` and `Tracking` only read. The test
  asserts there is no `FETCH_HEAD`. The fetch has a 60-second timeout derived
  from the pass context and runs with `GIT_TERMINAL_PROMPT=0`.
- **Failure and staleness.** Each pass builds a fresh `Status`, so `Error`
  clears on success. A failed query keeps the previous pull requests, `Open`,
  and `Updated` only when `Repo` is unchanged. `TestPullRequestsMapAndKeepOnFailure`
  checks null author, review, and rollup mapping, and the keep-on-failure case.
- **Schema.** `github_accounts` is a new `CREATE TABLE IF NOT EXISTS` with
  `ON DELETE CASCADE`. `agents/development.md` says startup creates missing
  tables, so adding a table needs no migration and changes no existing state.
  The store opens with `foreign_keys(1)`, and the store test confirms the row
  goes away with its workspace.
- **Socket.** The watcher's channel is taken before the first `sendState`,
  and a close marks the state stale and arms the existing coalesce timer, as
  a runtime change does. `sendState`'s byte comparison still skips unchanged
  states.
- **Action.** `github-account` rejects an account outside the workspace's
  `Choices` (and an unknown workspace, which has none) with 400, saves, and
  refreshes. `new` refreshes. The socket test covers both rejections and a
  valid choice reaching the browser.
- **Shapes for 004 and 005.** `workspaces[].github` carries `repo`, `url`,
  `account`, `choices?`, `branch`, `upstream?`, `ahead`, `behind`,
  `pullRequests` (never null from a pass), `open`, `updated`, and `error?`.
  `sessions[].pullRequest` carries `number`, `title`, `url`, `author`,
  `branch`, `fork`, `draft`, `review`, `checks`, and is omitted when there is
  none. These match the design's JSON, which plans 004 and 005 consume.

The log's deviations are sound. Every test workspace fetches from a local
bare repository. `Requests` counts only permission queries, so the 002
counts stay valid. A branch read error joins `Error`.

### Verification

- `reused`: `make check` at Code head; output in `/tmp/check003.out`, written
  the same minute as the commit. It has no `FAIL` line, Go packages
  `internal/github`, `internal/store`, `internal/ui`, `internal/workspace` are
  `ok`, and the browser (405) and desktop (28) bun tests pass.
- `reused`: the focused tests listed in the work log, at Code head.
- No new runs; the reused evidence covers the reviewed revision.

### Limitations

Stand-ins only, as in the log. A background fetch can still start Git's
automatic maintenance, as any fetch does; it does not touch the working tree
or `HEAD`. The SSH prompt advisory from the plan review still applies.
