# UI issue fixes

## Goal

Resolve issues #2, #6, #7, #8, #15, #16, #19, and #23 through existing UI owners.

## Decisions

- App shortcuts listen on document; Escape remains a bubbling action so focused controls can consume it.
- New-session context names the workspace and explains the worktree choice in visible text.
- Preserve unfinished new-session prompts, profile, and worktree choice across navigation using the existing draft state and storage.
- Notification expiry pauses while hovered or focused, preserving the remaining lifetime.
- Prompt-history extraction and pure stepping belong in history.ts; the composer owns recalled edits.
- Completed child proposals reuse transcript disclosure state and begin collapsed.

## Implementation plan

1. Update app.tsx and shortcuts.ts to handle keys outside the app root.
2. Update session selection, drafts, and sidebar navigation to retain and reopen unfinished prompts.
3. Add workspace context and a labeled worktree option to the new-session composer.
4. Pause reply-card expiry during interaction.
5. Exclude automatic reports from history and extract its pure transition calculation.
6. Collapse completed proposal cards with a disclosure for their brief.

## Test plan

- Mounted app shortcuts work from document.body and preserve dialog, terminal, and Escape handling.
- New-session text, profile, worktree, and attachments survive switching sessions and reopening.
- Existing composer history tests preserve multiline prompts, recalled edits, drafts, and mention priority; automatic reports are excluded.
- Reply-card hover/focus pauses and resumes expiry, without marking replies seen.
- Completed proposals hide their brief until expanded, for both outcomes.
- Run focused Bun tests and TypeScript checks; parent runs make check and coordinates browser inspection.
