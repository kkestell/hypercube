# Native menus and keyboard shortcuts

## Plan

`agents/plans/2026-11-03-008-native-shortcuts.md`

## Summary

Added native menus and page zoom, with a shortcut reference drawn from the shared bindings.

## Departures from the plan

- The reference follows GitHub's grouped shortcut dialog, as requested after planning.
- Browser placement scales with page zoom, and the dialog registers as an overlay so it covers the embedded browser.

## Checks run

- `make check` — passed outside the sandbox; sandboxed `sips` could not render the existing app icon.
- `make dev` — served the isolated browser view; stopped and removed its state.
