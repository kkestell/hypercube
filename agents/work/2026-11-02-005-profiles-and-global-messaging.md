# Shared profiles (Part A)

## Plan

`agents/plans/2026-11-02-007-profiles-and-global-messaging.md`, Part A only.

## Summary

Every configured profile is available to top-level sessions and children.
The workspace picker shows settings above a one-line truncated description,
with keyboard selection and the existing popup dismissal behavior. Messaging
permissions are unchanged.

## Decisions

- Kept the planned small popup hook: it moves the existing positioning and
  dismissal code unchanged so ContextMenu and ProfilePicker share that concern.
- The parent owns edits to the user's actual config and the subsequent Part B.

## Checks run

- Focused Go packages (`harness`, `session`, `mcp`, `report`, `ui`) passed.
- Focused browser tests: 58 passed.
- `make ui` regenerated the committed browser assets.
- `GOCACHE=/private/tmp/hypercube-profiles-go-cache make check` passed: fresh
  assets, 166 browser tests, Go vet, formatting, and the complete Go race suite.
  It used a writable build cache; plain `make check` could
  not write the shared Go cache under the worktree sandbox.
- Source diff whitespace check passed; generated bundles retain the bundler's
  whitespace and are verified by the fresh-build comparison.

## Follow-up work

- Browser rendering is left to the user, as requested; no screenshot or live
  browser rendering was performed.
- Active agents retain the tool descriptions and startup instructions they
  launched with until their family restarts.
