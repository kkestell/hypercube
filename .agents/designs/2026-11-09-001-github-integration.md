# GitHub integration

- Task ID: `2026-11-09-001-github-integration` (shared design; units 002–005)
- Status: ready; shared execution guidance for the four plans below
- Source HEAD: `47055e42f036f863ebb18110d3803fbce3d6f38b`
- Checkout: `/Users/kyle/projects/hypercube/.hypercube/1771c79b`, branch
  `hypercube/session-1771c79b`, clean when planning began
- Author: planner `planner`, Hypercube child of `1771c79b` (own Hypercube ID not
  exposed to the agent), Claude Code, native conversation
  `1e20740e-b018-49eb-9ae2-27fe8948db63`, model `claude-opus-5-5`
- All paths are repository-relative.

## Request

The user approved this design for GitHub support (parent `1771c79b` brief):

1. Use the GitHub CLI's stored accounts (`gh auth token --user`), never a pasted
   token or a keychain library. Without `gh`, or with no account, GitHub
   features are absent, with no errors shown.
2. Pick each workspace's account without setup: the checkout's
   `credential.githubAccount` Git setting; otherwise the one gh account that can
   push to the repository; otherwise (zero or several can push) the user picks
   from a list, saved per workspace. The repository comes from the remote the
   current branch tracks, falling back to `origin`. github.com only.
3. Put `GH_TOKEN` for that account in every agent's and panel shell's
   environment.
4. No new agent MCP tools.
5. Fetch each GitHub workspace in the background so ahead/behind stays fresh.
6. Browser: (1) workspace header with the repository linked, the account, and
   the branch's ahead/behind; (2) a pull requests list per workspace with
   review decision, checks, author, and row actions **Merge**, **Address
   review**, **Fix checks** that send an instruction to a session the user
   picks or start a new session; (3) a session whose branch has an open pull
   request shows its state on its sidebar row.
7. Poll GitHub about once a minute per workspace.

Out of scope: notifications for pull request events, GitHub Enterprise, remotes
beyond the fallback above.

## Units

| ID / plan | Responsibility | Depends on |
| --- | --- | --- |
| [002](../plans/2026-11-09-002-github-accounts.md) | `internal/github` watcher: gh accounts and tokens, repository and account resolution, `GH_TOKEN` in agent and shell launches, backend wiring | none |
| [003](../plans/2026-11-09-003-github-status.md) | Background fetch, ahead/behind, pull request query, status in the state snapshot, session pull request matching, saved choice table and account-choice action | 002 |
| [004](../plans/2026-11-09-004-github-workspace-header.md) | Browser: dashboard header, account chooser, session row pull request icon | 003 |
| [005](../plans/2026-11-09-005-github-pull-requests.md) | Browser: dashboard pull requests list, row actions, session picker | 004 |

Run them in order in the same checkout. Each unit leaves the app working: after
002 agents get the token but nothing shows; after 003 the state message carries
GitHub data the page ignores; 004 and 005 add the views.

Scope map: request 1–3 → 002; 5, 7 and the data for 6 → 003; 6(1) and 6(3) →
004; 6(2) → 005; 4 needs no work. The account dropdown of 2 is drawn in 004
using 003's action.

## Settled decisions

**Ownership.** A new package `internal/github` owns everything GitHub: running
`gh`, querying the API, resolving accounts, and polling. It knows workspaces
only through the store. The session runtime stays GitHub-free: it gains one
hook, `Runtime.Env func(workspace string) []string`, returning extra
environment for a workspace's new launches and shells. The UI reads statuses
from the watcher. Two parts with a one-function interface in each direction.

**Token path.** `harness.StartArgs.Env` is the MCP bridge's environment only
(every plugin passes it to the bridge's config, not the harness process), and
start commands are saved in the `commands` table. So the token goes in the
**holder's process environment** instead: `harnessclient.Launch` and
`terminal.Set.Open` take extra environment that is appended to
`os.Environ()`. Plugins already start their harness with `os.Environ()`, so the
harness, its tools, its `git`, and the MCP bridge all inherit `GH_TOKEN`. No
protocol or plugin change, and the token is never written to disk. `os/exec`
keeps the last of duplicate variables, so the appended value wins over a
`GH_TOKEN` the backend itself inherited.

**When the token applies.** The environment is fixed when a holder starts. A
running agent or shell keeps its environment; after the account changes (or is
first resolved), each agent gets the new token at its next launch: when it
parks after 15 idle minutes and is next used, or on **Restart Agent**. Launches
in the seconds before the watcher's first pass completes after backend start
get no `GH_TOKEN` and use gh's active account, which is how every launch worked
before this feature. `Env` never blocks on I/O: it reads the watcher's map
under a short lock, because `run` and `Shell` call it while holding the
runtime's lock.

**API transport.** The watcher calls GitHub's GraphQL endpoint directly with
`net/http` and the token it already holds for injection. It runs `gh` only for
`gh auth status --json hosts --hostname github.com` (account logins) and
`gh auth token --hostname github.com --user LOGIN`. One POST returns
permission and pull requests; `gh api`/`gh pr list` would add a subprocess per
call and `gh pr list --json statusCheckRollup` returns every check instead of
the rollup state. Tests serve GraphQL with `httptest` through a `Watcher.API`
field and stand in for `gh` with a script through `Watcher.GH`.

Verified with gh 2.86.0 against the live API: `gh auth status --json hosts`
returns `{"hosts":{"github.com":[{"login","state","active","host",...}]}}`
and contains no tokens; it tests each token over the network (about one second
for three accounts), so its `state` is not a reliable account list and the
watcher uses every listed login regardless of state. `repository(owner,name)`
answers `viewerPermission` (`ADMIN`, `MAINTAIN`, `WRITE`, `TRIAGE`, `READ`), and
`null` data with a `NOT_FOUND` error when the account cannot see the
repository. `PullRequest.statusCheckRollup { state }` and `reviewDecision`
exist directly on pull requests.

The watcher runs `gh` with `GH_TOKEN` and `GITHUB_TOKEN` removed from its
environment. Otherwise a backend started from an agent's shell (`make dev`),
which now has `GH_TOKEN`, would see only that one account.

**Merge semantics.** Row actions only send instructions to sessions, as the
existing worktree **Merge** button does (`requestMerge` in
`internal/ui/web/sessions/session-actions.ts`). **Merge** sends "merge pull
request #N"; the agent checks approval and checks and chooses the repository's
allowed merge method. No direct merge from the browser.

**Polling.** One watcher goroutine, owned by the backend app, runs a pass at
start, then every minute, and soon after `Refresh()`. A pass visits each Git
workspace in turn. Data reaches the browser in the existing state snapshot over
the `/state` WebSocket; the watcher's `Changed()` channel marks open sockets'
state stale, and the socket already skips sending an unchanged state.

**Failure and staleness.** gh missing or no logins: no statuses, so nothing
GitHub shows anywhere, and `Env` returns nothing. A failed fetch or query keeps
the last good data, records the message in `Status.Error`, and leaves
`Status.Updated` (the time of the last successful query) unchanged; the header
shows the error and how old the data is. Nothing retries early; the next pass
retries.

**Saved account choice.** A new table, created like every other table at
startup, so no migration:

```sql
CREATE TABLE IF NOT EXISTS github_accounts (
  workspace TEXT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
  account TEXT NOT NULL);
```

The choice is never written to the user's Git configuration (AGENTS.md: leave
project settings alone).

## Shared data shapes

Unit 002 creates these types with the first four `Status` fields; unit 003 adds
the rest. JSON names are the browser contract (`internal/ui/web/sessions/types.ts`
mirrors them in 004).

```go
// Status is a GitHub workspace's repository, account, branch, and pull requests.
type Status struct {
	Repo    string   `json:"repo"`              // owner/name
	URL     string   `json:"url"`               // https://github.com/owner/name
	Account string   `json:"account"`           // empty until the user chooses
	Choices []string `json:"choices,omitempty"` // accounts the user chooses among; set only when several qualify
	// Added by 003:
	Branch       string        `json:"branch"`             // checked-out branch; empty when detached
	Upstream     string        `json:"upstream,omitempty"` // such as origin/main
	Ahead        int           `json:"ahead"`
	Behind       int           `json:"behind"`
	PullRequests []PullRequest `json:"pullRequests"`       // open, most recently updated first, at most 50
	Open         int           `json:"open"`               // all open pull requests
	Updated      int64         `json:"updated"`            // Unix ms of the last successful query; 0 before one
	Error        string        `json:"error,omitempty"`    // why the last pass's fetch or query failed
}

type PullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Author string `json:"author"`
	Branch string `json:"branch"` // head branch
	Fork   bool   `json:"fork"`   // head is in another repository
	Draft  bool   `json:"draft"`
	Review string `json:"review"` // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, or empty
	Checks string `json:"checks"` // SUCCESS, PENDING, EXPECTED, FAILURE, ERROR, or empty with no checks
}
```

The state message gains `workspaces[].github` (omitted for a workspace that is
not on GitHub, or when gh is unavailable) and `sessions[].pullRequest` (a copy
of the matching `PullRequest`, omitted when none).

## Validation and release

Every code commit runs `make check`. Unit 002 changes how agents and shells
launch: per `agents/development.md`, the change reaches an active session only
after it restarts, and the parent must tell the user so before work starts and
when offering `make install-service`. No screenshots change.
