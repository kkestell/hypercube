# Keep chat visible beside file tabs

## Outcome

Chat fills its available width and lives outside the file-tab layout. Opening,
selecting, dragging, or closing file and diff tabs cannot cover or close chat.
The divider can narrow chat but cannot collapse it. With no files, chat fills
the center. Workspace dashboards retain their existing tab layout.

## Implementation

- Make the session view own a persistent chat pane on the left and the existing
  editor area on the right. Reuse the resizable split, remembering the editor
  width and hiding only the empty editor side. Give chat its own title bar and
  retain the existing layout controls and native window-button clearance.
- Remove agent tabs from the runtime model and start session editors empty.
  Migrate saved layouts by removing old agent tabs and pruning empty panes,
  preserving file tabs, file splits, scroll, and other saved choices.
- Keep Find shortcuts scoped to the focused view. Composer focus no longer
  needs to select a tab. Keep the transcript mounted when file tabs change.
- Remove chat's reading-width constraint and its fixed-width control, including
  the constraint on its composer and pending requests. Preserve Markdown file
  preview width preferences.

## Validation

Test chat and composer remaining mounted through opening, replacing, splitting,
and closing file tabs; focus/Find ownership; full-width chat; session and draft
views; saved-layout migration and divider-width persistence. Update existing
tests to expect file-only session layouts. Run focused UI tests, then `make check`.
Perform a self-review; the user requested no child sessions.
