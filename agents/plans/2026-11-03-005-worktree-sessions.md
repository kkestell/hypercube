# Top-level sessions in their own worktree

## Goal

A new top-level session can run in its own Git worktree. The composer's
Worktree switch, beside the Profile picker and off by default, chooses this
before the first prompt. The session gets a branch from the checkout's current
HEAD and a worktree under `.hypercube/`; the checkout's checked-out branch is
its merge target. Once created, the composer shows the branch beside the
profile, with a Merge button that asks the session's own agent to merge while
the branch has unmerged commits. The right panel's Changes tab shows the
branch's merge state and uncommitted file count, refreshed whenever a session
in the workspace finishes a turn. The sidebar marks worktree sessions with a
tinted branch icon after the title. Delete refuses while the worktree has
uncommitted files or unmerged commits, and otherwise removes the worktree,
branch, and session. Stop and Start behave as for any session. Children of a
worktree session branch from, and merge into, the parent's worktree, as they
already do for a checkout session.

This plan builds on the current working tree, which carries the uncommitted
explicit-stop work (`agents/plans/2026-11-03-004-explicit-stop-session-list.md`).
Leave that work's changes in the shared files as they are.

## Related code

- `internal/workspace/workspace.go` — `AddWorktree`, `WorktreeDir`,
  `ChildBranch`, `Removable`, `RemoveWorktree`, `isDirty`, `Branch`. Child
  worktrees already use these; top-level worktrees reuse them.
- `internal/session/children.go` — `CreateChild` and `EndChild` create and
  check child worktrees against `parent.Dir`, so a child of a worktree session
  already branches from the parent's worktree HEAD (its worktree nests at
  `<parent worktree>/.hypercube/<parent>/<name>`, which the shared
  `info/exclude` rule `/.hypercube/` hides) and merges into the parent's
  branch. No change.
- `internal/session/sessions.go` — `Create` (sets `s.Dir = w.Path`), `start`
  (overwrites `s.Branch` with the directory's current branch on every start),
  `Delete` (checks and removes child worktrees against `root = w.Path`).
- `internal/session/launch.go` — `live`, `decide`, and `run`, whose
  `client.Read` callback applies each record; `internal/session/restore.go`
  `Restore` and `attach`.
- `internal/session/runtime.go` — `Runtime` fields guarded by `mu`, `New`.
- `internal/session/event.go` — `eventLocked`; a turn ends with an event whose
  `State` is `harness.Idle`.
- `internal/store/store.go`, `internal/store/migrate.go` — `Session`, the
  `sessions` schema, `AddSession`, `sessionColumns`, `scanSession`,
  `addLaunchColumns` (the pattern for adding a column).
- `internal/ui/actions.go` — `sessionAction` `new` calls `Runtime.Create`.
- `internal/ui/snapshot.go`, `internal/ui/socket.go` — `shownWorkspace`,
  `shownSession`, `snapshot`; `sendState` rereads the snapshot when a
  `Change{}` marks it stale and sends `rt.Requests()` alongside.
- `internal/ui/web/agent/composer.jsx`, `profile-select.jsx`,
  `agent-pane.jsx` — the composer, its locked profile picker, and the pane
  that stacks transcript and composer.
- `internal/ui/web/sessions/selection.js` — `openUnsent`, `unsentProfile`;
  `sessions/session-actions.js` — `createSession`, `deleteSession`.
- `internal/ui/web/sidebar/session-item.jsx`, `session-item.css` — a session's
  row.
- `internal/ui/web/elements.js` — the vscode-elements imports.

## Decisions

- **One new stored field, `Session.Base`.** It is the commit a top-level
  session's worktree branch started from, and is empty for every other
  session. "No commits yet" and "Merged" both mean the branch is an ancestor of
  the target; only the start commit tells them apart, and Git keeps it only in
  the branch's expiring reflog. `Base != ""` is also how code recognizes a
  worktree session, so no separate flag is stored. `Base` never changes after
  creation, so `AddSession`, `sessionColumns`, and `scanSession` carry it and
  `SaveSession` does not. The column is `base TEXT NOT NULL DEFAULT ''`;
  `addBaseColumn` adds it to existing databases.
- **Naming.** The branch is `hypercube/session-<id>` and the worktree is
  `<checkout>/.hypercube/<id>`. A branch `hypercube/<id>` cannot coexist with
  the session's children's branches `hypercube/<id>/<name>` (Git stores refs
  as files), and `hypercube/<id>/<anything>` could collide with a child's
  name. `workspace.SessionBranch(id)` and `workspace.SessionDir(root, id)`
  sit beside `ChildBranch` and `WorktreeDir` so the workspace package keeps
  owning the `.hypercube/` layout and branch names.
- **Create makes the worktree before the session row.** `Create` reads the
  workspace, refuses a worktree in a `NoGit` workspace, calls `AddWorktree`,
  reads `Base` with `workspace.Head` from the new worktree (the checkout's
  HEAD can move during setup), then inserts the row. A failure after
  `AddWorktree` succeeded removes the worktree with `RemoveWorktree` (a fresh
  worktree is clean and merged, so it passes). This way no session row exists
  without its worktree, and the sidebar never shows a half-made session.
- **`start` keeps a worktree session's branch.** `start` refreshes `s.Branch`
  to report the checkout's current branch to the agent. A worktree session's
  `Branch` is fixed at creation and is what `Delete` removes, so `start`
  skips the refresh when `s.Base != ""`.
- **Merge status lives in memory in the Runtime.** It is derived from Git, so
  it is not stored. `Runtime.merges map[string]workspace.Merge` (by session
  ID, guarded by `mu`) is filled by `refreshMerges(workspaceID)`, which reads
  the workspace's top-level sessions with `Base != ""`, runs
  `workspace.MergeStatus` for each without holding `mu`, stores the results
  under `mu`, and publishes `Change{}` so pages reread the snapshot. Each
  launch has its own reader goroutine, so `refreshMu` (a `sync.Mutex` on
  `Runtime`) is held for the whole read-and-publish; an older refresh can then
  never overwrite a newer one. `mu` stays held only to store the results.
  `Runtime.Merges()` returns a copy, as `Requests()` does.
  `refreshMerges` runs:
  - in `run`'s `client.Read` callback, after `apply` succeeds for an event
    whose `State` is `harness.Idle` (a turn finished in any session of the
    workspace). The reader goroutine owns the call, so no goroutine is added;
    the git commands delay reading that agent's next record only while it is
    idle. `live` gains a `workspace` field, set in `decide` and `attach`, so
    the callback needs no database read.
  - in `Create`, after the row is saved, so the status appears at once;
  - at the end of `Restore`, once per workspace, so status shows after the
    app starts.
  A `MergeStatus` failure for one session is stored as that session's entry,
  `Merge{Branch: s.Branch, Error: err.Error()}`, and shown in the Changes tab.
  `refreshMerges` returns nothing because it runs inside the reader, where a
  returned error would end the agent's launch; the error reaches the user
  through the snapshot instead. Entries for deleted sessions are not removed; the snapshot reads only
  existing sessions.
- **`workspace.MergeStatus(root, dir, branch, base string) (Merge, error)`**
  measures the branch against the checkout at `root`:
  ```go
  // Merge is a worktree branch measured against the branch checked out in
  // its checkout.
  type Merge struct {
  	Branch      string `json:"branch"`
  	Target      string `json:"target"`      // the checkout's branch
  	Unmerged    int    `json:"unmerged"`    // branch commits not in the target
  	Committed   bool   `json:"committed"`   // the branch has moved past its base
  	Uncommitted int    `json:"uncommitted"` // changed and untracked files in the worktree
  	// Error is why the status could not be read; the other fields but
  	// Branch are then empty. MergeStatus leaves it empty.
  	Error string `json:"error,omitempty"`
  }
  ```
  `Target` is `Branch(root)`; `Unmerged` is `git rev-list --count
  HEAD..<branch>` in `root`; `Committed` is `git rev-parse <branch>` differing
  from `base`; `Uncommitted` counts `git status --porcelain
  --untracked-files=all` lines in `dir`, so each file in a new folder counts.
  `isDirty` becomes `uncommitted(dir) (int, error)`, and `Removable` tests
  `n > 0`, so both share one status reader. `MergeStatus` serves display only;
  removal stays with `Removable`. The JSON tags let the snapshot
  send `Merge` as is, as `session.Request` and `session.FileOpen` are sent.
- **Delete.** For a top-level session with `Base != ""` and its checkout
  present, `Delete` runs `workspace.Removable(w.Path, s.Dir, s.Branch)` and
  refuses with its error before changing anything, alongside the existing
  child checks. `Removable` already checks the branch and the worktree's
  checked-out commit and passes a worktree or branch that no longer exists. Children are now checked and removed against `s.Dir`
  instead of `w.Path`; for a checkout session these are equal, and for a
  worktree session it matches `CreateChild` and `EndChild`. When a worktree
  session's directory no longer exists, children are checked and removed
  against `w.Path`, which still holds their branches. After the child
  worktrees go (they nest inside the parent's), `RemoveWorktree(w.Path, s.Dir,
  s.Branch)` removes the session's worktree and branch, and only then is the
  row deleted, so a failed removal leaves the session for a retry, as for
  children. The `present` check stays on `w.Path`.
- **Stop, Start, and after merging** need no change. Stop leaves the worktree;
  Start relaunches in `s.Dir`; new commits make the status unmerged again on
  the next refresh.
- **Snapshot fields.** `shownWorkspace.NoGit` (`noGit`) hides the switch.
  `shownSession.Worktree` (`worktree`, from `Base != ""`) drives the sidebar
  icon even when status is missing.
  `shownSession.Merge *workspace.Merge` (`merge`, omitted when absent) drives
  the composer's branch, the Changes tab, and the icon's tint. `snapshot(db, merges)` takes `rt.Merges()`
  from `sendState`.
- **The switch** (`WorktreeToggle`, `agent/worktree-toggle.jsx`) is a
  `vscode-checkbox` with `toggle`, labeled "Worktree", in `composer-details`
  after `ProfileSelect`. It renders only for the unsent session in a workspace
  whose `noGit` is false, reflects `state.unsent.worktree` (`false` from
  `openUnsent`), and disables while sending. `createSession` posts
  `worktree=1` when on; `sessionAction` passes `q.Get("worktree") == "1"` to
  `Create`.
- **The branch** (`SessionBranch`, `agent/session-branch.jsx`) takes the
  switch's place for an existing session with `session.merge`: a
  `git-branch` icon and the branch name, then a Merge button shown only when
  `unmerged > 0` and there is no error, disabled while the session is not
  running. Merge posts the session's `input` action with the text "Commit any
  remaining work, then merge your branch into <target> in the checkout at
  <workspace path>, resolving any conflicts, and report the result."
  Sessions in the checkout show nothing there.
- **Profile select width.** `.composer-profile` sets `width: max-content`
  within its `max-width`, so the select is as wide as its selected label.
- **The Changes tab** (`ChangesPanel`, `changes/changes-panel.jsx`) shows
  plain lines for the shown session's `merge`: the branch, the status phrase,
  and the uncommitted file count; with an error, the branch and "Merge status
  unavailable: <error>". It is empty without `merge`. `sessions/merge.js`
  holds `mergeSummary(merge)` ("N commits not merged into T", "Merged into
  T", "No commits yet", or the unavailable phrase), `uncommittedSummary`, and
  `mergeTint(merge)` (`null` with an error, `'modified'` when `unmerged > 0 ||
  uncommitted > 0`, `'merged'` when `committed` and neither, else `null`), so
  the panel and the sidebar derive state one way.
- **Sidebar icon.** `SessionItem` puts `<vscode-icon name="git-branch"
  size="14">`, slightly smaller than the row's other icons, with `title` set
  to the branch after the title text when `session.worktree`.
  Its class adds `session-item-branch-modified`
  (`--vscode-list-warningForeground`) or `session-item-branch-merged`
  (`--vscode-disabledForeground`) per `mergeTint`; otherwise it inherits the
  row's color. Both variables are already in `dist/theme.css`.
- **Delete's confirmation** for a worktree session also names its worktree
  and branch.
- **Agent instructions.** `report.Instructions` (`internal/report/report.go`)
  tells every top-level agent that it works in the original checkout and shares
  it with other top-level sessions. For a worktree session it instead names
  its own worktree and branch, says its child sessions merge into that
  branch, and says its own work reaches the checkout's branch when the user
  asks it to merge. Sharing is stated only for sessions in the checkout.
- No active session needs a restart: existing sessions are not worktree
  sessions, and nothing changes in the plugins, the holder, or the MCP server.

## Implementation plan

1. `internal/workspace/workspace.go`: add `SessionBranch`, `SessionDir`,
   `Head(dir)` (`git rev-parse HEAD`), `Merge`, `MergeStatus`; replace `isDirty` with `uncommitted` and update
   `Removable`. Update the package comment to cover session worktrees.
2. `internal/store/store.go`: add `Session.Base`, the `base` column, and
   carry it in `AddSession`, `sessionColumns`, and `scanSession`.
   `internal/store/migrate.go`: add `addBaseColumn` (same shape as
   `addLaunchColumns`) and call it in the non-legacy branch after
   `addLaunchColumns`, independent of that function's early return.
3. `internal/session/runtime.go`: add `merges` to `Runtime` (under `mu`) and
   `refreshMu`, and initialize `merges` in `New`. Add `refreshMerges` and `Merges` in a new
   `internal/session/merges.go`.
4. `internal/session/launch.go`: add `live.workspace`, set in `decide`; set it
   in `attach` in `restore.go`. In `run`, after `apply` returns nil for an
   idle event, call `r.refreshMerges(l.workspace)`. At the end of `Restore`,
   call `refreshMerges` for each workspace.
5. `internal/session/sessions.go`: `Create(workspaceID, profileName string,
   worktree bool, text string, images ...harness.Image)` as decided; `start`
   skips the branch refresh when `Base != ""`; `Delete` as decided, and its
   doc comment covers the session's own worktree. Update the 10 test callers
   of `Create` and `fixture.session` to pass `false`.
5a. `internal/report/report.go`: the worktree session's instructions as
   decided, with the report's tests.
6. `internal/ui/actions.go`: pass `worktree` to `Create`.
   `internal/ui/snapshot.go`: add `NoGit`, `Worktree`, `Merge`; `snapshot`
   takes the merges map. `internal/ui/socket.go`: pass `d.rt.Merges()`.
7. `internal/ui/web/elements.js`: import `vscode-checkbox`.
   `agent/composer.css`: the profile select's width.
8. `internal/ui/web/sessions/selection.js`: `openUnsent` sets
   `worktree: false`; add `unsentWorktree(state, worktree)`.
   `sessions/session-actions.js`: `createSession` sends `worktree`;
   add `requestMerge(store, session, target, path)`; `deleteSession`'s
   detail names the worktree and branch for a worktree session.
9. `internal/ui/web/sessions/merge.js` (new): `mergeSummary`,
   `uncommittedSummary`, `mergeTint`.
10. `internal/ui/web/agent/worktree-toggle.jsx` and `session-branch.jsx`
    (new) and their use in `composer.jsx`; update the composer's opening
    comment.
11. `internal/ui/web/changes/changes-panel.jsx` and `changes-panel.css` (new),
    rendered in the Changes tab of `app/right-panel.jsx`.
12. `internal/ui/web/sidebar/session-item.jsx` and `session-item.css`: the
    branch icon and its tint classes; update the opening comment.
13. Run the build so `internal/ui/dist` is regenerated.

## Test plan

- `internal/workspace/workspace_test.go` `TestMergeStatus`, a table over one
  session worktree made with `AddWorktree`: fresh (not committed, 0
  unmerged, target `main`); after a commit in the worktree (committed, 1
  unmerged); with an untracked file (1 uncommitted); with three files in a
  new folder (4 uncommitted); after merging the branch
  in the checkout (committed, 0 unmerged).
- `internal/session/sessions_test.go`:
  - `TestWorktreeSession`: `Create` with a worktree puts `Dir` at
    `SessionDir`, creates `SessionBranch`, sets `Base` to the worktree's HEAD,
    and `Merges()` reports no commits. A commit in the worktree followed by a
    prompt that ends a turn makes `Merges()` report 1 unmerged. `Delete`
    refuses with `Removable`'s error and leaves the worktree and
    row. Stop then Start keeps the worktree, `Dir`, and `Branch`. After a
    merge in the checkout, `Merges()` reports merged once a turn ends, and
    `Delete` removes the worktree, branch, and session.
  - `Create` with a worktree in a non-Git workspace is refused before any row
    is written.
  - With a worktree session's directory removed, a turn ending in another
    session of the workspace leaves that session's `Merges()` entry with its
    branch and a nonempty `Error`, and the other session keeps running.
  - A child of a worktree session gets its worktree inside the parent's
    worktree, starting from the parent's HEAD; with the child's commit merged
    into the parent's branch and the parent's branch merged into `main`,
    `Delete` removes both worktrees and branches.
  - A stopped worktree session whose directory was removed, with a child,
    deletes with its child's and its own branches.
- `internal/store/migrate_test.go`: a database whose `sessions` table lacks
  only `base` opens with `Base` empty and stays openable twice; the older
  upgrade test also drops `base`. `internal/store/store_test.go`: `Base`
  round-trips through `AddSession` and `Session`.
- `internal/ui/ui_test.go`: `TestNewWorktreeSession` creates a session with
  `worktree=1`; the state message shows it with `worktree: true` and a
  `merge`, the workspace with `noGit: false`.
- `internal/ui/web/agent/composer.test.jsx`: the unsent composer shows a
  Worktree switch, off, in a Git workspace and none in a non-Git workspace;
  turning it on before sending posts `worktree=1`; it disables while sending.
  An existing worktree session shows its branch and no switch; checkout
  sessions and children show neither. Merge appears only with unmerged
  commits and no error, disables while stopped, and posts the exact message
  with the target and workspace path.
- `internal/ui/web/app/right-panel.test.jsx`: Changes shows the branch, each
  status phrase, and the uncommitted file count; with an error, "Merge status
  unavailable: <error>"; nothing for a checkout session.
- `internal/ui/web/sidebar/sidebar.test.jsx`: a worktree session's row shows
  the branch icon tinted modified while unmerged or uncommitted and merged
  once merged; a checkout session's row shows no icon.

## Documentation updates

- `docs/usage.md`: the new-session composer's Worktree switch; the branch and
  Merge button beside the profile; the Changes tab; the sidebar branch icon; Delete's guard for worktree
  sessions and that Stop keeps the worktree; children of a worktree session
  merge into the parent's branch ("merged into the original checkout's HEAD"
  becomes the parent's checkout or worktree).
- `docs/configuration.md` "Files needed by child worktrees": covers every
  managed worktree, which starts at the HEAD of the checkout or worktree it
  branches from.
- `docs/introduction.md`: top-level sessions share the checkout only when
  they run in it; children merge into their parent's branch, in the checkout
  or the parent's worktree.
- `docs/architecture.md`: every top-level session shares the checkout only
  when it runs there; worktrees, not only child worktrees, live inside the
  checkout; session IDs qualify their paths and branches.
- `README.md` "Worktree isolation": top-level sessions can opt in too.
- `agents/roadmap.md`: delete "Sessions can run in a worktree".
