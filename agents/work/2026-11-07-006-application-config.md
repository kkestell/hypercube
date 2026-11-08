# Application configuration package

## Plan

[Application configuration](../plans/2026-11-07-006-application-config.md)

## Summary

The plan's goal is met: `internal/config` owns preferences, defaults,
validation, atomic writes, and existing upgrades. Callers use its API;
`internal/harness` retains `Settings` and has no configuration declarations or
aliases. The file path, JSON document, defaults, and upgrade order are unchanged.

## Decisions

- Renamed the UI's GET handler to `getConfig` and its test fixture to
  `document` to avoid conflicts with the new `config` import. Routes and
  assertions are unchanged. No departures from the plan were needed.

## Checks run

- `go build ./...` — passed.
- `make check` — passed: browser build and TypeScript checks, 368 browser
  tests, 28 desktop tests, website build, Go build, vet, formatting, and all
  Go tests with the race detector, including `TestAppProcess`.
- Repository search for the former `harness` configuration API — no matches.
- `go list -deps ./internal/harness` — no dependency on `internal/config`;
  the planned existing dependency on `hold` remains.
- Compared the transferred tests with their originals — all assertions are
  unchanged; only package, imports, and API/type names changed.
- `git diff --check` — passed.

## Manual verification

No live app or installation check was needed for this source extraction.
Existing process and HTTP tests cover reading upgraded preferences, saving
complete documents, invalid requests, and child guidance. Linux packaging and
full runtime integration remain in the later integration plan.
