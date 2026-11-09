# Read shown tabs immediately

## Scope and coverage

Reviewed the staged implementation against
`agents/plans/2026-11-07-003-read-shown-tabs-immediately.md` and its matching
work log. Read the whole touched source and test files, plus the pane mounting,
session attachment, queued file opens, read versioning, scroll restoration,
and polling paths.

The mounted views own unread first reads. Explicit opens of loaded content
still refresh immediately, including agent requests, empty files, messages,
previews, and deleted files. The new effects use the existing read functions,
whose attachment and request versions reject superseded results. Existing
overlap tests exercise these guards for files and diffs. The background poller
is unchanged; an overlapping poll can still issue another read.

Live Electron timing and the manual session-switch check remain with the parent;
this review does not establish the final latency improvement.

## Summary

No findings. No code fixes were needed. The touched files remain straightforward
and the implementation stays within the measured problem.

## Checks run

- Independently ran `bunx tsc -p .` in `internal/ui` — passed.
- Independently ran the shown-tabs, editor, file-view, file-polling, and
  data-socket test files — 58 passed, 0 failed.
- `git diff --check --cached` — passed.
- The matching work log records all 359 UI tests and
  `GOCACHE=/tmp/hypercube-shown-tabs-go-cache make check` passing. No code was
  changed during review, so the full check was not repeated.

## Verdict

Approve. No unresolved review findings; live timing remains a separate check.
