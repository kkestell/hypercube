# Audit of the issue-triage fixes

## Scope and coverage

Audited the code changes in `5ee6d6c` against its parent and their current
callers at `da961f0`. Covered submission and draft ownership, transcript
rendering and scrolling, offers, file/diff/tree refresh ownership, folder
errors, workspace deletion admission, compact transcript reads, Codex and
OpenCode controls, RPC call ownership, installer encoding, worktree inclusion,
alerts, sidebar layout, tab focus styling, search presentation, and website
claims. Preserved the subsequent Linux checks, whole-project review, and
desktop-actions commits.

This is a source and regression-test audit. No Chrome or other browser was
launched. DOM tests do not establish native hover behavior or visual geometry.
The original missing-tooltip problem has not been reproduced or resolved by
this audit; the user requested restoration of its original implementation.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Scope and accessibility | Reverted | Replaced native hover text with an unnecessary popup |
| F2 | Low | Layout | Fixed | Chevron overlays short titles despite unused row width |

## F1. Replaced native hover text with an unnecessary popup

`internal/ui/web/agent/session-selection.tsx`

I approved adding a shadcn Tooltip component without reproducing why the
existing icon's `title` was absent. The chat transcript records the worker
proposing the component and my response: "Proceed with stock shadcn Tooltip
wrapper." No user requirement called for that replacement.

It added a new component, provider, trigger, popup, and focusable wrapper to a
status icon. The wrapper's `tabIndex=0` also added an extra keyboard stop inside
each child tree row, despite the sidebar owning its tree's focus order. The
new tests verified the added popup rather than the reported native behavior.

Restored `session-selection.tsx` exactly to its pre-triage implementation,
deleted the unused Tooltip component, and removed its replacement-specific
test. Issue #54 remains unresolved; the earlier claim that it was fixed is
withdrawn.

## F2. Chevron overlays short titles despite unused row width

`internal/ui/web/sidebar/session-item.tsx`

The absolutely positioned chevron was anchored to a flex child with intrinsic
text width. That child did not grow into the available title area, so even a
short title placed the chevron over its final characters while the rest of the
row remained empty. This follows from the containing block and flex sizing;
no browser geometry measurement is claimed.

Made the containing block fill the title area with `flex-1`. The chevron still
reserves no title width and the three decoration columns stay unchanged.
Retained the existing interaction regression for toggling, double-clicking,
and selection, and removed assertions that merely copied layout class names.

## Other results

No additional functional regression was confirmed in the remaining fixes.
That conclusion is limited to the source paths and checks listed here, not a
claim that passing tests establish every visual or native interaction.

The backend Finder endpoint added in `5ee6d6c` has already been removed by
`da961f0`; the current file menu uses the desktop bridge and hides unavailable
desktop actions in a browser. No changes were made to that later work.

## Checks run

- `git diff --check` — passed.
- `GOCACHE=/private/tmp/hypercube-go-cache make check` — passed: UI and website
  builds, TypeScript, 329 UI tests, 22 desktop tests, Go build/vet/format checks,
  and the full Go race suite.
- `GOCACHE=/private/tmp/hypercube-go-cache make install-service` — passed;
  the installed app returned HTTP 200 after the service restart.

## Verdict

The requested restoration is implemented. F1 is reverted and F2 is corrected.
No replacement tooltip, new subsystem, or delegated work was introduced.
