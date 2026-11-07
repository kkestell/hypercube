# Nested sessions

## Plan

`agents/plans/2026-11-02-004-nested-sessions.md`

## Summary

Replaced the lead/worker runtime and agents table with one agent per session,
parent-owned family state, and one level of child sessions. Ended children keep
their chats. Requests and status appear in the parent's saved transcript without
reaching its plugin. The browser selects any session and nests children in the
sidebar; the worker stack and `/agent` route are gone.

## Decisions

- Child-name validation lives in `CreateChild`, so direct runtime calls and MCP
  use the same rule.
- A null folder listing records a 404. It leaves missing nested folders empty
  and lets Files distinguish a missing session directory from an empty one.
- `write` carries its runtime so `putItem` and opening-request events share
  `mirrorLocked`; transcript upserts retain the original mirror position.

## Checks run

- Focused Go tests for store, harness, workspace, report, session, MCP, UI,
  plugins, and command integration — passed with isolated state and stand-ins.
- Focused browser tests for sessions, sidebar, and transcripts — passed.
- `make ui` — regenerated `internal/ui/dist`.
- `GOCACHE=/tmp/hypercube-nested-gocache BUN_INSTALL_CACHE_DIR=/tmp/hypercube-nested-bun-cache make check`
  — passed: build, generated-file freshness, 150 browser tests, vet, formatting,
  and all Go tests with the race detector.
- Source/documentation diff inspection — passed. Generated bundle whitespace
  comes from bundled dependency string literals and was left generated.

No plan departures or unfinished implementation. No live dev session or visual
checks were run, as directed by the assignment; the parent owns user verification.
