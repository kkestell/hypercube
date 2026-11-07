# Create a session with its first prompt

## Scope and coverage

Reviewed commit `14da7c9` against `bc8dd1f`, its callers, the plan and work log
in `agents/plans/` and `agents/work/`, and the fixes below. Checked that choosing
a lead creates only browser state and that the first submitted prompt creates,
starts, and titles the session.

The page was not rendered in a browser. Its unsent-session flow has no permanent
automated test; temporary checks evaluated the actual page functions with small
DOM and request stand-ins.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Resources | Fixed | Discarded composers remain visible |
| F2 | Medium | Concurrency | Fixed | Abandoned image reads attach to the next session |
| F3 | Medium | Correctness | Fixed | Creation drops a newly typed draft |

## F1. Discarded composers remain visible

Medium · Resources · Fixed · `internal/ui/static/index.html:99`

Opening a second unsent session deleted the first card from the map without
removing its element. With no attached server session, rendering appended a
second composer and could no longer find the old one to remove. The abandoned
text remained visible; submitting it used the replacement profile and workspace.

### Evidence

Replacing an unsent session left two children in the lead column. After the fix,
the same check leaves one.

### Fix

Remove the abandoned card's element before deleting its map entry.

## F2. Abandoned image reads attach to the next session

Medium · Concurrency · Fixed · `internal/ui/static/index.html:523`

An image read begun in one unsent session could finish after another replaced
it. Both use the key `new`, so the old read populated the replacement session's
attachments, which its next submission would send.

### Evidence

A deferred image read populated the replacement session's attachment map before
the fix. After the fix, that map remains empty.

### Fix

Capture the unsent session before reading images and ignore the result if that
session has since been discarded.

## F3. Creation drops a newly typed draft

Medium · Correctness · Fixed · `internal/ui/static/index.html:587`

The message field stays editable while the first submission starts the harness.
When that submission succeeded, selecting the created session discarded the
text and attachments entered during the request.

### Evidence

A deferred creation response discarded the second draft before the fix. After
the fix, both its text and attachments are stored under the real lead's ID,
including when the creation response precedes the session's state snapshot.

### Fix

As directed by the lead, keep editing enabled and return JSON containing the
session and lead IDs from the creation action. Transfer the current draft and
attachments to that lead before selecting the session. Update the HTTP test to
verify both IDs and the content type, and adapt the integration-test client to
the JSON response.

## Checks run

- Focused Go tests for session, UI, report, and store passed during review.
- Temporary function-level reproductions confirmed all three failures and now
  pass, including draft transfer before the state snapshot arrives.
- The page module parsed with `bun build --no-bundle`.
- The default `make check` was blocked by sandbox writes to the Go build cache.
  With `GOCACHE=/tmp/ec064d07-go-build`, its first run failed during unchanged
  `TestLeads` temporary-directory cleanup. The focused race tests for
  `TestLeads` and `TestNewSessionTakesItsFirstPrompt` then passed, and the full
  `make check` rerun passed, including 40 Bun tests and all Go race tests.
- `git diff --check` passed.

## Verdict

F1–F3 are fixed. The required first-message creation behavior remains intact.
