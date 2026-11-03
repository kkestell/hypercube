# Frontend review — frontend-sol-review

Reviewer: frontend-sol-review. Baseline: `760eeae`. Reviewed HEAD:
`f55f444d68615693cbd26f08702aaf1e9c599162`.

## Findings

### 1. [P2] Recheck browser coverage when an overlay changes size

Location: `internal/ui/web/shared/overlays.js:15` (also
`internal/ui/web/browser/placement.js:36`). New behavior.

Violation: overlays must remain usable above the native browser; DOM side
effects need a lifecycle that tracks the geometry they depend on.

Evidence: `useOverlay` calls `changed()` only when its wrapper mounts or
unmounts. The real `vscode-context-menu` renders its contents later, in a Lit
update. Opening a menu just above the Browser body can therefore register a
zero-height wrapper that does not intersect the body; the rendered menu then
extends into it without another placement update. The browser's observer
watches only its own body. `HCBrowsers.place` draws the native WKWebView above
the UI, so the overlapping menu portion is covered and cannot receive clicks.
Sidebar menu state is local, so opening the menu does not itself cause the
Browser component to render again after the menu's Lit update.

A focused check used the real menu control with controlled bounding boxes:
Browser `(200, 80, 400, 300)`, menu `(150, 60, 160, 0)` before its update and
height `50` afterward. After `await menu.updateComplete`, `covered()` changed
from `false` to `true`, but the last native message still placed session `s`
instead of hiding it. No screenshots or visual judgments were used.

Smallest fix: observe each registered overlay with a `ResizeObserver` that
calls `changed`, and disconnect it on cleanup. Expected production change:
about 3 added lines, no removed lines. The demonstrated inaccessible menu
justifies this additional observation; no new registry or recovery mechanism
is needed.

### 2. [P2] Restore focus when a context menu is dismissed with Escape

Location: `internal/ui/web/shared/popup.js:16` (menu owner:
`internal/ui/web/shared/context-menu.jsx:8`). Existing behavior retained in the
newly extracted module; included because the review covers whole touched files.

Violation: the requested complete keyboard accessibility includes focus return.

Evidence: the real VS Code menu focuses its shadow wrapper when `show` is set.
The Escape handler only unmounts it. Neither hook nor component remembers or
focuses the invoker. A focused check mounted the real control after focusing
an existing button, awaited its update, and pressed Escape. Focus moved from
`VSCODE-CONTEXT-MENU` to `BODY`; the original button remained connected.
Keyboard users lose their place in the session/workspace controls.

Smallest fix: retain the invoker before the menu takes focus and restore it
when Escape dismisses the menu. Keep outside-pointer dismissal from overriding
the pointer's new focus target. Expected change: roughly 5–8 added lines and
1 replaced handler line, plus one focused regression test. The observed focus
loss justifies the added code.

### 3. [P2] Label the actual controls inside VS Code form elements

Locations: `internal/ui/web/agent/profile-select.jsx:30`,
`internal/ui/web/transcript/attention-form.jsx:32`, and
`internal/ui/web/sidebar/session-item.jsx:79`. The profile selector is new;
the two text fields are existing omissions in touched files.

Violation: form controls need accessible names.

Evidence: `VscodeSingleSelect` renders its focusable `role="combobox"` with
`aria-label` taken from its `label` property, which defaults to an empty
string. Setting `aria-label="Profile"` on the outer custom element does not
set that property. A focused check with the real control produced host label
`Profile` and inner combobox label `""`. Happy DOM lacks `attachInternals`, so
that check supplied no-op form-association methods; label rendering was the
unmodified library implementation. `VscodeTextfield` similarly sets its
inner input's accessible name from `label`; neither the attention-answer field
nor the rename field sets it or has an associated `vscode-label`. The nearby
question and the current title value do not name those inputs.

Smallest fix: set the controls' supported `label` properties (for example,
`Profile`, an answer label using the question, and `Session title`), or connect
real `vscode-label` elements. Expected production change: replace the profile
selector's one attribute and add one property to each text field; no new hook
or component. Verify the inner control's name, rather than the host attribute.

### 4. [P2] Expose the selected session in the sidebar's semantics

Location: `internal/ui/web/sidebar/session-item.jsx:34`.

Violation: the redesigned sidebar must expose selection accessibly, not only
through color.

Evidence: `selected` now changes only `session-item-selected`, which CSS uses
to paint the row. The title button exposes no `aria-current`, selected state,
or relationship to the shown transcript. Inspecting the resulting DOM shows
identical button semantics for selected and unselected sessions. The replaced
`vscode-tree-item` received `selected`, and its implementation reflected that
as `ariaSelected`. A screen-reader user can no longer identify the session
whose transcript is open from these controls.

Smallest fix: add `aria-current={selected ? 'true' : undefined}` to the title
button. Expected change: one attribute, no extra state or keyboard handler;
add an assertion to the existing selected-row test. This is a concrete loss
of selection information, not a proposed tree redesign.

### 5. [P2] Reuse the existing toolbar controls and theme sizes

Locations: `internal/ui/web/browser/browser-view.jsx:33` and
`internal/ui/web/browser/browser-view.css:30` (also `:60`).

Violation: the explicit frontend standard requires VS Code elements for
standard controls, colors from `--vscode-*`, and shared sizes defined once.

Evidence: the new browser toolbar creates four plain icon buttons and
reimplements their layout, hover, focus, disabled opacity, border, radius,
and spacing in about 30 lines of CSS. The app already imports and uses
`vscode-toolbar-button`, with `icon`, `label`, `title`, and `disabled` support,
for these exact responsibilities. The stylesheet also introduces repeated
`0.3em` gaps and a literal `white` background. These are direct departures
from the requested control/theme standard, rather than a visual preference.

Smallest fix: use the existing toolbar buttons with the same actions and
labels, retain only container/address placement rules, use shared size
variables for spacing, and use the editor theme variable for the body behind
the native browser. Expected change: about 4 replaced button expressions,
25–35 CSS lines removed, and a few variable substitutions. Preserve the
existing navigation behavior; no new component or abstraction is needed.

### 6. [P2] Keep shared overlay state under the existing store owner

Locations: `internal/ui/web/shared/overlays.js:6` and
`internal/ui/web/browser/placement.js:9`.

Violation: the explicit frontend standard permits shared mutable state only
in the store, with component-owned handles as the exception.

Evidence: `overlays` and `listeners` are mutable module-level sets shared by
menus, notification stacks, and browser placement. They form a second state
owner and a second subscribe/notify/unsubscribe path alongside `createStore`.
The module-level `placed` string similarly outlives the Browser component
whose placement it caches. These are direct ownership departures visible in
the source, not a hypothetical concurrency failure. A reader must now trace
two independent notification systems to determine whether the browser hides.

Smallest fix: keep overlay rectangles in the existing store through pure
registration/removal changes, with each overlay's measurement observer owned
by its hook. Derive browser coverage from that state and retain placement
deduplication in the Browser hook's ref. Remove the extra listener set,
`changed`, `onOverlaysChange`, and their subscription cleanup. Expected change:
roughly 20–25 lines removed and 15–20 added across the hook, initial store
shape, and placement. This can share the geometry observation required by
finding 1; it does not need another provider, event bus, or manager.

## Verification and coverage

`git diff --name-only 760eeae HEAD` established the complete change inventory.
The exact in-scope inventory below contains 108 paths: 103 present files read
in full at the reviewed HEAD, and 5 deleted files read in full from the
baseline. No other reviewers' findings or reports were consulted. No source,
configuration, generated bundle, AGENTS.md, or skill file was edited or staged.

Focused verification:

- `bun test web/browser/browser-view.test.jsx web/agent/composer.test.jsx web/sidebar/sidebar.test.jsx`
  from `internal/ui`: 28 passed, 0 failed. Those tests render most controls as
  plain elements, so they do not exercise the real controls' asynchronous
  geometry or shadow-control labels.
- Three ad hoc Bun checks exercised overlay geometry after the real menu's
  update, menu Escape focus return, and the real profile selector's label.
  Their observed results are recorded with findings 1–3. No checks or test
  artifacts were added to the repository.
- Broad `make check`, live dev launches, installation, and visual checks were
  not run, as required for this review-only assignment.

Supporting code inspected: `Makefile`; `internal/ui/build.js`, `package.json`,
`bunfig.toml`, `jsconfig.json`, `test-dom.js`, and `test-render.jsx`;
`internal/ui/web/store.js`, `images.js`, `app/app.css`, `app/workbench.js`,
`agent/agent-pane.css`, `agent/usage.jsx`, `files/file-polling.js`,
`files/highlighted-code.jsx`, `files/highlight.js`, `files/file-icon.jsx`,
`files/language.js`, `editor/use-scroll-memory.js`, `notifications/errors.js`,
`notifications/notification-card.css`, `notifications/notification-stack.css`,
`shared/status-dot.jsx`, `shared/split.css`, `shared/context-menu.css`,
`shared/confirm-dialog.css`, `shared/use-tree-open.js`,
`terminal/terminal-panel.jsx`, `transcript/attention-form.css`,
`transcript/reasoning-item.jsx`, and `transcript/transcript-images.jsx`;
`internal/ui/snapshot.go`, `internal/ui/socket.go`,
`internal/desktop/browser.m`, and `Runtime.AddWorkspace`;
and relevant implementations in the locked VS Code elements dependency.
Unchanged build/package files were supporting reads; generated `dist/` and
Go runtime/plugin changes are outside the assigned frontend source scope.

Exact scoped coverage (`A` added, `M` modified, `D` deleted):

```text
M	internal/ui/web/agent/agent-pane.jsx
M	internal/ui/web/agent/composer.css
M	internal/ui/web/agent/composer.jsx
M	internal/ui/web/agent/composer.test.jsx
M	internal/ui/web/agent/drafts.js
M	internal/ui/web/agent/drafts.test.js
A	internal/ui/web/agent/profile-select.jsx
M	internal/ui/web/api.js
M	internal/ui/web/api.test.js
M	internal/ui/web/app/app.jsx
M	internal/ui/web/app/app.test.jsx
A	internal/ui/web/app/layout-toggles.css
A	internal/ui/web/app/layout-toggles.jsx
M	internal/ui/web/app/right-panel.css
M	internal/ui/web/app/right-panel.jsx
A	internal/ui/web/app/right-panel.test.jsx
D	internal/ui/web/app/title-bar.css
D	internal/ui/web/app/title-bar.jsx
M	internal/ui/web/base.css
A	internal/ui/web/browser/browser-view.css
A	internal/ui/web/browser/browser-view.jsx
A	internal/ui/web/browser/browser-view.test.jsx
A	internal/ui/web/browser/browser.js
A	internal/ui/web/browser/browser.test.js
A	internal/ui/web/browser/placement.js
M	internal/ui/web/data-socket.js
M	internal/ui/web/data-socket.test.js
M	internal/ui/web/editor/editor-area.css
M	internal/ui/web/editor/editor-area.jsx
M	internal/ui/web/editor/editor-pane.css
M	internal/ui/web/editor/editor-pane.jsx
M	internal/ui/web/editor/editor-tabs.css
M	internal/ui/web/editor/editor-tabs.jsx
M	internal/ui/web/editor/editor-tabs.test.jsx
M	internal/ui/web/editor/editor.js
M	internal/ui/web/editor/editor.test.js
M	internal/ui/web/editor/tab-layout.js
M	internal/ui/web/editor/tab-layout.test.js
M	internal/ui/web/elements.js
M	internal/ui/web/files/file-icon.css
M	internal/ui/web/files/file-polling.test.js
M	internal/ui/web/files/file-view.css
M	internal/ui/web/files/file-view.jsx
A	internal/ui/web/files/file-view.test.jsx
M	internal/ui/web/files/files-panel.css
M	internal/ui/web/files/files-panel.jsx
M	internal/ui/web/files/files.js
A	internal/ui/web/files/files.test.js
A	internal/ui/web/files/markdown-image.js
M	internal/ui/web/main.jsx
M	internal/ui/web/notifications/notification-card.jsx
M	internal/ui/web/notifications/notification-card.test.jsx
M	internal/ui/web/notifications/notification-stack.jsx
M	internal/ui/web/notifications/notifications.js
M	internal/ui/web/notifications/notifications.test.js
M	internal/ui/web/sessions/selection.js
M	internal/ui/web/sessions/selection.test.js
M	internal/ui/web/sessions/session-actions.js
A	internal/ui/web/sessions/session-actions.test.js
M	internal/ui/web/sessions/session-view.jsx
A	internal/ui/web/sessions/session-view.test.jsx
M	internal/ui/web/shared/agent-status.jsx
M	internal/ui/web/shared/confirm-dialog.jsx
A	internal/ui/web/shared/confirm.js
M	internal/ui/web/shared/context-menu.jsx
A	internal/ui/web/shared/overlays.js
A	internal/ui/web/shared/panel-tabs.css
A	internal/ui/web/shared/popup.js
M	internal/ui/web/shared/split.jsx
M	internal/ui/web/shared/split.test.jsx
A	internal/ui/web/shared/submit-key.js
A	internal/ui/web/shared/submit-key.test.js
A	internal/ui/web/sidebar/nested-sessions.jsx
M	internal/ui/web/sidebar/session-item.css
M	internal/ui/web/sidebar/session-item.jsx
M	internal/ui/web/sidebar/sidebar-menus.js
M	internal/ui/web/sidebar/sidebar.css
M	internal/ui/web/sidebar/sidebar.jsx
M	internal/ui/web/sidebar/sidebar.test.jsx
M	internal/ui/web/terminal/terminal-panel.css
A	internal/ui/web/terminal/terminal-panel.test.jsx
M	internal/ui/web/terminal/xterm.js
M	internal/ui/web/tesseract/tesseract.css
M	internal/ui/web/transcript/attention-form.jsx
M	internal/ui/web/transcript/attention-item.css
M	internal/ui/web/transcript/attention-item.jsx
M	internal/ui/web/transcript/markdown.css
M	internal/ui/web/transcript/markdown.jsx
M	internal/ui/web/transcript/markdown.test.jsx
M	internal/ui/web/transcript/message-item.css
M	internal/ui/web/transcript/message-item.jsx
M	internal/ui/web/transcript/reasoning-item.css
M	internal/ui/web/transcript/tool-items.css
M	internal/ui/web/transcript/tool-items.jsx
M	internal/ui/web/transcript/transcript-item.css
M	internal/ui/web/transcript/transcript-item.jsx
M	internal/ui/web/transcript/transcript-tree.js
M	internal/ui/web/transcript/transcript-tree.test.js
M	internal/ui/web/transcript/transcript.css
M	internal/ui/web/transcript/transcript.jsx
M	internal/ui/web/transcript/transcript.test.jsx
M	internal/ui/web/transcript/transcripts.js
M	internal/ui/web/transcript/transcripts.test.js
D	internal/ui/web/workers/workers-panel.css
D	internal/ui/web/workers/workers-panel.jsx
D	internal/ui/web/workers/workers-panel.test.jsx
M	scripts/dev
A	scripts/dev-seed/main.go
```
