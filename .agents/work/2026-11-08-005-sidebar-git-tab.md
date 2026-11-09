# Git tab in the right panel

## Plan

`agents/plans/2026-11-08-002-sidebar-git-tab.md`

## Summary

The plan's goal is met. Git lists the current view's uncommitted paths with
shared status colors and supplied line counts; each row opens its existing
diff. Files keeps status colors and navigation without inline count controls.
Activation, polling, saved layouts, workspace/session targeting, and the two
existing guides are updated without backend changes or another changes cache.

## Decisions

- No behavioral departures. The existing editor view performs the first diff
  read after `openDiffTab` opens a tab; Git uses that path without adding reads.
- Removing Files' count controls also removes their now-unnecessary row wrapper
  and hover overrides, leaving a full-width native file button.

## Checks run

- In `internal/ui`, `bun test web/changes/git-panel.test.tsx
  web/files/files-panel.test.tsx web/files/file-polling.test.ts
  web/app/app.test.tsx web/app/view-storage.test.ts` — 49 passed, 0 failed.
- In `internal/ui`, `bunx tsc -p .` — passed.
- In `internal/ui`, `bun build.ts dist` — passed on the final implementation.
- The focused suite emits a React `act` warning from `ChatPane`; the updated
  workspace/session Git integration test passes alone without that warning
  (`bun test web/app/app.test.tsx -t 'workspace heading opens recent sessions'`).
- `make check` — passed: UI and website builds, TypeScript, 380 UI tests,
  28 desktop tests, Go build/vet/format checks, and all Go tests with `-race`.
  Other existing suites also emit non-failing React `act` warnings.
- `git diff --check` — passed.

## Manual verification

No live browser check or screenshots were performed. Component and app tests
exercise status/count presentation, activation, cached and empty/error states,
diff opening from paths and counts, and workspace/session request targets;
polling and storage tests exercise refresh visibility and restored choices.
