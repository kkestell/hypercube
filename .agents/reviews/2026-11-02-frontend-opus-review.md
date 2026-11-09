# Frontend review, 760eeae..f55f444

Reviewer: frontend-opus-review. Review only; no code changed.

## Conclusion

The frontend is in good shape overall: one store, pure state changes, effects
limited to real side effects, and tests of what users see. Twelve findings
remain; 1-6 block. Most are deletions: three one-caller modules, a duplicated
disclosure, a dead null check, and literal sizes that already have shared
variables. One
is a regression: the file poller's `tabPath` check stopped working when
`tabPath` changed. Another undoes `memo` on two transcript items. Nothing
here is a user-visible failure beyond extra work per render.

## Findings

### 1. The file poller's null check is dead since `tabPath` changed (blocking: misleading code)

- `internal/ui/web/files/file-polling.js:27-28`, `internal/ui/web/editor/editor.js:20-21`
- At 760eeae, `tabPath` returned `null` for a tab that was not a file
  (`id.startsWith('file:') ? … : null`). It now always slices:
  `id => id.slice('file:'.length)`. `file-polling.js` still guards with
  `if (path !== null)`, which is always true now.
- Traced: an active agent tab gives `'agent'.slice(5) === ''` and an active
  Browser tab gives `'browser'.slice(5) === 'er'`. Every poll calls
  `readOpenFile(store, '')` and `readOpenFile(store, 'er')`. These are dropped
  only because `files.open[path]` is undefined. If the session has a
  root-level file named `er` open in another pane, a pane showing the Browser
  tab reads it a second time each second.
- Fix: in `file-polling.js`, write
  `if (pane.active.startsWith('file:')) await readOpenFile(store, tabPath(pane.active));`.
  That replaces two lines with one, and the comment on `tabPath` stays true.

### 2. `memo` on MessageItem and AttentionItem no longer skips anything (blocking: misleading code)

- `internal/ui/web/transcript/message-item.jsx:9-10`, `internal/ui/web/transcript/attention-item.jsx:10-12`
- This range added `useStore()` inside both `memo` components to look up a
  sender's title. In Preact 11.1.1, a Provider whose value changes forces
  every `useContext` subscriber to update (`n.__g|=4, j(n)` in
  `preact.mjs`), even when `memo` would skip it. App's context value is a new
  `[state, store]` on every store update (`app/app.jsx:44`). So each keystroke
  in the composer (a `drafts` update) re-renders every message and attention
  item in the transcript, and the `memo` wrappers claim an optimization that
  no longer happens.
- Fix: look up the sender once in `TranscriptRow`
  (`transcript-item.jsx:23`), which re-renders anyway. Pass it down as a
  string: `sender={item.from && (sessionOf(state, item.from)?.session.title || item.from)}`.
  Remove `useStore` and `sessionOf` from both items. Outgoing messages in
  `ToolRun` need no sender. Expected change: about 3 lines added and 6 removed.

### 3. `shared/popup.js` has one caller (blocking: one-caller abstraction)

- `internal/ui/web/shared/popup.js:1-28`, `internal/ui/web/shared/context-menu.jsx:5,11`
- This range moved ContextMenu's clamping and closing effects into a
  `usePopup` hook in a separate module. ContextMenu is its only user, so a
  reader now follows an extra module to learn how a menu behaves.
- Fix: move the two effects back into `context-menu.jsx` and delete
  `popup.js`. Expected change: one file removed, about 20 lines moved, and one
  import gone.

### 4. `useSidebarMenu` is a one-caller hook that passes Sidebar's state back through itself (blocking: hard to follow)

- `internal/ui/web/sidebar/sidebar-menus.js:20-37`, `internal/ui/web/sidebar/sidebar.jsx:28-33,52`
- With the lead picker gone, the hook only wraps `useState` and returns five
  closures. Sidebar owns `showStopped` and `toggleStopped`, hands them to the
  hook, and gets them back inside `showSession` and `hasSessionMenu`.
  `hasSessionMenu` builds each row's full item list on every render just to
  test whether the list is empty, and `showSession` builds it again on click.
- Fix: Sidebar holds `const [menu, setMenu] = useState(null)` and
  `const closeMenu = useCallback(() => setMenu(null), [])`. Keep `onClose`
  stable and argument-free, because the popup's `blur` listener passes an
  Event. Export `sessionItems(store, session, showStopped, toggleStopped)` and
  `workspaceItems` as plain functions. In `sessionRow`, compute
  `const items = sessionItems(...)` once, then
  `onMenu={items.length ? b => setMenu({...below(b), items}) : null}`. Expected
  change: the hook (about 17 lines) is removed and about 6 lines are added.

### 5. `sidebar/nested-sessions.jsx` has one caller (blocking: one-caller abstraction)

- `internal/ui/web/sidebar/nested-sessions.jsx:1-7`, `internal/ui/web/sidebar/sidebar.jsx:84-87`
- This is a 7-line component used once. It takes a `row` render prop that
  calls back into Sidebar. A reader has to open a second file to see a single
  filter.
- Fix: in `sidebar.jsx`, write
  `const nested = session.children.filter(s => s.running || showStopped[session.id] || s.id === state.selected);`
  and then `{nested.length > 0 && <div class="sidebar-nested">{nested.map(sessionRow)}</div>}`.
  Delete the file. Expected change: 7 lines and one import removed, about 2
  lines added.

### 6. Messages between sessions duplicate the tool-call disclosure (blocking: duplication)

- `internal/ui/web/transcript/message-item.jsx:26-44`, `internal/ui/web/transcript/message-item.css:23-60`, `internal/ui/web/transcript/tool-items.jsx:11-24`, `internal/ui/web/transcript/tool-items.css:8-45`, `internal/ui/web/transcript/reasoning-item.css:5-29`
- This range turned agent messages into a `<details>` element. It has a
  heading, a trailing `chevron-right`, and a one-line preview while closed.
  That is the same structure as `Disclosure` in `tool-items.jsx`. The same
  CSS rules now appear three times, once each in message-item, tool-items,
  and reasoning-item:
  - `display: block`, the row line height, `cursor: pointer`, and `list-style: none`
  - `::-webkit-details-marker { display: none }`
  - the hover color
  - a chevron that does not shrink
  - `[open] … chevron { transform: rotate(90deg) }`
- Fix: move `Disclosure` and its summary and chevron CSS into
  `transcript/disclosure.jsx` and `disclosure.css`. In MessageItem, use
  `<Disclosure open={expanded} onToggle={open => setOpen(item.id, open)} summary={heading and status icons} preview={firstLine}>`.
  Delete the summary, heading, chevron, and preview rules from
  `message-item.css`, about 35 lines. Reasoning-item can share the summary and
  chevron rules too. Update the class names in the `transcript.test.jsx`
  message assertions. Expected change: about 45 lines removed and about 8
  added.

### 7. Module-level `placed` in browser placement (standard: module state outside the store)

- `internal/ui/web/browser/placement.js:9-19`
- `let placed` is module state that only this hook uses. Only one Browser
  `Page` is mounted at a time: the attached session's, because SessionView is
  keyed by session. When it remounts, for example after a split, the old
  Page's cleanup sends `place(null)` before the new Page's layout effect
  sends its own placement. A per-hook ref therefore sends exactly the same
  messages.
- Fix: `const placed = useRef('')` inside `useBrowserPlacement`, with `place`
  as a closure that reads it. Line count stays about the same, and the module
  holds no state.

### 8. Dead literal-color fallback in the split handle styles

- `internal/ui/web/shared/split.jsx:21`
- `var(--vscode-sash-hoverBorder, #0078d4)`: the build writes
  `--vscode-sash-hoverBorder` into `theme.css` (checked in `dist/theme.css`),
  so the fallback never applies, and it is a literal color.
- Fix: `var(--vscode-sash-hoverBorder)`.

### 9. Stale comment: the title bar was deleted

- `internal/ui/web/notifications/notification-stack.jsx:1`
- The comment says "stacked in the top right corner below the title bar", but
  `app/title-bar.jsx` was deleted in this range. The offset in
  `notification-stack.css:3` (`--size-bar-height`) is now the height of the
  editor tab bar.
- Fix: "The notification cards, stacked in the top right corner below the tab
  bars."

### 10. Shared sizes written as literals

- `internal/ui/web/sidebar/sidebar.css:66-67,127-128`, `internal/ui/web/sidebar/session-item.css:15,39-40,97,103,110-111`, `internal/ui/web/files/files-panel.css:16,20,30`
- `base.css` defines `--size-icon: 16px`, `--size-border: 1px`, and
  `--size-space: 4px`. The new sidebar CSS repeats `16px` six times, uses
  `1px` and `-1px` for focus outlines and font-size steps, and
  `files-panel.css` repeats `8px` three times.
- Fix: use `var(--size-icon)`, `var(--size-border)`, and
  `calc(var(--size-space) * 2)`. Line count does not change.

### 11. Stale `agent` field in transcript test messages

- `internal/ui/web/transcript/transcript.test.jsx:18`, `internal/ui/web/transcript/transcripts.test.js:6`
- The test messages still carry `agent: 'a'`. This range removed transcripts
  kept per agent: `receivedTranscript` reads only `session`, and
  `internal/ui/socket.go` sends no agent field.
- Fix: delete `agent: 'a'` from both helpers.

### 12. `post` and `fetchProfiles` repeat the same request handling

- `internal/ui/web/api.js:8-36`, `internal/ui/web/api.test.js:86-102`
- Both functions run the same steps: try `fetch`, read the body, show an error
  and return null when the response is not ok, and show an error and return
  null when `fetch` throws. The tests repeat each failure case for both copies.
- Fix: one `request(store, href, init)` returns the response text or null.
  `post` becomes `request(store, url(path, params), {method: 'POST', body})`,
  and `fetchProfiles` becomes
  `(await request(store, 'profiles'))` passed to `JSON.parse` when not null.
  Test the failure cases once. Expected change: about 10 lines removed from
  `api.js` and about 8 from the tests.

## Checked and not reported

These were examined and set aside, because the change would add code without
an evidenced failure, or the current code already follows the standard
where it matters.

- `editor-tabs.jsx:29-32` moves the focus with `requestAnimationFrame` after
  `store.update`. It works, because Preact renders in a microtask before the
  frame. Moving it into a layout effect needs a focus request that survives a
  closed pane's unmount, which adds code.
- `split.jsx:42` waits on `updateComplete`, the control's documented
  completion promise, not a timing guess.
- `browser-view.jsx` uses plain buttons because `vscode-toolbar-button` has no
  `disabled` property (`vscode-toolbar-button.d.ts`), and Back and Forward
  need one.
- `shared/overlays.js` keeps a registry of DOM elements from more than one
  component. Its contents are DOM handles, not state the store could hold.
- `split.test.jsx` runs its tests in child processes. The comment explains
  why: Bun shares one registry of custom elements across test files.

## Coverage

Each file below was read in full in its current state at f55f444. Deleted
files were read at 760eeae. Generated `internal/ui/dist/*` is out of scope;
`make check` verifies it.

Changed, frontend (`internal/ui/web`):

- `agent/agent-pane.jsx`, `agent/composer.css`, `agent/composer.jsx`,
  `agent/composer.test.jsx`, `agent/drafts.js`, `agent/drafts.test.js`,
  `agent/profile-select.jsx`
- `api.js`, `api.test.js`, `base.css`, `data-socket.js`, `data-socket.test.js`,
  `elements.js`, `main.jsx`
- `app/app.jsx`, `app/app.test.jsx`, `app/layout-toggles.css`,
  `app/layout-toggles.jsx`, `app/right-panel.css`, `app/right-panel.jsx`,
  `app/right-panel.test.jsx`
- `browser/browser-view.css`, `browser/browser-view.jsx`,
  `browser/browser-view.test.jsx`, `browser/browser.js`,
  `browser/browser.test.js`, `browser/placement.js`
- `editor/editor-area.css`, `editor/editor-area.jsx`, `editor/editor-pane.css`,
  `editor/editor-pane.jsx`, `editor/editor-tabs.css`, `editor/editor-tabs.jsx`,
  `editor/editor-tabs.test.jsx`, `editor/editor.js`, `editor/editor.test.js`,
  `editor/tab-layout.js`, `editor/tab-layout.test.js`
- `files/file-icon.css`, `files/file-polling.test.js`, `files/file-view.css`,
  `files/file-view.jsx`, `files/file-view.test.jsx`, `files/files-panel.css`,
  `files/files-panel.jsx`, `files/files.js`, `files/files.test.js`,
  `files/markdown-image.js`
- `notifications/notification-card.jsx`, `notifications/notification-card.test.jsx`,
  `notifications/notification-stack.jsx`, `notifications/notifications.js`,
  `notifications/notifications.test.js`
- `sessions/selection.js`, `sessions/selection.test.js`,
  `sessions/session-actions.js`, `sessions/session-actions.test.js`,
  `sessions/session-view.jsx`, `sessions/session-view.test.jsx`
- `shared/agent-status.jsx`, `shared/confirm-dialog.jsx`, `shared/confirm.js`,
  `shared/context-menu.jsx`, `shared/overlays.js`, `shared/panel-tabs.css`,
  `shared/popup.js`, `shared/split.jsx`, `shared/split.test.jsx`,
  `shared/submit-key.js`, `shared/submit-key.test.js`
- `sidebar/nested-sessions.jsx`, `sidebar/session-item.css`,
  `sidebar/session-item.jsx`, `sidebar/sidebar-menus.js`, `sidebar/sidebar.css`,
  `sidebar/sidebar.jsx`, `sidebar/sidebar.test.jsx`
- `terminal/terminal-panel.css`, `terminal/terminal-panel.test.jsx`,
  `terminal/xterm.js`, `tesseract/tesseract.css`
- `transcript/attention-form.jsx`, `transcript/attention-item.css`,
  `transcript/attention-item.jsx`, `transcript/markdown.css`,
  `transcript/markdown.jsx`, `transcript/markdown.test.jsx`,
  `transcript/message-item.css`, `transcript/message-item.jsx`,
  `transcript/reasoning-item.css`, `transcript/tool-items.css`,
  `transcript/tool-items.jsx`, `transcript/transcript-item.css`,
  `transcript/transcript-item.jsx`, `transcript/transcript-tree.js`,
  `transcript/transcript-tree.test.js`, `transcript/transcript.css`,
  `transcript/transcript.jsx`, `transcript/transcript.test.jsx`,
  `transcript/transcripts.js`, `transcript/transcripts.test.js`

Deleted (read at 760eeae; the only remaining reference is the comment in
finding 9):

- `app/title-bar.css`, `app/title-bar.jsx`, `workers/workers-panel.css`,
  `workers/workers-panel.jsx`, `workers/workers-panel.test.jsx`

Changed, scripts:

- `scripts/dev`, `scripts/dev-seed/main.go`. The frontend-relevant parts are
  `make ui`, the sample files, and the seeded preview session. Nothing to
  report there; the Go and shell lifecycle belongs to the backend review.

Unchanged build and test setup, read in full:

- `Makefile`, `internal/ui/build.js`, `internal/ui/package.json`,
  `internal/ui/bunfig.toml`, `internal/ui/jsconfig.json`,
  `internal/ui/test-dom.js`, `internal/ui/test-render.jsx`
- `bun.lock` was not read line by line. It is unchanged in this range.

Unchanged dependencies, read in full:

- `web/index.html`, `web/store.js`, `web/images.js`, `app/workbench.js`,
  `app/app.css`, `agent/agent-pane.css`, `agent/usage.jsx`, `agent/usage.css`,
  `agent/usage.test.js`
- `editor/use-scroll-memory.js`, `files/file-icon.jsx`,
  `files/file-icon.test.jsx`, `files/file-polling.js`, `files/highlight.js`,
  `files/highlight.test.js`, `files/highlighted-code.jsx`, `files/language.js`,
  `files/language.test.js`
- `notifications/errors.js`, `notifications/notification-card.css`,
  `notifications/notification-stack.css`
- `shared/status-dot.jsx`, `shared/status-dot.css`, `shared/agent-status.css`,
  `shared/confirm-dialog.css`, `shared/context-menu.css`, `shared/split.css`,
  `shared/use-tree-open.js`
- `terminal/terminal-panel.jsx`, `transcript/reasoning-item.jsx`,
  `transcript/transcript-images.jsx`
- Also checked: the Provider source in Preact 11.1.1, the
  `vscode-toolbar-button` type declarations, the `"agent"` field in
  `internal/ui/socket.go`, and `dist/theme.css` for `sash-hoverBorder`.
