# Workspace dashboard

## Plan

`agents/plans/2026-11-07-004-workspace-dashboard.md`

## Summary

The workspace heading opens a dashboard of its ten most recent top-level
sessions, including archived sessions. A separate chevron controls its sidebar
list. The shared editor supports files, diffs, search, Quick Open, previews,
and the terminal against the registered checkout and its current branch.

Editor choices and cached trees belong to explicit session or workspace targets;
only real sessions attach transcripts. Existing session URLs, browser layouts,
terminal-visibility migration, draft parking, and queued agent file requests
retain their behavior. Workspace view choices persist independently and disappear
when their workspace is removed.

## Decisions

- Extracted editor attachment and saved-view operations from session navigation,
  and snapshot lookup from selection, to keep ownership clear without a target
  framework or a synthetic session.
- Shared directory resolution also covers Markdown images, whose route is
  separate from browser preview metadata.
- Workspace shells use the existing terminal set under a short `w-` owner
  prefix. The initial longer prefix exceeded macOS's Unix socket path limit in
  isolated runtime tests; shortening it fixed the concrete failure.
- Workspace holders are adopted before serving requests. This lets deletion
  stop a restored shell even when the workspace has no sessions, and prevents
  deletion between the restore snapshot and adoption from orphaning its shell.

## Checks run

- `bunx tsc -p .` and focused Bun tests in `internal/ui` — passed.
- Existing and added browser tests cover dashboard navigation, archived and empty
  states, independent collapse, file/diff/search operations, workspace Quick
  Open and terminal parameters, saved choices and old data, draft preservation,
  socket isolation, queued agent opens, and stale cross-view reads.
- Focused Go route and runtime tests — passed. They cover the real checkout
  despite a session in another directory, branch changes, owner validation,
  HTML relative resources, Markdown images, workspace WebSocket terminal routing,
  and shell admission, restore, deletion, and quit.
- `make check` — passed after the socket-prefix fix: browser and website builds,
  TypeScript, 367 browser tests, 28 desktop tests, Go vet/formatting, and the full
  Go race suite.
- `git diff --check` — passed.

## Manual verification

The parent started `make dev` on port 4281. Live requests to the development
workspace's page, file tree, README, changes, and diff all returned HTTP 200;
changes reported its checked-out `main` branch. No screenshots were taken.
The development server remains available for the requested user test, and the
parent opens its workspace dashboard after review and commit.
