# Replace unsupported integrations

## Goal

Remove the browser overlay, native button repositioning, private terminal
renderer patch, Codex home mirroring and Neuron script parsing, and OpenCode
child attribution by title. Use supported platform paths; remove the affected
capability where no supported replacement exists.

## Decisions

The baseline is deletion of the six workarounds and their supporting state.
Preserve agent browsing with ordinary native browser windows: AppKit owns the
window, controls, and web view layout. This adds only the navigation controls
needed to replace the existing browser toolbar. No browser rectangles, overlay
registry, or browser tabs remain in React. A browser closes when its window is
closed or its agent calls browser_close. Browsers keep the default persistent
website data store. The Window menu lists browser windows.

Keep the integrated tab row using AppKit's full-size content view, transparent
title bar, and unified compact toolbar. Read the native geometry to size the
row and clear the system buttons, leaving their frames entirely under AppKit
control. Keep the supported window drag and zoom bridge. Remove the added
height and repeated button repositioning. Use xterm's default renderer,
removing the optional WebGL add-on and its private method replacements.

Codex inherits its native environment and CODEX_HOME. Delete storage.go and
its tests; neither sqlite_home nor a private CODEX_HOME is necessary once the
separate Recents behavior is removed. Keep stable thread/start and
thread/resume. Preserve legacy private storage and Hypercube transcripts;
there is no supported cross-home import. Mark new opaque conversation
references with `native:` and reject old unprefixed references before resuming:
the native home may hold a stale original with the same ID as an imported
private conversation. Keep failures explicit and never silently create a new
conversation. Remove automatic Neuron account detection. No schema changes.

OpenCode ACP child metadata has no initiating tool ID. Remove child transcript
display for that harness by ignoring child updates, retaining main task tool
results, usage, and final answers. Do not add a protocol just to preserve
unsupported nesting. Other harnesses' supported child display is unchanged.

## Related code

- internal/desktop/browser.m, window.m, desktop.h, app.m, browser.go,
  desktop.go: native window and browser ownership.
- internal/mcp/browser.go: browser tool descriptions presented to agents.
- internal/ui/web/browser/, shared/overlays.ts, state.ts, native.d.ts,
  main.tsx, editor/, sessions/selection.ts: embedded browser and title-bar
  integration; overlay consumers throughout the UI.
- internal/ui/web/terminal/xterm.ts, internal/ui/package.json, bun.lock:
  renderer and dependency removal.
- plugins/codex/main.go, storage.go, main_test.go, storage_test.go:
  native environment and saved-conversation behavior.
- plugins/opencode/main.go, events.go, main_test.go: child metadata handling.

## Implementation plan

1. Give each native browser an ordinary window and native navigation controls,
   using normal content layout. Close pending navigation requests when closed.
   Parent file-open sheets to the initiating browser window and update tool
   descriptions to describe windows rather than tabs.
   Remove Browser tabs, placement, browser reports, and overlay tracking from
   React. Preserve dialog focus restoration independently of overlay state.
2. Preserve the integrated title-bar tab row and its geometry/drag bridge;
   remove only the height adjustment and native button repositioning.
3. Remove the optional WebGL renderer and private xterm API declarations.
4. Remove Codex storage/account selection code; test inherited environment,
   ordinary startup/resume, and an explicit failed resume without fallback.
5. Remove OpenCode child display and title matching; test that nested updates
   cannot leak child output into main text, usage, or final-answer selection.
6. Remove obsolete tests, adapt user-facing tests to retained behavior, rebuild
   generated UI assets, and review whole touched files.

## Test plan

- Browser navigation, title/URL updates, history controls, eval, shared login
  storage, close/reopen, and pending request cleanup in isolated native smoke
  tests; compile Objective-C through the normal Go build.
- Existing UI tests for tab layout, menus, dialogs, focus return, shortcuts,
  and terminal lifecycle, with browser-placement tests removed.
- Harness stand-ins for inherited Codex environment and failed native resume;
  recorded OpenCode nested output for suppression and main-answer correctness.
- Required repository checks. Use the isolated dev app for native behavior and
  the dev browser view for the user's visual inspection; do not install.

## Documentation updates

Update docs/architecture.md, docs/usage.md, docs/installation.md, and
docs/harness-support.md to describe native browser windows, native Codex
storage, unavailable legacy private resume, and OpenCode child display removal.
