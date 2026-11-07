# Stock shadcn frontend

## Goal
Replace VS Code controls and colors with vanilla shadcn while preserving the
workspace/session sidebar, resizable panes, draggable editor tabs, transcript,
composer, notifications, file previews, diffs, browser integration, and terminal.
Work directly in this checkout so the running dev view shows progress.

## Decisions
The baseline is the existing application and store, React in place of Preact,
unmodified upstream shadcn components, the default neutral dark theme, and
Tailwind layout utilities. No bespoke component CSS or theme overrides. Keep
xterm and Shiki for terminal and syntax rendering, using their library defaults
and a bundled non-VS-Code syntax theme. Use stock progress bars and badges in
place of the existing custom meters and decorations. Keep behavior modules and
backend APIs unchanged. The user requested solo implementation; no delegation.

## Implementation plan
- Add React, Tailwind, Lucide, and upstream shadcn source in internal/ui.
  Update the existing Bun build and its license collection; remove the VS Code
  generated theme, icon font, and element registrations.
- Replace shared controls with stock buttons, tabs, alert dialogs, menus,
  collapsibles, resizable panels, inputs, selects, cards, badges, and progress.
- Migrate the app frame, sidebar, editor, composer, transcript, notifications,
  files, diffs, browser toolbar, and terminal integration. Retain layout,
  keyboard shortcuts, focus, draft ownership, lazy loads, scroll memory,
  native-browser overlay reporting, drag/drop, and all session actions.
- Remove obsolete presentation styles and icon assets. Update affected tests
  to use React and real controls. Correct existing frontend guidance for this
  requested stack change.

## Test plan
- Exercise menus, confirmation, tree navigation, pane collapse/restore,
  editor selection/close/drag, compose/send/history/mentions/images, requests,
  notifications, file reveals, and find behavior through the existing tests.
- Build the frontend, run make check, verify the running dev server serves this
  checkout and the rebuilt assets, and leave visual assessment to the user.
