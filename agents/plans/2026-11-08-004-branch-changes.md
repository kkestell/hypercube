# Complete Branch changes

## Goal

Replace the approved Git panel's Branch changes placeholder with the net file
changes from the branch's merge-base with its workspace checkout to its current
working files, including committed, staged, unstaged, and untracked work. Rows,
statuses, line counts, and opened diffs must describe the same comparison.
Keep Uncommitted and Files behavior, the thin directory/current-branch bar, and
the merge control's approved placement in Branch changes. Do not change merge
actions, session placement, saved Git history, or documentation.

## Related code

- `internal/workspace/changes.go` — `Changes` combines porcelain untracked
  records with HEAD-to-working-tree raw/numstat output; `Diff` supplies literal,
  whole-file patches with size bounds and untracked-file support.
- `internal/workspace/workspace.go` — `MergeStatus` compares every managed
  worktree session against its workspace checkout's current HEAD/branch.
  `Session.Base` records the starting commit for merge-state bookkeeping; it
  is not the comparison base for this feature. `internal/session/merges.go`
  supplies `Workspace.Path` as the checkout for children as well as parents.
- `internal/ui/changes.go` — changes/diff routes, untracked line counting,
  missing-file handling, and binary/size responses. `files.go` resolves session
  and workspace file targets and validates local diff paths.
- `internal/ui/web/changes/{changes.ts,types.ts,git-panel.tsx,diff-view.tsx}` —
  changes requests and cache, placeholder panel, and diff rendering.
- `internal/ui/web/editor/{editor.ts,types.ts,attachment.ts,view-state.ts}` —
  tab identity, preview replacement, cached reads, and restoration of saved
  tabs. Diff caches currently use only the path, which cannot distinguish the
  two comparisons.
- `internal/ui/web/files/file-polling.ts` — one polling owner for Files, Git,
  the chat branch bar, and active file/diff panes.

## Decisions

### Comparison and HTTP contract

- Use the actual session directory's `HEAD` and the workspace checkout's
  current `HEAD`: resolve the checkout HEAD to a commit ID, then run
  `git merge-base HEAD <checkout-head-id>` in the session directory. Use that
  merge-base for both the file list and patch. Do not use saved `Session.Base`,
  the parent's branch, an upstream branch, or a guessed default branch.
- Resolve this on each request. After checkout branch movement, commits,
  rebases, or merges, the next poll uses the new merge-base. This is a live
  comparison, not a permanent historical fork point. No database changes or
  durable Git state are needed.
- Extend the existing `changes` and `diff` endpoints with
  `comparison=uncommitted|branch`. Omitted means `uncommitted`; reject other
  nonempty values with 400. Clients do not supply revisions or checkout paths.
  Resolve branch comparison from the requested stored session and its stored
  workspace. It is available only for sessions with nonempty `Base`, matching
  the existing `worktree` UI flag. Workspace targets and ordinary shared-folder
  sessions receive 409 for branch comparison; their existing default requests
  are unchanged.
- Keep the existing `{branch, files}` response. Add optional `target` only to
  successful branch results, naming the workspace checkout's actual branch.
  A detached checkout still compares against its HEAD and uses `detached HEAD`
  as its target label. `branch` remains the actual session branch, empty when
  detached. Missing directories keep the current 404 handling. Git failures,
  unavailable HEADs, and no common ancestor are visible errors, never an empty
  successful comparison or a fallback to HEAD/empty-tree. Unborn repositories
  retain existing Uncommitted behavior.
- Keep path scope relative to the session directory, with the existing
  containing-root and symlink handling. Keep literal diff path validation,
  read-only Git flags, no rename detection, size limits, and existing
  unsupported-file responses. No new routes or browser timers.

### Net changes and matching patches

- Add `BranchChanges(dir, top, checkout string) (ChangesResult, error)` and
  `BranchDiff(dir, checkout, path string, limit int) (string, error)` in
  `internal/workspace/changes.go`. Keep `Changes` and `Diff` as the existing
  Uncommitted entry points. Factor the shared raw/numstat parser and Git patch
  invocation within this file rather than copying them or growing a general
  revision framework. Resolve one merge-base per entry-point call.
- Compare the merge-base tree to current working files, not just to HEAD and
  not a concatenation of commit and uncommitted lists. A reverted change
  disappears; a branch-added file edited again remains added; a branch-added
  file deleted again disappears. Renames remain deletion/addition pairs.
  Include ignored-excluding untracked files and keep the existing untracked
  status/counting behavior for paths absent from the base. Each path appears
  once. Binary counts are absent; empty and mode-only changes remain rows.
- Account locally for a concrete Git limitation: if the branch committed a
  deletion and the file is recreated untracked, ordinary `git diff <base>`
  still reports deletion. Find overlaps between untracked paths and base diff
  paths. For these paths only, create a temporary alternate index, initialize
  it with `git read-tree <base>`, and rerun the same base-to-working-file diff
  with `GIT_INDEX_FILE` set to it. Replace the ordinary deletion with the net
  result and do not append a second untracked row. Identical recreation omits
  the path; changed recreation is modified, with counts against the old file.
  This also preserves Git's symlink/mode/binary handling without writing blob
  comparison code. Use an absent index path inside an owned temporary folder;
  remove it on return. Never refresh or write the real index or objects.
- Branch patches use that same overlap treatment when the requested path is
  currently untracked but exists in the base. All other branch tracked paths
  use the ordinary base patch; untracked paths absent from the base use the
  existing `/dev/null` patch path. Keep this alternate-index behavior out of
  Uncommitted. Reuse the HTTP route's `untrackedLines` for ordinary untracked
  rows, including disappearing-file filtering, nested repositories, symlinks,
  binary files, and large files.

### Browser ownership and tab identity

- Introduce `ChangeComparison = 'uncommitted' | 'branch'`. Keep
  `editor.changes` exclusively Uncommitted, so Files status colors, deleted
  rows, context-menu Open Changes, and the current-branch bar do not acquire
  branch-wide status. Add `editor.branchChanges: Changes` and
  `editor.gitComparison: ChangeComparison`, initially null and uncommitted
  respectively. These are transient editor state; do not expand saved view
  storage to persist a Git subtab or server results.
- Add `branchChangesVersion` alongside `changesVersion` in `state.ts`.
  `loadChanges(store, comparison = 'uncommitted')` selects its own result field
  and request generation. Requests for different comparisons must not cancel
  or overwrite each other. Retain editor ID, attachment version, and newest
  request checks, sorted paths, and current error/missing representations.
- Preserve `diff:<path>` for existing Uncommitted tab IDs and saved views;
  use `branch-diff:<path>` for branch diffs. Both preserve the full literal
  path after the first colon. Cache open diffs and diff request generations by
  full tab ID for both kinds, not path or an ambiguous concatenated cache key.
  This avoids collisions even for filenames containing colons. Put identity
  helpers beside changes behavior in `changes.ts`; `editor.ts` can re-export
  its existing public helpers to preserve caller imports without a new cycle.
- Extend `diffTabId(path, comparison = 'uncommitted')`, `isDiffTab`, and a
  `diffComparison(id)` helper. `openDiffTab` and `readOpenDiff` accept path
  plus optional comparison. Update opening, selecting, replacing previews,
  closing, tab entry creation, DiffView reads, and restoration to use full IDs
  for cache access. `tabPath` still removes only the first prefix.
- Titles must explicitly distinguish `<name> (Uncommitted)` from
  `<name> (Branch changes)`, retaining useful non-modified status text after
  the comparison label. Hover titles include the full relative path and the
  comparison. Resolve statuses from the corresponding result, never from
  the other comparison. Saved `diff:` tabs continue to restore as Uncommitted;
  recognize both prefixes when rebuilding unread diff records. Closing one
  comparison cannot discard the other's patch or scroll.

### Panel and polling

- Make GitPanel's existing Tabs controlled by `editor.gitComparison`. Keep
  Branch changes disabled for non-worktree targets. Do not hide the entire
  Tabs component just because Uncommitted has not loaded or has failed: a
  branch result/error must remain independently visible. Share the file-row
  rendering inside `git-panel.tsx`, passing comparison to `openDiffTab`.
  Preserve native buttons, accessible path/status/count labels, count omission,
  and deleted-file presentation. Remove the prototype explanation and preview
  warning. An empty branch comparison says `No branch changes.`
- Keep the comparison target line and MergeButton in Branch changes only.
  On a successful branch response use its `target` for the comparison line,
  not the independently refreshed merge snapshot. Leave MergeButton's existing
  availability and action semantics unchanged. Do not repeat the current branch
  already shown in the thin bar.
- Load the selected comparison immediately on visible Git activation, target
  attachment, and comparison selection. Retain cached rows during a pending
  refresh. Switching tabs does not allow a late response to fill the wrong list.
- Extend the existing poller to collect the comparisons needed for this pass:
  Uncommitted for visible Files or the chat branch bar; the selected comparison
  for visible Git; each active diff pane's comparison even when Git is hidden.
  Deduplicate list reads per comparison, then read each active diff by its own
  comparison/path. Preserve tree reads only for Files, active ordinary file
  reads, hidden-page suppression, serial requests, skipped overlapping polls,
  and attachment-change checks between awaits. Do not poll branch changes only
  because a worktree chat exists or because an inactive diff tab is open.

## Implementation plan

1. Modify `internal/workspace/changes.go` for the branch entry points, optional
   target field, common list/patch helpers, and branch-only overlap comparison.
   Extend `internal/workspace/changes_test.go` at the real-Git boundary.
2. Modify `internal/ui/changes.go` to validate the comparison, resolve the
   session's workspace checkout from the database for branch requests, and
   dispatch both routes. Keep common untracked counting and file errors in one
   path. Extend `internal/ui/changes_test.go` for request/owner/error behavior.
3. Modify `internal/ui/web/changes/types.ts`, `api.ts`, `state.ts`,
   `editor/types.ts`, and `editor/attachment.ts` for the optional target,
   comparison requests, independent branch result/generation, and editor defaults.
   Keep the existing file-tree cache Uncommitted-only.
4. Modify `changes/changes.ts`, `editor/editor.ts`, `editor/view-state.ts`,
   and `changes/diff-view.tsx` for separate loads, full-ID caches, both tab
   prefixes, comparison-specific titles/status, and matching diff reads.
   DiffView already receives the tab ID, so its parent need not gain another
   prop. Update affected fixtures/assertions in `editor/editor.test.ts`,
   `editor/shown-tabs.test.tsx`, `files/file-view.test.tsx`, and
   `app/view-storage.test.ts`; add restore coverage for both diff kinds.
5. Modify `changes/git-panel.tsx` and `files/file-polling.ts` for real branch
   rows and comparison-aware refresh. Update `changes/git-panel.test.tsx` and
   `files/file-polling.test.ts`. Replace the explicit placeholder assertion;
   retain merge placement/action tests and existing Files tests.

## Test plan

- In real Git repositories create a common commit, a managed feature worktree,
  feature commits, and independent checkout commits. Verify the checkout-only
  edits are not feature changes: comparison starts at merge-base, not the
  checkout tip. Test branch advancement and checkout branch switching on
  subsequent requests; a child uses its workspace checkout, not its parent.
- Cover committed and subsequently staged/unstaged edits to the same file,
  additions, deletions, untracked text, binary, symlink, empty/mode-only files,
  rename-as-delete/add, and net reversions. Assert exact unique paths/statuses
  and meaningful line counts, with corresponding patches against the same
  base, including literal special-character paths and directory-relative scope.
- Exercise committed deletion followed by identical and changed untracked
  recreation: no duplicate rows, identical content omitted, changed content
  modified with correct old/new patch and counts. Extend the existing
  index-unchanged test to branch reads, including the alternate-index case.
- HTTP tests verify server-owned target resolution despite request parameters,
  default Uncommitted compatibility, invalid comparison 400, unsupported target
  409, missing directory 404, and visible no-common-ancestor/Git failures.
  Preserve binary 415, size 413, invalid path 400, and untracked count behavior.
- Component tests show independent list success/errors and empty states,
  immediate selection refresh, sorted rows/counts, no preview copy, and correct
  target line. Row activation requests `comparison=branch` and opens the
  matching diff; Uncommitted retains its original request and Files semantics.
- Keep both comparisons for one path open in different panes, with distinct
  titles, text, scroll, close/preview behavior, and restored unread records.
  Include a colon in a path to catch identity collisions. An older or failed
  read cannot overwrite another comparison or a newer attachment; retain the
  existing rule that transient failed reads keep already displayed diff text.
- Polling tests verify each required comparison is loaded once per pass,
  both active diffs request matching comparisons, branch lists are not polled
  when unneeded, chat/Files still use Uncommitted, and target switches during
  an await stop the old pass. Preserve hidden-page and overlapping-poll tests.
