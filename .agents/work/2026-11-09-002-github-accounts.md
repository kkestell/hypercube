# Work log: GitHub accounts

- Task ID: `2026-11-09-002-github-accounts`
- Plan (repository-relative): `.agents/plans/2026-11-09-002-github-accounts.md`, blob `665bfe3e1eb2079209f553481eb073501290aa8f`
- Guidance: `.agents/designs/2026-11-09-001-github-integration.md`, blob `1be166e4fffb44898b53d6c18ad9f4fb89bd4152`
- Plan review: `.agents/reviews/plans/2026-11-09-002-github-accounts.md` (accepted, no findings)
- Author: impl-accounts, Claude Code, native conversation `6b90ac08-5b58-495c-bfc7-dd46b87ac0db`, `claude-sonnet-5-5`
- Status: complete
- Code base: `fbcd38a9c459b2f0cd915ee6a72e26f176ed9674`
- Code head: `c0176eb8956e012fca70f5bf0d382df2c1982675`
- Implementation commits: `c0176eb8956e012fca70f5bf0d382df2c1982675`

## Changes

- `internal/github` (new): `Watcher` with `Run`, `Refresh`, `Env`, `Statuses`, `Changed`; `Status` (first four fields); gh logins and tokens (`gh.go`), `parseRepo` (`repo.go`), GraphQL permission query (`graphql.go`).
- `internal/github/githubtest` (new): fake gh script and GraphQL stand-in.
- `internal/workspace/remote.go`: `Config`, `Remote`.
- `harnessclient.Launch` and `terminal.Set.Open` take extra environment; `session.Runtime.Env` hook, `r.env`, used in `run`, `Shell`, `WorkspaceShell`.
- `cmd/hypercube/backend.go`: watcher created, `rt.Env` set before `Restore`, run after restore, awaited in `close` before the database closes.
- `harnesstest`: `!env NAME`.
- Docs: `agents/development.md`, `agents/architecture.md`, `docs/interface.md` (`## GitHub`).
- Tests: `internal/github` (`parseRepo`, watcher cases), `internal/workspace/remote_test.go`, `internal/session/env_test.go`.

## Deviations

- The no-gh watcher test uses a bare executable name that exists on `PATH`, then removes it, so the case runs after an account resolved and proves statuses clear. The plan's bare name never on `PATH` would only show an empty start.
- Tests wait on `Changed` or on a second workspace's status as a pass marker; no sleeps.

## Verification (target: Code head, macOS, Go from repo)

- `go test ./internal/github -race -count=5`: pass.
- `go test ./internal/session -run TestWorkspaceEnv -count=1`: pass.
- `make check` (run on the code before the final reorder of the `internal/` list in `agents/development.md`, a doc-only edit committed in the same code commit): exit 0, all packages ok including `internal/github`, `internal/session`, `internal/terminal`, `internal/workspace`.

## Provenance

| Role | Coordinator | Harness | Native conversation | Model |
| --- | --- | --- | --- | --- |
| implementer | impl-accounts | Claude Code | `6b90ac08-5b58-495c-bfc7-dd46b87ac0db` | `claude-sonnet-5-5` |

## Limitations

Launch changes reach a running session only after it restarts. No live gh or GitHub API was used; stand-ins only.

## Repair 1 (work review `.agents/reviews/work/2026-11-09-002-github-accounts.md`, changes-required)

- Code base (unchanged): `fbcd38a9c459b2f0cd915ee6a72e26f176ed9674`
- Code head: `6a2b40db49e2945e02cd450091df78fd887be1ed` (fix commit; earlier work commit `c0176eb8956e012fca70f5bf0d382df2c1982675`)
- Author: impl-accounts, native conversation `6b90ac08-5b58-495c-bfc7-dd46b87ac0db`, `claude-sonnet-5-5`

Findings addressed:
1. One revoked or expired login broke detection for every workspace without `credential.githubAccount`. `query` now returns `errUnauthorized` for HTTP 401 and `canPush` treats it like NOT_FOUND (cannot push). `pushers` logs and skips a login whose token gh cannot give. 5xx and network errors still fail the pass and stay uncached. `githubtest.API.Revoke(token)` answers 401; new `TestRevokedAccountDoesNotBlockDetection` (alice revoked, bob can push, resolves to bob).
2. `docs/interface.md` GitHub section reworded: applies when the current branch tracks a github.com remote, or tracks no remote and has a github.com `origin`.

Verification at the new Code head:
- `go test ./internal/github -race -count=3`: pass.
- `make check`: exit 0, no failures (output in `/tmp/check002.out`).

## Completion

- Accepted work review: `.agents/reviews/work/2026-11-09-002-github-accounts.md` (round 1 changes-required, round 2 accept)
- Final reviewed Code head: `6a2b40db49e2945e02cd450091df78fd887be1ed`
- Implementation commits: `c0176eb8956e012fca70f5bf0d382df2c1982675`, `6a2b40db49e2945e02cd450091df78fd887be1ed`
- Changes after the reviewed head on `hypercube/session-1771c79b` are workflow records under `.agents/` only and the code reviewed in later units' accepted ranges.

| Role | Coordinator | Harness | Native conversation | Model |
| --- | --- | --- | --- | --- |
| parent | Hypercube session 1771c79b | Claude Code | `2fb9801a-4985-41f3-bd41-371c1ae39e8e` | `claude-opus-5-5` |
| planner | planner | Claude Code | `1e20740e-b018-49eb-9ae2-27fe8948db63` | `claude-opus-5-5` |
| plan and work reviewer | plan-reviewer | Claude Code | `7989b8d0-1436-4b4f-8ad3-10b45e003508` | `claude-opus-5-5` |
