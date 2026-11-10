# Worktree branch choice and discarding a worktree session's work

- Task IDs: 2026-11-09-012-worktree-branch-choice-and-discard (unit 1),
  2026-11-09-013-discard-worktree-work (unit 2)
- Status: ready
- Source HEAD: aadeae94d694eafe119f44800afa6932983b8742
- Author: planner child of Hypercube session 3703b04c (Claude Code, Opus 5.5,
  native conversation ed140e50-7ea6-40f1-84c8-faaf6329d964)

## Request

1. When creating a worktree session, the user chooses the new worktree's
   starting point (a branch or commit) and its branch name.
2. Pull requests from worktree sessions do not have to use the internal
   `hypercube/session-<id>` branch. The chosen name is the session's branch;
   the pull request icon, merge status, and cleanup (including unarchive) track it.
3. A discard action throws away a top-level worktree session's uncommitted
   files and unmerged commits so that it can then be archived or deleted.

`docs/workflows.md` is the parent's; each unit updates the other user docs
whose facts it changes.

## Units and scope map

| Request item | Unit | Plan |
| --- | --- | --- |
| 1, 2 | Named branch and starting point for new worktree sessions | [.agents/plans/2026-11-09-012-worktree-branch-choice-and-discard.md](../plans/2026-11-09-012-worktree-branch-choice-and-discard.md) |
| 3 | Discard all changes in a worktree session's family | [.agents/plans/2026-11-09-013-discard-worktree-work.md](../plans/2026-11-09-013-discard-worktree-work.md) |

Item 2 needs no separate mechanism. Every consumer already reads the saved
`store.Session.Branch`: `pullRequest` (`internal/ui/snapshot.go`),
`refreshMerges`/`MergeStatus`, `Removable`/`RemoveWorktree`, `Unarchive`'s
`AddWorktree`, and the agent instructions (`internal/report/report.go`). Only
`Runtime.create` derives the name from the session ID. Unit 1 saves the chosen
name there and covers the consumers with tests.

## Dependencies and ordering

The units are independent in behavior. Run them serially in the task checkout
(012, then 013) because both edit `internal/ui/actions.go`,
`internal/ui/web/sessions/session-actions.ts`, and `docs/usage.md`. Unit 2
does not depend on any unit 1 interface.

## Shared facts both units rely on

- A top-level worktree session has a non-empty `Base`: the commit its branch
  started from. `Removable` keeps commits reachable from the checkout's
  `HEAD`, `Base`, or any remote-tracking branch; a child's kept refs are its
  parent worktree's `HEAD` and remote-tracking branches.
- Removal claims a family with `Runtime.claimRemoval` and releases it with
  `releaseRemoval`; the claim blocks wakes, child creation, and new shells.
- No database change: `sessions.branch` already stores any branch name.
  Existing sessions keep their `hypercube/session-<id>` branches.

## Rejected whole alternatives

- **Checking out an existing branch in the new worktree.** Git refuses a
  branch already checked out elsewhere (usually the case for the checkout's
  branch), and archive deletes a session's branch, which would delete a branch
  the user owned before the session. Every worktree session creates a new
  branch; an existing branch can only be its source.
- **Folding discard into Archive/Delete as a forced removal.** That would
  satisfy "then archived or deleted" in one step, but the request asks for a
  separate action, and resetting is also useful for a session the user keeps.
