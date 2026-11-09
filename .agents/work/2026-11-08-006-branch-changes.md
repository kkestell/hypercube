# Complete Branch changes

## Plan

`agents/plans/2026-11-08-004-branch-changes.md`

## Summary

Branch changes now lists net working-file changes against the live merge-base
with the workspace checkout. Rows and opened patches use the same comparison,
including recreated committed deletions. Uncommitted remains the default and
the sole source of Files status and the current-branch bar.

The browser keeps comparison results, diff identities, requests, and polling
separate. Both comparisons of one literal path can coexist and restore without
sharing patches or scroll. Branch changes retains the approved merge control
and displays the comparison response's target instead of preview copy.

## Checks run

- `go test ./internal/workspace ./internal/ui` — passed before the additional
  focused branch tests.
- `go test ./internal/workspace -run TestBranchChanges -v` — passed.
- `go test ./internal/ui -run TestBranchChangesRoutes -v` — passed.
- Focused browser tests for changes, editor tabs, polling, file views, and saved
  views — passed.
- `make check` — passed: browser build and TypeScript, 393 browser tests,
  28 desktop tests, website build, Go build/vet/format checks, and all Go tests
  with the race detector. Repeated after the final patch-helper extraction.
  Existing React `act(...)` warnings appeared in unrelated test files; no
  failures.
- `git diff --check` — passed.

## Manual verification

- Reused the existing development server; `curl --fail --silent --output
  /dev/null --write-out '%{http_code}\n' http://localhost:4281/` returned 200.
  Did not start another server or modify the installed app. Visual judgment
  remains with the user; browser behavior is covered by component tests.

## Follow-up work

- Parent owns independent review, commit, merge, and reinstall. This work is
  staged only, as assigned.
