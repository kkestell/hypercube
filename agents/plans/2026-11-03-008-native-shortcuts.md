# Native menus and keyboard shortcuts

## Goal

Expose app shortcuts in native menus and a keyboard reference, and add native window page zoom.

## Decisions

Keep shortcut definitions in the web app. The page sends native menu definitions to the window, which dispatches selected actions back to the page. Use the same definitions for keyboard matching and the reference. Native page zoom uses WKWebView pageZoom; browser zoom remains the browser's responsibility.

## Implementation plan

- Define shortcuts and actions in `internal/ui/web/app/shortcuts.js`, preserving existing focus and terminal behavior, and share the definitions with transcript find and layout tooltips.
- Send menu definitions through the existing window bridge; build View, Session, and Help menus in `internal/desktop/app.m` and forward their actions to the page. Handle page zoom in `internal/desktop/window.m`.
- Add a stock dialog listing shortcuts in `internal/ui/web/app/` and connect keyboard and native menu actions in `app.jsx`.
- Update `docs/usage.md` and regenerate the UI bundle.

## Test plan

- Exercise shortcuts, menu dispatch, and dialog through the app UI; retain existing keyboard tests.
- Run the required checks and provide a development server for visual review.
- Scale native browser placement with page zoom, and register the dialog as an overlay.
- Use the shadcn Kbd component for the grouped shortcut reference.
