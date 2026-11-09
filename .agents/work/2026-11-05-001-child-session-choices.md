# Child session choices, fork placement, and completed results

## Plan

`agents/plans/2026-11-05-004-child-session-choices.md`

## Summary

`create_session` has three forms: `initiation: "offer"` saves an app-owned
New session / Do it here question in the parent's transcript, `offer_id`
starts the accepted offer's child, and `initiation: "own_initiative"` saves a
proposal card. `requested` is gone, so a child starts only after a New session
answer or a Start click. Forks now follow the parent's folder setup. Codex
async questions carry `Async`, and OpenCode closes the questions of a form it
withdraws. Completed reports reach the parent while another child waits for
the user, and the report names that child. The plan's goal is met. The live
choice has not been exercised.

## Departures from the plan

- The offer no longer waits inside the tool call. Most harnesses time out MCP
  tool calls (Codex `tool_timeout_sec` defaults to 60 s; Pi and OpenCode have
  their own limits), and the bridge does not cancel the app request when that
  happens, so a waiting call would leave accepted offers that the agent never
  learns about. With the parent's approval, the offer returns at once. The
  answer reaches the parent as an automatic message (`From` empty) through the
  existing delivery path: New session carries the `offer_id`, and Do it here
  carries no ID. An unanswered offer stays open across interruption, stopping,
  reload, and reattachment until it is answered or its session is removed.
  The plan's "Record the choice" and "Separate offering" sections, steps 1 and
  3, and its test plan were updated to match. This removed passing the MCP
  context through, cancellation, and the reattachment cleanup.
- Report batching saves only the report bodies (`outgoing.batch`), and
  delivery adds the heading. The heading now names waiting children, so
  stripping a fixed heading from a saved batch would no longer work.

## Decisions

- Offers are `harness.Item` attention items with `Offer` set. Their `Input`
  holds the settings of the child the offer started, and an empty `Input`
  means the offer is unused. They are not launch-scoped pending items, so they
  do not appear in `Runtime.Requests` (notification cards) and do not set the
  session's `Attention` flag. They are pinned above the composer in the
  parent's own transcript.
- `createChild` takes an `approval` (proposal or offer ID) that registration
  uses up and a cleaned-up failed start restores. `CreateChild` stays as the
  runtime's unapproved path for tests; no MCP form reaches it.
- A fork from an old saved proposal may keep a `dir` only when it resolves to
  the parent's folder in the original checkout; otherwise the start fails with
  an error that names the required placement.
- A child counts as waiting for the user when its main conversation has an
  open synchronous question, or when it is idle and has any open question.
- Proposal cards for forks show no folder, because a fork works where its
  parent works.

## Review fixes

Fixes for `agents/reviews/2026-11-05-005-child-session-choices.md`:

- F1: an event whose question opens or closes is saved and re-checks report
  delivery even when the session's attention flag stays the same.
  `TestSecondQuestionReleasesReports` covers this through the stand-in's new
  `!question` keyword (a question without a work state). `!asyncask` now uses
  its own item ID. Without the fix, the test fails.
- F2: an accepted offer needs a nonblank `message`, checked before the offer
  is used. `TestCreateSessionForms` covers this through the MCP handler.
- F3: `eventLocked` is now a short entry point. `applyItems` makes the live
  changes (namespacing, grants, merging, publishing), `saveEvent` saves the
  durable effects in one transaction, and the scheduling follows.
- F4: Codex image reading and decoding moved to `plugins/codex/images.go`, and
  tool payload presentation moved to `plugins/codex/tools.go`.
  `plugins/codex/events.go` keeps notification routing and turn handling.

## read_session

Top-level sessions have a `read_session` tool, addressed like `send_message`:
its own children by name, or top-level sessions by ID. It reads the stored
transcript (`Runtime.Transcript`) and shows user and assistant text, tool
names, errors, questions, suggestions, and how each turn ended, oldest first.
It leaves out reasoning and native subagents' items. Pages hold up to 40,000
characters, counting the blank lines between entries, and are read from the
end backward. When earlier entries remain, the page names a `cursor` (the
position of its oldest entry). A single entry longer than the budget is cut.
A clipped report now ends with "(first 400 of N characters) Read the rest with
read_session on <child>." The child instructions and `docs/introduction.md`
say so.

## Checks run

- `make check`: passed.

## Manual verification

Not done. The New session / Do it here choice and the child it starts must be
exercised live with the user (`make dev`, ask an agent for a separate task,
answer the question, and confirm that the parent starts one child from the
`offer_id`).

## Follow-up work

- Open offers do not show the amber "waiting" dot or a notification card.
  They are visible only in the parent's own transcript.
- Active sessions must restart to load the new tool contract, instructions,
  and plugin changes after installation.
