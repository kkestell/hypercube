# Configured models

## Plan

`agents/plans/2026-11-08-001-configured-models.md`

## Summary

Implemented global, manually configured model entries and immediate Settings
editing. The picker now reads page state only, with harness tabs, focused search,
native model IDs, and a commit-based effort slider. Model discovery remains an
agent capability; sessions keep independent settings through entry changes.

## Decisions

- Duplicate entries use a store error sentinel so HTTP status selection does
  not depend on matching error text.
- The Settings dialog now reads its open section directly from page state,
  including navigation from its own default-model picker.
- Work is isolated on `hypercube/d18efb62/configured-models-work`; the original
  checkout and its staged review files were not changed.

## Checks run

- `bun install --frozen-lockfile` in `internal/ui` — passed; needed because the
  new managed worktree initially had no browser dependencies.
- Focused picker, Settings dialog, and transcript tests — 33 passed.
- `bun test web/app/settings-models.test.tsx` — 3 passed.
- `go test ./internal/store ./internal/ui` — passed after building UI assets.
- `go test ./internal/mcp` — passed with agent model discovery unchanged.
- The first `make check` caught an unknown-entry update returning 409 instead
  of 404. Added explicit not-found handling; the focused HTTP/store tests passed
  afterward.
- Final `make check` — passed: browser and website builds, TypeScript, 368
  browser tests, 28 desktop tests, Go build/vet/formatting, and all Go tests
  under `-race`. Browser tests emitted React `act` warnings in unrelated tests.
- `git diff --check` — passed.

## Manual verification

No live native-agent smoke test was performed. Browser interactions are covered
by the component tests; native start/configure validation code is unchanged.
To reproduce the plan's remaining live check in isolated state:

```sh
HYPERCUBE_DEV_DIR="$PWD/bin/configured-models-smoke" HYPERCUBE_DEV_PORT=4287 make dev
```

Open `http://localhost:4287`, add models for two harnesses in Settings › Agents,
choose one below the composer, start a session, and change effort. Delete its
entry and confirm its saved settings remain visible. Add an entry with a
misspelled model ID and verify the start error appears in chat. Stop the
development process afterward.

## Follow-up work

- Complete the live native-agent smoke test and independent `/kreview`.
