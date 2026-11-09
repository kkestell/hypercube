# Work review: 2026-11-09-002-github-accounts

## Round 1

- Task ID: `2026-11-09-002-github-accounts`
- Mode: `work`
- Author: reviewer `plan-reviewer`, Hypercube child of `1771c79b`, Claude Code,
  native conversation `7989b8d0-1436-4b4f-8ad3-10b45e003508`, model
  `claude-opus-5-5`. Independent of the implementer
  (`6b90ac08-5b58-495c-bfc7-dd46b87ac0db`); this reviewer also wrote the plan
  review, not the plan or code.
- Work log (repository-relative): `.agents/work/2026-11-09-002-github-accounts.md`
  (committed in `9b7d9643cdadab2bc71dd33ae0aa4b644b48066c`)
- Plan: `.agents/plans/2026-11-09-002-github-accounts.md`, blob
  `665bfe3e1eb2079209f553481eb073501290aa8f` (matches the execution decision)
- Guidance: `.agents/designs/2026-11-09-001-github-integration.md`, blob
  `1be166e4fffb44898b53d6c18ad9f4fb89bd4152` (matches)
- Code base: `fbcd38a9c459b2f0cd915ee6a72e26f176ed9674` (ancestor of head, verified)
- Code head: `c0176eb8956e012fca70f5bf0d382df2c1982675`
- Checkout: HEAD `9b7d9643cdadab2bc71dd33ae0aa4b644b48066c`; the only change
  after Code head is the work log. Working tree otherwise clean.

### Verdict

`changes-required`. The implementation follows the plan closely and the token
path is correct, but one plan-level gap makes detection fail outright for every
workspace when any signed-in account's token is revoked or expired.

### Findings

1. **A stale token on any gh account blocks account detection (required;
   plan guidance gap).** `internal/github/watcher.go` `pushers` returns an
   error as soon as one login's permission query fails, and `pass` then caches
   nothing and keeps no status for a workspace it has not resolved before.
   `graphql.go` `query` treats every non-200 answer as an error. GitHub answers
   **401** for a bad token (confirmed: `curl` POST to
   `https://api.github.com/graphql` with an invalid bearer token returned
   `401`). So if one of the user's gh accounts has a revoked or expired token,
   which `gh auth status` still lists (the design says the watcher uses every
   listed login "regardless of state"), every workspace without a configured
   `credential.githubAccount` never gets an account. Each pass then logs one
   error per such workspace, every minute. The implementation follows the plan
   ("If any query fails other than `errNotFound`, do not cache"), so the gap is
   in the guidance.
   Correction: an account whose token cannot be used cannot push. In
   `canPush`, treat a 401 answer like `errNotFound` (`false, nil`). In
   `pushers`, treat a token read error the same way: skip that login rather
   than failing the workspace. Keep "do not cache" for other failures (5xx,
   network). Add a watcher case: one login's token answers 401, another can
   push, and the latter is chosen. `githubtest.API` needs a way to answer 401
   for a token.

2. **The GitHub docs misstate the origin fallback (required, small).**
   `docs/interface.md` says Hypercube applies "for a workspace whose current
   branch tracks a github.com remote, or whose `origin` is one". `workspace.Remote`
   falls back to `origin` only when the branch tracks no remote with a URL.
   A branch tracking a non-GitHub remote gets no GitHub status even when
   `origin` is on github.com. Correction: "whose current branch tracks a
   github.com remote, or tracks no remote and has a github.com `origin`".

### Independent checks of the difficult invariants

- **Token reaches the holder's environment.** `harnessclient.Launch` appends
  `env` after `os.Environ()`, and `terminal.Set.Open` appends it after
  `TERM`/`COLORTERM`. `hold.Start` passes that slice as the holder's `cmd.Env`.
  The holder runs its child with the inherited environment, and the plugins
  start their harness with `os.Environ()`. `run`, `Shell` (session's
  workspace), and `WorkspaceShell` (its ID) all pass `r.env(...)`; attaching to
  a surviving holder is unchanged. `internal/session/env_test.go` proves an
  agent (`!env GH_TOKEN`), a session shell, and a workspace shell each see the
  value.
- **No token on disk or in logs.** The holder writes only `lock`, `in`, `tty`,
  `exit`, and output segments (`internal/hold/holder.go` `publish`). The
  environment is never persisted, and the `commands` table is untouched. Error
  messages carry gh's stderr, HTTP status, or GraphQL error text, never a
  token. The token appears only in gh's stdout, the `Authorization` header,
  and the holder environment.
- **Locking.** `Env` takes only the watcher's `mu`, does no I/O, and the
  watcher never calls the runtime, so calling it under `r.mu` cannot deadlock.
  `detected` is touched only by `Run`'s goroutine.
- **gh isolation.** `run` strips `GH_TOKEN` and `GITHUB_TOKEN`, and the fake gh
  fails if `GH_TOKEN` is set while every test sets an outer `GH_TOKEN`.
- **Shutdown.** `close` cancels, waits for `githubDone`, then closes the
  database. A cancelled pass publishes nothing.
- **Interfaces for 003–005.** `Watcher{DB, GH, API, Interval}` with `Run`,
  `Refresh`, `Env`, `Statuses` (cloned map; `Choices` slices are never
  mutated after publishing), and `Changed` (lazily created, closed on a status
  difference) match the design. A zero `Watcher` never closes `Changed`, as
  003's tests assume. `githubtest.GH` and `githubtest.API` are exported for
  003 to extend.

Notes, not findings: the comment in `TestDetectionRepeatsOnlyWhenInputsChange`
about workspace order is unnecessary, because statuses are published once per
pass, but the assertion is still correct. The deviation in `TestGHMissing`
(gh disappearing after a resolved pass) is a stronger test than the plan's.

### Verification

- `reused`: `make check`, exit 0, from the work log. It ran on the Code head
  code before a reorder of the `internal/` list in `agents/development.md`. That
  reorder is Markdown-only, so the result applies to Code head.
- `reused`: `go test ./internal/github -race -count=5` and
  `go test ./internal/session -run TestWorkspaceEnv -count=1`, both pass, from
  the work log at Code head.
- `run`: `curl` POST to GitHub's GraphQL endpoint with an invalid token →
  `401`. This settles whether finding 1's failure mode occurs against the real
  API.
- No Go tests rerun; the reused evidence covers the reviewed revision, and
  inspection found no doubtful assertion.

### Limitations

No live gh account with a revoked token was exercised; finding 1 rests on the
code path and the confirmed 401 answer.

## Round 2

- Author: reviewer `plan-reviewer`, Hypercube child of `1771c79b`, Claude Code,
  native conversation `7989b8d0-1436-4b4f-8ad3-10b45e003508`, model
  `claude-opus-5-5`. Independent of the implementer.
- Code base: `fbcd38a9c459b2f0cd915ee6a72e26f176ed9674` (unchanged)
- Code head: `6a2b40db49e2945e02cd450091df78fd887be1ed`; reviewed the full
  base..head range, with the fix diff `c0176eb..6a2b40d` read in full.
- Work log repair entry: `b7760e88db4da04fdbd947ca6d7b955da8d09aa5`. The
  checkout's HEAD is that commit; the only change after Code head is the work log.

### Verdict

`accept`. Both findings are resolved, and the fix breaks nothing.

### Resolution

| Finding | Status | Evidence at `6a2b40d` |
| --- | --- | --- |
| 1. A stale token blocks detection | resolved | `graphql.go` `query` returns `errUnauthorized` on 401; `canPush` treats it like `errNotFound`. `watcher.go` `pushers` logs and skips a login whose token gh cannot give. 5xx and network errors still return an error and stay uncached. `TestRevokedAccountDoesNotBlockDetection` (alice answers 401, bob can push → bob, no choices) uses the new `githubtest.API.Revoke`. |
| 2. Docs misstate the origin fallback | resolved | `docs/interface.md` now says the branch "tracks a github.com remote, or tracks no remote and has a github.com `origin`", matching `workspace.Remote`. |

No new findings. Note: a login skipped for a failed token read is left out of
the cached detection until the login list changes. If that read failed only
transiently and the login could also push, the workspace gets the other pusher
instead of a choice. Both accounts can push, so this is harmless.

### Verification

- `reused`: `make check`, exit 0, at `6a2b40d`. Output is in `/tmp/check002.out`,
  written the same minute as the fix commit. It contains no `FAIL`, and
  `internal/github` is `ok`.
- `reused`: `go test ./internal/github -race -count=3`, pass, from the repair
  entry.
- No new runs; the reused evidence covers the reviewed head.
