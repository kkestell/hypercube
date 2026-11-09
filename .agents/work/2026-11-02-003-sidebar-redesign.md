# Workspaces sidebar redesign and independent session attention

## Plan

`agents/plans/2026-11-02-005-sidebar-redesign.md`

## Summary

The sidebar is plain markup following the three design images: workspace
headers with caret and "+", rounded 32px rows, a hooked guide under nested
sessions, faded long titles, one dot per row that swaps to "…" on hover, and a
collapsed parent that keeps its waiting nested sessions plus a summary row.
Nested sessions now get unseen replies, blue dots, and reply cards, and a card
hides only while its own session is shown. The plan's goal is met; the visuals
are unjudged.

## Decisions

- A row's title button stretches over the whole row with an absolutely
  positioned `::after`, so clicking anywhere on the row selects it while the
  caret and status column sit above it. Double-click to rename is on the title
  button for the same reason.
- The workspace "…" uses `visibility: hidden` when not hovered, so "+" stays
  in the right-hand column; a row's "…" uses `display: none` and the dot is
  hidden by `:has()` while the row is hovered or focused.
- Workspaces are 8px apart; the plan gave no figure.
- The summary row says "1 more nested session" in the singular.

## Checks run

- `make check` — passes (152 bun tests, all Go packages).

## Manual verification

None; the user judges the rendering. `make dev` shows the sidebar.

## Follow-up work

- Check the look against the design images: caret glyph size (10px), the
  hook width (12px), and the gap between faded titles and the dot.
