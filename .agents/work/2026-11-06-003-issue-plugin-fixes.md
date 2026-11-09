# Holder and harness lifecycle fixes

## Plan

`agents/plans/2026-11-06-003-issue-plugin-fixes.md`

## Summary

#9, #10, and #11 are implemented. The holder reacts to output write failure;
main Codex completion preserves children and their approvals; Pi dialogs can
be answered before prompt acceptance. Serve owns and joins the pending input,
and retains the order of ordinary commands and replies.

## Departures from the plan

- Coordinated #3 changes in the same shared dispatcher and Codex/Pi request
  owners: answer callbacks carry choice lists, the dispatcher accepts saved
  singular choices, and Codex enumerated array forms preserve every selection.
  Pi remains single-choice. These changes use `harness.CheckAnswer`, supplied
  by the coordinating agent's multiple-answer plan.

## Checks run

- `go test -race ./internal/hold ./plugins/internal/plugin ./plugins/codex ./plugins/pi`
  — passed, including FIFO writer failure, late native child completion and
  approval, ordered replies, pending input shutdown, and Pi public Serve dialog
  before prompt acceptance.
- `go test -race ./plugins/codex ./plugins/pi` — passed after replacing duplicate
  answer validation with the shared validator.

## Follow-up work

- Coordinating agent runs independent review and `make check` for the combined
  issue set. These process/plugin changes require restarting active sessions
  after installation.
