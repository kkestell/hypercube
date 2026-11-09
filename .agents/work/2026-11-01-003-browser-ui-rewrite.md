# Browser UI rewrite: Go half

## Plan

`agents/plans/2026-11-01-003-browser-ui-rewrite.md`, Go half only.

## Summary

The runtime owns database writes and publishes committed changes and merged
streaming items. The data socket implements the planned state, request, and
transcript messages. The UI handlers are split by concern; database polling,
whole-transcript comparison, and injected development reload are removed.
The Go half is complete, with no protocol or file-boundary changes.

## Decisions

- `TranscriptItem` embeds `harness.Item` and carries `seq` for resets and
  updates. Transcript reads append pending items directly instead of encoding
  and decoding them again; `putItem` owns saved-item serialization.
- The snapshot ordering test moved with the snapshot implementation. Existing
  runtime tests already cover releasing queued messages on user input.
- The refusal test queues both messages before starting delivery. Its previous
  two separate writes could retry the refused message when the second write
  arrived after the first delivery attempt ended.

## Checks run

- Focused Go tests for session, report, store, MCP, UI, and the command passed.
- `bun install --frozen-lockfile` installed missing local test dependencies
  without changing the lockfile.
- `GOCACHE=/private/tmp/hypercube-go-half-cache make check` passed: build,
  all 48 browser tests, vet, formatting, and all Go tests with the race detector.
  The temporary build cache avoids sandbox restrictions on the default cache.
  Go also printed nonfatal module stat-cache write warnings during the build.
- Production Go searches show `db.Tx` only in `save`, and `PutEntry` only in
  `putItem` and its store definition. Test fixtures still write isolated data.
- `git diff --check` passed.

## Manual verification

Browser verification belongs to the lead after merging both halves, using
`make dev`. The old page in this worktree still expects the previous protocol.
