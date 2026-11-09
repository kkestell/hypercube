# Supported integrations review

## Scope and coverage

Reviewed the working changes against the plan, concentrating on native browser
ownership and lifetime, Codex storage and resume, OpenCode child updates, and
the affected UI paths. Generated UI assets, native smoke tests, and final
documentation polish were still in progress and were not reviewed.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Resources | Fixed | Closing the last browser leaves Dock activation enabled |

## F1. Closing the last browser leaves Dock activation enabled

Medium · Resources · Fixed · `internal/desktop/browser.m:220`

When the main window closes while a browser is visible, its close handler
retains the regular activation policy. Closing the browser afterward removes
the last visible window without switching back to the accessory policy, leaving
the app in the Dock and app switcher. The main window's close handler performs
the visibility check, but the browser close handler did not. The app now owns
the check and both close handlers invoke it, excluding the closing window.

## Checks run

Traced the window delegates, browser and harness event handling, and affected
UI changes and inspected the subsequent Dock policy and navigation-title fixes.
Also checked the Codex saved-reference marker used to reject older references
without resuming a stale native copy. The implementation session reports that
native smoke and focused Codex tests passed; repository checks remain with that
session. No tests were run in this review.

## Verdict

F1 is fixed. The reviewed harness changes follow the planned storage and
child-update boundaries. Final checks remain with the implementation session.
