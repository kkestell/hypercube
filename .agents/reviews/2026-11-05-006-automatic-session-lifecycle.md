# Automatic session lifecycle

## Scope and coverage

Reviewed the lifecycle implementation against plan
`agents/plans/2026-11-05-005-automatic-session-lifecycle.md` and work log
`agents/work/2026-11-05-002-automatic-session-lifecycle.md`, including whole
touched lifecycle source files and relevant delivery, removal, migration,
socket, and UI callers. Plan 004's choices were outside scope except where
offers interact with this lifecycle. Applied the whole-file readability
standard; no additional blocking readability finding remains in this scope.

Wake, park, and Unarchive release `Runtime.mu` before waiting or running Git.
The parking predicate checks viewers, activity, questions, pending input,
commands, and queued messages. No further defect was found in Unarchive,
the migration, or removal of running/stopped states from the sidebar.

Gaps: no live harness, desktop, or browser smoke test. No running sessions
were touched. This review reports findings only; fixes were not applied.

## Summary

| ID | Severity | Category | Status | Title |
| -- | -------- | -------- | ------ | ----- |
| F1 | Medium | Correctness | Blocking; not fixed | Refused removal strands messages to parked recipients |
| F2 | Medium | Correctness | Blocking; not fixed | Archived offers accept answers their parent cannot receive |
| F3 | Medium | Resources | Blocking; not fixed | Refused removal can leave the idle timer disarmed |

All fixes are Trivial in the skill's terminology: existing requirements
determine the behavior, with no user decision needed.

## F1. Refused removal strands messages to parked recipients

Medium · Correctness · Blocking · `internal/session/removal.go:249`

A message can reach a parked session while Delete or Archive checks its
worktrees. `SendMessage` saves it, but delivery's `wake` returns `errDeleting`.
If the worktree check refuses removal, `releaseRemoval` only clears the
claim; nothing retries delivery. The session remains parked with the message
queued until another view, prompt, message, or backend restart happens. A
claim on the parent also affects parked children.

Evidence: `TestReviewRefusedRemovalWakesQueuedRecipient` paused a real Git
preflight on a dirty worktree, sent a message through `Runtime.SendMessage`,
then released the refusal. The recipient never woke and the message remained
queued; the probe failed under `-race`.

Fix: after releasing a refused removal's claim, call `Deliver` outside
`mu` for surviving, non-ended sessions affected by that claim, including
children for a family claim. Cover this parked-recipient sequence.

## F2. Archived offers accept answers their parent cannot receive

Medium · Correctness · Blocking · `internal/session/offers.go:59`

Archive leaves an unanswered offer in the saved transcript. Opening that
archived chat still shows enabled answer controls. `answerOffer` never checks
whether its parent has ended, so answering changes the read-only chat and
queues a message for the archived parent. `Deliver` intentionally refuses to
wake ended recipients, leaving the accepted answer undelivered.

Evidence: `TestReviewArchivedOfferCannotQueueAnAnswer` created an offer,
archived the parent, and answered New session. It failed under `-race` with
`done=true, queued=1, awake=false`. `AttentionForm` disables its buttons only
while sending, and `Transcript` renders pending offers for archived chats.

Fix: check the parent's `Ended` flag in the answer transaction and refuse
before changing the offer or queueing its answer. Make unanswered offers in
archived chats read-only. Preserve their ability to be answered after
Unarchive.

## F3. Refused removal can leave the idle timer disarmed

Medium · Resources · Blocking · `internal/session/removal.go:249`

A command reply or saved event during removal preflight calls
`settleLocked`, which cancels the idle timer because the removal claim makes
the session ineligible. A timer expiring during preflight is also removed.
When removal is refused, clearing `deleting` can make an idle session
eligible again, but `releaseRemoval` never settles it. With no later activity
or viewer change, its agent stays awake indefinitely. A family claim can
leave the same gap for children.

Evidence: `TestReviewRefusedRemovalRearmsIdleTimer` paused Delete's dirty
worktree preflight, completed a public `SelectProfile` call, then released
the refusal. The session was parkable with no timer; the probe failed under
`-race`.

Fix: after clearing the claim, settle each surviving launch affected by it
under `mu`. Cover refusal after a command reply cancels the timer, and
consider the family membership when releasing a parent claim.

## Checks run

- `GOCACHE=/tmp/hypercube-review-go-cache make check`: passed, including
  generated UI freshness, TypeScript, UI and desktop tests, Go vet,
  formatting, and all Go packages under `-race`. Go emitted a nonfatal
  module stat-cache permission warning.
- Isolated Go overlay probes under `-race`: all three failed as described
  above, confirming the findings. Overlay and probe sources are in
  `/tmp/lifecycle-review-2cczoqnx/`; repository source was untouched.
- No install, development server, staging, or commits were performed. Only
  this unstaged review file was created.

## Verdict

Three blocking medium findings remain. Resolve F1–F3 before merging.
