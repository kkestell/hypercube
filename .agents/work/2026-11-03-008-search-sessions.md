# Search sessions

## Plan

`agents/plans/2026-11-03-008-search-sessions.md`

## Summary

Searches saved session titles and conversation messages across workspaces and opens the selected session.

## Checks run

- `GOCACHE=/tmp/hypercube-sidebar-go-cache make check` — passed outside the sandbox; the sandboxed build could not render the app icon with `sips`.
- `make dev` with an isolated temporary state directory and port 4293 — search route answered; server stopped and state removed.

## Departures from the plan

- The server-side search is tested through the store entry point rather than the HTTP route; the browser test exercises opening and selecting a search result.
