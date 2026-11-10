# Work review: 2026-11-10-001-injected-state-paths

## Round 1

- Task ID: 2026-11-10-001-injected-state-paths
- Mode: work
- Author: Hypercube child session `go-test-review` of coordinator `b9d8c3e5`
  (Claude Code, Opus 5.5 `claude-opus-5-5`). Native conversation ID:
  unavailable; this session exposes no session-metadata tool. Review started
  2026-11-10 in checkout `.hypercube/b9d8c3e5-go-test-speed`. I did not write
  the code; the implementer was Hypercube worker `go-test-speed` (OpenCode,
  `openai/gpt-6.1-sol`).
- Work log: `.agents/work/2026-11-10-001-injected-state-paths.md` (commit
  `322c3c3b421176e01835e2843c8105bb9f1c28ae`)
- Plan: `.agents/plans/2026-11-10-001-injected-state-paths.md`, blob
  `8a99ed08b3a55ab433af127e3e750044322738c3` (matches the log). No separate
  plan review; the coordinator approved the design in chat.
- Code base: `55fdf51766a938dac3160c152e2c94418d234f6a`
- Code head: `c9d024eeeccd2819b25909af7caccfc0dffacd34`. The base is an
  ancestor of the head. The range contains `848727d` (test-only pass),
  `071b023` (plan), and `c9d024e`. The checkout is at `322c3c3`, which adds
  only the work log; the working tree was clean.

### Verdict

`changes-required`, for one coverage gap. The production change is correct and
keeps behavior: every former `store.Dir()` and `config.Load()`/`Save()` user now
receives the same XDG-resolved paths from `serveUI`
(`cmd/hypercube/backend.go:111`), `store.Open` keeps its order (create, chmod
0700, schema, column additions, last-active reset, then the model-entry move
into the injected config file), and the reopen/migration tests in
`internal/store/reopen_test.go` still exercise the move through the default
paths. No production code resolves XDG paths anywhere else.

Parallel execution is sound. Every remaining `t.Setenv` is in a test that does
not call `t.Parallel`, and Go finishes all sequential top-level tests before it
resumes paused parallel ones, so environment changes never overlap parallel
tests. Fixture environment reaches plugins and shells only through
`Runtime.Env`. All calls to `Runtime.env` hold `r.mu`
(`launch.go:139`, `shells.go:30,62`), and `fixture.setenv` takes the same
lock. The package-level `TestMain` environment settings are fixed before
`m.Run`. The helper-process holder runs the same `hold.Main` as
`cmd/hypercube-hold`.

The Git-hook barriers hold the same admission windows as the old wrappers for
every remaining caller. Those callers need the removal claim held during a
status check, not a stale status result. The partial-removal test now makes the
second worktree truly dirty between the initial checks and its removal. That
is a more realistic refusal than the old forced `git worktree remove` failure,
and it still proves that partial cleanup is recorded. However, the
`post-checkout` barrier only fires when `git worktree add` succeeds. The cases
that held a *failed* creation were deleted, and with them the only coverage of
`AddWorktree`'s setup-failure rollback (finding 1).

### Findings

1. **Medium: the worktree setup-failure rollback lost its only test.**
   `internal/workspace/workspace.go:266-269` removes the new worktree and
   deletes its branch when `setUp` fails (copying `.worktreeinclude` paths or
   updating submodules). Only the deleted
   `TestWorkspaceDeletionRefusesUntilFailedCreationCleansUp` exercised that
   path; it made `.worktreeinclude` a directory. The `setup-failure` case
   removed from `TestFamilyRemovalWaitsForAdmittedChildCreation` was also the
   only session-level coverage of a child creation that fails inside
   `AddWorktree` while a family removal waits for it
   (`internal/session/children.go:114-119`: `r.end`, `close(l.done)`,
   `discard`), including the check that the directory occupying the child's
   path survives. Failure scenario: a change drops the `branch -D` in that
   rollback. A user whose `.worktreeinclude` names an unreadable path then
   gets an error, and the leftover `hypercube/<id>` branch makes every retry
   with that branch fail with "already exists". No test fails. These cases
   were dropped because the new barrier cannot hold a failed `worktree add`,
   not because the cases were judged low-value. The work log presents them as
   redundant synthetic failures.
   Correction: add a parallel `internal/workspace` test in which
   `.worktreeinclude` is a directory. `AddWorktree` must return an error and
   leave no branch, no directory, and no `git worktree list` entry; it costs
   one repository and needs no runtime. For the session-level case, hold the
   failing creation with a repository-local `reference-transaction` hook
   (`git worktree add -b` creates the branch before it fails on an occupied
   directory), or record in the work log that this case is deliberately
   dropped.

2. **Low: deleting `TestChildSettings` removed the only check that a child
   asking for another model starts at that model's configured default effort.**
   `internal/session/selection.go:83-89`. The table ran no processes, so
   deleting it saved no time. Its other rules are still covered:
   another-harness-without-model by `TestCreateSessionRefusals` and
   inheritance by `TestChildInheritsTheParentsCurrentSettings`. Default effort
   from a `config.ModelEntry` is not covered anywhere. Failure scenario: a
   change drops the entries loop, and children asked to use the user's
   configured model start with the harness's own default effort instead of the
   one the user configured. No test fails. Correction: restore the table (it is
   fast), or add one model-entry case to
   `TestChildInheritsTheParentsCurrentSettings` using `f.config.Save`.

3. **Low: `pauseGit` is harder to read than necessary.**
   `internal/session/removal_test.go:25-112` switches between two unrelated
   mechanisms on a `command` string in four places: an `fsmonitor` hook with a
   `$PWD` condition and `exit 1`, and a `core.hooksPath` `post-checkout` hook
   with a null-OID condition. `dir` does not affect the checkout variant. The
   comment "holds the first Git status in dir" is also too narrow: the
   `fsmonitor` hook fires for any command that refreshes the index in that
   directory. Because `core.fsmonitor` is written to the repository's shared
   config, every other worktree runs the hook until release. Correction: split
   it into `pauseStatus(dir)` and `pauseWorktreeAdd()`, each writing its own
   hook and sharing a small barrier helper (reached/release pipes and
   cleanup). State in the comments which commands each hook catches.

### Notes (non-blocking)

- `TestDeliveryTimeoutHoldsLaterMessagesUntilAcceptance` now sets
  `f.rt.requestTimeout` after `f.session()` has started the agent
  (`internal/session/restart_test.go:331`). It is race-free only because the
  next delivery is ordered after the write through the runtime lock. Setting it
  before `f.session()` with a value the first turn tolerates would not depend
  on that.
- The unarchive rollback when `workspace.Head` fails after recreating the
  worktree (`internal/session/removal.go:76-77`) lost its test
  (`TestFailedUnarchiveCanBeRetried`). Triggering it needs a synthetic Git
  failure, and the remaining code is a single `RemoveWorktree` call, so this
  loss is acceptable.
- The work log says hooks "are removed after their barrier fires"; they are
  removed at release.

### Verification

- Run: `timeout 300 go test -race -count=1 ./...` at the reviewed code
  (checkout `322c3c3`, whose code equals `c9d024e`), macOS arm64, Git 2.50.1:
  passed. Session package 30.5s, ui 17.9s, workspace 16.8s, github 12.3s.
  Output was kept locally at `/tmp/xreview-gotest/full.txt`. I ran it once to
  confirm the parallel suite passes on a machine other than the implementer's
  load profile, as the coordinator asked.
- Reused: the implementer's three uncached race runs, `go vet`,
  `git diff --check` and `make check` at `c9d024e`, as recorded in the work log
  with evidence files under the worker's temporary directory. I did not
  re-inspect those files.
- Not run: Linux (`scripts/linux/run`). The hook barriers depend on Git's
  `fsmonitor` and `post-checkout` behavior, which is stable across the Git
  versions in the Linux images, but this round did not confirm it there.
