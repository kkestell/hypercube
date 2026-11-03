# Top-level sessions in their own worktree

## Plan

`agents/plans/2026-11-03-005-worktree-sessions.md`

## Summary

A top-level session can run in its own worktree, chosen with the composer's
Worktree checkbox. The runtime measures its branch against the checkout after
each finished turn, at creation, and at restore; the merge bar, Merge button,
sidebar branch icon, Delete guard, agent instructions, and docs follow the
plan. The plan's goal is met.

## Departures from the plan

- The UI test for creating a worktree session is a new `TestNewWorktreeSession`
  instead of a case in `TestActions`. `TestActions` uses a handler with no
  profiles or plugins and a non-Git workspace, so it cannot create a session;
  the new test uses the `newApp` runtime with a Git workspace.
- README.md has no "Worktree isolation" bullet at this commit, so README.md is
  unchanged.

## Decisions

- The Delete refusal for a worktree session's own worktree is listed under the
  session's title alongside any child problems.
- The delete confirmation names the worktree by its directory and the branch
  from the merge status when present.
- `mergeSummary` includes the uncommitted-file count, so the bar renders
  `branch · summary`.

## Checks run

- `make ui` — regenerated `internal/ui/dist`.
- `make check` — passed (bun tests, go vet, gofmt, `go test -race ./...`, dist
  freshness).

## Follow-up work

- Worktree sessions are not exercised in a live `make dev` session; the user
  judges the checkbox, bar, and icon in the browser.
