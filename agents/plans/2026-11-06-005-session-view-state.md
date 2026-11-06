# Preserve session view state

## Goal

Resolve #1: retain session tabs, open files and diffs, editor split ratios,
expanded transcript and file-tree items, and scroll positions across selection
changes and browser/app restarts. Retain the last selection, sidebar state,
global panel visibility, and the sidebar/right/terminal panel sizes.

## Decisions

- Use browser-local storage. Session view choices belong to each session;
  overall panel sizes and sidebar choices belong to the workbench.
- Keep the existing attached session as the active owner. A `sessionViews` map
  stores only inactive sessions' view snapshots, moving a view into and out of
  that map on attachment changes. Persist a snapshot of the active view too.
  Do not retain transcripts, file contents, Git results, DOM handles, drag
  state, or in-flight requests in saved views.
- Store tabs, transcript expansion, scroll memory, expanded folders, open-file
  source/highlight choices, and active editor pane in a session view. Move
  activePane from Workbench into AttachedSession because pane IDs are local to
  that session. Recreate empty file/diff reads when restoring tabs; existing
  polling refreshes contents and expanded directories.
- One view-storage subscriber replaces the wrap/fixed-width-only subscriber.
  Restore defaults from those existing keys when no new snapshot exists; remove
  legacy keys only after successfully saving their values. Keep draft storage
  and its format unchanged. Honor an explicit selected-session URL.
- Save scroll on real scroll/pagehide/unmount events, including late cleanup
  into an inactive view. Restore only when transcript/file/diff data is ready,
  preserving whether the transcript follows its end. Use existing layout
  effects and content readiness; add no render timers or hidden DOM cache.
- Move sidebar tab and collapsed workspace/family IDs to Workbench. Explicit
  selection actions reveal the appropriate tab; mounting the sidebar must not
  overwrite a restored manual sidebar choice. Keep focus/menu/rename handles
  component-local. Panel size callbacks report pixels for fixed panels and
  percentages for editor splits, only after user interactions.
- Validate the saved JSON shape at the storage boundary. On storage failure,
  retain usable in-memory state and show an error. Prune snapshots only for
  sessions/workspaces removed from the server's complete state.

## Implementation plan

1. Add session view snapshot/restore types and pure functions, attach/detach
   transitions, session-owned active pane, and removal pruning. Keep refreshed
   server data in the existing active session only.
2. Add browser view storage and validation, replace the old workbench subscriber
   in main.tsx, and migrate existing wrap/fixedWidth keys without changing drafts.
3. Persist sidebar choices and workbench pixel sizes through the current store;
   update Split user-resize callbacks and remove replaced component-local state.
   This bounded portion may be delegated to the UI agent after plan review.
4. Restore content-dependent scroll, save late unmounts/pagehide correctly, and
   reload expanded folders using existing file polling. Include file-tree and
   sidebar scrolling in their respective view owners.
5. Update the usage guide for remembered views. Run focused state/storage and
   mounted switching/reload regressions, then the repository checks and dev
   smoke test. Obtain an independent findings-only code review.

## Test plan

- Switching between sessions keeps independent tabs, ratios, file source mode,
  expansion, scroll, and active pane; deleted sessions lose snapshots without
  resurrecting them during cleanup. Existing drafts remain intact.
- A new store restores the last selection and every saved view, while an
  explicit URL selects its session. Persisted JSON contains no transcript/file
  contents, account data, or in-progress dragging.
- Mounted file/transcript views restore scroll after delayed data; switching
  saves the departing view's final scroll; following transcripts stay following.
- Sidebar manual tab/collapse choices and user-resized panel sizes survive a
  reload, while mount/collapse/window resizing do not overwrite saved sizes.
- Malformed or unavailable browser storage leaves the app usable and reports
  its failure. The previous wrap/fixedWidth settings migrate successfully.
