# Automatic session lifecycle

## Plan

`agents/plans/2026-11-05-005-automatic-session-lifecycle.md`

## Summary

Each session now wakes when it is viewed, prompted, or sent a message
(including a child's report or failure and an offer answer), and parks 15
minutes after it last became eligible. Start, Stop, `Running`, `AwaitingUser`,
and Show Stopped Sessions are gone; Unarchive is added. The plan's goal is met.

## Departures from the plan

- `wake` returns an existing, non-stopping launch before checking for a
  family removal in progress. Checking the removal claim first refused prompts
  to a running session during Delete's preflight, which
  `TestRefusedRemovalKeepsTheFamilyWorking` covers.
- `SaveSession` now writes `base`, so Unarchive can record the recreated
  worktree's base.
- The stand-in harness answers its asynchronous question (`aq`) by closing it
  without a work state. This gives a test a blocker that ends with no Idle
  event.
- `docs/architecture.md`, `docs/installation.md`, `cmd/hypercube/main.go`, and
  `scripts/dev` described the running mark; they now describe the new
  lifecycle. The installation guide's reference to a seeded preview session
  was already inaccurate and now describes only the sample repository.

## Decisions

- The idle timer is an `idleTimer` holding the `time.Timer`, so the callback
  compares an identity that exists before the timer can fire (the race
  detector caught the direct form).
- A timer is not cancelled when a session becomes busy; `park` rechecks
  `parkableLocked`, and the next blocker removal arms a fresh timeout.
- Restore wakes no lost session; it settles every attached launch and calls
  `Deliver` for each non-ended session.
- Tests check eligibility through `parkableLocked` and parking through a short
  `idleTimeout`, with no fixed sleeps.

## Checks run

- `make check` — Passed.
- `go test -race -count=5` on the lifecycle, restart, offer, and messaging
  tests — Passed.

## Review fixes

Fixes for review `agents/reviews/2026-11-05-006-automatic-session-lifecycle.md`:

- F1 and F3: `releaseRemoval` reads the sessions the claim covered (the
  session and, for a top-level claim, its children), settles each surviving
  one under `mu`, and then calls `Deliver` for each outside `mu`. Tests:
  `TestRefusedRemovalWakesQueuedRecipients`,
  `TestRefusedRemovalRestartsTheIdleTimeout`.
- F2: `answerOffer` refuses with `errEnded` when the parent has ended, before
  writing. `AttentionForm` disables its controls in an ended session's
  transcript. Tests: `TestArchivedOfferWaitsForUnarchive` and the transcript
  test for an archived offer.

Each test fails without its fix.

## Manual verification

No `make dev` smoke test with real harnesses was run.

## Follow-up work

- Agents started before the install keep their old tool descriptions, which
  mention running and stopped recipients, until they next park and wake.
