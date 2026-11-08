# Persistent chat pane review

Self-review; the user explicitly requested no child sessions.

- Chat is owned by the session view, outside the file-tab tree. The split keeps
  chat mounted through file opens, preview replacement, file splits, and closes.
  Only the editor side can collapse; chat has a 20% minimum. Layout controls
  remain in the chat header, including when the editor is absent or collapsed.
- Existing tab operations remain file-editor concerns. Removed agent-tab types,
  initialization, rendering, and composer-focus tab selection rather than
  retaining a second path for chat.
- Saved views remove the old agent tab through existing layout pruning. Tests
  preserve nested file splits, source selections, transcript scroll, and the new
  editor width across reloads. Workspace dashboards remain unchanged.
- Transcript, composer, and pending requests no longer use a reading-width
  constraint. Markdown file previews retain their own existing width control.
- Shared focus tracking gives background Find to chat and keyboard focus to
  the appropriate pane without changing highlighting's independence from focus.
- Reviewed the user's additional transcript fixes: automatic-message labeling,
  missing tool-detail explanations, and omission of empty text/reasoning while
  retaining image-only content. Their regression tests are included.

Validation: focused UI tests and `make check` passed (375 browser tests, 28
desktop tests, builds, TypeScript, Go vet, and Go race tests). Existing React
`act` warnings remain non-fatal. No unresolved findings.
