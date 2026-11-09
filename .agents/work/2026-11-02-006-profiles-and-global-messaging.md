# Running top-level messaging across workspaces

## Plan

`agents/plans/2026-11-02-007-profiles-and-global-messaging.md`, Part B only.

## Summary

Implemented the approved messaging scope using the existing store queries and
queue/delivery path. Children remain parent-only. Top-level peers can communicate
across workspaces, and stopped or ended recipients are refused before queuing.
Listings include workspace paths and parent IDs; peer messages identify the
sender's title and workspace while preserving ID replies.

## Checks run

- `GOCACHE=/private/tmp/hypercube-parent-messaging-go-cache go test ./internal/store ./internal/session ./internal/mcp ./internal/report` — passed.
- `GOCACHE=/private/tmp/hypercube-parent-messaging-go-cache go test ./cmd/hypercube -run 'TestMessagesAcrossWorkspaces|TestSessionsAreIsolated'` — passed.
- `GOCACHE=/private/tmp/hypercube-parent-messaging-go-cache make check` — passed, including asset freshness, 166 browser tests, vet, formatting, and all Go race tests. Go printed nonfatal module-cache stat-write warnings under the sandbox.
- `git diff --check` — passed.

## Follow-up work

The parent owns independent review, integration, the user's configuration edit,
and installation. Active families need restarting for new startup instructions
and tool descriptions. No visual changes or manual browser checks were made.
