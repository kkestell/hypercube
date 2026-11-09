# Child session choices

## Scope and coverage

Reviewed staged changes against
`agents/plans/2026-11-05-004-child-session-choices.md`, including the whole
changed source files and relevant callers. Returning offers immediately and
sending answers automatically is approved. This review reports findings only;
no application files, staging, commits, or running sessions were changed.

The MCP launch gate holds: offers save questions, suggestions save proposals,
and fills consume an accepted, unused offer in the child-registration
transaction. No reviewed `create_session` form starts a child without a New
session answer or Start click.

Gaps: no live offer interaction or native harness smoke test was performed.
Generated UI output was checked against a fresh build by `make check`.

## Summary

| ID | Severity | Category | Status | Title |
| -- | -------- | -------- | ------ | ----- |
| F1 | Medium | Correctness | Blocking; not fixed | A second question can leave ready reports queued |
| F2 | Medium | Correctness | Blocking; not fixed | Accepted offers can launch without a brief |
| F3 | Medium | Readability | Blocking; not fixed | Event application interleaves too many responsibilities |
| F4 | Medium | Readability | Blocking; not fixed | Codex lifecycle and payload conversion share one file |

All fixes are Trivial in the skill's terminology: no user decision is needed.
They were not applied, as requested.

## F1. A second question can leave ready reports queued

Medium · Correctness · Blocking · `internal/session/event.go:152`

If a working child already has an asynchronous question open, opening a
synchronous question changes report readiness without changing Attention.
With no completed items or other state change, the early return skips the
new question-triggered delivery at line 202. Other children's completed
results stay queued while this child waits for the user.

Evidence: a temporary Go overlay created Bob's async question, queued Alice's
report, then applied Bob's synchronous question event. `reportsReady` returned
true and named Bob, but no delivery command was registered. The regression
probe failed under `-race`.

Fix: let opening or closing a question reach checkpointing and delivery even
when the aggregate Attention value stays unchanged. Cover this sequence.

## F2. Accepted offers can launch without a brief

Medium · Correctness · Blocking · `internal/session/offers.go:105`

The accepted-offer form promises to supply the child's assignment after the
user answers. Neither the MCP form nor offer validation requires a nonblank
message. Omitting it consumes the accepted offer and launches a fresh child
with no task; `createChild` queues nothing for it.

Evidence: a temporary test through the real MCP handler accepted New session
and filled its offer without `message`. The call returned Created task and
left a running, idle child with no assignment. The refusal probe failed under
`-race`.

Fix: reject a missing or whitespace-only brief for accepted-offer fills before
consuming approval or starting the child. Leave existing proposal and other
runtime behavior unchanged.

## F3. Event application interleaves too many responsibilities

Medium · Readability · Blocking · `internal/session/event.go:82`

`eventLocked` combines item namespacing, grant lookup, streaming merges, feed
publication, state-marker ordering, attention calculation, durable effects,
report queueing, checkpointing, and delivery scheduling. The early return
between live updates and the transaction makes a reader retain every concern
to understand when the final scheduling step runs; F1 demonstrates that cost.
This whole-file defect also remains recorded in the staged complexity review.

Fix: extract the per-item live update and durable event effects into direct
helpers. Keep their ordering and the single transaction visible in a short
entry point; preserve the streaming-only path.

## F4. Codex lifecycle and payload conversion share one file

Medium · Readability · Blocking · `plugins/codex/events.go:135`

The file mixes notification routing and main/child turn ownership with image
file reads, MIME detection, base64 conversion, generated-image fallback, and
tool payload presentation. Image helpers at lines 135–200 interrupt the
lifecycle code; payload conversion resumes at line 455. Readers changing
question or completion ordering must navigate unrelated I/O and presentation.
This whole-file defect also remains recorded in the staged complexity review.

Fix: move image handling and tool payload conversion into files named for
those concerns within the same plugin package. Keep notification routing and
turn ownership together, using the existing concrete functions and data.

## Checks run

- `GOCACHE=/tmp/hypercube-review-go-cache make check`: passed, including
  generated UI freshness, TypeScript, 235 UI tests, 18 desktop tests, Go vet,
  formatting, and all Go packages under `-race`. Go emitted a nonfatal module
  stat-cache permission warning. An initial probe using the default Go build
  cache could not run in the sandbox; the temporary cache resolved it.
- Temporary regression probes failed as expected for F1 and F2:
  `TestReviewSyncQuestionAfterAsyncReleasesReport` and
  `TestReviewAcceptedOfferNeedsBrief`. Source overlays are in
  `/tmp/child-review-18o1aw6g/overlay.json`; repository source was untouched.
- No live sessions were started, stopped, installed, or modified.

## Verdict

Four blocking medium findings remain. The approval gate is sound in the
reviewed tool paths. This review is left unstaged; no fixes were applied.
