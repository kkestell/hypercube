# Git tab in the right panel

## Goal

Add a **Git** tab to the right panel that lists changed files with status
colors and right-aligned added and removed line counts. Selecting a row,
including its counts, opens the existing diff view. Remove inline counts and
their diff buttons from **Files**, keeping its status colors and ordinary
file browsing.

## Related code

- `internal/ui/web/app/right-panel.tsx` and `shared/panel-tabs.tsx` — the
  right-panel tab definitions and mounted panel contents.
- `internal/ui/web/app/types.ts` and `app/saved-views.ts` — the right-panel
  selection and validation of saved choices.
- `internal/ui/web/changes/changes.ts` and `changes/types.ts` — shared Git
  results, path sorting, status labels, and protection against stale reads.
- `internal/ui/web/files/files-panel.tsx` — current status colors, inline
  counts, and Files activation refresh.
- `internal/ui/web/files/file-polling.ts` — refreshes visible file tools and
  active editor files and diffs.
- `internal/ui/web/editor/editor.ts` — `openDiffTab` already opens and reads
  a diff for the attached session or workspace.
- `internal/workspace/changes.go` — existing backend supplies uncommitted
  paths, statuses, and optional counts within the target directory. No
  backend or protocol changes are needed.

## Decisions

- Add `git` to `RightTab`, with the title **Git** and existing
  `source-control` icon. Keep Files first and the default; use the order
  Files, Git, Search.
- Use a flat list of paths relative to the current view's directory, in the
  order already supplied by `loadChanges`. Show the full relative path in
  each row, truncated visually with a full-path hover title. Do not add
  staged/unstaged sections, repository controls, or another changes cache.
- One native button per row opens `openDiffTab` from its label or counts,
  with normal keyboard focus and activation. Render counts as nonshrinking,
  right-aligned `+N −N` text; include zero counts when supplied. When counts
  are absent, omit them rather than inventing zeros; the row still opens its
  diff. Preserve the existing backend treatment of untracked and binary files.
- Move the existing status-color map into `changes/changes.ts`, beside
  `statusLabel`, for both panels to use. Keep the existing colors, deleted
  strike-through, accessible status labels, and neutral count styling.
- Remove only the Files tree's inline count buttons. Preserve its existing
  deleted-file row activation and the shared context-menu **Open Changes**
  action, as well as normal file preview and double-click behavior.
- Read `state.editor.changes` directly in the new panel, including the
  existing error and missing-directory results. With no attached editor,
  show a selection prompt; with null changes, use a neutral no-data message
  rather than claiming whether the directory is outside Git or still loading.
  A successful empty result says there are no uncommitted changes. No new
  loading state or request lifecycle is needed.
- Refresh changes on Git activation or target change, just as Files does
  today. Extend the existing poller to refresh changes while either Files or
  Git is visible, but list folders only for Files. Preserve polling of active
  diffs when neither panel is visible. Reuse existing attachment/request
  version checks and the existing polling interval.
- Accept `git` in saved-view validation; existing Files/Search choices need
  no migration. Keep panel contents mounted through `PanelTabs` and key the
  Git panel by editor ID like the other panels.

## Implementation plan

1. In `internal/ui/web/changes/changes.ts`, export the existing status-color
   map. Update `internal/ui/web/files/files-panel.tsx` to use it and remove
   the count-button markup, leaving file navigation and status presentation
   intact. Update the changes-state comment in `changes/types.ts` to describe
   both consumers.
2. Create `internal/ui/web/changes/git-panel.tsx`. Render the current changes
   as a semantic list of full-width buttons with file icons, status-colored
   paths, accessible status/count descriptions, and right-aligned counts.
   Use `openDiffTab` for activation and `loadChanges` for visible activation
   refresh. Render the no-selection, no-data, clean, missing-directory, and
   error messages described above.
3. Register the panel and tab in `internal/ui/web/app/right-panel.tsx`, extend
   `RightTab` in `app/types.ts`, and accept the value in `app/saved-views.ts`.
   Update the right-panel description to include Git.
4. Extend `internal/ui/web/files/file-polling.ts` so Git visibility requests
   changes without requesting folder listings. Keep active file/diff reads,
   hidden-page behavior, and target-change protection unchanged.
5. Add `internal/ui/web/changes/git-panel.test.tsx` and update
   `files/files-panel.test.tsx`, `files/file-polling.test.ts`,
   `app/app.test.tsx`, and `app/view-storage.test.ts` for the behaviors below.
   Move the existing workspace-dashboard count-click assertion to the Git
   tab rather than retaining an obsolete Files control.
6. Update `docs/review-changes.md` and `docs/interface.md` as described below.

## Test plan

- Git shows modified, added, deleted, and untracked paths, accessible status
  labels, supplied positive and zero counts, and no fabricated counts for
  files without them. Check shared status styling where preserving color is
  the behavior being protected.
- Clicking a Git row or its count text opens the matching existing diff tab.
  Rows are native focusable buttons with an accessible path and status.
- Git activation loads changes immediately, uses cached changes while a read
  is pending, and renders clean, no-selection/no-data, missing-directory, and
  error results without stale rows.
- Files no longer contains inline counts or count diff buttons, but retains
  file/folder status colors, ordinary file opening, and deleted-file behavior.
- A poll with Git visible loads changes but not `/files/tree`; hidden Git
  does not cause extra polling. Existing Files and active-diff polling stays
  intact, including hidden-page and target-switch handling.
- The right-panel Git tab selects and shows its panel. The workspace
  dashboard integration opens a Git diff using `workspace`, not `session`,
  while a selected session uses its own target.
- Saving and restoring a layout with Git selected preserves the choice
  without dropping session/workspace views. Existing Files/Search saved
  selections continue to restore.

## Documentation updates

- `docs/review-changes.md` — direct diff review through **Git** in the right
  panel, describe the right-aligned counts and retained Files colors, and
  state that the list covers the current view's directory, not the whole
  containing repository. Include workspace-dashboard scope alongside sessions.
- `docs/interface.md` — identify the Git panel and its scope/count behavior
  in the existing file-tools reference, correcting the same repository-scope
  claim. No new guides or screenshots.
