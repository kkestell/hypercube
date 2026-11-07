# Workspace deletion admission

## Goal

Fix #41: workspace deletion must not cascade a top-level session that was
created after its membership snapshot, or remove the workspace while creation
is still preparing or cleaning up a worktree. Keep dirty-worktree refusal.

## Decisions and implementation

- In `internal/session/runtime.go`, add workspace-scoped deletion claims and
  counts of admitted top-level creations, guarded by `Runtime.mu`. Keep these
  separate from the existing session-family claims.
- In `internal/session/sessions.go`, admit `Create` under that mutex before
  reading the workspace or doing Git work. Reject admission while deletion is
  claimed. Retain the creation admission until registration has completed or
  every failure cleanup has finished; release it on every return. Holding it
  through the existing initial `Input` is also safe. Support concurrent creates
  with a count, and leave filesystem, Git, and plugin operations outside the
  mutex.
- Under the same mutex, `DeleteWorkspace` rejects a concurrent deletion or
  an admitted creation, claims the workspace, and takes its top-level session
  snapshot before releasing the mutex. Refusal during creation avoids waiting
  or adding a coordination subsystem. Defer claim release on every result,
  including a dirty-worktree refusal and database errors. Existing `Delete`
  retains family claims and its normal cleanup and worktree checks. Return a
  clear error so callers can retry; do not change unrelated workspace behavior.

## Test plan

- In `internal/session/sessions_test.go` or `removal_test.go`, use the existing
  Git command barrier pattern to pause workspace deletion at a worktree status
  check after its snapshot, with the existing worktree already dirty. Attempt
  top-level creation and another workspace deletion while paused; both must
  refuse. Release the check, verify deletion refuses and preserves the session and
  worktree, then verify a new creation is admitted after claim release.
- Deterministically pause a worktree creation inside Git before registration
  (and, if needed, adapt the existing FIFO-based barrier). Deletion must refuse
  while creation is admitted. Release creation, verify its session and worktree
  are registered, and verify a later deletion handles them normally. Exercise
  failure cleanup as well so a failed creation does not leave the claim held.
  Include concurrent creations in the admission-count coverage and verify
  unrelated workspaces remain usable.
- Run focused session tests under `-race`, then `make check` as required for
  code changes. Expected production files: `internal/session/runtime.go` and
  `internal/session/sessions.go`; tests may use the existing removal barrier.
