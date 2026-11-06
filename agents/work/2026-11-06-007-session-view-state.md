# Preserve session view state

## Plan

`agents/plans/2026-11-06-005-session-view-state.md`

## Summary

Implemented #1 using browser-local view snapshots. Active session data stays
attached and refreshes normally; inactive sessions retain only view choices.
Global layout and sidebar choices share the same storage subscriber, replacing
the previous wrap/fixed-width subscriber and migrating its values. Drafts keep
their existing storage. Explicit session URLs retain priority.

The UI agent implemented global layout and sidebar ownership; its contribution
is recorded in `2026-11-06-006-session-layout-contribution.md`. An independent
agent reviewed the architecture before implementation and the combined code
after implementation.

## Decisions and findings

- Scroll restoration waits for fetched content. Late unmounts save into the
  departing session without resurrecting deleted sessions or closed tabs.
- Independent review reproduced Markdown images clamping restored scroll before
  loading. The fix retries on initial images' load/error events, retains the
  intended position through pagehide, and yields to subsequent user scrolling.
  The reviewer verified the fix without further findings.
- Mounted splitter tests reproduced keyboard callbacks exposing the previous
  pixel size. Fixed panels now convert the callback's new percentage using the
  available panel pixels.
- Browser and desktop origins retain separate views. No backend migration or
  active-agent restart is needed; loading the UI change requires a reload.

## Checks run

- Focused storage, scroll, sidebar, and splitter tests, plus TypeScript — passed.
- Final `make ui && make check` — passed: 269 browser tests, 27 desktop tests,
  TypeScript/build freshness, Go vet, and all Go tests under the race detector.
- Independent findings-only review and bounded fix verification — complete.

## Manual verification

Started `HYPERCUBE_DEV_DIR=$(mktemp -d /tmp/hc-view.XXXXXX) make dev` with its
isolated state and scratch repository. The backend served port 4281; version,
JavaScript, and CSS requests succeeded, and creating a scratch workspace
succeeded. Opened `http://localhost:4281/` in the user's browser. No rendering
or screenshots were taken; visual judgment remains with the user. Stopped the
development server and removed only its isolated temporary directory.

The installed app was left unchanged. Concurrent backend security changes and
their review document are excluded from this issue's commit.
