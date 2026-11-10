# Verified review fixes: submodule removal, merge target, clone auth, turns, review threads, collapse

- Task ID: 2026-11-09-011-verified-review-fixes
- Status: ready
- Source HEAD: 310c5a6945ab50f0d0c79f3df2036d83b7c410af (revised at
  4063381fe97d6aa6b1ef2c00df69f75d3479e876, after plan review round 1)
- Author: planner, Hypercube child session plan-verified-fixes-opus (Claude
  Code, claude-opus-5-5, effort not exposed); native conversation ID
  unavailable (not exposed to this session). Parent: coordinator 0987244a
  (OpenCode, openai/gpt-6.1-sol, xhigh), native conversation
  ses_e3d29b976ffehNoBOsD5RvO3Xv.
- Dependencies: none.
- Request: fix the six verified review findings (#1 high; #5b, #6, #10
  narrowed, #12, #14 medium) listed below. Everything else from the review is
  out of scope.

## Outcome and boundaries

1. **Submodule commits survive removal (#1).** Removing a managed worktree
   refuses when a submodule in it has commits that no remote branch of that
   submodule has. Removing a worktree whose submodules sit on commits that are
   already on a remote branch still works.
2. **Merge goes where the user sees (#5b).** **Merge into X** never asks the
   agent to merge into a branch other than the one the checkout is on when the
   click happens. The session-work count still excludes imported pull request
   commits, and Merge is not offered just because such commits exist.
3. **Clone authenticates with the chosen account (#6).** Cloning a private
   repository from the Pull Requests view works with only the gh account's
   token, with no Git credential helper configured. The token does not appear
   in arguments, the clone's config, or any credential store.
4. **A reviewer's comment after a push is the author's move (#10).** On the
   author's own pull request, after a push answers requested changes, a newer
   conversation comment or thread reply makes it the author's move. Unresolved
   threads from the original change request do not.
5. **All review threads count (#12).** Unanswered and responded state use every
   review thread of a pull request, not only the first 20.
6. **Collapsing a region does not break saved views (#14).** Dragging or using
   the keyboard to collapse the sidebar, right panel, or terminal panel hides
   that region. No size of 0 is saved, and reload restores tabs, layout, and
   selection.

Non-goals: the other review findings, including the "0 commits ahead" label
(#5a), the global 50-result feed cap and team request cap (#11), and the same
missing credential helper for the watcher's background `Fetch`. No changes to
effort policy, legacy saved-view formats, packaging, or user global config.

## Current system and decisions

### 1. Submodule commits (`internal/workspace/workspace.go`, `docs/storage.md`)

`Removable` checks the worktree's branch and HEAD against kept refs and checks
`git status`. When the agent commits inside a submodule and then commits the
gitlink, status is clean. The submodule's repository lives under the worktree's
own Git dir (`.git/worktrees/<name>/modules/<path>`; confirmed on Git 2.50 by
the earlier review's scratch test and by the plan reviewer's experiment). `git worktree remove --force`
deletes it, so if that commit was never pushed, the only copy is gone and the
checkout's gitlink points at a missing commit.

**Decision:** in `Removable`, after the uncommitted-files check, check each
initialized submodule of the worktree, recursively. A submodule passes when
`git rev-list --count HEAD --branches --not --remotes` run in it is 0. That
rule mirrors the outer rule ("on a remote branch") and needs no backup. Local
branches are included because an agent may leave work on a submodule branch
that HEAD no longer points at.

- List submodules with
  `git -C dir submodule foreach --quiet --recursive 'printf "%s\0" "$displaypath"'`.
  `foreach` visits only initialized submodules, whatever their
  `submodule.<name>.ignore` setting, and `$displaypath` is relative to `dir`
  at every nesting level (`s`, `s/n`). Run `rev-list` in
  `filepath.Join(dir, path)`. Do not build paths from `$toplevel`, which Git
  resolves through symlinks (`/private/var/...` on macOS), so the error path
  would be wrong. Skip this step when `dir` has no `.gitmodules`, as `setUp`
  does.
- Error: `submodule <displaypath> has commits that are not on a remote branch`.
  Name it in a small helper next to `unkept`, for example `unpushedSubmodule(dir) (string, error)`.
- `RemoveWorktree` keeps `worktree remove --force`, because `Removable` has
  now checked submodules too. Update its comment to say so.
- Accepted limit: a submodule pinned to a commit that upstream has only by
  SHA, on no branch, blocks removal. This does not happen in normal use.
- Docs: in the removal-check paragraph of `docs/storage.md` (lines 43–50),
  add that each submodule's commits must be on a remote branch of that
  submodule. This rule applies whatever Git's submodule ignore settings say.
  Narrow the last sentence so that it describes the uncommitted-files check
  only: it honors Git's submodule settings, including settings that ignore
  submodule changes. Read `docs/dev/documentation.md` first.

### 2. Merge target (`internal/ui/web/changes/merge-button.tsx`, `internal/session/merges.go`)

`session.merge` is measured by `refreshMerges` after a turn ends and at a few
lifecycle points. The Committed tab reads the live target from
`workspace.BranchChanges`. If the checkout switches branches while the agent is
idle, the button keeps the old target, and `requestMerge` puts that target into
the prompt.

**Decision:** the backend checks the target at the moment the user clicks.

- Add `func (r *Runtime) Merge(id, target string) error` in `internal/session/merges.go`.
  It reads the session and its workspace. It uses the same eligibility as
  `refreshMerges`: it refuses an ended session and a session with
  `s.Base == ""`, which covers non-worktree sessions and children whose
  checkout is their parent's directory. It refuses before reading any branch or
  sending input. It then reads `workspace.Branch(root)`. If that differs from
  `target`, it calls `r.refreshMerges(workspaceID)` (which publishes the
  current target) and returns
  `fmt.Errorf("the checkout is now on %s, not %s", live, target)`. A detached
  HEAD gives `live == ""`, which never equals a branch name. Otherwise it calls
  `r.input(id, "", prompt)` with the prompt the browser sends now:
  `Commit any remaining work, then merge your branch into <target> in the checkout at <root>, resolving any conflicts, and report the result.`
- Add `case "merge": err = s.rt.Merge(id, q.Get("target"))` to `sessionAction`
  in `internal/ui/actions.go`. Its error is the existing 409 text.
- `requestMerge(store, session, target)` posts
  `{action: 'merge', id, target}` with no body. `MergeButton` drops the path
  argument. The label still comes from `session.merge.target`. After a refused
  click, the refreshed state relabels the button, so no timing is involved.
- `MergeStatus`, `Unmerged`, and the button's visibility rule stay unchanged.

### 3. Clone authentication (`internal/github/clone.go`, `internal/workspace/remote.go`)

`Watcher.Clone` passes `GH_TOKEN` in the environment, but Git reads tokens only
through a credential helper. Without one, the clone sends no credentials
(reproduced: 0 authenticated requests; the same token works once a helper
reads it).

**Decision:** `workspace.Clone` takes the token and authenticates this one
command:

```go
// Clone clones url into dir with each config entry written into the new
// repository. A token, when set, answers Git's HTTPS credential requests for
// this clone only, through a helper that reads it from GH_TOKEN.
func Clone(ctx context.Context, url, dir string, config []string, token string) error
```

When `token != ""`, prepend these arguments before `clone`:
`-c credential.helper=` (empty, which clears every configured helper, so
osxkeychain and other helpers neither answer nor store the token) and
`-c credential.helper=!f() { test "$1" = get && printf 'username=x-access-token\npassword=%s\n' "$GH_TOKEN"; }; f`.
Also add `GH_TOKEN=<token>` to the environment. Use `-c`, not `--config`, so
nothing is written into the clone. The helper is not limited to one host
because this command contacts only the clone URL. `github.Clone` passes `token`
instead of the env slice. Shell helpers run through `sh` on both macOS and
Linux.

### 4. Conversation comment after a push (`internal/github/pulls.go`, `internal/ui/web/github/turns.ts`)

`ownTurn` returns "Waiting for another review." for
`review === 'CHANGES_REQUESTED' && responded` before it checks `unanswered`.
The conversation comment never surfaces. Checking `unanswered` first, as it is
counted now, would make the original change request's unresolved threads (the
reviewer spoke last) the author's move again after every push.

**Decision:** the backend owns which comments count, and the frontend checks
unanswered comments before waiting.

- In `fromNode`, for `mine && Review == "CHANGES_REQUESTED" && Responded`, a
  thread counts only if its last comment is newer than the last commit
  (`n.Commits.Nodes[0].Commit.CommittedDate`), the same rule
  `unansweredConversation` already applies. Other states keep the current
  thread rule.
- `ownTurn` order becomes: draft, required check failed, DIRTY, BEHIND;
  `CHANGES_REQUESTED && !responded` → fix "Changes requested.";
  `unanswered` → fix "N unanswered comments."; `CHANGES_REQUESTED` → theirs
  "Waiting for another review."; the rest unchanged.
- Update the own-pull-request bullet in `docs/interface.md` (about lines
  160–165). After a push answers requested changes, only comments newer than
  the last commit count as unanswered. Read `docs/dev/documentation.md` first.

### 5. All review threads (`internal/github/pulls.go`)

`reviewThreads(first:20)` drops the newest threads. GitHub's
`PullRequest.reviewThreads` takes only `first/last/after/before`, with no
`isResolved` filter. Raising the limit to 100 in the search query multiplies
the rate-limit cost: about 50 PRs × 100 threads × 2 comment connections ≈ 100
points a pass, each minute, per account. That exceeds the 5,000 points an hour
budget.

**Decision:** keep `first:20` in the search and page the rest only for pull
requests that have more.

- Move the thread fields into `fragment thread on PullRequestReviewThread{isResolved first:comments(first:1){…} last:comments(last:1){…}}`
  and use it in both queries. Add `id` to the `pr` fragment and
  `pageInfo{hasNextPage endCursor}` to `reviewThreads(first:20)`.
- Split `accountPullRequests` into three steps: dedupe the nodes first, then
  `w.remainingThreads(ctx, token, nodes)`, then `fromNode`. `remainingThreads`
  loops while any node has `hasNextPage`. It sends one query for all of them,
  with an alias per pull request
  (`p0:node(id:$i0){...on PullRequest{reviewThreads(first:100,after:$c0){pageInfo{hasNextPage endCursor} nodes{...thread}}}}`)
  and variables for IDs and cursors, so nothing is quoted by hand. It appends
  the nodes and updates each `pageInfo`. A query error fails the pass the same
  way `requiredFailures` does.
- No general pagination helper. The 50 and 20 search limits stay.

### 6. Collapse (`internal/ui/web/shared/split.tsx`, `app/app.tsx`, `app/workbench.ts`)

A fixed child is `collapsible` so the `collapsed` prop can call `collapse()`;
the library ignores `collapse()` on a non-collapsible panel. That also lets the
user drag or press keys past the minimum to collapse it. `onLayoutChanged` then
reports 0, which `resizeRegion` saves. `readSavedViews` requires positive sizes,
so the next load throws and every saved view falls back to defaults.

**Decision:** a user collapse hides the region, as in editors such as VS Code.

- Add `onCollapse?: () => void` to `Split`. In `onLayoutChanged`, when
  `meta.isUserInteraction` and the fixed child's layout value is 0, call
  `onCollapse?.()` and skip `onResize`. `kept` still holds the last positive
  pixel size, so showing the region again restores it.
- Add `hideRegion(state, region)` to `workbench.ts`. For `'panel'` it reuses
  `closePanel`; otherwise it sets the workbench flag to false. `app.tsx` passes
  `onCollapse={() => store.update(s => hideRegion(s, '<region>'))}` to the
  three region splits.
- The session view's editor split passes no `onCollapse`. A drag-collapse
  there lasts only for the page, the handle stays usable, and the saved size
  stays positive.
- `saved-views.ts` stays unchanged.

## Implementation sequence

Each step can be tested on its own. Commit all task code together after
`make check`, as `docs/dev/development.md` requires.

1. Workspace submodule check, its comment, its tests, and the
   `docs/storage.md` removal-check paragraph.
2. `workspace.Clone` signature and arguments, `github.Clone`, and the clone
   tests.
3. `Runtime.Merge`, the `merge` action, `requestMerge`/`MergeButton`, and their
   tests.
4. `fromNode` thread timing, `ownTurn` order, the doc bullet, and tests.
5. Thread fragment, `pageInfo`/`id`, `remainingThreads`, fake API support, and
   tests.
6. `Split.onCollapse`, `hideRegion`, `app.tsx` wiring, and tests.

## Verification

Real Git and HTTP in temp directories; no network.

- **#1** `internal/workspace/workspace_test.go`. Write a new test for this
  scenario (the earlier scratch test is outside this checkout): an agent commit in the
  worktree's submodule plus a gitlink commit fast-forward merged into root, so
  `RemoveWorktree` errors, naming the submodule. The worktree dir still exists,
  and `git cat-file -e <commit>` succeeds in the worktree's submodule. Then
  push the submodule commit to the upstream repository (`git push origin HEAD:refs/heads/w`
  from the submodule), and `RemoveWorktree` succeeds. Keep
  `TestWorktreeSetUp` passing, since it covers the plain submodule removal.
  Also cover a commit left only on a submodule branch with HEAD moved back.
- **#2** `internal/session` (reuse the fixture in `merges_test.go` and
  `input_test.go`). Switch root to another branch after measuring. `Merge(id, "main")`
  returns an error, delivers no input, and `Merges()[id].Target` becomes the
  new branch. `Merge(id, <live branch>)` delivers the prompt naming that branch
  and root. A child session with its own worktree (`Base` empty) and a
  non-worktree session are refused with no input delivered. Update
  `changes/git-panel.test.tsx` so the button posts
  `action=merge&target=main` and no prompt body.
- **#3** `internal/workspace/remote_test.go`. Serve a bare repository over dumb
  HTTP (`update-server-info`, `http.FileServer`) behind Basic auth that
  requires the token. Set `GIT_CONFIG_NOSYSTEM=1` and a `GIT_CONFIG_GLOBAL`
  file that names a `credential.helper` of `store --file <tmp>`. The clone
  succeeds, the store file is not created, and the clone's `.git/config`
  contains neither the token nor `credential.helper`. Keep the existing
  `credential.githubAccount` test, updated to the new signature.
- **#4** `internal/github/watcher_test.go` and `web/github/turns.test.ts`.
  Bob changes-requested on c1, Alice's head is c2 committed at 11:00. (a) A
  Bob conversation comment at 12:00 gives Unanswered 1 and the turn
  "fix … 1 unanswered comment.". (b) Only an unresolved Bob thread from 10:00
  gives Unanswered 0 and "Waiting for another review.". (c) A Bob thread reply
  at 12:00 gives Unanswered 1.
- **#5** `internal/github/githubtest` and `watcher_test.go`. Teach the fake API
  to answer the follow-up query from per-PR pages keyed by node ID and cursor.
  It must read the request's `variables`. A pull request with 20 resolved
  threads on the first page and a 21st unresolved thread from someone else on
  the next page gives Unanswered 1 and is not ready to merge. A pull request
  with `hasNextPage: false` makes no follow-up request. The fake counts thread
  follow-up queries separately from permission queries, so `Requests()` still
  counts only permission queries while paging.
- **#6** `web/shared/split.test.tsx` using its `geometry()` helper. Keyboard
  collapse of a fixed end split calls `onCollapse` once and never reports 0.
  Re-show restores at least the minimum, as the existing tests check. In
  `web/app/view-storage.test.ts`, applying `hideRegion(s, 'rightPanel')` after
  a resize and reloading keeps saved tabs and selection, `rightPanel` false,
  and the earlier positive size.
- Commands while working: `go test ./internal/workspace ./internal/github ./internal/session ./internal/ui -race`;
  in `internal/ui`, `bunx tsc -p .` and `bun test`. Before the code commit,
  run `make check`. No screenshots. The tests use only `git`, `sh`, and Go's
  HTTP server, which both macOS and Linux CI have.

## Handoff

Ready. No open engineering decisions or blockers.

Work size: six independent fixes. Fixes #2, #3, #4, and #6 are each brief-sized.
Fixes #1 and #5 are each about a third of a normal assignment, and #5 adds fake
API support. Together they are at the top of one normal assignment and below a
split. The fixes share no state, so splitting would add only workflow overhead.
Detail: each fix states the root cause, the chosen rule and owner, the
interface changes, and its acceptance tests. Coding details are left to the
implementer.

Metrics (`plan_metrics.py`, experimental default band 600–1,500, no curated
references): about 2,500 words after the round 1 corrections, 49 in code fences, above the band. The excess
comes from six unrelated root causes, at about 370 words each. Each needs its
evidence, rule, and acceptance cases. Splitting would produce six brief-sized
units, which the parent asked to avoid. No section repeats another, and none
transcribes an implementation.
