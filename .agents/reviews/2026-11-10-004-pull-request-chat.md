# Review: Chat about a pull request from its row

- Task ID: 2026-11-10-004-pull-request-chat
- Work log (repository-relative): `.agents/work/2026-11-10-004-pull-request-chat.md`
- Plan (repository-relative): `.agents/plans/2026-11-10-004-pull-request-chat.md`,
  blob `89d7a44eb3910849d383acbd847cd29e501bdc3e` (matches the log's executed blob)

## Round 1

- Author: reviewer child of Hypercube session 32a6c484 (Claude Code,
  native conversation ID `3a6882f8-2edc-4c46-982a-6e4de94863b7`,
  claude-opus-5-5 / default effort)
- Code base: `42cc82bd4f5936e077612f2ce20caa0552c3f0be`
- Reviewed Code head: `26c8ebbf3dfae19234be869baf6909b454cbb382`
  (one implementation commit, `26c8ebb`; base is its parent)
- Checkout: `hypercube/session-32a6c484` at
  `9dcdcbf4c7819149274bf520bff04c349c776fc2`, clean; `9dcdcbf` changes only the
  work log.
- Verdict: **accept**
- Independence: I did not plan or implement this change.

### Assessment

The change does what the request asks: **Clone** is gone. Every row on the
Pull Requests view and on a dashboard has **Chat**, with **Review** to its left
where shown. Chat opens the workspace's new-session page set to **New
worktree on branch** at the fetched head commit and sends nothing. An
untracked repository opens **Add {repo}**, which offers Clone into a folder the
user picks, or **Use Existing Folder…** with a remote check. The tricky
invariants hold:

- **Composer focus in every path.** The center shows a chat pane keyed by the
  session ID only when the new-session page is shown, and the dashboard or the
  Pull Requests view otherwise (`sessions/session-view.tsx`). Clicking Chat
  therefore always mounts a fresh `Composer`, whose mount effect focuses the
  text box. That includes reopening a parked new-session page. The one case
  where `openUnsent` returns the state unchanged (that page already shown)
  cannot occur, because no pull request row is visible then. After a clone or
  existing-folder add, the dialog's close-focus targets the Chat button.
  Leaving the view detaches that button, so the focus attempt does nothing and
  does not take focus away from the composer. A scratch test confirmed this
  (see Verification).
- **Draft and model survive; a sending page is left alone.** `startChat` uses
  `openUnsent`, which restores the parked draft and settings, and skips
  `unsentWorktree` while sending. The dashboard test covers the first part.
- **Branch and token.** `PullRequestHead` reads the branch from the account's
  pull requests rather than the workspace's status. It works right after an
  add, before the watcher publishes, and the Go integration test exercises
  exactly that. `FreeBranch` checks only `refs/heads`, and `AddWorktree`
  still refuses an existing name at send, so a branch created in between fails
  safely.
- **Existing-folder check** uses the watcher's own mapping (`workspace.Remote`
  plus `parseRepo`), so an accepted folder maps to the row's repository, and
  `PullRequestHead` fetches from the same remote.
- **Removing the pending-workspace selection** is safe. The remaining
  `selectSession(workspaceKey…)` callers are the sidebar (always a listed
  workspace) and page load, where `initialState` already holds the hash
  selection and `receivedWorkspaces` clears it if absent.

### Findings

None blocking.

Notes for the parent, not required corrections:

- N1 (request wording versus plan). The user asked that the folder have "a
  remote that matches the repo". The plan and code require the remote the
  watcher maps from: the current branch's remote, else `origin`. A common fork
  layout fails this check: `origin` is the user's fork, `upstream` is the
  repository, and `main` tracks `origin`. The dialog then says the folder "is
  a checkout of me/r, not o/r". The stricter rule is correct as designed.
  Under "any remote matches", the added workspace would map to the fork, the
  row would stay untracked, and `PullRequestHead` would fetch
  `refs/pull/N/head` from the fork and fail. Worth one line to the user,
  because it is a deliberate narrowing of their wording.
- N2 (`internal/ui/web/github/all-pull-requests.tsx:61`). The plan keys Chat
  by account and URL "so the row stays disabled through a clone". During the
  clone, the modal dialog already blocks the row. After the add, though, an
  untracked row's `onChat` is the open-dialog handler, which ignores the
  running key. So the row's Chat stays enabled during the pull request fetch
  that follows, until the watcher publishes the new workspace. A second click
  in that window only reopens **Add {repo}**: a clone then fails with Git's
  "destination exists" error, and adding the same folder is harmless. Low
  impact, so not a required change.

### Verification

| Check | Target | Result | Source |
| --- | --- | --- | --- |
| `go test -race ./internal/workspace ./internal/github ./internal/ui`, then `go test -race -count=2 ./internal/ui` | `26c8ebb` | ok | reused, work log |
| `bunx tsc -p .`, `bun test` in `internal/ui` | `26c8ebb` | no errors; 480 pass | reused, work log |
| `make check` | `26c8ebb` | exit 0 | reused, work log |
| `make screenshots`; four captures committed | `26c8ebb` | Pull Requests light capture inspected: Chat on every row, Review left of it, Review disabled for the untracked `harbor-docs` | reused, plus independent inspection |
| Scratch `bun test` of Chat focusing the composer through `SessionView`, from: the dashboard; the dashboard after the new-session page was shown with and without a draft; the Pull Requests view; the Pull Requests view after a drafted new-session page | `26c8ebb`, detached scratch worktree in `/tmp` (removed; nothing added to the repository) | 5 pass; each sets the branch and focuses the `TEXTAREA` | run, to settle the focus question the log left to the manual check |

I read the tests' assertions, not only their names. The Go integration test
checks the exact JSON (`feature`, then `feature-2`, at the pushed commit),
409 and 400 bodies, and that a refused `new` adds no workspace. The browser
tests check request parameters, the resulting `unsent` state, preserved draft
and model, the error toast with the selection unchanged, the disabled state
while a request runs, both dialog paths, and the dialog's absence without the
desktop bridge.

### Limitations

- I did not run `make dev` against a real GitHub pull request. The real fetch
  of `refs/pull/N/head` is covered only through a local bare repository via
  `insteadOf`. Focus was checked in happy-dom, not a real browser.
- Use Existing Folder… was not exercised through the real native folder
  chooser.

## Round 2

- Author: reviewer child of Hypercube session 32a6c484 (Claude Code,
  native conversation ID `3a6882f8-2edc-4c46-982a-6e4de94863b7`,
  claude-opus-5-5 / default effort)
- Code base: `42cc82bd4f5936e077612f2ce20caa0552c3f0be`
- Reviewed Code head: `d1d33e8e4211fbd3a2a07f53ae804b14ce8c2ae3`
  (implementation commits `26c8ebb` work, `d1d33e8` fix)
- Checkout: `hypercube/session-32a6c484` at
  `a619aa45a5980c621a55f0d4bbf12c2a962054ac`, clean. After `26c8ebb`, the only
  changes outside the fix are the round 1 review (`b742dcf`) and the work log
  (`a619aa4`).
- Verdict: **accept**
- Independence: I did not plan or implement this change or its fix.

### Assessment

Round 1 reviewed `26c8ebb` and still applies. The fix changes only
`row-action.ts`, both lists, and one test. `useRowAction` now returns
`{run, running}`. Both callers of each hook (`all-pull-requests.tsx` and
`pull-requests.tsx`) switched to `run` with the same keys as before, so Review
and the dashboard's Chat behave as in round 1.

On the Pull Requests view, an untracked row's Chat is now undefined, and so
disabled, while `running(rowKey(account, pr))` holds. The chat that follows an
add runs under that same key through `chatIn`. The row is disabled from the
add until that chat finishes, whether or not the watcher has listed the new
workspace. Once it does list it, the row switches to `chatIn`, whose `run`
returns undefined for the running key. A refused chat clears the key and
re-enables the row.

The implementer did not use the round 1 suggestion of wrapping the
open-dialog handler in the running key. Wrapping it would have disabled the
opener while the dialog opened and broken focus returning to it. Tracking
only the chat after the add is the right choice. The new test checks the
button really is disabled while the post-add `pull-request-branch` request is
pending, and enabled after it answers.

### Resolution

| Finding | Status at `d1d33e8` | Evidence |
| --- | --- | --- |
| N1 (stricter remote rule than the request's wording) | Unchanged by design; for the parent to mention to the user | No code change needed; reasoning in round 1 |
| N2 (untracked row's Chat enabled during the chat after an add) | Resolved | `all-pull-requests.tsx` `onChat` checks `chat.running(rowKey(account, pr))`; test "an untracked row stays disabled while the chat after an add is starting" in `all-pull-requests.test.tsx` |

No new findings.

### Verification

| Check | Target | Result | Source |
| --- | --- | --- | --- |
| `bunx tsc -p .`, `bun test` in `internal/ui` | `d1d33e8` | no errors; 481 pass, 0 fail | reused, work log repair round 1 |
| `make check` | `d1d33e8` | exit 0 | reused, work log repair round 1 |
| Go race tests, screenshots | `26c8ebb` | as in round 1 | reused; the fix touches only browser code under `internal/ui/web/github/`, with no rendering change |
| `git grep useRowAction` at `d1d33e8` | `d1d33e8` | the only callers are the two lists, both updated | run, to confirm the return-shape change reached every caller |

### Limitations

Same as round 1: no `make dev` run against a real GitHub pull request, and no
run through the real native folder chooser.
