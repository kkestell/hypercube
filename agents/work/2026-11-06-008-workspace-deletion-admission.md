# Workspace deletion admission

## Plan

`agents/plans/2026-11-06-009-workspace-deletion-admission.md`

## Summary

Implemented #41 as planned. Workspace deletion and top-level creation now
exclude each other through short mutex-protected admission checks. Creation
keeps its admission through registration, initial input, and failure cleanup;
deletion releases its claim after success or refusal. Git, filesystem, and
plugin operations remain outside the new critical sections.

The existing Git barrier now also pauses worktree creation and cleanup. Three
deterministic regressions cover dirty-worktree refusal and claim release,
concurrent creations with independent admission counts, unrelated workspaces,
and failure cleanup before branch deletion completes.

## Checks run

- `go test -race ./internal/session -run 'TestWorkspaceDeletion' -count=1` —
  passed.
- `go test -race ./internal/session -run 'Test(WorkspaceDeletion|FamilyRemovalClaimsMembership|EndChildBlocksShellAdmission|RefusedRemoval|PartialFamilyRemoval|Create|WorktreeSession|Delete|Deleting)' -count=1`
  — passed, including existing removal users of the shared barrier.
- `git diff --check` on the implementation files — passed.

## Follow-up work

- Parent will coordinate the required final `make check` after concurrent
  edits finish, then arrange independent `kreview` of these staged changes.
