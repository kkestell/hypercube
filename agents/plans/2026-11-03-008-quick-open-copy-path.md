# Quick Open and copy paths

## Goal

Open session files through Command+P and copy paths from file tree rows and file tabs.

## Decisions

Use the existing mention-files endpoint and the stock Command dialog's fuzzy filtering. Open results through the same preview action as the Files tree. Use the session directory for absolute paths and retain absolute paths opened by agents.

## Implementation plan

- Add a Quick Open dialog in `internal/ui/web/files/`, opened by the application shortcut and bound to the attached session.
- Add shared path context menu behavior for tree rows and file tabs using `shared/context-menu.jsx`, including keyboard access.
- Update `docs/usage.md` and add UI tests for opening, filtering, and copying paths.
