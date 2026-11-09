# Serve the file browser from the session's working directory

## Goal

The Files tab mockup lists and reads a stub. After this change it shows the
selected session's working directory, read-only: two GET routes list one
folder and read one file, confined to that directory. Expanded folders and
open file tabs update within about a second when they change on disk. The
approved UI and UX stay as they are.

## Related code

- `internal/ui/static/files.js` — the stub `files` object (`list(dir)`,
  `read(path)`), and `FileTree`, which loads a folder's entries once, on first
  expand, in `load`.
- `internal/ui/static/index.html` — `attach` resets the tree for the selected
  session (`attached`); `openFile`, `showFile`, `keepOpen`, `fileTabs`, and
  `renderTabs` own the file tabs and their scroll positions.
- `internal/ui/static/tabs.js` — `TabContainer.render` takes tab entries
  (`label`, `title`, `icon`, `preview`, `keepOpen`).
- `internal/ui/style.css` — `.transcript-tab.preview .tab-label`, `.file-view`.
- `internal/ui/ui.go` — `Handler` registers routes on the mux behind
  `localOnly`; `refused` answers errors.
- `internal/session/session.go` — `Create` gives the lead `Dir: w.Path`, the
  workspace's folder. `Runtime.Shell` (`runtime.go`) opens the session's shell
  in the same folder, found through `tx.Session` and `tx.Workspace`.
- `internal/ui/ui_test.go` — `open`, `handler`, and `change` set up a
  database and server for route tests.
- `internal/ui/files.test.js` — tests the stub.

## Decisions

- **Root.** The session's working directory is its workspace's `Path`, the
  lead's directory and the shell's. The browser already addresses sessions by
  ID (`attached`, `terminal?session=`), so both routes take `session=<id>` and
  the server looks up the path in the database on each request. The handlers
  do not check whether the session is running.
- **Confinement.** Paths are relative, slash-separated, `""` for the root.
  The server opens the directory with `os.OpenRoot` and does every list, stat, and read through the `os.Root`, so
  `..` and symbolic links that lead outside the directory fail. A symbolic link
  inside the directory is listed as a folder or file by what it points to
  (`root.Stat`); a broken or escaping link is listed as a file and fails to
  open.
- **Hidden entries.** VS Code's default `files.exclude` (`.git`, `.svn`,
  `.hg`, `.DS_Store`, `Thumbs.db`) and `.hypercube`, the folder of worker
  worktrees, matched by name at any depth. Only listings filter them; reads
  are confined by `os.Root` alone.
- **Errors.** An unknown session, and a path that does not exist
  (`os.ErrNotExist`), get 404. Other failures, such as a database or
  permission error, are answered through `refused` with their error text.
  During a poll, any failure other than 404 leaves the listing or tab as it
  is.
- **Change detection: polling, no new dependency or transport.** While the
  page is visible and a session is attached, the page polls once a second.
  While the Files panel is shown, it re-lists the root and each expanded
  folder among the visible rows (a folder under a collapsed ancestor is not
  re-listed). It re-requests each visible file tab (`clientHeight > 0`) with
  `If-None-Match`; a tab shown later is refreshed by the next tick. Requests go one at a
  time, so polling never holds more than one of the browser's six
  connections, and a poll is skipped while the previous one runs. Cost per
  tick is one `ReadDir` per expanded folder and one `stat` per open file,
  which is negligible locally. This meets the requirement with existing
  routes; file watching (FSEvents) would add a dependency and a push channel
  over the data socket to save about a second of delay.
- **File ETag.** `"<size>-<mtime in nanoseconds>"`. The read route stats the
  file first and answers 304 without reading when the ETag matches.
- **Large files.** Files over 1 MiB are refused with 413 and the tab shows
  VS Code's message: "The file is not displayed in the text editor because it
  is very large." 1 MiB keeps Shiki highlighting under about a second.
- **Binary files.** A NUL byte in the first 8000 bytes (Git's test) is refused
  with 415 and the tab shows VS Code's message: "The file is not displayed in
  the text editor because it is either binary or uses an unsupported text
  encoding."
- **Unreadable entries.** A file that cannot be opened shows the server's
  error text in its tab. A folder that cannot be listed on first expand shows
  no children.
- **Deleted while open.** As in VS Code, a deleted file's tab keeps its last
  contents and its label is struck through, with "(deleted)" in its title.
  If the file returns, the next poll shows it again. A deleted folder drops
  out of its parent's listing; when its own listing returns 404, the tree
  forgets its entries and expansion.
- **Re-rendering.** The tree re-renders only when a listing differs from the
  one it has. A file tab re-renders on any response other than 304, unless
  it shows the same message it already shows. `showFile` saves and restores
  the view's `scrollTop` and `scrollLeft`. Marking a tab deleted clears its
  ETag, so a file restored with the same size and mtime renders again.
- **Highlight cost.** Measure the time Shiki takes to re-highlight a file
  near the size limit after its grammar is loaded, and lower `maxFile` so a
  re-highlight stays near 100 ms.
- **Reading.** The read route reads through `io.LimitReader(maxFile+1)`, so
  a file that grows after the stat is still refused with 413.

## Test plan

Go, in a new `internal/ui/files_test.go`, with a workspace in `t.TempDir()`
holding `a.go`, `sub/b.md`, `.git/config`, `.DS_Store`, a link to `sub`, a
link to a file outside the workspace, a binary file, and a sparse file one
byte over 1 MiB:

- `GET /files?session=&path=` lists the root's entries with `dir` flags, the
  link to `sub` as a folder, and no `.git` or `.DS_Store`; `path=sub` lists
  `b.md`.
- `GET /file` returns `a.go`'s text with an ETag; the same ETag in
  `If-None-Match` gets 304; after the file changes it gets 200 with the new
  text.
- `..`, `/etc/passwd`, and the escaping link are refused; a
  missing file is 404; the binary file is 415; the large file is 413; a
  folder path on `/file` is 400; an unknown session is 404.

Bun, in `internal/ui/files.test.js`, replacing the stub test: `files.read`
with a stubbed `fetch` maps 200 to text and ETag, 304 to unchanged, 404 to
deleted, and 413 and 415 to VS Code's messages; `files.list` sends the
session and path.

## Implementation plan

1. `internal/ui/files.go` (new): `hidden` (the excluded names), `sessionRoot`
   (session ID to `*os.Root` through `tx.Session` and `tx.Workspace`),
   and the handlers `listFiles` (`GET /files`, JSON array of
   `{name, dir}`) and `readFile` (`GET /file`, `text/plain; charset=utf-8`,
   `Cache-Control: no-cache`, ETag, 304, 413, 415). Constant `maxFile = 1 <<
   20`. Register both routes in `Handler`.
2. `internal/ui/files_test.go` (new): the Go tests above.
3. `internal/ui/static/files.js`: delete the stub and sample data. `files`
   becomes `list(session, dir)` and `read(session, path, etag)`, which
   resolves to `{text, etag}`, `{unchanged: true}`, `{deleted: true}`, or
   `{message}`. `FileTree` takes `list` bound to the session (rebound in
   `reset`), and gains `refresh()`, which re-lists the root and each expanded
   folder among the visible rows in turn, replaces a listing that differs, drops a folder
   whose listing is 404, and renders once if anything changed.
4. `internal/ui/static/index.html`: `showFile` uses `files.read` with the
   tab's ETag and renders text, a message, or the deleted state;
   `fileTabs` entries keep `etag` and `deleted`. Add `poll()`, run by a
   one-second interval: it returns when `document.hidden`, no session is
   attached, or a poll is running; otherwise it awaits `fileTree.refresh()`
   while the Files panel is shown, and then each visible file tab's
   `showFile` in turn, guarding against a
   session change mid-poll as `FileTree.reset` does with `generation`.
   `renderTabs` passes `deleted` and the "(deleted)" title.
5. `internal/ui/static/tabs.js`: toggle a `deleted` class on the tab from
   `entry.deleted`.
6. `internal/ui/style.css`: `.transcript-tab.deleted .tab-label` with
   `text-decoration: line-through`; a style for the tab message, centered as
   VS Code's.
7. `internal/ui/files.test.js`: the bun tests above. Run `make ui` to rebuild
   `static/ui.css`.
