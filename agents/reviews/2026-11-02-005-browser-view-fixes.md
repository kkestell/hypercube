# Browser view fixes review

## Scope and coverage

Reviewed `hypercube/a4bcbaca/fix-web` against `main` through `57bbb5b`,
excluding generated `internal/ui/dist`. Read all 31 touched source and test
files in full, and traced the affected callers, state transitions, and
cleanup. Checked network failures, the stopped-session fallback, scroll
retention, the moved helpers and draft actions, and the added tests.

Browser appearance, screenshots, and live harness checks were outside this
review. Findings were reported before edits; the lead approved F1, the
review document, and the commit.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | High | Error handling | Fixed | Response-body failures still lose drafts |

## F1. Response-body failures still lose drafts

High · Error handling · Fixed · `internal/ui/web/api.js:9`

The new error handler caught only `fetch()`. Reading the response with
`text()` or `json()` happened outside it. A connection lost after the
response headers therefore still rejected instead of showing an error and
returning null. Sending a draft cleared it without restoring it, and new
session creation left `unsent.sending` true, disabling further sends.
Fetching lead profiles could also reject without showing an error.

### Evidence

Before the fix, supplied a response whose `ReadableStream` errored during
body consumption. `sendPrompt` rejected; the draft was empty, the unsent
session remained sending, and the errors list stayed empty. `fetchLeads`
also rejected for the same response.

### Fix

As approved by the lead, moved fetching and body consumption into one
`try`/`catch` in each API function, replacing the fetch-only helper. Both
functions now show the error and return null when reading fails. Tests use
erroring response streams to check successful and refused HTTP responses,
and verify draft restoration and re-enabled sending for an unsent session.

## Checks run

- Initial `bun test`: 138 tests passed.
- Focused API and session-action tests after the fix: 21 tests passed.
- `make ui`: rebuilt the generated assets.
- `make check`: passed, including generated asset freshness, 143 browser
  tests, Go vet, formatting, and all Go tests with the race detector. Go
  printed non-fatal module-cache write permission warnings.
- Source diff whitespace check: passed. The generated bundle contains
  dependency whitespace reported by the unrestricted diff check; it was
  left as emitted by the build.

## Verdict

F1 is fixed. No unresolved findings remain in the reviewed scope.
