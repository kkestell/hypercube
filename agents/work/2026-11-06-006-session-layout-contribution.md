# Session view state: global layout contribution

## Plan

`agents/plans/2026-11-06-005-session-view-state.md`

## Summary

Moved global splitter sizes, sidebar tab, collapsed workspace/family IDs, and
sidebar scroll into Workbench. Removed replaced component-local state and the
old wrap/fixed-width storage subscriber. The parent owns cohesive view storage,
startup migration, session transitions, and session-owned active editor panes.

## Decisions

- Each sidebar tab retains its own native scroller and ref; only the active
  tab mounts rows. Scroll saves on native events/pagehide/unmount and restores
  when a tab activates or workspace data arrives.
- Explicit selection transitions choose the sidebar tab. Mounting preserves a
  manually restored tab, including Archived while an active session is selected.
- Fixed Split callbacks convert the new percentage to pixels using the two
  panels' available width/height. A real-library keyboard regression reproduced
  the tempting direct getSize().inPixels implementation reporting the previous
  size before React committed panel styles.

## Checks run

- `bunx tsc -p .` in `internal/ui` — passed.
- `bun test web/shared/split.test.tsx web/sidebar/sidebar.test.tsx web/sidebar/sidebar-view.test.tsx web/app/app.test.tsx` — 18 passed.
- Coverage includes actual localStorage reload of sidebar choices and panel
  sizes; independent tab scroll and pagehide; delayed workspace arrival;
  keyboard resize units for fixed-start, fixed-end, and proportional splits;
  and ignoring mount/collapse as persistence changes.
- The existing app Escape test emits its previous React act warning.

## Follow-up work

Parent owns the combined checks, generated UI rebuild, independent code review,
and opening the development view for the user's visual inspection.
