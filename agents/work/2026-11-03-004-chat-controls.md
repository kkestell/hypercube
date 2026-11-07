# Chat controls

## Plan

`agents/plans/2026-11-03-008-chat-controls.md`

## Summary

Added clipboard image attachments, a button to return to the latest output, and an interrupt button in an empty composer while the agent works.

## Checks run

- `bun test` in `internal/ui` — 308 tests passed.
- `make check` — passed outside the sandbox; macOS icon tools and the Go cache were blocked inside it.

## Manual verification

Started an isolated development server at `http://localhost:14381/` for user visual review; no visual inspection was performed.
