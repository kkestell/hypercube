# Quick Open and copy paths

## Plan

`agents/plans/2026-11-03-008-quick-open-copy-path.md`

## Summary

Quick Open searches the existing session file listing and opens previews. Tree rows and file tabs offer path copying through their context menus.

## Checks run

- `make check` — passed outside the sandbox; sandboxed icon conversion could not access macOS image services.
- Focused UI tests — passed.
- Isolated `make dev` on port 4387 — served the app successfully; cleaned up after the smoke test.

## Departures from the plan

- Command+P works from the terminal as well as the rest of the window.
