# Restore the worker stack

## Goal

Arrange the page from left to right as the existing sidebar, lead transcript,
vertical worker stack, open-file editor, and right sidebar. Worker headers
switch between summary cards and full transcripts with composers in place.
Show the file area only while files are open. Keep Files and Changes in the
right sidebar and remove Workers.

## Decisions

- Reuse `AgentPane` for worker transcripts and `Split` for column widths.
  Keep the existing file tab tree, preview behavior, drag/drop, and pane splits;
  remove its worker-transcript branches rather than creating another editor.
- Rework `WorkersPanel` in its existing files into the center's worker stack.
  Start workers collapsed. Keep expansion and height weights in component state,
  keyed to the attached session. Hide the worker column when no workers exist.
  No disk persistence or Go changes are needed.
- Restore `e55210f`'s resizing behavior: expanded cards share available height;
  collapsed cards retain natural height. A horizontal divider transfers height
  between the nearest expanded workers on either side, skipping collapsed cards.
  Reuse that direct flex-weight approach rather than nesting binary splits,
  which would give collapsed cards a share of the height.
  At drag start, measure all expanded workers into height weights before
  changing the pair, so unrelated expanded workers retain their space.
- Keep worker drafts and transcript scroll through collapse/expand using the
  existing draft and scroll paths. `rememberScroll` must accept any shown agent,
  in the attached session, rather than only the lead or an agent in a tab.

## Implementation plan

1. Modify `internal/ui/web/workers/workers-panel.jsx` and
   `workers-panel.css`: replace Open Transcript with an accessible header toggle;
   retain names, status indicators, latest reports, ages, and attention styling.
   Expanded bodies render `AgentPane`. Add horizontal drag dividers with bounded
   minimum transcript/composer space and clean up pointer handlers on unmount.
   Keep stack sizing separate from card rendering if needed for readability.
2. Modify `internal/ui/web/sessions/session-view.jsx`: nest column `Split`s for
   lead/workers/files in the requested order. Preserve independent column shares,
   collapse absent worker/file regions, and leave the no-lead view unchanged.
3. Modify `internal/ui/web/app/right-panel.jsx` and `internal/ui/web/main.jsx`:
   remove Workers and select Files initially. Update related app fixtures.
4. Make `internal/ui/web/editor/editor.js`, `editor-area.jsx`,
   `editor-pane.jsx`, and `editor-tabs.jsx` file-only. Remove `openWorkerTab`,
   agent lookup, transcript rendering, and agent tab decorations; retain the lead
   composer focus fallback when the final file closes. Update `rememberScroll`
   to recognize current agents. Remove obsolete worker-tab cleanup and comments
   from `internal/ui/web/sessions/selection.js` without changing file cleanup.
5. Update the affected tests: `workers/workers-panel.test.jsx`,
   `sessions/session-view.test.jsx`, `sessions/selection.test.js`,
   `editor/editor.test.js`, `editor/editor-tabs.test.jsx`, and
   `app/app.test.jsx`. Replace worker-tab fixtures with file fixtures where the
   test covers generic editor behavior. Add `app/right-panel.test.jsx` for the
   two remaining tabs if existing coverage does not exercise them.
6. Correct `docs/usage.md`'s layout, resizing, worker-toggle, and tab-area text.
   Keep the existing file split/tab instructions and describe their file scope.

## Test plan

- Headers toggle workers independently in place, expose expanded state, and
  render a transcript/composer without adding editor tabs. Reports, status,
  attention, and age remain visible in the appropriate card state.
- Collapsing and reopening preserves drafts and transcript scroll; session
  changes do not reuse another session's expansion or sizing state.
- Divider drag changes only the nearest expanded pair, including when collapsed
  workers lie between them; other expanded sizes and collapsed heights remain
  unchanged. Minimum heights constrain drag, and fewer than two expanded workers
  have no usable divider.
- Lead, worker stack, and file area have the requested order; no-worker and
  no-file states collapse the corresponding column. Opening the first file and
  closing the last file changes file-area visibility without removing agents.
- Column resize callbacks retain independent shares. Existing file preview,
  keyboard navigation, drag/drop splitting, scroll, and close focus behavior
  continue to work with file tabs. Files/Changes are the only sidebar tabs.
