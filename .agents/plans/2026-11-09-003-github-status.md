# Fetch, measure, and query each GitHub workspace and send it to the browser

- Task ID: `2026-11-09-003-github-status`
- Status: ready
- Source HEAD: `47055e42f036f863ebb18110d3803fbce3d6f38b` (plan written before 002)
- Author: planner `planner`, Claude Code, native conversation
  `1e20740e-b018-49eb-9ae2-27fe8948db63`, `claude-opus-5-5`
- Shared design (repository-relative): `.agents/designs/2026-11-09-001-github-integration.md`,
  [linked here](../designs/2026-11-09-001-github-integration.md).
- Prerequisite: [002](2026-11-09-002-github-accounts.md) committed. This plan
  extends its `Watcher` pass, `Status`, and `githubtest` stand-ins.

## Outcome and boundaries

Each pass also fetches the workspace's remote, measures the checked-out branch
against its upstream, and lists open pull requests. The `/state` message carries
`workspaces[].github` and `sessions[].pullRequest`, resent when the watcher
changes. A browser action saves the user's account choice, and the watcher uses
it when several accounts qualify. The page does not
display any of it yet (units 004–005). No pull request writes.

## Current system

- `internal/ui/socket.go:state` subscribes to runtime changes, sends a state
  from `snapshot(d.db, d.rt.Merges())`, and on a change without a session sets
  `d.stale` and flushes after `coalesce`; `sendState` skips bytes equal to the
  last state sent.
- `internal/ui/snapshot.go:showSession`/`show` build `shownSession` from
  `store.Session`. `Branch` names a managed worktree's branch; for a top-level
  session in the registered checkout, `internal/session/lifecycle.go` records
  the checkout's branch at each launch, so it goes stale when the checkout
  switches branches while the session runs or is parked.
- `internal/ui/actions.go:workspaceAction` handles `new` and `delete`.
- `internal/ui/ui.go:Handler(db, rt)` is called by `cmd/hypercube/http.go:handler`
  and by tests in `internal/ui` and `cmd/hypercube/http_test.go`.

## Changes

### `internal/workspace/remote.go`

- `Fetch(ctx, dir, remote string, env []string) error`: `git fetch --quiet
  --no-write-fetch-head REMOTE` with `env` and `GIT_TERMINAL_PROMPT=0` appended
  to `os.Environ()`, killed when `ctx` ends; errors carry Git's stderr like
  `gitError`. `--no-write-fetch-head` keeps the background fetch from replacing
  `FETCH_HEAD` between an agent's own `git fetch` and merge.
- `Tracking(dir) (upstream string, ahead, behind int, err error)`:
  `git rev-parse --abbrev-ref --symbolic-full-name @{upstream}`; failure means
  no upstream and returns zeros and no error. Otherwise
  `git rev-list --left-right --count HEAD...@{upstream}` gives `ahead\tbehind`.

### Watcher pass (`internal/github`)

Add the remaining `Status` and `PullRequest` fields from the shared design.
After step 4 of 002's account resolution, per workspace:

1. `Branch` from `workspace.Branch(path)`.
2. With a token: `Fetch` the remote from `Remote` with `GH_TOKEN=token`, under a
   60-second timeout derived from the pass context.
3. `Tracking` sets `Upstream`, `Ahead`, `Behind` (after the fetch, so they are
   fresh; without a token they reflect the last fetch anyone made).
4. With a token: the pull request query below sets `PullRequests` (never nil;
   `[]` when none), `Open` (`totalCount`), and `Updated` (now, Unix ms).
5. Failures in 2–4 do not stop later steps. Join their messages with
   `errors.Join` into `Error`. When the query fails, keep the previous status's
   `PullRequests`, `Open`, and `Updated` if its `Repo` is the same.

A workspace without a token (choice pending, or 002's step 3 failed to read
it) skips 2 and 4: `PullRequests` is `[]`, `Updated` 0. When the token read
failed, `Error` is that error's message, so the header explains the missing
data; a pending choice leaves `Error` empty. `Updated` changes every successful pass, so the watcher
publishes every pass; the socket sends at most one state per pass per page.

```graphql
query($owner:String!,$name:String!){repository(owner:$owner,name:$name){
  pullRequests(states:OPEN,first:50,orderBy:{field:UPDATED_AT,direction:DESC}){
    totalCount
    nodes{number title url isDraft isCrossRepository headRefName
      author{login} reviewDecision statusCheckRollup{state}}}}}
```

`author`, `reviewDecision`, and `statusCheckRollup` may be null: map them to
empty strings. `isCrossRepository` → `Fork`, `headRefName` → `Branch`,
`isDraft` → `Draft`, `statusCheckRollup.state` → `Checks`.

### State snapshot (`internal/ui`)

- `Handler(db, rt, gh *github.Watcher)`; `server` keeps `gh`. Update
  `cmd/hypercube/http.go:handler` to take and pass the watcher from 002's
  `startApp`, and tests to pass `&github.Watcher{}` (the zero value never runs,
  reports no statuses, and never closes `Changed`).
- `snapshot(db, merges, statuses map[string]github.Status)`; `shownWorkspace`
  gains `GitHub *github.Status \`json:"github,omitempty"\`` from `statuses`.
- `shownSession` gains `PullRequest *github.PullRequest
  \`json:"pullRequest,omitempty"\``, set for every non-ended session, top-level
  or child, in a GitHub workspace. The session's branch is, by `s.Dir`: when it
  is the workspace path or inside it but not inside `<path>/.hypercube/`, the
  status's `Branch` (the checkout's current branch, ignoring `s.Branch`); when
  it is inside `<path>/.hypercube/`, `s.Branch` (the managed worktree's
  branch, which may be empty); otherwise none. The
  pull request is the first with `!Fork` and `Branch` equal to that non-empty
  branch.
- `socket.go:state`: before the first `sendState`, take `changed :=
  s.gh.Changed()`; add a select case that, when it closes, takes the next
  channel, sets `d.stale`, and arms the coalesce timer as a runtime change
  does. `sendState` passes `d.gh.Statuses()`.

### Saved account choice

Add the `github_accounts` table from the shared design to the schema in
`internal/store/store.go`, with `Tx.GitHubAccounts() (map[string]string,
error)` (by workspace ID) and `Tx.SetGitHubAccount(workspace, account) error`
(upsert). The watcher's pass reads the map in the same store read as the
workspaces. In 002's step 2, when several candidates qualify, `Account` is the
saved choice if it is a candidate, else empty; `Choices` stays set either way
so the user can change it.

### Account choice action

`POST /workspace?action=github-account&id=W&account=A`: if `A` is not in
`gh.Statuses()[W].Choices`, answer 400 `not an account to choose`; else save
it with `tx.SetGitHubAccount`, then `gh.Refresh()`. `workspaceAction`'s `new`
also calls `gh.Refresh()` after adding the workspace, so its status arrives
within a pass rather than a minute.

### Documentation

Extend the `## GitHub` section of `docs/interface.md`: Hypercube fetches each
GitHub workspace's tracked remote and reads its open pull requests about once a
minute, using the workspace's account.

## Verification

- `internal/workspace`: `Fetch` from a local bare remote brings a new commit
  and leaves no `FETCH_HEAD`; `Tracking` reports ahead 1/behind 2 after
  diverging, and empty upstream with zeros for a branch without one.
- `internal/github` watcher tests (002's stand-ins, extended with pull request
  answers): the workspace's `remote.origin.url` is
  `https://github.com/o/r.git` and its local `url.<bare path>.insteadOf` is the
  same URL, so `Remote` sees GitHub and `git fetch` reads the bare repository.
  Cases: a commit pushed to the bare repository from another clone gives
  `Behind` 1 after a pass; two pull requests, one with null author, rollup and
  review, map as specified with `Open` from `totalCount`; switching the server
  to 500 keeps the previous pull requests and `Updated` and sets `Error`;
  a pending choice gives `PullRequests` `[]`, empty `Error`, and no fetch; a
  fake gh whose `auth token` fails gives the account, `Updated` 0, and `Error`
  naming the token failure; with two pushers,
  after `SetGitHubAccount` and `Refresh` the saved one is `Account`, `Choices`
  still lists both, and `Env` returns its token; a saved choice that is no
  longer a candidate is ignored.
- `internal/store`: `SetGitHubAccount` round trip and overwrite, and the row
  goes away with its workspace.
- `internal/ui` `snapshot` table test with fixed statuses: workspace `github`
  present and absent; worktree session matched by `Branch`; checkout session
  whose saved `Branch` is a stale other branch matched by the status branch,
  not the stale one; fork pull request with the same branch not matched;
  ended session not matched; session in another managed worktree without a
  branch not matched.
- `internal/ui` socket test with a running watcher on the stand-ins: a state
  message with `workspaces[0].github.repo == "o/r"` arrives; the
  `github-account` action rejects an account outside `Choices` with 400 and,
  for a valid one, a later state shows it as `account`.
- `make check` before committing.

## Handoff

Ready. Work size: extends one pipeline (watcher pass → snapshot → socket) with
two Git helpers, one query, a matching rule, and one action, plus Git, watcher,
snapshot, and socket tests; a normal execution assignment. Detail: failure,
staleness, freshness, and matching behavior are settled; no open decisions.
