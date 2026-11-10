# Plan review: 2026-11-09-011-verified-review-fixes

## Round 1

- Task ID: 2026-11-09-011-verified-review-fixes
- Mode: plan
- Author: independent plan reviewer, Hypercube child session
  review-fix-plan-opus (Claude Code, claude-opus-5-5; effort not exposed).
  Native conversation ID: unavailable (not exposed to this session); lookup
  context: checkout below, 2026-11-09. Parent: coordinator 0987244a (OpenCode,
  openai/gpt-6.1-sol, xhigh), native conversation
  ses_e3d29b976ffehNoBOsD5RvO3Xv.
- Plan (repository-relative): `.agents/plans/2026-11-09-011-verified-review-fixes.md`,
  blob `b7880e790dd55d41bb70d1941b9a8d0be0163f05`
- Guidance blobs: `AGENTS.md` `da2afb1eac69ad0b8f99faf9d986246965fcc051`;
  `docs/dev/development.md` `58c66cdf38859ff4e8b3e558f3d65d144dbc95c3`;
  `docs/dev/documentation.md` `91d766f9a831875a497bb8e0b9472778130df738`.
  No shared design.
- Source HEAD: `9fb7cda566a39ba70d0f7a16f44792566e6bbe60`, branch
  `hypercube/session-0987244a`, checkout
  `/Users/kyle/src/hypercube/.hypercube/0987244a` (clean at start).

### Verdict

- Work size: right-sized, at the top of one normal assignment. Six
  independent, bounded fixes with no shared state; splitting would produce
  brief-sized units and only add workflow overhead.
- Detail: appropriate. Each fix names its root cause, rule, owner, interface
  change and acceptance test without transcribing code.
- Readiness: not yet. Two medium corrections (findings 1–2) and one low one
  (finding 3) are needed. All are short edits for the parent to verify; no
  re-review is needed.
- Actions: primary `resolve-decisions`; secondary none.

### Findings

1. **Medium, correctness: Merge eligibility admits child sessions.** The plan,
   §2 says `Runtime.Merge` "fails if the session has no worktree branch".
   Child sessions with their own worktree have `Branch` set and `Base` empty
   (`internal/session/children.go:113-114` passes base `""`).
   `refreshMerges` never measures them (`internal/session/merges.go:35`,
   `s.Base == ""` skips), and their checkout is `parent.Dir`, not the
   workspace path. A direct `action=merge` call for a child, or a stale
   client, would compare against the workspace checkout's branch and tell the
   agent to merge into the wrong checkout. **Correction:** use the same
   eligibility as `refreshMerges`. Refuse unless `s.Base != ""`, which
   identifies a top-level worktree session. Ended sessions already fail in
   `r.input` → `wake` with `errEnded`
   (`internal/session/lifecycle.go:92`). The plan could also check `Ended`
   before reading the branch, so that refusal comes first. Add one test case
   for a refused session that was not measured.
2. **Medium, docs: the removal-check reference does not get the submodule
   rule.** `docs/storage.md:43-50` is the reference for the checks run before
   a managed worktree is removed. It lists uncommitted files and the
   branch/HEAD rules. Fix #1 adds a user-visible refusal: a submodule's
   commits must be on a remote branch of that submodule. The plan updates
   docs only for #4. `docs/dev/documentation.md:32` requires docs to match
   the code. **Correction:** add one sentence to that paragraph and list the
   file in §1 and in the implementation sequence. #2 and #6 need no doc
   change. The existing Merge text (`docs/teamwork.md:130-133`,
   `docs/interface.md:111,211`) stays accurate.
3. **Low, guidance accuracy: submodule paths.** §1 builds absolute paths
   from `$toplevel/$sm_path` and reports the path relative to `dir`.
   `$toplevel` is resolved through symlinks. On macOS, `t.TempDir()` under
   `/var/folders` prints as `/private/var/...` (observed below), so
   `filepath.Rel(dir, …)` gives `../../…/private/var/…/s`. The error text is
   then wrong and an exact-match test fails. **Correction:** print
   `$displaypath`, which is already relative to `dir` at every nesting level
   (observed: `s`, `s/n`), and run Git in `filepath.Join(dir, displaypath)`.
   Separately, the plan's verification for #1 says "Adapt
   `TestReviewSubmoduleCommitLost`", but no test of that name is in the
   checkout. It came from the earlier review's scratch work. The scenario the
   plan describes is enough to write the test from scratch. Rewording that
   line is optional.

Minor note, not a finding: the fake API in
`internal/github/githubtest/githubtest.go:176-180` counts every query that is
neither the pull request search nor the required-check query as a permission
query (`a.requests++`). When the fake learns the follow-up thread query, keep
it out of that count so existing `Requests()` assertions keep their meaning.

### Consequential choices checked

- **#1 rule and normal cleanup.** `git rev-list --count HEAD --branches --not
  --remotes` in each initialized submodule gives 0 right after
  `submodule update --init --recursive`. The submodule clone's default local
  branch equals its remote branch. A new commit in a nested submodule gives 1.
  The submodule's Git dir is under `root/.git/worktrees/<wt>/modules/<path>`.
  The parent worktree's status shows only ` M s`. That confirms why the outer
  checks pass and `worktree remove --force` loses the commit. Every
  `RemoveWorktree`/`Removable` caller (creation rollback in
  `internal/session/sessions.go:114,119`, unarchive rollback, archive/delete,
  child cleanup) runs on worktrees whose submodules are fresh or agent-made.
  Fresh submodules pass, so normal cleanup is unaffected. The limit the plan
  accepts (a pinned commit reachable only by SHA) is stated.
- **#2 action path.** `sessionAction` (`internal/ui/actions.go:107`) returns
  errors as 409 through `refused`. `requestMerge` is the only browser caller
  (`internal/ui/web/sessions/session-actions.ts:111`, from `merge-button.tsx`).
  It already sends no submission ID, so `r.input(id, "", prompt)` keeps the
  current delivery. `MergeStatus` and its handling of imported pull request
  commits (`base`) stay unchanged. The live-branch check and the refresh on
  refusal are sound, apart from finding 1.
- **#3 clone credentials.** Tested with Git 2.50.1, `GIT_CONFIG_NOSYSTEM=1`,
  and a global `credential.helper=store --file <tmp>`. A dumb-HTTP server
  required Basic `x-access-token:SECRET`. Given the plan's exact `-c
  credential.helper=` reset, the inline helper and `GH_TOKEN`, the clone
  succeeded. The store file was not created. The clone's `.git/config` held
  neither the token nor `credential.helper`. Without the helper, the clone
  failed with "could not read Username". This matches the plan's test design.
- **#4 ordering.** `fromNode` sets `Responded` for
  `mine && CHANGES_REQUESTED` before the thread loop (`pulls.go:225-243`).
  The new thread-time rule can therefore read it. The proposed `ownTurn`
  order (`turns.ts:48-54`) gives the three test outcomes the plan lists.
- **#5 paging cost.** The plan's estimate matches GitHub's per-connection cost
  model: about 20 points per pass now, about 100 at `first:100`. Paging only
  pull requests with `hasNextPage` keeps current costs when no pull request
  has more than 20 threads. `node(id:)` with `...on PullRequest
  {reviewThreads(first:100,after:)}` is a valid GraphQL form. Variables avoid
  hand quoting. The global feed limit and team request limit stay unchanged.
- **#6 collapse.** `split.tsx` makes only the fixed child collapsible and
  passes 0 from `onLayoutChanged` to `onResize`. `kept` ignores 0, so
  re-showing restores the last positive size. `closePanel` and the workbench
  flags are the right owners for `hideRegion`. The three region splits are at
  `app.tsx:106-109`. The session-view editor split is the only other fixed
  split, and the plan treats it explicitly. The editor-area split is
  `fixed="none"`, so it cannot collapse. The existing keyboard test
  (`split.test.tsx:60-88`) already shows keyboard collapse can reach 0, so the
  proposed test can be written. `readSavedViews` (`saved-views.ts:51`)
  rejecting 0 confirms the reload failure.

### Normalization

- Active target: 600–1,500 words (xreview `normalization.md`, experimental
  v1). No curated references.
- Measured (`plan_metrics.py`): 2,301 words, 49 in code fences, above the
  band. The helper suggests `split-work`/`trim-detail`.
- Assessment: the length comes from six unrelated root causes. Each needs
  about one paragraph of evidence, a rule and acceptance cases. No section
  repeats another or transcribes an implementation. Splitting would give six
  units smaller than the "normal assignment" reference, each at brief size,
  which adds review overhead without simplifying any decision. No cut would
  save meaningful text without dropping a constraint. The band does not fit
  a plan that bundles several unrelated brief-sized fixes. The out-of-band
  length is accepted for that reason, not on the count alone.

### Handoff

To pass, the plan needs:

1. §2: define Merge eligibility as `s.Base != ""` (top-level worktree
   session), with ended sessions refused, plus a test.
2. §1 and the sequence: update the removal-check paragraph in
   `docs/storage.md` with the submodule rule.
3. §1: list submodules by `$displaypath`, relative to `dir`, instead of
   `$toplevel/$sm_path`.

The parent can verify these edits directly.

### Checks and limitations

- Run: two temporary Git experiments in `mktemp` directories with an isolated
  `GIT_CONFIG_GLOBAL` (submodule rule and `foreach` paths; credential helper
  clone over local dumb HTTP served by a temporary Python server). Both were
  needed to settle the Git behavior the plan relies on. No repository files,
  user config or live state changed. Also ran `plan_metrics.py`.
- Not run: builds and tests (plan review). The GitHub GraphQL cost and the
  `node(id:)` paging form were checked against GitHub's documented API
  model, not against the live API. The keyboard-collapse path in
  react-resizable-panels 4.18.0 was confirmed only through the existing test's
  stated behavior.
