# Git Changes panel and diff tabs

## Plan

`agents/plans/2026-11-03-007-git-changes-panel.md`

## Summary

Implemented the repository-wide Changes list, branch and ahead row, and unified
syntax-highlighted diff tabs using the approved mockup markup and styles. The
existing preview tabs and visible-page poller own the new views and refreshes.

## Departures from the plan

- Named the response type `ChangesResult`: Go cannot declare a type and a
  function both named `Changes`. The planned function and JSON shape are intact.
- `/diff` rejects empty, absolute, and escaping paths with 400 to enforce the
  route's repository-relative file input.

## Checks run

- `make ui` — passed; generated assets rebuilt.
- `GOCACHE=/private/tmp/hypercube-shared-gocache make check` — stopped at
  `sips` while building the app icon: “Cannot extract image from file”, Error 13.
- `bun test` in `internal/ui` — 271 passed, zero failures.
- Fresh `bun build.js` into a temporary directory matched `internal/ui/dist`.
- `GOCACHE=/private/tmp/hypercube-shared-gocache go vet ./...` — passed.
- `GOCACHE=/private/tmp/hypercube-shared-gocache go test -race ./...` — passed.
- Focused race tests for `internal/ui` and `internal/workspace` also passed
  after the final size-check preservation and staged-plus-unstaged regression.
- All Go sources are formatted. Source diffs pass `git diff --check`; generated
  `dist/main.js` retains the bundler's whitespace inside a dependency literal.

## Manual verification

Attempted an isolated preview:

```sh
HYPERCUBE_DEV_DIR=/private/tmp/hypercube-git-panel-preview \
HYPERCUBE_DEV_PORT=4297 GOCACHE=/private/tmp/hypercube-shared-gocache make dev
```

The same `sips` icon error stopped the build before the server started. Removed
that preview's temporary state. Browser rendering remains for the user to judge.
No installed service or running session was changed.
