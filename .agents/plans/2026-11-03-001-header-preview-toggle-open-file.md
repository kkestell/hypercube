# Window header, preview/source toggle, and the open_file tool

## Goal

- A header across the top of the window holds the layout toggles, which leave
  the top-right tab bar.
- Each tab bar whose active tab is a file with a preview (today, Markdown)
  shows a button that switches that file between its preview and its source.
  A file opens in its preview.
- Agents get an `open_file` tool that opens a file from their session's
  directory in a kept, selected tab, optionally highlighting line ranges and
  scrolling to the first. A Markdown file with highlighted lines opens in its
  source; any other Markdown file opens in its preview. The file opens in the
  page that views the agent's session; while no page views it, the request
  waits, and it opens when the user next views that session.

## Related code

- `internal/mcp/mcp.go` — `newServer` registers each session's tools and reads
  the caller's `store.Session`, whose `Dir` is its working directory.
- `internal/session/feed.go` — `Change` and `feed.publish`; an empty `Change`
  tells each data socket its state is stale.
- `internal/session/runtime.go` — `Runtime` and the fields `mu` guards.
- `internal/ui/socket.go` — the data socket: `selectSession` runs when the page
  names its session, `flush` runs after each burst of changes.
- `internal/ui/files.go` — `sessionRoot` and `readFile`: the page reads files by
  path relative to the session's directory through `os.Root`.
- `internal/ui/web/data-socket.js` — dispatches server messages by `type`.
- `internal/ui/web/editor/editor.js` — tab state: `tabEntry`, `fileTabOpened`,
  `openFileTab`, `closeTab`, `keepOpen`.
- `internal/ui/web/files/files.js` — `session.files.open[path]` and `fileRead`.
- `internal/ui/web/files/file-view.jsx` — chooses the Markdown preview or the
  highlighted source by language.
- `internal/ui/web/editor/editor-tabs.jsx`, `editor-pane.jsx`,
  `editor-area.jsx`, `tab-layout.js` — the `toggles` prop and `cornerPane`
  exist only to put `LayoutToggles` in the top-right tab bar.
- `internal/ui/web/app/app.jsx`, `layout-toggles.jsx`, `layout-toggles.css`.

## Decisions

- **Header.** A `<header class="app-header">` in `App`, above `.app-body`,
  holding `LayoutToggles` at its right, `--size-bar-height` tall, colored with
  `--vscode-titleBar-activeBackground`, `--vscode-titleBar-activeForeground`,
  and a bottom border of `--vscode-titleBar-border`. It is markup in `app.jsx`,
  not its own component, because it holds one child. Moving the toggles out of
  the tab bar deletes `cornerPane`, the `corner` and `toggles` props, and their
  tests.
- **Toggle in every pane.** Each pane's tab bar shows the button when its own
  active tab has a preview, as VS Code shows editor actions per editor group.
  This replaces the corner-pane rule rather than keeping it for the new
  button, which would preserve `cornerPane` and hide the button for a Markdown
  file in any other pane.
- **Where the mode lives.** Each open file already has one entry,
  `session.files.open[path]`, which `closeTab` and preview-tab replacement
  delete. It gains `source` (show the source instead of the preview), `lines`
  (highlighted ranges, `[{start, end}]`), and `reveal` (scroll to the first
  range once the text arrives). `fileRead` spreads the existing entry so a read
  keeps these fields. The existing word "preview" also names the replaceable
  tab (`tabs.preview`); the new state uses `source` so the two never share a
  name.
- **`hasPreview(name)`** in `files/language.js`, true for Markdown, is the one
  rule both `FileView` and the tab bar use.
- **Highlights.** In the source view, each range is an absolutely positioned
  `.file-view-highlight` inside `.file-view`, with
  `top: calc((var(--start) - 1) * 1lh)` and `height: calc(var(--count) * 1lh)`,
  sharing the code's font and line height, colored
  `--vscode-editor-rangeHighlightBackground`. `.file-view` gets
  `position: relative; isolation: isolate` and the highlights `z-index: -1`,
  so they scroll with the code and paint under its text. A highlight lasts
  while the tab is open, until the agent opens the file again; the preview
  shows none.
- **Reveal.** `FileView` runs a layout effect on `file.text`, `file.reveal`,
  and the view mode: when `text !== null`, reveal is set, and a highlight
  exists it calls `scrollIntoView({block: 'center'})` on the first highlight
  and only then clears `reveal` through the store. It runs after
  `useScrollMemory` restores the remembered scroll, so it wins once and later
  remounts keep the user's scroll.
- **Waiting requests live in the runtime.** `Runtime` gains
  `opens map[string][]FileOpen`, guarded by `mu`. `OpenFile(id, open)` appends
  and publishes an empty `Change`, which already wakes every data socket.
  `TakeFileOpens(id)` returns and removes the session's requests. The data
  socket takes them in `selectSession` (after the transcript) and in `flush`,
  for its selected session, and sends each as
  `{"type":"open","session":id,"path":…,"lines":[…]}`. The first page that
  views the session receives them; a request is in memory only and does not
  survive an app restart. A request received while another session is attached
  waits in the page store, keyed by session, and opens when that session is
  next attached.
- **Path and line validation.** The tool accepts a path relative to the
  caller's `Dir`, or an absolute path inside it, which it makes relative with
  `filepath.Rel`. It reads the file through `os.OpenRoot(Dir)`, so `..` and
  symlinks that leave the directory, missing files, and folders are refused
  with the `os.Root` error. Each range needs `1 <= start <= end <= line count`,
  where an omitted `end` is `start` and the line count is
  `bytes.Count(content, "\n") + 1`, the count the source view shows. The
  queued path is `filepath.ToSlash` of the cleaned relative path, the form the
  Files tree uses. `lines` is never nil, so the page always receives an array.
- **Tool availability.** `open_file` is registered for every session, before
  the child-only early return in `newServer`.
- **Restart.** The app serves the tool, but each harness reads its tool list
  when its agent starts, so running agents get `open_file` only after their
  sessions restart. The header and toggle load with `make install-service`.

## Test plan

- `internal/mcp`, `TestOpenFile`, a table over one session's directory:
  relative and absolute paths inside it queue a `FileOpen` with the slash
  relative path (read back with `rt.TakeFileOpens`); `../x`, an absolute path
  outside, a missing file, a folder, `start` 0, `end < start`, and a range past
  the last line fail with an error result and queue nothing. `TestTools`
  lists `open_file` for top-level and child sessions. An MCP call with
  `lines:[{start:1}]` succeeds and queues `end:1`.
- `internal/ui`, `TestFileOpensReachTheViewingPage`: an open for a session no
  page selects sends nothing; selecting it delivers the `open` message; a
  second open while selected arrives without reselecting; a second page that
  selects the session afterwards receives none.
- `data-socket.test.js`: an `open` message for the attached session opens a
  kept, selected tab and reads the file; one for another session waits and
  opens when the page returns to that session.
- `editor.test.js`: `openRequestedFile` opens a kept tab beside the chat and
  selects it; it keeps an existing preview tab of the same file; a Markdown
  file with lines opens with `source` set and without lines in its preview; a
  repeat open replaces `lines` and sets `reveal`.
- `file-view.test.jsx`: a Markdown file shows its source when `source` is set;
  highlighted ranges render one highlight each; once the text arrives the first
  highlight scrolls into view and `reveal` clears.
- `editor-tabs.test.jsx`: replace the corner-pane test: the button shows only
  in panes whose active tab is a Markdown file, and clicking it switches
  between preview and source and its label between "Show Source" and "Show
  Preview". Agent and browser tabs never show the button.
- `app.test.jsx`: the header holds the three layout toggles, and they show and
  hide their regions.
- `tab-layout.test.js`: delete the `cornerPane` case.

## Implementation plan

1. `internal/session/opens.go` (new): `LineRange {Start, End int}` with JSON
   `start`/`end,omitempty`; `FileOpen {Path string; Lines []LineRange}` with JSON
   `path`/`lines`; `Runtime.OpenFile` and `Runtime.TakeFileOpens`. In
   `runtime.go`, add the `opens` field under `mu` and create it in `New`.
2. `internal/session/feed.go`: change the `Change` comment's "Other writes" to
   "Other changes".
3. `internal/mcp/mcp.go`: `openFileArgs {Path string; Lines []session.LineRange}`
   (`lines` omitempty), and an `openFile(rt, dir, self, args)` function that
   validates as decided and calls `rt.OpenFile`. Register `open_file` with the
   description "Open a file from your directory in a tab beside your chat,
   which the user sees, and select it. path is relative to your directory or
   absolute inside it. lines highlights ranges of lines, each {start, end}
   with end optional, and scrolls to the first; a Markdown file with lines
   shows its source, otherwise its preview. The tab opens now if the user is
   viewing your session, or the next time they do." It returns
   "<path> opens in a tab in your session's view."
4. `internal/ui/socket.go`: `openMessage {Type, Session, Path, Lines}`;
   `dataSocket.sendOpens()` takes and sends the selected session's opens;
   call it at the end of `selectSession` and `flush`.
5. `internal/ui/web/files/language.js`: `hasPreview(name)`.
6. `internal/ui/web/files/files.js`: `fileRead` spreads the existing entry;
   export `setSource(state, path, source)` and
   `revealed(state, sessionId, path)`.
7. `internal/ui/web/editor/editor.js`: `unread` gains
   `source: false, lines: [], reveal: false`. Add
   `openRequestedFile(store, {session, path, lines})`: queued in the page store
   by session unless the attached session is `session`; when attached, it
   clears `tabs.preview` if it is
   this file's tab, opens with `layout.openBeside(tabs, id, agentTabId)`, sets
   `files.open[path]` to the existing entry or `unread` with
   `source: hasPreview(path) && lines.length > 0`, `lines`, and
   `reveal: lines.length > 0`, then calls `readOpenFile`. `tabEntry` adds
   `source` to file entries: the file's `source` flag when `hasPreview`,
   otherwise `null`.
8. `internal/ui/web/data-socket.js`: dispatch `type === 'open'` to
   `openRequestedFile`; mention open requests in the header comment.
9. `internal/ui/web/files/file-view.jsx` and `file-view.css`: choose the
   preview when `hasPreview` and not `file.source`; render the highlights and
   the reveal effect as decided.
10. `internal/ui/web/editor/editor-tabs.jsx` and `editor-tabs.css`: drop
    `toggles` and the `LayoutToggles` import; when the active entry's `source`
    is a boolean (`typeof active?.source === 'boolean'`), render a
    `vscode-toolbar-container class="editor-tabs-actions"`
    with one `vscode-toolbar-button`: icon `go-to-file` and label "Show Source"
    in the preview, icon `open-preview` and label "Show Preview" in the source,
    calling `setSource`. Move the toolbar styling from `layout-toggles.css`
    that the tab bar used (`margin-left: auto`, padding, the muted icon colors,
    the focus border) to `.editor-tabs-actions`.
11. `internal/ui/web/editor/editor-pane.jsx`, `editor-area.jsx`,
    `tab-layout.js`: remove `toggles`, `corner`, and `cornerPane`, and their
    comments.
12. `internal/ui/web/app/app.jsx` and `app.css`: add the header with
    `LayoutToggles`; `layout-toggles.css` keeps only what the header needs and
    drops `margin-left: auto` in favor of the header's `justify-content: flex-end`.
13. Tests as listed in the test plan.

## Documentation updates

- `docs/usage.md`: "The buttons at the right of the top-right tab bar" becomes
  the buttons at the right of the header at the top of the window. After the
  Files sentence, add that a Markdown file opens in its preview and the button
  at the right of its tab bar switches to its source and back, and that an
  agent can open a file from its directory with `open_file`, highlighting
  lines, which opens when you view its session.
- `docs/introduction.md`: "at the right of the top tab bar" becomes "in the
  header at the top of the window".
- `docs/architecture.md`: after the browser paragraph, one sentence that
  `open_file` requests wait in the app until a page views the agent's session
  and arrive over that page's data socket.
