# Git Changes panel and diff tabs

## Goal

The Changes panel shows a worktree session's branch, a merge sentence, and an
uncommitted file count, and shows nothing for other sessions. Replace it with
the approved design in `agents/mockups/left-panel.{html,css,js}` (the Changes
panel and the diff tabs; not its sidebar):

- A top row with the session's branch name and, for a worktree session with
  commits not merged into the checkout's branch, an up arrow and that count.
- One flat list of uncommitted files compared with HEAD (staged and unstaged
  together, plus untracked). Each row: a status square (modified, added or
  untracked, deleted), the file name, its folder as dim description, and its
  added and removed line counts.
- It works for every session in a Git workspace.
- Clicking a row opens the file's diff in an editor tab titled like
  `foo.py (Working Tree)`, or selects that tab if it is open. The row whose diff
  tab is active in a pane is marked.
- The diff view is unified: old and new line numbers, a `+`/`-` column, added
  and removed line backgrounds, and syntax highlighting from
  `internal/ui/web/files/highlight.js`.

No change touches the harness plugins, `hypercube hold`, `hypercube mcp`, or how
agents launch, so `make install-service` loads it without restarting sessions.

## Related code

- `internal/workspace/workspace.go` — `git`/`gitWith` run Git and trim output;
  `Branch`; `MergeStatus` and `uncommitted` feed `Merge`.
- `internal/session/merges.go` — `refreshMerges` measures worktree sessions
  after each finished turn; the snapshot carries `Merge` per session
  (`internal/ui/snapshot.go`). Unchanged by this work.
- `internal/ui/files.go` — `sessionRoot`, `fileError`, `readFile` with its
  `maxFile` size limit and binary check; routes registered in
  `internal/ui/ui.go`.
- `internal/ui/web/api.js` — `listFiles` and `readFile`, which turns 413 and 415
  into VS Code's messages.
- `internal/ui/web/files/file-polling.js` — once a second while visible: the
  Files tree when shown, and each pane's active file tab.
- `internal/ui/web/files/files.js` — store updates for listings and open files,
  dropping results for a session no longer attached.
- `internal/ui/web/editor/editor.js` — tab IDs, `tabEntry`, preview tab,
  `closeTab`, `openFileTab`; `editor-pane.jsx` picks a tab's view;
  `editor-tabs.jsx` draws the strip with `FileIcon name={entry.label}`.
- `internal/ui/web/files/highlighted-code.jsx`, `file-view.jsx`,
  `language.js`, `use-scroll-memory.js` — the file view this diff view mirrors.
- `internal/ui/web/sessions/selection.js` — `emptySession`, the attached
  session's state.
- `internal/ui/web/sessions/merge.js` — `mergeSummary`, `uncommittedSummary`,
  `mergeTint` (sidebar).
- `internal/session/children.go` — `CreateChild` accepts any folder as a
  child's directory, which may be a repository subfolder, outside Git, or
  later removed with the child's worktree.
- `internal/ui/web/changes/changes-panel.{jsx,css}` — the panel to replace;
  tested in `internal/ui/web/app/right-panel.test.jsx`.

## Decisions

**Refresh.** The existing visible-page file poller loads the changed-file list
once a second while the right panel shows the Changes tab, and the panel loads
it once when it mounts. Each load runs three Git commands (top level, status,
tracked diff) and reads each untracked file to count its lines, about 26 ms per
poll in this repository. Each pane's active diff tab is re-read on the same
poll, one or two Git calls each, as file tabs are. The ahead count is the
snapshot's `merge.unmerged`, which already refreshes after every finished turn;
it is not polled.

**Two plain routes, no server state.** `GET /changes?session=` returns
`{branch, files}`, or `null` when the session's directory is outside Git;
`GET /diff?session=&path=` returns Git's patch text for one file, whatever its
status. Git work lives in a new `internal/workspace/changes.go` beside the
existing Git helpers; the HTTP side lives in `internal/ui/changes.go` beside
`files.go`.

**Scope: the whole repository.** A child session's `Dir` can be a repository's
top level, a subfolder, a folder outside Git, or a folder that was removed with
its worktree. As in VS Code, the panel shows the whole repository that contains
the session's directory. Both routes:

1. Stat the session's directory. A missing directory answers 404 through
   `fileError`, and the panel shows "The folder no longer exists." in place of
   any earlier list.
2. Find the top level with `workspace.Root(dir)`. When that fails, the folder
   is outside Git: `/changes` answers `null` and the panel shows nothing.
3. Run every Git command at the top level. Every path in the routes and in diff
   tabs is relative to the top level, not to the session's directory; diff tabs
   never go through `/file`, so the two kinds of path never mix.

**Literal paths.** Every Git command that takes a pathspec runs as
`git --literal-pathspecs ...`, so a name such as `x[1].txt` matches only that
file.

**Base commit.** A repository with no commits is a valid workspace. The base is
`HEAD`, or the empty tree (`git hash-object -t tree /dev/null`) when HEAD is
unborn, so staged and untracked files still list and diff.

**Listing.** `workspace.Changes(root) (Changes, error)` returns `{Branch,
Files}` from:

- `git status --porcelain=v2 --branch --untracked-files=all -z`: `# branch.head`
  gives the branch (`(detached)` becomes empty), `# branch.oid (initial)` marks
  an unborn HEAD, and `?` records are the untracked files, each file inside
  untracked folders.
- `git diff <base> --raw --numstat --no-renames -z`: the status letter and line
  counts of every tracked change, staged or not, in one call (the raw records
  come first, then the numstat records in the same order). `A` → `added`, `D`
  → `deleted`, any other letter → `modified`; numstat's `-` (binary) leaves the
  counts out. `--no-renames` shows a rename as a deletion and an addition,
  matching the flat list.

Files sort by path with the same collator the Files tree uses, in the browser.

**Untracked line counts.** Git counts an untracked file's lines only with one
`--no-index` call per file, so `/changes` counts them itself through an
`os.Root` at the top level, matching Git's numstat: a symbolic link (by
`Lstat`) is one added line, its target path; other files use the rule
`readFile` applies, extracted from it as `readText(file, size) ([]byte,
error)` returning `errTooLarge` (over `maxFile`) or `errBinary`, which
`fileError` maps to 413 and 415. Either error leaves the counts out. A final
line without a newline counts as a line.

**Diff.** `workspace.Diff(root, path string, limit int) (string, error)`:

1. `git --literal-pathspecs diff <base> --no-color --no-ext-diff --no-renames -U1000000 -- <path>`.
2. When that is empty and
   `git --literal-pathspecs ls-files --others --exclude-standard -- <path>`
   lists the file, `git diff --no-index --no-color --no-ext-diff -U1000000 --
   /dev/null <path>`, whose exit code 1 means "differs" and is success here.
3. Otherwise the empty string, which the view shows as "No changes" (for
   example, after the file is committed).

Because Git produces every diff and the server decides each time, a diff tab
follows the file's current status on every refresh, and symbolic links and
binary files show as Git shows them. The huge context makes Git return the
whole file in one hunk, as the mockup shows; a file long enough to exceed it
exceeds the limit first. Output containing a `\nBinary files ` line is
refused with 415.

**Bounded output.** Diff output is read through a pipe, at most `limit + 1`
bytes; past `limit`, the command is killed and `Diff` returns
`workspace.ErrTooLarge`, which `/diff` answers with 413. `/diff` passes
`maxFile`. Patch text is not trimmed, since trimming drops a final empty
context line. `workspace.go`'s runner becomes `run(dir, env, stdin, limit,
args...) (string, error)`: untrimmed, bounded when `limit > 0`, returning the
output with a non-zero exit's error so `Diff` can accept exit code 1 for
`--no-index`. `git` and `gitWith` trim its output as today.

**Tabs.** A diff tab's ID is `diff:` + path, beside `file:` + path. `tabPath`
becomes `id.slice(id.indexOf(':') + 1)`. Diff tabs use the same preview tab as
file tabs: a click opens the diff in the preview tab, a double-click on the tab
keeps it, and clicking a row whose diff is open selects it. Diff data lives in
`session.diffs[path] = {text, message}`, beside `session.files.open`. The
title's suffix comes from the file's current row in `session.changes`:
`Working Tree` for a modified file and the status label otherwise (`Added`,
`Untracked`, `Deleted`), as in the mockup; a file no longer listed shows
`Working Tree`. The tab icon is the file's Seti icon, so `tabEntry` gives every
file and diff entry a `name` (the file name) and `EditorTabs` draws
`FileIcon name={entry.name}`.

**Highlighting.** `DiffView` rebuilds the old text (context and removed lines)
and the new text (context and added lines), highlights each once with
`highlight(text, language(name))`, and draws each row with the HTML of its old
or new line number. Plain text shows until `highlight.js` loads, as in
`HighlightedCode`. Because the hunk holds the whole file, both texts are the
whole old and new files, so multi-line tokens highlight correctly.

**Panel states.** `null` (outside Git) shows nothing; 404 (missing folder)
shows "The folder no longer exists."; another failure shows Git's message. The
ahead count shows when `merge.unmerged > 0` and has `mergeSummary(merge)` as
its tooltip and accessible label (`2 commits not merged into main`). A
detached HEAD shows no branch name.

## Test plan

Go (`internal/workspace`, temporary repositories):

- `Changes` reports the branch, a modified file with counts, an unstaged
  deletion, a staged new file as `added`, an untracked file inside an untracked
  folder with its full path, a binary modification without counts, and nothing
  for a clean tree.
- In a repository with no commits, `Changes` lists a staged file as `added` and
  an untracked file, and `Diff` returns the staged file's patch.
- `Diff` of a modified file returns one hunk holding every line, including a
  trailing empty context line; `x[1].txt` diffs only that file when `x1.txt`
  also changed; an untracked file and an untracked symbolic link return Git's
  new-file patch; a committed file returns ""; output over the limit returns
  `ErrTooLarge`.

Go (`internal/ui`, through `Handler` as `files_test.go` does):

- `/changes` from a subfolder session lists repository-relative paths, with
  line counts for an untracked text file and none for an untracked binary file;
  a folder outside Git answers `null`; a removed folder answers 404.
- `/diff` returns the patch; a binary change is 415; an over-limit patch is
  413. `/file` keeps its 413 and 415 answers after the `readText` extraction.

Browser (`bun test` in `internal/ui`):

- `diff.test.js`: `diffRows(patch)` numbers context, removed, and added rows;
  headers before the first `@@` are skipped; a `\ No newline at end of file`
  line is skipped; an empty patch has no rows.
- `editor.test.js`: opening a diff opens a preview tab titled
  `foo.py (Working Tree)`; opening it again selects it; opening a file replaces
  the diff preview and forgets its data and scroll; closing a kept diff tab
  forgets its data.
- `right-panel.test.jsx` (replacing the merge-sentence cases): rows show their
  status label, name, folder, and counts; the ahead count shows only with
  unmerged commits; clicking a row opens its diff tab; the row of the active
  diff tab has `aria-current`; `null` shows nothing; a missing folder shows its
  message.
- `file-polling.test.js`: the Changes list is polled only while the Changes tab
  is shown, and an active diff tab is re-read.
- `diff-view.test.jsx`: rows show old and new numbers, `+`/`-` marks, and
  added and removed classes; an empty patch shows "No changes"; a message shows
  centered as in the file view.

## Implementation plan

1. `internal/workspace/workspace.go`: replace `gitWith`'s body with the
   bounded, untrimmed `run` described above; `git` and `gitWith` trim its
   output.
2. Create `internal/workspace/changes.go`: `ErrTooLarge`; `Change{Path, Status
   string; Added, Removed *int}` with JSON names `path`, `status`, `added`,
   `removed` (`omitempty` on the counts); `Changes{Branch string; Files
   []Change}`; `Changes(root)`, `Diff(root, path, limit)`, and the unexported
   base lookup, as in Decisions. Update the package comment to include listing
   uncommitted files and their diffs. Tests in
   `internal/workspace/changes_test.go`.
3. `internal/ui/files.go`: add `sessionDir(id)` and build `sessionRoot` on it;
   extract `readText` with `errTooLarge` and `errBinary`; map both, and
   `workspace.ErrTooLarge`, in `fileError`; `readFile` uses `readText`.
4. Create `internal/ui/changes.go`: a helper resolving a session to its top
   level (404 for a missing folder, `ok == false` outside Git); `listChanges`
   (`workspace.Changes` plus untracked counts) and `readDiff` (`workspace.Diff`
   with `maxFile`, 415 for binary output, `Cache-Control: no-cache`,
   `text/plain`). Register `GET /changes` and `GET /diff` in `ui.go`. Tests in
   `internal/ui/changes_test.go`.
5. `internal/ui/web/api.js`: `fetchChanges(session)` (throws an error carrying
   the status on failure, like `listFiles`) and `readDiff(session, path)`
   returning `{text}`, `{message}` (the same VS Code messages as `readFile` for
   413 and 415), or `{error}`.
6. Create `internal/ui/web/changes/changes.js`: `loadChanges(store)` storing
   `session.changes` as `null`, `{missing: true}`, `{error}`, or `{branch,
   files}` with files sorted (unchanged results leave the state as it is, as
   `listed` does), and `readOpenDiff(store, path)` storing `text` or `message`
   in `session.diffs[path]` and keeping shown text after a failed read. Both
   drop results for a session no longer attached. Also `statusLabel(status)`
   and `diffTitle(path, status)`.
7. Create `internal/ui/web/changes/diff.js`: `diffRows(patch)` →
   `[{kind: 'context'|'added'|'removed', old, new, text}]` from the `@@` hunk.
8. `internal/ui/web/editor/editor.js`: add `diffTabId`; generalize `tabPath`;
   factor a `forgetTab(session, id)` that removes a file's or diff's data and
   its scroll, used by `closeTab` and by preview replacement; generalize
   `fileTabOpened` into `tabOpened(state, id, data)` used by `openFileTab` and a
   new `openDiffTab(store, path)` that stores `{text: null, message: null}` and
   calls `readOpenDiff`. `tabEntry` returns diff entries (`diff: true`, `name`,
   label and title from `diffTitle` with the status of the path's row in
   `session.changes`) and adds `name` to file entries. Update the module
   comment.
9. `internal/ui/web/sessions/selection.js`: `emptySession` gains
   `changes: null` and `diffs: {}`.
10. Create `internal/ui/web/changes/diff-view.{jsx,css}`: `DiffView({id,
    path})` with `useScrollMemory`, the rows grid from the mockup's
    `.mockup-diff` styles (renamed `diff-view-*`), the file view's centered
    message style, and highlighting as in Decisions. `editor-pane.jsx` shows
    it for diff entries; `editor-tabs.jsx` uses `entry.name` for `FileIcon`.
11. Rewrite `internal/ui/web/changes/changes-panel.{jsx,css}` from the
    mockup's `.mockup-scm-*` markup and styles (renamed `changes-panel-*`):
    branch row, `<ul>` of row buttons with `vscode-icon` `diff-modified`,
    `diff-added` (added and untracked), or `diff-removed` in the editor
    gutter colors, name, folder, and `+N`/`−N` counts with an accessible
    label; the row title is `path • Label`. `aria-current` marks rows whose
    diff tab is some pane's active tab. The panel states from Decisions. Load
    once on mount with an effect keyed by session ID.
12. `internal/ui/web/files/file-polling.js`: poll `loadChanges` when the right
    panel shows the Changes tab; re-read active diff tabs with `readOpenDiff`.
    Update its comment.
13. `internal/ui/web/sessions/merge.js`: delete `uncommittedSummary`, now
    unused.
14. Update the tests listed in the test plan.

## Documentation updates

- `docs/usage.md`: replace the description of Changes (branch, merge state,
  uncommitted count, blank for other sessions) with the branch row and
  unmerged count, the uncommitted file list of the repository containing the
  session's folder, and clicking a file to open its diff in a tab.
