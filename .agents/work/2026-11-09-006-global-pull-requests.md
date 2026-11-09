# Work log: global pull requests, part 1

- Task ID: 2026-11-09-006-global-pull-requests
- Plan (repository-relative): `.agents/plans/2026-11-09-006-global-pull-requests.md`, blob `e13158b1d647d7805d17a3d5460d6e210e6c3860`
- Plan review and execution decision (`proceed`): `.agents/reviews/plans/2026-11-09-006-global-pull-requests.md`; no guidance blobs
- Author: xwork child session `work-global-prs` (parent 04ef1b14), Claude Code, claude-sonnet-5-5
- Status: implemented
- Code base: 1dd840307885152d1716294456dab2f5b8f5557f
- Code head: ab9673b1cddc3cd9782c08b92503382fae031e8c
- Implementation commits: ab9673b1cddc3cd9782c08b92503382fae031e8c

## What changed

- `internal/github/pulls.go`: one `accountPullRequests` query per account across repositories; `PullRequest.Repo`; pull requests keyed by URL; `requiredFailures` takes the failing list and makes one aliased query per account.
- `internal/github/watcher.go`: `Account`, `Accounts()`, accounts read first each pass, workspace `PullRequests`/`Updated` filtered from the account (`strings.EqualFold`), account errors added to a workspace's error only when its own token read succeeded; `Status.Open` and the query in `measure` removed; `publish` also compares accounts.
- `internal/github/githubtest`: `SetPullRequests(involved, requested, direct ...string)`, `SetRequiredFailures(urls ...)` answering the `r…`/`p…` aliases.
- `internal/ui/socket.go`: `accounts` in the state message.
- Browser: `types.ts`, `state.ts`, `data-socket.ts`, `turns.ts` (`yourMoves`), new `pull-request-row.tsx` and `all-pull-requests.tsx`, `pull-requests.tsx` (URL keys, "All open pull requests"), `selection.ts` (`showHome`, `homeShown`), `session-view.tsx`, `sidebar.tsx` (Pull Requests button), `workspace-section.tsx` (count).
- Docs: `docs/interface.md` GitHub section, `docs/dev/architecture.md` watcher paragraph.
- Tests: `watcher_test.go`, `internal/ui/github_test.go`, `pull-requests.test.tsx`, `all-pull-requests.test.tsx`, `turns.test.ts`, `sidebar.test.tsx`, plus type fixes in `github-header.test.tsx`, `wording.test.ts`, `data-socket.test.ts`.

## Deviations

- `Watcher.Accounts()` returns a non-nil empty slice so the state message carries `[]`, not `null`.
- The aliased required-check query takes no variables (owner and name are quoted inline, as the plan decided), so `requiredFailures` passes nil variables.
- The "Pull Requests" button also renders only when `accounts` is non-empty (as the plan says); the count beside it uses `yourMoves` across accounts.

## Verification (all at Code head `ab9673b…`, macOS, run)

- `go test -race ./internal/github ./internal/ui`: ok (both packages).
- `bunx tsc -p .` in `internal/ui`: no errors.
- `bun test` in `internal/ui`: 440 pass, 0 fail.
- `make check`: exit 0 (includes go vet and `go test -race ./...`).
- Live, read-only, against the running `make dev` server on port 4281 (not restarted): the state socket carries `accounts` for kkestell (18 pull requests), kkestell-livefront (19) and kylekestell-startribune (10), all with `updated` set and no error; `kkestell/hypercube-pr-lab` has a direct review request under kkestell (#12) and kkestell-livefront (#10); the hypercube-pr-lab workspace holds 16 of kkestell's pull requests, and workspaces on other repos hold only theirs.

## Limitations

- The rendered home view and sidebar badge were not inspected in a browser; they are covered by component tests only.

## Session provenance

| Stage | Session | Harness | Native conversation ID | Model |
| --- | --- | --- | --- | --- |
| work | `work-global-prs` (child of 04ef1b14) | Claude Code | unavailable (not exposed to the session) | claude-sonnet-5-5 |
