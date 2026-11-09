# File and diff navigation

## Plan

`agents/plans/2026-11-03-009-file-diff-navigation.md`

## Summary

Shared transcript find with files and diffs, added a saved word-wrap toggle, and added navigation between consecutive changed lines.

## Departures from the plan

- Layout visibility settings are not persisted, so wrap uses its own browser storage key.

## Checks run

- `make check` — run outside the sandbox because sandboxed `sips` cannot load the app icon.
