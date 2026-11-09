# Global pull requests, part 1

- Task ID: 2026-11-09-006-global-pull-requests
- Status: ready
- Source HEAD: 854540adf2cba5f1b739ddd06135b3f1bb4df562
- Author: xplan child session plan-global-prs (parent 04ef1b14), Claude Code, claude-opus-5-5
- Dependencies: none

## Outcome and boundaries

Read pull requests once per signed-in gh account instead of once per workspace.
Each workspace dashboard shows that data filtered to its repository and account.
A new global view lists every account's **Your move** pull requests across all
repositories, including repositories with no workspace. Each workspace's sidebar
row shows its **Your move** count.

Keep: the classification in `internal/ui/web/github/turns.ts` and `fromNode` in
`internal/github/pulls.go`; the required-check follow-up query; per-workspace
account resolution, fetch, ahead/behind; session pull request icons; the
workspace actions (Merge, Address review, Fix checks).

Out of scope: cloning an untracked repository into a workspace, desktop
notifications, merging rows for one pull request seen by two accounts.

## Current system

`Watcher.pass` (`internal/github/watcher.go`) resolves each workspace's repo and
account, reads tokens through a per-pass cache, and calls `measure`, which
fetches, reads tracking, and calls `pullRequests(ctx, token, login, owner, name)`
(`pulls.go`) with three repo-scoped searches plus the repo's open count. A
failed query keeps the previous pull requests. `publish` swaps `statuses` and
`tokens` and closes `changed`. `internal/ui/snapshot.go` copies each `Status`
into `shownWorkspace.GitHub` and matches session branches against
`Status.PullRequests`; `socket.go` resends state when `Changed` fires.
`pull-requests.tsx` sorts with `sortTurns` and links "N other open pull
requests" from `Status.Open`. With workspaces present and nothing selected,
`SessionView` (`sessions/session-view.tsx`) shows an empty center.

## Decisions

**Query and cost (measured live, read-only, 2026-11-09).** One query per account:

```graphql
query($login:String!,$involved:String!,$requested:String!,$direct:String!){
  involved:search(query:$involved,type:ISSUE_ADVANCED,first:50){nodes{...pr}}
  requested:search(query:$requested,type:ISSUE,first:20){nodes{...pr}}
  direct:search(query:$direct,type:ISSUE,first:50){nodes{...on PullRequest{url}}}}
```

The `pr` fragment is today's, plus `repository{nameWithOwner}`. Search strings:
`is:pr is:open sort:updated-desc (involves:@me OR review-requested:@me OR reviewed-by:@me)`,
`is:pr is:open review-requested:@me`, `is:pr is:open user-review-requested:@me`.
Measured cost: 32 points per account per pass (45 with `requested` at 50; 18
with everything at 20). All three accounts answered; kkestell spans 2 repos,
kkestell-livefront 3, kylekestell-startribune 7. At one pass a minute that is
about 1,900 of each account's 5,000 hourly points, versus today's ~9 points per
workspace per minute. It is cheaper whenever an account backs four or more
workspaces and is a known cost otherwise; the nested `reviewThreads`/`comments`
connections dominate it and are needed by the classification, so they stay.

**Identity.** Pull requests now span repositories, so key them by `url`
everywhere: dedupe of involved+requested, the requested and direct sets, the
required-failure results, and React keys. `PullRequest` gains
`Repo string \`json:"repo"\`` (owner/name, from `nameWithOwner`).

**Required checks.** `requiredFailures(ctx, token, failing []PullRequest)`
returns `map[string]bool` by URL and makes one query per account per pass,
grouping by repository with aliases: `r0:repository(owner:%q,name:%q){p12:pullRequest(number:12){…}}`.
Owner and name come from GitHub's own `nameWithOwner`, so `%q` quoting is safe.

**Ownership and data shape.** The watcher still owns everything; Run, Refresh,
and the one-minute interval stay. New type and accessor:

```go
// Account is a signed-in gh account's open pull requests in every repository.
type Account struct {
	Login        string        `json:"login"`
	PullRequests []PullRequest `json:"pullRequests"` // never nil; most recently updated first, at most 50, then further review requests
	Updated      int64         `json:"updated"`         // Unix ms of the last successful query; 0 before one
	Error        string        `json:"error,omitempty"` // why the last token read or query failed
}
func (w *Watcher) Accounts() []Account // sorted by login
```

`pass` order: logins → read every login's account (token via the existing
per-pass cache, then the query) → the existing workspace loop. Every signed-in
login is read, whether or not a workspace uses it. `measure` keeps the fetch and
tracking and no longer queries. After `measure`, a workspace with an account
gets `PullRequests` = that account's pull requests whose `Repo` matches the
workspace repo by `strings.EqualFold`, and `Updated` = the account's `Updated`.
The workspace's `Error` gains the account's `Error` only when the workspace's
own token read succeeded, so a token failure is not reported twice. A workspace
without an account keeps an empty list and `Updated` 0. `publish(statuses,
tokens, accounts)` closes `changed` when either statuses or accounts differ.
`Status.Open` is deleted. Snapshot session icons keep working unchanged because
`Status.PullRequests` is still filled, now by filtering.

**Failures.** A failed token read or query sets that account's `Error` and keeps
its previous `PullRequests` and `Updated` when the previous pass had that login;
otherwise the list is empty and `Updated` is 0. A rejected token
(`errUnauthorized`) is an ordinary failure. Other accounts are unaffected. A
login gh no longer lists disappears. A Refresh pass (workspace added, account
chosen) re-reads all accounts; these are rare user actions.

**Account choice.** Unchanged: `credential.githubAccount`, detection, and the
chooser decide `Status.Account`; the dashboard filters by it. Choosing an
account calls `Refresh`, and the next pass fills the list. Until then the
dashboard says "Choose an account to see pull requests."

**Other open pull requests link.** Drop the count. Below the lists, always show
"All open pull requests" linking `${url}/pulls`. It costs nothing, and the old
count could not survive a cross-repository search without one extra query
field per repository and account. The "No open pull requests." text goes away;
an empty **Your move** still says "Nothing needs you."

**Global view placement.** The home view: shown in the center when nothing is
selected (`state.selected === null`, no unsent session, no editor). A new
**Pull Requests** button at the top of the sidebar's Sessions tab body, above
the workspaces, opens it; it shows the git-pull-request icon, the label, the
total **Your move** count across accounts when non-zero, and `aria-pressed` while
the home view is shown. It renders only when `accounts` is non-empty. Opening
the app without a hash keeps today's behavior of selecting a session.

**Global rows.** Only **Your move** pull requests, for every account, each
account in login order and stably sorted by `Turn.rank` across all. One pull
request under two accounts gives two rows. Rows reuse the workspace row: icon,
`#N title` link, the turn reason, "From author." when someone else wrote it,
and a line "In {repo} for {account}." with the repo linking
`https://github.com/{repo}`. No action buttons; those need a workspace. Above
the rows, one warning line per failed account, with the existing header's form:
"Could not read pull requests for {login}: {first line}." plus "Showing data
from {time}." when `updated > 0`. While every account has `updated` 0 and no
error: "Checking GitHub." With no rows: "Nothing needs you."

**Sidebar count.** In `WorkspaceSection`'s header, right after the name, when
the count is non-zero: the git-pull-request icon and the number in the same
11px muted text, with `title` and accessible name "N pull requests need you"
(singular for 1). Count = `sortTurns(github.pullRequests, github.account).mine.length`.

## Implementation sequence

1. **`pulls.go`.** Replace `pullRequestsQuery` and `pullRequests` with
   `accountPullRequests(ctx, token, login) ([]PullRequest, error)` per the query
   above; `fromNode` sets `Repo` and otherwise is unchanged. The failing list
   stays "own pull requests with FAILURE or ERROR checks". Rewrite
   `requiredFailures` as decided. Update the comments.
2. **`watcher.go`.** Add `Account`, the `accounts` field (guarded by `mu`),
   `Accounts()`, the new `pass` order, the filter, and the error rule. Delete
   `Open` and the query from `measure`; update `Status` comments.
3. **`githubtest`.** `SetPullRequests(involved, requested []map[string]any,
   direct ...string)` with direct URLs, no total. Detect the pull request query by
   `involved:search` instead of `pullRequests`. `SetRequiredFailures(urls ...)`
   answers under the `r…`/`p…` aliases, which it reads from the request body
   with a regexp, marking a failure for each alias whose repository and number
   form one of the URLs.
4. **`internal/ui`.** `stateMessage` gains `Accounts []github.Account
   \`json:"accounts"\``, filled from `d.gh.Accounts()` in `sendState`.
5. **Browser data.** `types.ts`: `repo` on `PullRequest`, new `GitHubAccount`,
   drop `open`. `state.ts` gains `accounts: GitHubAccount[]` (initial `[]`);
   `data-socket.ts` stores `message.accounts`. `test-state.ts` defaults follow.
6. **`turns.ts`.** Add `yourMoves(accounts)` returning
   `{account, pr, turn}[]`, as decided. Classification stays untouched.
7. **Row and lists.** Move `Row` to `github/pull-request-row.tsx` with an
   optional place line and optional actions. `pull-requests.tsx` keys by URL,
   uses the new link and empty texts. New `github/all-pull-requests.tsx` renders
   the global view, `aria-label="Your move across accounts"`.
8. **Home and sidebar.** `selection.ts` gains
   `showHome = (state) => attach({...parkUnsent(state), selected: null, pending: null})`.
   `SessionView` renders the global view in the home case, before the existing
   no-workspaces invitation. Add the sidebar button and the workspace count.
9. **Docs.** `docs/interface.md` GitHub section: per-account reading, the global
   view and its sidebar button, the sidebar count, the filter, the new link.
   `docs/dev/architecture.md` watcher paragraph: per-account reads and
   `accounts` in the snapshot. Read `docs/dev/documentation.md` first; no
   screenshots.

## Verification

Go, in `internal/github/watcher_test.go`, through `Run`/`Statuses`/`Accounts`:

- Map and keep on failure: nodes from two repos; `Accounts()` has both with
  `Repo`; the workspace for `o/r` has only its own. After `Fail(true)`, both the
  account and the workspace keep data and `Updated`, with `Error` set.
- Repo match is case-insensitive (`O/R` node, `o/r` remote).
- Two logins each get an account with its own list; the same URL under both stays
  in both.
- Direct versus team: `requested` keyed by URL across repos with equal numbers
  (`o/r#1` direct, `o/s#1` team).
- Required failures across two repos mark only the listed URLs.
- Token failure: login `broken` has an account with `Error` and `Updated` 0,
  another login still reads; a workspace on `broken` reports the error once.
- A workspace pending a choice has an empty list and `Updated` 0 while
  `Accounts()` is filled.
- Keep `TestPullRequestsDescribeTheAccountsPart` passing with the new stand-in.

`internal/ui/github_test.go`: the state carries `accounts`. Snapshot icon test
stays as is.

Browser (`bun test` in `internal/ui`): `pull-requests.test.tsx` drops `open`,
checks the "All open pull requests" link; new `all-pull-requests.test.tsx`
covers rows from two accounts and two repos with the place line, duplicates
across accounts, rank order, waiting rows excluded, a failed-account warning,
"Checking GitHub.", and "Nothing needs you."; `turns.test.ts` covers
`yourMoves`; `sidebar.test.tsx` covers the count's accessible name, its absence
at zero, and that the Pull Requests button shows the home view.

Commands: `go test -race ./internal/github ./internal/ui`, `bunx tsc -p .` and
`bun test` in `internal/ui`, then `make check`. Live: the running `make dev` on
port 4281 rebuilds; open it, confirm the home view lists hypercube-pr-lab
review requests under kkestell and kkestell-livefront and that the sidebar
counts match the dashboards. Do not restart that server.

## Handoff

Ready. Work size: one normal-to-large assignment of one mechanism (per-account
reads) carried through backend, wire, and three browser surfaces; about ten
production files and four test files. If it proves too large, the clean split
is steps 1–4 (backend, with step 5's type change) and steps 5–9 (browser).
Detail: decisions are settled; helper names and markup are left to the
implementer. `plan_metrics.py` is not installed; `wc -w` measures about 1,760
words, above the 1,500 band. The excess is the measured query cost and the
three UI placement decisions the request asked to settle, not code; splitting
would add a wire-format handoff between units without removing any decision.
