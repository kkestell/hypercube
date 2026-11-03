# Top-level sessions in their own worktree

## Plan

`agents/plans/2026-11-03-005-worktree-sessions.md`

## Summary

A top-level session can run in its own worktree, chosen with the composer's
Worktree switch. The runtime measures its branch against the checkout after
each finished turn, at creation, and at restore; the composer's branch and
Merge button, the Changes tab, the sidebar branch icon, the Delete guard,
agent instructions, and docs follow the plan. The plan's goal is met. Review
fixes from `agents/reviews/2026-11-03-003-worktree-sessions.md` (F1–F3) are
applied with tests.

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
- The profile select's dropdown list takes the select's width, so a profile
  label longer than the selected one ends in an ellipsis in the list.

## Checks run

- `make ui` — regenerated `internal/ui/dist`.
- `make check` — passed (bun tests, go vet, gofmt, `go test -race ./...`, dist
  freshness).

## Follow-up work

- Worktree sessions are not exercised in a live `make dev` session; the user
  judges the switch, branch, Changes tab, and icon in the browser.
