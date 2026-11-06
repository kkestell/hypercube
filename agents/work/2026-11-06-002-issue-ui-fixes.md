# UI issue fixes

## Plan

`agents/plans/2026-11-06-002-issue-ui-fixes.md`

## Summary

Implemented #2, #6, #7, #8, #15, #16, #19, and #23. Existing store ownership,
component state, transcript disclosures, and draft storage carry the changes.

## Decisions

- Parent independently reviewed the per-workspace draft design. Only the shown
  context lives in `unsent`; the other contexts live in `unsentDrafts`. Text and
  images keep stable keys in the existing drafts map. Asynchronous image reads
  and new-session creation follow those keys across navigation.
- Previous single-new-session localStorage data upgrades when read. Images
  remain in memory, preserving the existing persistence boundary.
- Renamed the old worktree toggle module to `new-session-options.tsx`, since it
  now owns the workspace context and explanation above the composer.
- Added shared choice state requested by the parent for #3. Shared selection
  files also include the session agent's #20 waiting-count changes; usage.md
  includes that behavior.

## Checks run

- `bunx tsc -p .` in `internal/ui` — passed.
- Focused Bun suite across app, shortcuts, settings, composer, selection,
  session actions, drafts, draft storage, notification cards, and proposal
  cards — 77 passed. Covers body-focused shortcuts, per-workspace navigation,
  reload and legacy migration, pending creation and image reads, automatic
  reports, multiline/recalled edits, hover/focus expiry, and both proposal
  outcomes.
- `bun test web/transcript/transcript.test.tsx` — 12 passed after updating the
  old assertion that completed proposal cards contain no buttons.
- After final proposal readability edits and asynchronous test cleanup,
  typecheck and the app/proposal subset passed again (6 tests).
- One existing Escape test emits a React act warning; assertions pass.

## Follow-up work

- Parent owns independent review, combined `make check`, generated UI rebuild,
  and opening the development browser view for the user's visual checks.
- These UI changes need a page reload; they do not require agent-session
  restart. Other agents' plugin and launch changes have separate requirements.
