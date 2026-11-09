# Browser UI rewrite review

## Scope and coverage

Reviewed the complete source files under `internal/ui/web/`, including tests,
and `internal/ui/{build.js,test-dom.js,test-render.jsx,jsconfig.json,bunfig.toml,
package.json,assets.go}`, the Makefile's `ui` and `check` targets, and
`scripts/dev`. Compared behavior with `11f395c` and checked the data socket
contract in `agents/plans/2026-11-01-003-browser-ui-rewrite.md`. Read the browser
work log, repository instructions, and Hypercube skill. The vendored Seti
theme, font, and license are unchanged from the old UI.

Generated `dist/` was excluded from source review and regenerated for the fixes.
Browser appearance and a live browser smoke test remain for the user and lead;
verification here used headless component tests and the required build checks.

The lead received all findings before any edits and selected every fix except
F6. Locations below name the resulting source files.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | High | Concurrency | Fixed | App misses an early state message |
| F2 | High | Correctness | Fixed | Creation completion consumes another unsent draft |
| F3 | Medium | Correctness | Fixed | Pending image reads strand the created composer |
| F4 | Medium | Correctness | Fixed | A split loses the transcript's scroll position |
| F5 | Medium | Correctness | Fixed | A late parent hides an open request |
| F6 | Medium | Correctness | Skipped | Right-panel keyboard behavior differs from the old UI |
| F7 | Medium | Correctness | Fixed | Pending lead picker ignores cancellation |
| F8 | Medium | Correctness | Fixed | Open Transcript does not focus an active worker |
| F9 | Low | Readability | Fixed | Ordinary actions duplicate standard controls |
| F10 | Low | Readability | Fixed | CSS repeats literal sizes |
| F11 | Low | Simplicity | Fixed | Build emits an empty JavaScript chunk |

## F1. App misses an early state message

High · Concurrency · Fixed · `internal/ui/web/app/app.jsx:37`

App read its initial snapshot during render but subscribed in a passive effect
without reading the snapshot again. A state message arriving before that effect
updated the store without redrawing the page. With a 16 ms effect schedule and
state delivery at 5 ms, the store attached to a session while the page still
showed no composer after 300 ms.

### Fix

As directed by the lead, App subscribes in a layout effect and immediately
synchronizes from `store.get()`. A regression test sends state before passive
effects and checks the visible workspace.

## F2. Creation completion consumes another unsent draft

High · Correctness · Fixed · `internal/ui/web/sessions/session-actions.js:37`

After awaiting creation, the code checked whether any unsent session existed.
Sending A, opening B, and resolving A moved B's draft to A's lead and discarded
B. A refused creation could also restore A's prompt into B. The old UI guarded
the particular unsent session.

### Fix

Each unsent session has an identity that survives immutable sending-status
updates. A completion checks that identity before changing status, transferring
draft content, selecting a session, or allowing prompt restoration. Tests cover
accepted and refused creation after replacement with the same workspace and
profile.

## F3. Pending image reads strand the created composer

Medium · Correctness · Fixed · `internal/ui/web/sessions/selection.js:133`

An image attached while creation was pending could move `reading: 1` into the
created lead's draft. Its completion then rejected the discarded unsent session
and never cleared the transferred counter. Sending stayed disabled. The old UI
kept the counter on the old composer.

### Fix

As selected by the lead, only completed draft content transfers, with its
reading count reset. Pending reads of the discarded session are ignored. The
stable identity from F2 also lets reads finish when creation is refused and
only the sending flag changes. Tests cover both outcomes and verify that the
remaining composer can send.

## F4. A split loses the transcript's scroll position

Medium · Correctness · Fixed · `internal/ui/web/editor/use-scroll-memory.js:17`

A replacement view initialized its scroll ref during render, before the
departing view's cleanup saved its scroll. Splitting a pane containing a worker
scrolled to 500 restored that worker at 0, even though its stored position was
500 with following disabled.

### Fix

The mounting layout effect reads the saved scroll after the previous view's
cleanup. A component regression checks the remounted position and following
state after a split.

## F5. A late parent hides an open request

Medium · Correctness · Fixed · `internal/ui/web/transcript/transcripts.js:19`

The transcript tree reparents children when their tool parent arrives, but
reveal examined only arriving items. An existing open attention item could
therefore move into a closed tool and run. Its selected session had no
notification card to expose the hidden request.

### Fix

As directed by the lead, every update derives reveal from all open attention
items in the resulting transcript. A regression delivers the request before
its parent and checks both enclosing expansion keys.

## F6. Right-panel keyboard behavior differs from the old UI

Medium · Correctness · Skipped · `internal/ui/web/app/right-panel.jsx:20`

The old UI selected tabs with arrows, Home, and End. The installed `vscode-tabs`
moves focus with arrows, selects with Enter, and does not handle Home or End.
This was verified in the dependency's keyboard handler.

### Decision

The lead accepted vscode-tabs' own VS Code behavior and declined an adapter.
The right panel is unchanged.

## F7. Pending lead picker ignores cancellation

Medium · Correctness · Fixed · `internal/ui/web/sidebar/sidebar-menus.js:27`

Cancellation listeners existed only while the fetched ContextMenu was mounted.
An outside press, Escape, or window blur during the initial profile fetch did
not invalidate it, so the menu appeared after cancellation. The old UI's
listeners also covered the pending fetch.

### Fix

The menu owner invalidates pending requests on those events and on unmount.
ContextMenu continues to own closing its visible menu. Regression tests cover
all three cancellation events before the fetch resolves.

## F8. Open Transcript does not focus an active worker

Medium · Correctness · Fixed · `internal/ui/web/workers/workers-panel.jsx:20`

Focus relied on Composer mounting. Opening an already active worker kept its
composer mounted and left focus on the card's action. The old handler
explicitly focused the worker's composer.

### Fix

As directed by the lead, the click handler opens the tab and imperatively
focuses its composer after the redraw. No focus state was added. A component
regression checks an already active worker.

## F9. Ordinary actions duplicate standard controls

Low · Readability · Fixed · `internal/ui/web/agent/composer.jsx:48`,
`internal/ui/web/transcript/message-item.jsx:30`,
`internal/ui/web/transcript/reasoning-item.jsx:8`

Remove Image, Show more/less, and Thinking used custom native buttons despite
the standard-controls requirement and the planned native Thinking disclosure.

### Fix

Remove Image and Show more/less use vscode-button, with their duplicated control
styles removed. Thinking uses native details and summary, retaining its latest
six-line preview while collapsed. Existing transcript tests verify expansion,
streaming text, and message actions.

## F10. CSS repeats literal sizes

Low · Readability · Fixed · `internal/ui/web/base.css:8`

Component styles repeated 1 px borders and 2 px adjustments; the scrollbar's
width and height repeated 10 px. These bypassed the shared size definitions.

### Fix

Following the lead's narrowed decision, repeated values use shared
`--size-border` and `--size-scrollbar` variables, with multiples expressed by
calc. Numeric sizes are unchanged. Values used once, including the 3 px tab
scrollbar and 11 px sidebar heading, remain literal in their rules.

## F11. Build emits an empty JavaScript chunk

Low · Simplicity · Fixed · `internal/ui/build.js:33`

The combined JavaScript and codicon CSS split build emitted an unreferenced,
zero-byte `main-<hash>.js`. Trying cssChunking enabled and disabled did not
remove it.

### Fix

As approved by the lead, JavaScript builds with splitting and the codicon
stylesheet builds separately without splitting. License inputs are collected
from both metafiles. Regenerated output contains only main.js and the highlight
JavaScript chunk; the empty artifact is removed without an output cleanup rule.

## Checks run

- `bun install --frozen-lockfile`: passed.
- Existing browser suite before fixes: 99 passed.
- Focused component and transition suite after fixes: 40 passed.
- `make ui`: passed; generated JavaScript contains no empty file.
- `make check`: passed, including stale-output comparison, 110 browser tests,
  Go builds, vet, formatting, and race tests. The first attempt was blocked by
  sandbox access to the shared Go cache; the rerun used that default cache
  with the required access.
- Source-only `git diff --check`: passed. The whole-output whitespace check
  flags a bundled template literal in generated main.js; generated files were
  left exactly as built.

## Verdict

All ten selected findings are fixed and the required checks pass. F6 is closed
by the lead's decision to keep the dependency's keyboard behavior. Browser
appearance remains for the user's review.
