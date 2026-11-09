# Read a shown file or diff tab immediately

## Goal

Switching to a session whose active tab is a file or diff shows that tab's
content without waiting for the next file poll.

A session's open files and diffs are saved without their content. When the
page attaches to the session again, `restoreView` restores them unread, and
nothing reads the active tab until the 1-second poller runs. The poller then
lists the tree, waits for `/changes` (30–60 ms of Git), and only then reads
the file. The content therefore appears 0–1000 ms after the click, plus about
60 ms.

Measured on the installed build (601b07a) in its own desktop app, with a copy
of the user's database and saved view state, switching to the Docs session
with `agents/development.md` active: the file text appeared after 84, 182, 257,
395, 536, 662, 918, and 946 ms (mean about 500 ms). The `/file` request always
started right after a poll's `/changes` response. The other work in the switch
is small. The transcript tab of the same session is fully drawn at 100–130 ms,
cold or warm. Client main-thread work is 25–45 ms per switch. Selecting a
restored, unread tab inside a session waits for the poll in the same way.

When the work is done, the content of a shown file or diff is requested as
soon as its tab is shown unread. For the Docs case, the content should appear
about 70 ms after the click instead of 500 ms on average.

## Related code

- `internal/ui/web/sessions/view-state.ts` — `restoreView` restores open files
  as `unreadFile` and diffs as `{text: null, message: null}`.
- `internal/ui/web/files/file-polling.ts` — refreshes the shown tree,
  changes, and each pane's active file or diff once a second, one request at a
  time.
- `internal/ui/web/files/file-view.tsx`, `internal/ui/web/changes/diff-view.tsx`
  — the views of the active file or diff tab. `EditorPane` mounts only each
  pane's active tab.
- `internal/ui/web/files/files.ts` `readOpenFile` and
  `internal/ui/web/changes/changes.ts` `readOpenDiff` — versioned reads that
  ignore superseded responses and responses for a previous attachment.
- `internal/ui/web/editor/editor.ts` — `openFileTab`, `openDiffTab`, and
  `openRequestedFile` set a file or diff unread and read it themselves.

## Decisions

- The view owns the first read. `FileView` reads its file when the file is
  unread: no text, message, preview, or deletion. `DiffView` reads its diff when
  it has no text or message. Each uses an effect keyed by session ID, path, and
  that unread condition. This covers attaching to a session, selecting a
  restored tab, moving a tab into a new pane, and opening a file, with one rule.
- `openFileTab` and `openDiffTab` leave the first read to the shown view,
  but still refresh an already-loaded tab on an explicit click.
  `openRequestedFile` likewise refreshes a loaded file after an agent's change. A new or unread file is read by its view. Readiness includes
  empty text, messages, previews, and deleted files.
- The poller is unchanged and keeps refreshing shown content once a second.
- Rejected approaches:
  - Running the poller on attach. The file read would still wait behind
    `/changes`, the tree and changes requests would duplicate the Files panel's
    attach effect, and selecting a restored tab would not be covered.
  - A shorter poll interval. It costs a Git status per second per tick and
    still adds delay.
  - Store, sidebar, transcript caching, or virtualization changes. The
    measurements above do not show a cost there that is large enough.

## Test plan

- `FileView` mounted for an attached session with a restored unread Markdown
  file requests `/file` once and shows its text without a poll. A file that
  already has text is not requested on mount.
- `DiffView` mounted with an unread diff requests `/diff` once and shows it.
- Opening a file into a mounted pane (`openFileTab` with the editor pane
  rendered) makes exactly one `/file` request. Clicking a Markdown link in a
  file view still opens the target and shows its text.
- `openRequestedFile` for a file that already has content requests it again.
- Existing editor, file view, and poller tests pass after their expectations
  are moved to the view where they relied on the open actions' reads.
- Manual check with `make dev`: a session with a file tab active shows the file
  immediately after switching to it, and a restored inactive file tab shows
  its content immediately when selected.

## Implementation plan

1. In `file-view.tsx`, add the effect that calls `readOpenFile(store, path)`
   when the shown file is unread.
2. In `diff-view.tsx`, add the matching effect that calls
   `readOpenDiff(store, path)`.
3. In `editor.ts`, refresh only already-loaded content in `openFileTab`,
   `openDiffTab`, and `openRequestedFile`.
4. Update the tests in `editor/editor.test.ts` and `files/file-view.test.tsx`
   that relied on the open actions' reads, and add the tests above.
5. Run `bunx tsc -p .` and `bun test` in `internal/ui`, then `make check`.
