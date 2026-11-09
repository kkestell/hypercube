# Work log: GitHub status

- Task ID: `2026-11-09-003-github-status`
- Plan (repository-relative): `.agents/plans/2026-11-09-003-github-status.md`, blob `6a839caf26b790cc2707c6398aed2e6529358e27`
- Guidance: `.agents/designs/2026-11-09-001-github-integration.md`, blob `1be166e4fffb44898b53d6c18ad9f4fb89bd4152`
- Plan review: `.agents/reviews/plans/2026-11-09-003-github-status.md` (findings 1 and 2 resolved in the plan text)
- Author: impl-status, Claude Code, native conversation `59409197-d670-4900-b15a-31e269b91831`, `claude-sonnet-5-5`
- Status: complete
- Code base: `bfa7c7b5c20f19da12c3ab707ab90afaa531ae07`
- Code head: `8717353c8035eb4154039c703c05c7801d873634`
- Implementation commits: `8717353c8035eb4154039c703c05c7801d873634`

## Changes

- `internal/workspace/remote.go`: `Fetch` (no `FETCH_HEAD`, no prompts, cancellable) and `Tracking` (upstream, ahead, behind).
- `internal/github`: `Status` gains the 003 fields and `PullRequest` is added. `pass` reads saved choices in the same store read as the workspaces, applies one when several accounts qualify (`Choices` stays set), and calls the new `measure` (branch, fetch under a 60-second timeout, tracking, pull request query, joined errors, previous pull requests kept on a failed query for the same repo). A token read failure is the status's `Error`. `graphql.go` adds `pullRequests`.
- `internal/store/store.go`: `github_accounts` table, `GitHubAccounts`, `SetGitHubAccount`.
- `internal/ui`: `Handler(db, rt, gh)`; `snapshot` takes statuses, sets `workspaces[].github` and `sessions[].pullRequest` by the plan's `s.Dir` rule (checkout session: the status branch; session under `<path>/.hypercube/`: `s.Branch`; fork pull requests and ended sessions never match); `socket.go` resends state when `Changed` closes; `github-account` workspace action (400 `not an account to choose` outside `Choices`), and `new` calls `Refresh`.
- `cmd/hypercube`: handler takes and passes the watcher.
- `githubtest`: pull request answers (`SetPullRequests`), `Requests` counts permission queries only, `GH` fails the token for login `broken`.
- Docs: `docs/interface.md` (`## GitHub`), `agents/architecture.md`.

## Deviations

- The watcher test stand's `workspace` always points the remote URL at a local bare repository through `url.<bare>.insteadOf`, so every case's fetch stays off the network (the plan named this for the fetch cases; it is applied to all).
- `githubtest.API.Requests` now counts only permission queries; the pull request query is served by the same endpoint and would have broken the existing counts.
- Branch read failure is joined into `Error` like the other steps.

## Verification (target: Code head, macOS)

- `go test ./internal/workspace -run 'TestFetchAndTracking|TestRemote' -count=1`: pass.
- `go test ./internal/store -run GitHubAccount -count=1`: pass.
- `go test ./internal/github -race -count=3`: pass.
- `go test ./internal/ui -run 'TestSnapshotShows|TestStateCarries' -race -count=2`: pass.
- `make check`: exit 0, all packages ok (output in `/tmp/check003.out`), run on the code as committed.

## Provenance

| Role | Coordinator | Harness | Native conversation | Model |
| --- | --- | --- | --- | --- |
| implementer | impl-status | Claude Code | `59409197-d670-4900-b15a-31e269b91831` | `claude-sonnet-5-5` |

## Limitations

Stand-ins only; no live gh or GitHub API. Nothing in the browser reads the new fields yet (units 004 and 005).

## Completion

- Accepted work review: `.agents/reviews/work/2026-11-09-003-github-status.md` (round 1 accept)
- Final reviewed Code head: `8717353c8035eb4154039c703c05c7801d873634`
- Implementation commits: `8717353c8035eb4154039c703c05c7801d873634`
- Changes after the reviewed head on `hypercube/session-1771c79b` are workflow records under `.agents/` only and the code reviewed in later units' accepted ranges.

| Role | Coordinator | Harness | Native conversation | Model |
| --- | --- | --- | --- | --- |
| parent | Hypercube session 1771c79b | Claude Code | `2fb9801a-4985-41f3-bd41-371c1ae39e8e` | `claude-opus-5-5` |
| planner | planner | Claude Code | `1e20740e-b018-49eb-9ae2-27fe8948db63` | `claude-opus-5-5` |
| plan and work reviewer | plan-reviewer | Claude Code | `7989b8d0-1436-4b4f-8ad3-10b45e003508` | `claude-opus-5-5` |
