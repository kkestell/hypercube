# Chat about a pull request from its row

- Task ID: 2026-11-10-004-pull-request-chat
- Status: ready
- Source HEAD: f18af0a0f01b0eaeb160ee212ebb24b9c72c72e8
- Author: planner child of Hypercube session 32a6c484 (Claude Code, Opus 5.5)
- Builds on: 2026-11-09-007 (clone), 2026-11-09-009 (Review fetches the head),
  2026-11-09-012 (new worktree on a named branch)
- Request: replace **Clone** with a **Chat** button on pull request rows that
  opens a preset new-session page instead of starting a turn; summarized below.

## Outcome and boundaries

- Every row on the Pull Requests view and on a workspace dashboard shows
  **Chat**, at the row's right edge; **Review**, where shown, sits to its left
  and keeps its behavior. The **Clone** button is gone.
- Chat on a row whose repository has a workspace: Hypercube fetches the pull
  request's head commit, then shows that workspace's new-session page with the
  location **New worktree on branch** set to the pull request's head branch,
  starting at that commit. The composer has the focus. Text already typed in
  that workspace's unsent session, and its model, stay.
- Chat on a row with no workspace opens the clone dialog, now titled
  **Add {repo}**, which offers **Clone** into the typed folder (unchanged) and,
  in the desktop app, **Use Existing Folder…**. Either success adds the
  workspace and continues as above. A folder that is not a checkout of the
  repository is refused with an error in the dialog.
- Non-goals: changing Review, setting an upstream on the new branch, reusing an
  existing local branch, sending any prompt, a browser-view folder chooser for
  the existing-folder path (Add Workspace is desktop-only too).

## Current system

- Rows (`internal/ui/web/github/`): `PullRequestRow` draws Review and an
  optional `trailing` node, which `all-pull-requests.tsx` fills with Clone when
  `tracking(workspaces, pr.repo)` finds no workspace; `pull-requests.tsx` is the
  dashboard list. `useReview` (`review.ts`) disables a button while it runs.
- `CloneDialog` uses `cloneWorkspace` (`api.ts`; errors stay in the dialog) and
  on success selects the dashboard through the pending-workspace path in
  `selectSession`/`receivedWorkspaces` (`sessions/selection.ts`), its only user.
- New-session page: `openUnsent` reopens or creates the workspace's unsent
  session; `unsentWorktree(state, true, {name, start})` sets the location that
  `WorktreeSelect` shows and `deliverSubmission` sends. `Composer` focuses its
  text box on mount. `workspace.AddWorktree` creates the branch at send and
  refuses an existing name.
- Server: `workspace.FetchPullRequest` fetches `refs/pull/<n>/head` (forks
  too) and returns the commit. `Watcher.FetchPullRequest` needs the workspace's
  published status and token, absent until the watcher's next pass after an
  add; `Watcher.Clone` instead reads `w.token(login)` and
  `w.accounts[login].PullRequests`. `Watcher.pass` maps a workspace to a
  repository by `workspace.Remote(path)` (the current branch's remote, else
  `origin`) and `parseRepo`.

## Decisions

**Start point: the fetched head commit**, as Review does, not a
remote-tracking branch: exact right after a push and works for forks.

**Token: the row's account's** (`w.token`, as `Clone` does; the dashboard
passes `status.account`), so Chat works right after an add, before the
watcher publishes the new workspace's status.

**Branch name: the pull request's head branch, made unique.** The name is
`pr.branch` when the checkout has no local branch of that name, else the first
free of `<branch>-2`, `<branch>-3`, …. The user's own pull request branches
often exist locally, and a second Chat on the same pull request finds the first
session's branch, so a fixed name would fail at send. The user can still edit
the name in the existing dialog before sending. Pushing to the pull request is
`git push origin HEAD:<branch>`; the agent is told nothing automatically.

**Existing folder must map to the repository the way the watcher maps it.**
The check is `workspace.Remote(dir)` + `parseRepo`, equal to `pr.repo` ignoring
case, not "any remote matches". A folder whose current branch tracks a fork
would otherwise be added, still map to the fork, and leave the row without a
workspace. Messages:
`<dir> is not a Git checkout`, `<dir> has no GitHub remote`,
`<dir> is a checkout of <other>, not <repo>`.

### Server

- `internal/workspace/branches.go`: `FreeBranch(root, name string) string`
  returning `name` or the first free `name-N` (N from 2), using `exists`.
- `internal/github/chat.go`:

  ```go
  // PullRequestHead fetches login's open pull request number in repo into the
  // checkout at dir with login's token and returns its head branch and commit.
  func (w *Watcher) PullRequestHead(ctx context.Context, login, repo string, number int, dir string) (branch, commit string, err error)
  // CheckRepo returns an error unless the checkout at dir is on GitHub as repo.
  func CheckRepo(dir, repo string) error
  ```

  `PullRequestHead` finds the pull request under `w.mu` in
  `w.accounts[login].PullRequests` by `strings.EqualFold(Repo, repo)` and
  `Number` (error `pull request #<n> in <repo> is not open for <login>`), reads
  the token, takes the remote from `workspace.Remote(dir)`, and calls
  `workspace.FetchPullRequest` with `GH_TOKEN=<token>`. `CheckRepo` uses
  `workspace.Root` for the not-Git case, then the mapping rule above.
- `internal/ui/actions.go` `workspaceAction`:
  - New case `"pull-request-branch"` (`id`, `account`, `repo`, `number`):
    a non-positive or non-numeric number is 400; read the workspace path as
    `startReview` does; call `PullRequestHead`; answer JSON
    `{"name": FreeBranch(root, branch), "start": commit}` (the browser's
    `NewBranch`). Every other error is 409 through `refused`.
  - Case `"new"`: when `repo` is set, `github.CheckRepo(dir, repo)` before
    `AddWorkspace`; a refusal is 409 and adds nothing.

### Browser

- `github/chat.ts`: `startChat(store, workspaceId, account, pr)` posts
  `workspace?action=pull-request-branch` with `post` (errors go to the toast and
  return null), parses the `NewBranch`, then in one `store.update`:
  `openUnsent(s, workspaceId)` and, unless that unsent session is sending,
  `unsentWorktree(…, true, branch)`. The composer's mount focus does the rest.
- Generalize `useReview` into one hook (in `review.ts` or a new module) that
  takes the action and a row key and returns a handler, or undefined while that
  key's request runs; Review keys by workspace and number as today, Chat by
  `account + pr.url` so the row stays disabled through a clone.
- `PullRequestRow`: replace `trailing` with `onChat?: () => void`, rendering
  `<Button variant="secondary" size="sm">Chat</Button>` after Review, disabled
  without `onChat`.
- `pull-requests.tsx`: `onChat` = `startChat(store, workspace.id, status.account, pr)`.
- `all-pull-requests.tsx`: tracked rows start the chat directly; untracked rows
  open the dialog with `{pr, account}` as Clone did.
- `clone-dialog.tsx`: title **Add {repo}**, description "Clone it with
  {account}, or use a folder that already has it." Replace the success
  selection with an `onAdded(workspaceId)` prop; the page passes
  `id => startChat(store, id, account, pr)`. In the desktop app the footer adds
  a secondary **Use Existing Folder…** (disabled while cloning) that calls
  `chooseFolder()`; an empty result does nothing; otherwise it posts
  `workspace?action=new&path=<chosen>&repo=<repo>` and shows an error in the
  dialog's message, or calls `onAdded` and closes. Generalize
  `cloneWorkspace` in `api.ts` into one function that posts workspace params
  with an optional signal and returns `{workspace} | {error}`, used by both.
- `sessions/selection.ts`: remove the pending-workspace selection (the
  `'workspace' in target` waits in `selectSession` and `receivedWorkspaces`)
  and its test; no caller selects an unlisted workspace any more. A workspace
  key for a missing workspace leaves the selection unchanged.
- After an add, `openUnsent` follows a fetch, so the state listing the saved
  workspace arrives first, as `addWorkspace` already relies on.

## Implementation sequence

1. `FreeBranch`, `PullRequestHead`, `CheckRepo`, the two handler changes, with
   Go tests.
2. Browser: hook, `startChat`, row, both lists, dialog, `api.ts`, selection
   cleanup, with bun tests.
3. Docs and screenshots.

## Verification

Go:

- `internal/workspace`: `FreeBranch` table: free name; name existing → `-2`;
  name and `-2` existing → `-3`.
- `internal/github`: `CheckRepo` on temporary repositories: `origin`
  `https://github.com/O/R.git` for `o/r` passes; `o/other` fails naming both;
  a non-GitHub remote and a plain folder fail with their messages; a branch
  tracking `upstream` = `o/r` while `origin` is a fork passes.
- `internal/ui/github_test.go`, modeled on `TestCloneAction` (insteadOf rule to
  a bare repository, `githubtest` giving alice pull request #7 in `o/r` with
  branch `feature`, `refs/pull/7/head` made with `git update-ref` in the bare
  repository): `pull-request-branch` answers `{"name":"feature","start":<that
  commit>}`; after `git branch feature` in the checkout, `feature-2`; number 8
  → 409; `number=x` → 400. `new` with `repo=o/r` on that checkout → 200; on a
  checkout of `o/s` → 409 and the workspace list is unchanged.

Browser (`bun test` in `internal/ui`):

- `all-pull-requests.test.tsx` / `pull-requests.test.tsx`: every row has Chat,
  none has Clone, Review rules unchanged; Chat on a tracked row posts
  `pull-request-branch` with `id`, `account`, `repo`, `number` and leaves
  `state.unsent` in that workspace with `worktree: true` and the answered
  branch; an existing draft text survives; an error response shows the toast
  and keeps the selection; the button is disabled while the request runs.
- `clone-dialog.test.tsx`: Clone success calls `onAdded` with the workspace;
  with a stubbed `hypercube` bridge, **Use Existing Folder…** posts `new` with
  the chosen path and `repo`, shows a 409's text in the dialog, and calls
  `onAdded` on success; an empty choice posts nothing; without the bridge the
  button is absent.
- Remove the pending-workspace case from `selection.test.ts`.

Commands: focused `go test -race ./internal/workspace ./internal/github
./internal/ui`, `bunx tsc -p .` and `bun test` in `internal/ui`, then
`make check`.

Manual (`make dev`): Chat on a tracked pull request opens the new-session page
with the branch on the location trigger and the cursor in the composer;
sending creates a worktree whose `HEAD` equals `gh pr view <n> --json
headRefOid`. Use Existing Folder… on a checkout of another repository shows
the error.

## Docs and screenshots

Read `docs/dev/documentation.md` first. `docs/interface.md`: replace the
Clone paragraph with Chat (tracked and untracked rows, the branch name rule,
the existing-folder check, nothing is sent) and update the picture's alt text.
`docs/workflows.md`, "Review a pull request": replace the Clone sentence with
Chat adding the workspace, then Review; same alt text. Regenerate with
`make screenshots` and commit the changed `pull-requests` and `dashboard`
captures; their scenes need no edits beyond what the new button renders.

## Handoff

Ready; no open engineering decisions. One mechanism (a row opens a preset new
session, adding the workspace first when needed) across one Git helper, one
GitHub method and check, two handler changes, and the row, lists, dialog and
selection cleanup in the browser, with tests: a normal assignment comparable to
2026-11-09-007. The existing-folder check is stricter than "any remote
matches", for the mapping reason above. At 1,630 words (`plan_metrics.py`,
default 600–1,500 band) it is above the band because the dialog, naming and
mapping contracts need stating; splitting server from browser would leave
neither half usable, as with 2026-11-09-012 (1,571).
