# Review title

Delete placeholder text and unused sections. Give each finding an ID (`F1`,
`F2`, and so on) and its own section.

## Scope and coverage

State what you reviewed and any material gaps in coverage.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | High | Correctness | Fixed | Short finding title |

If there are no findings, replace the table with `No findings.` and omit the
finding sections. Use `Open` in the Fix column for anything left unresolved.

## F1. Short finding title

High · Correctness · Fixed · `path/to/file:line`

Describe the finding and its effect.

### Evidence

State how you confirmed the finding.

### Resolution

Describe the change or what remains open. If the fix needed a user decision,
record it here.

## Checks run

- `command` — Result.

## Verdict

Summarize the result, referring to finding IDs where useful.
