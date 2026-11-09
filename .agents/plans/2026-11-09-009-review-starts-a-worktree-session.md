# Review starts a worktree session at the pull request's head

- Task ID: 2026-11-09-009-review-starts-a-worktree-session
- Status: ready
- Source HEAD: 7f5898c9354bb571128d05e189256af01f4b3295
- Author: parent coordinator, Hypercube session 777ef702 (Claude Code, Opus 5.5)
- Dependencies: 2026-11-09-008-worktree-start-and-safe-removal (complete):
  `Runtime.CreateSubmission(..., worktree, start, ...)` and the removal rule
  that keeps a session's `Base`. Phase 2 of 3; phase 3 moves the model and
  prompt chosen here into config with a Settings page.

## Outcome and boundaries

Clicking **Review** on a pull request whose review was requested from the
account starts a new top-level worktree session at the pull request's head
commit, sends it a review prompt, and opens it. No session picker, no draft.

- The button's visibility and enabled rules stay as they are
  (`canReview`, `tracking` in `internal/ui/web/github/`).
- The session is an ordinary worktree session. Nothing records that it is a
  review.
- Model: `config.DefaultSelection`; when unset, the first entry of
  `config.Models` with its `DefaultEffort`; when there are no models, an error
  ("Choose a model in Settings to start a review").
- Prompt (fixed in this phase):
  `Review pull request #<n> (<url>). Its head commit is checked out in your
  working directory. Read its description, discussion, and diff with gh, and
  report what you find. Post nothing to GitHub.`

Non-goals: config/settings for the model and prompt (phase 3); moving
sessions; changing agent instructions in `internal/report`.

## Current system

- Browser: `PullRequestRow` (`pull-request-row.tsx`) renders **Review**;
  `pull-requests.tsx` (workspace dashboard) and `all-pull-requests.tsx`
  (Pull Requests view) open `SendDialog` (`send-dialog.tsx`), which sends
  `reviewInstruction(pr)` (`instructions.ts`) to a chosen session or opens a
  draft. `sendInstruction`/`startInstruction` have no other callers.
- HTTP: `sessionAction` (`internal/ui/actions.go`) case `"new"` creates a
  session; `workspaceAction` case `"clone"` shows the pattern for a GitHub call
  through `s.gh` (`*github.Watcher`).
- GitHub: `Watcher.Statuses()` gives each workspace's `Status` (`Repo`, `URL`);
  `Watcher.Env(workspace)` gives `GH_TOKEN` for the workspace's account;
  `workspace.Remote(dir)` picks the remote the watcher fetches;
  `workspace.Fetch` (`internal/workspace/remote.go`) runs
  `git fetch --quiet --no-write-fetch-head <remote>` with that environment.
- Config: `config.Load()` (`internal/config`), used from `internal/ui/config.go`.
- Merge status: `workspace.MergeStatus` counts `HEAD..branch` as `Unmerged`;
  the Git panel offers **Merge into …** and the sidebar shows unmerged work
  when it is non-zero.

## Chosen design and decisions

### Fetching the head

`workspace.FetchPullRequest(ctx, dir, remote string, number int, env []string)
(string, error)` in `internal/workspace/remote.go`:

1. `git fetch --quiet --no-write-fetch-head <remote>
   +refs/pull/<n>/head:refs/hypercube/fetch/<random>` with `env` and
   `GIT_TERMINAL_PROMPT=0`, errors wrapped like `Fetch`.
2. `git rev-parse --verify <that ref>^{commit}`.
3. `git update-ref -d <that ref>` (always, also on a rev-parse failure).
4. Return the commit ID.

A temporary ref, rather than `FETCH_HEAD`, avoids racing agents that fetch in
the same checkout; deleting it right away leaves no refs behind. The fetched
objects stay until Git's normal pruning window, and the session's branch keeps
the commit once the worktree exists. `GitHub` serves `refs/pull/<n>/head` for
pull requests from forks too, so no fork remote is needed.

`github.Watcher.FetchPullRequest(ctx, workspaceID, dir string, number int)
(commit, url string, err error)` in a new `internal/github/review.go`: reads the
workspace's `Status` (error "the workspace has no GitHub repository" when
missing), picks the remote with `workspace.Remote(dir)`, fetches with
`w.Env(workspaceID)` (nil is fine for public repositories), and returns the
commit and `Status.URL + "/pull/<n>"`.

### Starting the session

New `sessionAction` case `"review"` with query `workspace` and `number`
(positive integer, else 400):

1. Look up the workspace path (the runtime already exposes workspaces to the
   server; use the same read as other handlers).
2. `commit, url, err := s.gh.FetchPullRequest(r.Context(), ...)`.
3. Settings from `config.Load()` per the model rule above, in a small helper
   in `internal/ui` (`reviewSettings(cfg) (harness.Settings, error)`).
4. `s.rt.CreateSubmission(workspace, settings, true, commit, "", prompt)`.
5. Respond with the session ID as plain text. Any error, before or after
   creation, is `http.Error` with 409 (400 for a bad `number`), so the
   browser's `post` shows it. A session created before its first input failed
   still appears in the sidebar through the normal snapshot.

### Merge status

`MergeStatus` gains no parameters; `Unmerged` becomes
`git rev-list --count <branch> --not HEAD <base>` when `base` is non-empty
(it always is for top-level worktree sessions), counting only commits the
session made that the checkout lacks. For sessions started from the checkout's
`HEAD` the count is unchanged, because their base is already in `HEAD`'s
history. A review session therefore shows no unmerged work and no **Merge
into …** until it commits something. Leave `Committed` and the Git panel's
**Committed** diff as they are; the diff against the checkout's branch shows
the pull request's changes, which is useful for review.

### Browser

- New `internal/ui/web/github/review.ts` exports `canReview` (moved from
  `instructions.ts`) and `startReview(store, workspaceId, number)`: posts
  `session?action=review&workspace=…&number=…` with `post` (`api.ts`), then
  `chooseSession` on the returned ID; `post` already shows an error response
  and returns null. Disable the clicked button while the
  request is in flight (local state in the row's caller).
- `PullRequestRow`'s `onReview` stays the interface; both lists pass
  `() => startReview(store, workspace.id, pr.number)`.
- Delete `send-dialog.tsx`, `instructions.ts`, and their tests' obsolete
  cases; keep the tests' coverage of the button through the new flow.
- `docs/interface.md`: replace the Review paragraph with what the button now
  does (fetch, worktree at the head, the default model rule, prompt sent, the
  session opens).

## Implementation sequence

1. `internal/workspace/remote.go`: `FetchPullRequest`; `workspace.go`:
   `MergeStatus` count.
2. `internal/github/review.go`: `Watcher.FetchPullRequest`.
3. `internal/ui/actions.go`: `"review"` case and `reviewSettings`.
4. Browser files and docs as above.

## Verification

- `internal/workspace`: with a bare "remote" repository that has a commit at
  `refs/pull/7/head` (create it with `git update-ref` in the bare repo),
  `FetchPullRequest` returns that commit and leaves no `refs/hypercube/fetch/*`
  ref; a missing pull request returns an error and also leaves no ref.
- `internal/workspace`: `MergeStatus` for a worktree started at a commit not
  in `HEAD` reports `Unmerged` 0, and 1 after one new commit; a session started
  at `HEAD` with one commit still reports 1.
- `internal/ui`: `reviewSettings` picks `DefaultSelection`, else the first
  model with its default effort, else errors. The `"review"` handler rejects a
  missing or non-numeric `number`. (End-to-end creation is covered by phase 1's
  session tests plus the fetch test; do not add a network test.)
- Browser (`bun test`): clicking **Review** posts the review action with the
  workspace and number and selects the returned session; an error response
  leaves the selection unchanged and shows the error; the Pull Requests view's
  disabled rule is unchanged.
- `make check` from the repository root.
- Manual (`make dev`): click **Review** on a real requested review; the new
  session's worktree is at the pull request's head
  (`git -C <worktree> rev-parse HEAD` equals `gh pr view <n> --json headRefOid`),
  the agent receives the prompt, and the Git panel offers no merge.

## Handoff

Ready. One flow — fetch, create, open — across the workspace, GitHub, HTTP,
and browser layers, plus one merge-status rule; comparable to a normal
assignment. Phase 3 replaces `reviewSettings`' model rule and the fixed prompt
with config values.
