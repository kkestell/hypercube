# Session restoration, shutdown, and requests

## Plan

`agents/plans/2026-11-06-001-issue-session-fixes.md`

## Summary

Implemented #12, #14, #17, #18, and #20. Restored launches register before HTTP
admission; detaching interrupts blocked plugin writes while holders survive;
watchers seed busy sessions' old reply positions; family cleanup has direct
membership and persistence helpers; saved offers reach request notifications,
sidebar status, and waiting counts while agents can still park.

## Departures from the plan

- Restore returns registration errors to startup instead of logging them and
  serving incomplete state. Orphan cleanup precedes the HTTP callback so early
  requests cannot create holders that the old snapshot would then remove.
- Holder.Close permanently closes its attachment so a sender that reaches Write
  after detach cannot reopen the connection. The durable holder remains alive.
- Requests now returns read errors through the socket and alert watcher. Session
  test helpers adapt existing callers; shared answer-list call updates belong
  to the concurrent #3 implementation.

## Checks run

- Focused session/holder/startup regression tests with `-race` — passed.
- `go test -race ./internal/session ./internal/hold ./cmd/hypercube` — passed.
- `bun test web/sidebar/sidebar.test.tsx web/sessions/selection.test.ts web/app/badge.test.ts`
  in `internal/ui` — 21 passed.
- `git diff --check` on modified paths — passed.

## Follow-up work

- Parent runs `make check`, coordinates the independent review, and opens the
  development server for the user's browser checks. No service installation or
  session restart was performed.
