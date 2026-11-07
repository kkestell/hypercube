# Immediate prompt feedback

## Scope and coverage

Independent review of issue #34 against
`agents/plans/2026-11-06-011-immediate-prompt-feedback.md` and
`agents/work/2026-11-06-010-immediate-prompt-feedback.md`.

Read the complete touched production files: session input and creation; UI
actions and API; page state; draft and unsent selection lifecycle; submission
state and delivery; AgentPane, Transcript, transcript reconciliation, and the
tesseract component. Traced command acceptance, sidebar discard, and scroll
ownership through their callers. Preserved unrelated staged admission, file,
and transcript changes.

Covered immediate first and follow-up text and images, actual AgentPane
rendering, identical prompts and ID reconciliation with resets and either HTTP
response order, newer drafts, retry clicks, session switches, parked unsent
contexts, created-session failures, discarded drafts, session deletion, and
external correlation ID validation. The implementation retains existing command
acknowledgment semantics and adds no delivery or recovery guarantees. The final
files are readable and responsibilities remain separated.

No live browser, service installation, or full `make check` was run in this
review. The parent owns the combined full check. Happy DOM tests model scroll
dimensions; they do not verify browser layout. The author's work log reports
focused Go race tests passing; the reviewer independently reran the HTTP test.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Correctness | Fixed | Discarded first prompts reappear in a new draft |
| F2 | Medium | Correctness | Fixed | First prompts cannot follow the newest content |

## F1. Discarded first prompts reappear in a new draft

Medium · Correctness · Fixed · `internal/ui/web/agent/drafts.ts:102`

The sidebar's Discard Draft action removed the unsent context and composer
draft but retained its submissions. A newly opened unsent session in the same
workspace reuses that key, so the discarded pending or failed message appeared
again. A late response ignored by the unsent identity guard did not clean it up.

### Evidence

Added regressions for both pending and failed first prompts. Before the fix,
both failed immediately after discarding and reopening: the submission array
still contained the discarded message. Both pass after the fix, including a
late successful creation response that cannot take over the new context.

### Resolution

Discarding an unsent session clears its submissions as well. Successful
creation now moves submissions to the real session before `unsentCreated`
discards the placeholder, preserving successful messages and retained failures.
Existing creation, parked-context, and failed-first-input tests pass.

## F2. First prompts cannot follow the newest content

Medium · Correctness · Fixed · `internal/ui/web/transcript/transcript.tsx:40`

An unsent session has no attached authoritative transcript. Its provisional
Transcript therefore always used `ready=false`, which disables follow-to-end,
scroll tracking, and the scroll-to-newest action. Long first messages and their
failure controls could extend below the viewport without normal following.

### Evidence

An actual AgentPane regression models a long first message and a smaller
viewport. Before the fix, a layout update left `scrollTop` at zero instead of
following the end. After the fix it follows the end, preserves the user's
explicit upward scroll during later updates, and returns to the end on request.

### Resolution

An unsent provisional transcript is ready immediately. Existing attached
sessions continue to wait for their authoritative transcript. The existing
scroll restoration and image-following tests pass unchanged.

## Checks run

- Red checks: two discard regressions and one first-prompt scrolling regression
  failed against the implementation before their respective fixes.
- `bun test web/transcript/submissions.test.tsx
  web/sessions/session-actions.test.ts web/agent/drafts.test.ts
  web/editor/use-scroll-memory.test.tsx web/transcript/transcript.test.tsx
  web/agent/composer.test.tsx web/agent/model-select.test.tsx
  web/app/settings-dialog.test.tsx web/sessions/selection.test.ts` in
  `internal/ui` — 109 passed, zero failed. An existing composer mention-key test
  emitted a React `act` warning.
- `bunx tsc -p .` in `internal/ui` — passed.
- `go test ./internal/ui -run TestHTTPSubmissionCorrelation -count=1` — passed.
- `git diff --check` — passed.

## Verdict

Pass after fixing F1 and F2. No unresolved medium or high findings. Parent must
complete the combined `make check` before delivery.
