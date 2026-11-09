# Follow system appearance

## Plan

`agents/plans/2026-11-03-009-system-appearance.md`

## Summary

The page, terminal, highlighting, and animation follow the system appearance. AppKit already owns the native window appearance.

## Departures from the plan

- After Dark has no light counterpart. The requested GitHub Light theme supplies light syntax colors.

## Checks run

- `make check` — passed before and after rebasing.
- Isolated `make dev` on port 14381 — the page was served; the server was stopped. Visual inspection is left to the user.
