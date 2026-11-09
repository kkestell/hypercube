# Worktree sessions start anywhere and are removed when no work would be lost

- Task ID: 2026-11-09-008-worktree-start-and-safe-removal
- Status: ready
- Source HEAD: 42ea40701594839684afb179b72ff886d7d72ccf
- Author: parent coordinator, Hypercube session 777ef702 (Claude Code, Opus 5.5)
- Dependencies: none. Phase 1 of 3; phase 2 (Review button starts a session at
  a pull request's head) and phase 3 (review model and prompt settings) are later
  tasks that build on the interface defined here.

## Outcome and boundaries

Managed worktree sessions stop assuming that their work must be merged into the
original checkout's branch. Three behaviors change, for every worktree session
alike:

1. **Start point.** A new top-level worktree session can start at any commit
   or ref that resolves locally, not only the checkout's `HEAD`. The default
   stays `HEAD`.
2. **Safe removal.** A worktree (top-level or child) is removable when nothing
   is uncommitted and every commit on its branch and in its checked-out `HEAD`
   is reachable from a kept ref: the checkout's `HEAD`, the session's starting
   commit (`Base`, top-level only), or any remote-tracking branch. This replaces
   "merged into HEAD".
3. **Restore on unarchive.** Archiving records the branch's tip. Unarchiving
   recreates the branch at that tip and keeps the original `Base`.

Non-goals: no session kinds or flags; no fetching of remote refs (phase 2 owns
fetching a pull request's head); no browser UI change; no change to the MCP
`create_session` tool or to how children start (children keep starting from
the parent's `HEAD`); no handling of squash-merged branches beyond the rule
above.

## Current system

- `workspace.AddWorktree(root, dir, branch)` (`internal/workspace/workspace.go`)
  runs `git worktree add -b branch dir HEAD`. Callers: top-level creation
  `Runtime.create` (`internal/session/sessions.go`), unarchive
  (`internal/session/removal.go` `Unarchive`), child creation
  (`internal/session/children.go`).
- `workspace.Removable(root, dir, branch)` requires the branch tip and the
  worktree's `HEAD` to be ancestors of `root`'s `HEAD`, and no uncommitted
  files. `RemoveWorktree` calls it, removes the worktree, and deletes the
  branch. Callers: `checkWorktrees`/`removeFamily` (archive and delete of a
  family, `removal.go`) and `EndChild` (`removal.go`).
- `store.Session.Base` (column `base`) is the commit a top-level worktree
  session started from; non-empty `Base` also marks a top-level worktree session
  (`snapshot.go`, `merges.go`, `lifecycle.go`, `child_options.go`,
  `report.go`, `changes.go`). `Unarchive` recreates the branch from `HEAD` and
  replaces `Base` with the new `HEAD`.
- HTTP creation: `sessionAction` case `"new"` (`internal/ui/actions.go`) passes
  `q.Get("worktree") == "1"` to `Runtime.CreateSubmission`.
- Schema: `const schema` in `internal/store/store.go` uses
  `CREATE TABLE IF NOT EXISTS`; there is no existing column migration.
  `docs/dev/development.md` requires migrations that preserve user state.

## Chosen design and decisions

### Start point

- `AddWorktree(root, dir, branch, start string)` resolves `start` in `root`
  with `git rev-parse --verify --end-of-options <start>^{commit}` and passes the
  resolved commit ID to `git worktree add -b branch dir <commit>`. Empty
  `start` means `HEAD`. A start that does not resolve returns an error naming
  it before any branch is created. Children and the restore fallback pass `""`.
- `Runtime.CreateSubmission(workspaceID, settings, worktree, start, submission,
  text, images...)` gains `start`. A non-empty `start` without `worktree` is an
  error ("a starting point needs a worktree session"). `Base` is the resolved
  commit (`workspace.Head(s.Dir)` after creation already yields it).
- HTTP: `sessionAction` `"new"` reads `q.Get("start")`. The browser does not
  send it yet; phase 2 will.

### Safe removal

`Removable(root, dir, branch, base string) error`. For each tip — the branch's
commit if the branch exists, and the worktree's `HEAD` if the worktree exists —
run in `root`:

```text
git rev-list --count <tip> --not HEAD [<base>] --remotes
```

A non-zero count refuses with an error that names the branch (or worktree) and
says its commits are not on the checkout's branch, its starting commit, or a
remote branch. `base` is omitted when empty (children, sessions created before
`Base` existed). Uncommitted-file refusal is unchanged. `RemoveWorktree` takes
the same `base` and passes it through; its `git branch -D` stays correct
because the check guarantees nothing is lost. Callers pass `s.Base` for
top-level sessions and `""` for children.

Rejected: keeping the branch on archive instead of recording its tip. It needs
no schema change but leaves a branch per archived session forever, and child
branches would accumulate the same way.

### Restore on unarchive

- New `store.Session.Tip` / column `tip TEXT NOT NULL DEFAULT ''`: the branch
  tip of a top-level worktree session when it was archived; empty otherwise.
  Add it to the schema and to every session insert/update/select in
  `store.go`. Migration in `Open`, after the schema runs: if `PRAGMA
  table_info(sessions)` lacks `tip`, `ALTER TABLE sessions ADD COLUMN tip TEXT
  NOT NULL DEFAULT ''`. Existing archived sessions keep an empty `Tip`.
- Archive (`removeFamily` with `archive` true, top-level worktree session):
  read the branch tip (`git rev-parse <branch>` in `root`) after the final
  `checkWorktrees` and before `RemoveWorktree`, and save it with the ended
  record in `recordRemovedFamily`. If the branch is already gone, save `""`.
  Delete clears nothing (the row is deleted).
- Unarchive: if `Tip` is non-empty and `git cat-file -e <tip>^{commit}`
  succeeds in the checkout, `AddWorktree(checkout, s.Dir, s.Branch, s.Tip)` and
  keep `s.Base`. Otherwise (empty `Tip`, or the commit was garbage-collected
  after a remote branch moved) fall back to today's behavior:
  `AddWorktree(..., "")` and set `Base` to the new `HEAD`. Either way clear
  `Tip` when saving the unarchived session. The fallback loses nothing: removal
  only allowed the archive because the commits were on a kept ref.

## Implementation sequence

1. `internal/workspace/workspace.go`: `AddWorktree` start parameter with
   resolution; `Removable` and `RemoveWorktree` `base` parameter and the
   reachability rule; a small helper to test that a commit exists. Update doc
   comments to describe the new rules.
2. `internal/store/store.go`: `Tip` field, schema column, migration, and the
   insert/update/scan lists.
3. `internal/session`: pass `start` through `CreateSubmission`/`create`;
   update `AddWorktree`/`Removable`/`RemoveWorktree` callers (`sessions.go`,
   `children.go`, `removal.go`); record `Tip` on archive and restore from it
   on unarchive.
4. `internal/ui/actions.go`: read `start`.
5. User docs: `docs/storage.md` (archive/unarchive table rows and the removal
   paragraph), `docs/lifecycle.md` (Archive/Unarchive paragraph), and
   `docs/usage.md` ("Archive or delete completed work") describe the new rule
   and restore behavior: work must be committed and either merged into the
   checkout's branch or pushed. Keep the wording declarative, per
   `docs/dev/documentation.md`.

## Verification

Tests in `internal/workspace/workspace_test.go` (real temporary Git repos, as
the existing tests do):

- `AddWorktree` with a commit other than `HEAD` checks out that commit on the
  new branch; an unresolvable start errors and creates no branch.
- `Removable` passes for: branch merged into `HEAD`; branch with no commits past
  `base` where `base` is on no ref; branch whose commits are on a
  remote-tracking ref (create a bare remote, push, so `refs/remotes/origin/*`
  exists). It refuses: a commit only on the branch; a detached worktree `HEAD`
  with an unreachable commit; uncommitted files.

Tests in `internal/session` (follow `lifecycle_test.go`/`sessions_test.go`
patterns):

- Creating a worktree session with `start` set to an older commit sets `Base`
  to that commit; `start` without `worktree` errors.
- Archive of a session with a commit pushed to a remote-tracking ref succeeds
  (it was refused before); the ended row has `Tip` equal to the branch tip; the
  branch is deleted. Unarchive recreates the branch at `Tip` with the original
  `Base` and an empty `Tip`.
- Unarchive with a `Tip` whose object is missing (use a fabricated ID) falls back
  to `HEAD`.

Store: a test that opens a database created with the old `sessions` table (no
`tip` column) and confirms the column is added and existing rows load
(`reopen_test.go` has the reopen pattern).

Checks: `make check` from the repository root.

## Handoff

Ready. One cohesive mechanism (worktree lifecycle Git rules) across the
workspace package, the session runtime, one schema column, and the HTTP
parameter, with focused Git-backed tests — a normal execution assignment.
Phase 2 relies on `CreateSubmission`'s `start` and on `start` resolving any
local ref, such as one it fetches for a pull request.
