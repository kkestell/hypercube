# Resolve each workspace's GitHub account and give agents and shells its token

- Task ID: `2026-11-09-002-github-accounts`
- Status: ready
- Source HEAD: `47055e42f036f863ebb18110d3803fbce3d6f38b`
- Author: planner `planner`, Claude Code, native conversation
  `1e20740e-b018-49eb-9ae2-27fe8948db63`, `claude-opus-5-5`
- Shared design (repository-relative): `.agents/designs/2026-11-09-001-github-integration.md`,
  [linked here](../designs/2026-11-09-001-github-integration.md). Read its
  Settled decisions and Shared data shapes first.
- Prerequisites: none. Units 003–005 build on this one.

## Outcome and boundaries

A backend watcher finds, for each Git workspace whose remote is on github.com,
the gh account to use, and every new agent launch and panel shell in that
workspace gets `GH_TOKEN` for it. Nothing is shown in the browser yet; statuses
are readable from the watcher for unit 003. No plugin, protocol, or harness
change. No fetch, pull requests, saved account choice, or account-choice action
(unit 003).

## Current system

- `internal/session/launch.go:run` starts a new launch's holder with
  `harnessclient.Launch(r.programs.Holder, dir, path)` under `r.mu`;
  `l.workspace` is set before `run` starts. `harnessclient.Launch` passes
  `os.Environ()` to `hold.Start`, whose child inherits it.
- `internal/session/shells.go:WorkspaceShell` and `Shell` call
  `r.Terminals.Open(owner, dir, cols, rows)` under `r.mu`;
  `internal/terminal/terminal.go:Open` uses `os.Environ()` plus `TERM` and
  `COLORTERM`.
- `cmd/hypercube/backend.go:startApp` builds the runtime, restores launches,
  then serves; `app.close` cancels `ctx` and closes the database.

## Changes

### Launch environment hook

1. `harnessclient.Launch(holder, dir, plugin string, env []string)`: pass
   `append(os.Environ(), env...)` to `hold.Start`.
2. `terminal.Set.Open(session, dir string, env []string, cols, rows int)`:
   append `env` after `TERM` and `COLORTERM`. Update the callers in
   `internal/terminal/terminal_test.go`.
3. `session.Runtime` gains an exported field, documented as set before
   `Restore` and never blocking:

   ```go
   // Env returns extra environment for a workspace's new agent launches and
   // panel shells. It is called with the runtime's lock held.
   Env func(workspace string) []string
   ```

   A nil `Env` adds nothing. `run` passes `r.env(l.workspace)`; `Shell` passes
   the session's `Workspace`; `WorkspaceShell` passes its `id`. Attaching to a
   surviving holder is unchanged.

### `internal/workspace/remote.go`

- `Config(dir, key string) (string, error)`: `git config --get KEY`; exit
  status 1 (unset) returns `""` and no error.
- `Remote(dir string) (name, url string, err error)`: the current branch's
  `branch.<b>.remote` if `remote.<that>.url` is set, otherwise `origin` if
  `remote.origin.url` is set, otherwise empty strings. Read the configured URL
  with `git config --get remote.<name>.url`, not `git remote get-url`, which
  applies `insteadOf` (unit 003's tests rewrite the fetch URL to a local
  repository). A detached HEAD or a branch tracking `.` falls back to `origin`.

### `internal/github` package

Responsibilities (file layout is the implementer's):

**gh.** Run `w.GH` (default `"gh"`) with `GH_TOKEN` and `GITHUB_TOKEN` removed
from the environment and the pass's context.
`logins`: `auth status --json hosts --hostname github.com`, every
`hosts["github.com"][].login`, de-duplicated and sorted, regardless of `state`.
`token(login)`: `auth token --hostname github.com --user LOGIN`, trimmed.

**Repository.** `parseRepo(url) (owner, name string, ok bool)` accepts
`https://github.com/O/R`, `http://`, `ssh://git@github.com/O/R`,
`ssh://git@github.com:22/O/R`, `git://github.com/O/R`, and scp form
`git@github.com:O/R`, each with optional `.git` and trailing slash; the host
compares case-insensitively; exactly two non-empty path segments. Anything else
is not GitHub.

**GraphQL.** `query(ctx, token, query string, variables map[string]any, data
any) error`: POST `{"query","variables"}` to `w.API` (default
`https://api.github.com/graphql`) with `Authorization: bearer TOKEN`, using a
30-second context timeout. A non-200 status is an error naming the status. The
body is `{"data": ..., "errors": [{"type","message"}]}`; an error of type
`NOT_FOUND` returns a sentinel `errNotFound`, any other returns its message.
Permission query:

```graphql
query($owner:String!,$name:String!){repository(owner:$owner,name:$name){viewerPermission}}
```

An account can push when `viewerPermission` is `ADMIN`, `MAINTAIN`, or `WRITE`;
`errNotFound` means it cannot.

**Watcher.** Zero value usable; maps and channels are created lazily under `mu`.

```go
type Watcher struct {
	DB       *store.DB
	GH       string        // gh executable; "gh" when empty
	API      string        // GraphQL endpoint; GitHub's when empty
	Interval time.Duration // between passes; one minute when zero
	// unexported: mu guards statuses, tokens, changed, refresh;
	// detected belongs to Run's goroutine.
}
func (w *Watcher) Run(ctx context.Context)               // passes until ctx ends
func (w *Watcher) Refresh()                              // a pass soon; never blocks
func (w *Watcher) Env(workspace string) []string         // ["GH_TOKEN=..."] or nil
func (w *Watcher) Statuses() map[string]Status           // copy, by workspace ID
func (w *Watcher) Changed() <-chan struct{}              // closed at the next change
```

`Run` does a pass immediately, then waits for the interval timer, `Refresh`
(a one-slot channel), or `ctx`. `Changed` returns the current channel; a status
change closes it and installs a new one.

**Pass.** Read `logins`. On any error, or no logins, clear all statuses and
tokens, publish if anything was there, and end the pass; log errors other than
`exec.ErrNotFound` to stderr. Otherwise read the workspaces (`Tx.Workspaces`),
then for each workspace without `NoGit`, in order:

1. `Remote`, then `parseRepo`. Not GitHub or a Git error: the workspace has no
   status.
2. Account: if `Config(path, "credential.githubAccount")` names a login, use it
   (a value naming no login is ignored). Otherwise take pushers (below);
   candidates are the pushers, or all logins when none can push. One
   candidate: use it. Several: set `Choices` to the candidates and leave
   `Account` empty (unit 003 fills it from the saved choice).
3. With an account, read its token (once per login per pass). A token error is
   logged, and the workspace keeps an account but gets no token.
4. Set the workspace's `Status{Repo: "owner/name", URL:
   "https://github.com/owner/name", Account, Choices}` and token, and publish
   when the status differs from the previous one.

After the loop, drop statuses and tokens of workspaces not seen and publish if
any were dropped.

Pushers: for each login, run the permission query with its token. Cache the
result per workspace ID under the key `repo + "\n" + strings.Join(logins,
",")`, so detection repeats only when the repository or the login list
changes. If any query fails other than `errNotFound`, do not cache; that
workspace keeps its previous status and token for this pass.

**Env** returns `[]string{"GH_TOKEN=" + token}` for a workspace with a token,
else nil, reading under `mu` only.

### Backend wiring

In `startApp`, create `&github.Watcher{DB: db}`, set `rt.Env = watcher.Env`
before `rt.Restore`, and after a successful restore start
`go func() { watcher.Run(ctx); close(done) }()`. Keep the watcher and `done` on
`app`; `close` waits for `done` after `a.cancel()` and before `a.db.Close()`.
Unit 003 passes the watcher to the browser handler.

### Test stand-in

Add `!env NAME` to `internal/harness/harnesstest` (document it in the keyword
list): a quick turn whose final text is the plugin's `os.Getenv(NAME)`.

### Documentation

- `agents/development.md` Code list: add `github` to the `internal/` packages.
- `agents/architecture.md`, "Backend, desktop app, and terminals": a short
  paragraph: the GitHub watcher resolves each workspace's gh account; new
  agents and panel shells get `GH_TOKEN` through their holder's environment.
- `docs/interface.md`: new `## GitHub` section (units 004–005 extend it):
  requires the GitHub CLI signed in to github.com; how the account is chosen
  (`credential.githubAccount`, the one account that can push, or the
  workspace's choice); agents and panel shells get `GH_TOKEN` for it, so `gh`
  and Git pushes through `gh auth git-credential` use that account; a running
  agent or shell keeps the account it started with until it restarts.

## Verification

- `internal/workspace`: `Remote` table test with temporary repositories:
  tracking a non-origin remote, no upstream (origin), detached HEAD, no remotes;
  `Config` set and unset.
- `internal/github`: `parseRepo` table test, including non-GitHub hosts and
  three-segment paths.
- Test stand-ins in a new package `internal/github/githubtest`, reused by unit
  003's UI tests: a fake gh, written as a shell script in `t.TempDir()` for a
  given list of logins, that prints a hosts JSON for `auth status`,
  `token-LOGIN` for `auth token ... --user LOGIN`, and exits 1 if `GH_TOKEN` is
  set; and an `httptest.Server` GraphQL stand-in whose answers a test sets per
  bearer token (permission now; unit 003 adds pull requests) and can switch to
  answering 500.
- `internal/github` watcher tests through `Run`, `Statuses`, `Env`, waiting on
  `Changed` until a condition holds (no sleeps), with
  `t.Setenv("GH_TOKEN", "outer")` to prove gh runs without it. Workspaces are
  temporary repositories with remote `https://github.com/o/r.git`, added through
  an isolated store (`XDG_STATE_HOME` in `t.TempDir()`). Isolate Git
  configuration (`GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`) here
  and in the `Config` tests: a developer's global `credential.githubAccount`
  would otherwise decide every case. Cases:
  - `credential.githubAccount` names a login: used without a permission query.
  - one of three logins can push: used; `Env` returns its token.
  - two can push: `Choices` lists them, `Account` empty, `Env` nil.
  - none can push and one login: used.
  - a non-GitHub remote and a `NoGit` workspace: no status.
  - gh missing (`GH` is a bare name not on `PATH`, giving
    `exec.ErrNotFound`): no statuses, `Env` nil.
  - permission query returns 500: no status cached; after the server recovers
    and `Refresh`, the account is resolved.
- `internal/session`: with `rt.Env` returning `GH_TOKEN=t`, an agent answering
  `!env GH_TOKEN` replies `t`; a session shell and a workspace shell with
  `Terminals.Command` writing `$GH_TOKEN` to a marker file (the pattern in
  `shells_test.go`) write `t`.
- `make check` before committing.

## Handoff

Ready. Work size: one mechanism (account resolution and token delivery)
through two Git helpers, a new package, and three launch call sites, with a
substantive stand-in test suite: a normal execution assignment. Detail:
ownership, token path, resolution order, caching, and failures are settled;
names and file layout are the implementer's. Launch changes reach a running
session only after it restarts; the parent warns the user.
