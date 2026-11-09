# Review of the harness plugin restructure

## Scope and coverage

Reviewed `a1e6ec6..401183f` on `hypercube/1ea38c11/lead`, excluding the plan
file from the diff. Read every shared plugin file, all four plugins and their
Go tests, `internal/harness/harness.go`, and `go.mod` whole. Checked against
`agents/plans/2026-11-01-003-plugin-restructure.md`, the plugin sections and
harness `Do` item in `agents/reviews/go-audit.md`, and the Hypercube skill's
review requirements. The plan's expressly excluded audit items remain outside
this review.

Reviewed event translation, request replies, CLI wire messages, startup and
exit behavior, locking, and test assertions. Used stand-ins and recorded
fixtures; no live smoke test with the installed harness CLIs was run.

## Summary

| ID | Severity | Category       | Outcome  | Title                                          |
| -- | -------- | -------------- | -------- | ---------------------------------------------- |
| F1 | Medium   | Error handling | Fixed    | A CLI exit can add an extra prompt error        |
| F2 | Low      | API design     | Accepted | ACP error replies omit their former data fields |
| F3 | Low      | Testing        | Fixed    | Claude tests check private conversation state   |

## F1. A CLI exit can add an extra prompt error

Medium · Error handling · Fixed · `plugins/opencode/main.go:368`

OpenCode's prompt finishes on a separate goroutine. When the CLI exits,
`RPC.End` releases that goroutine while the agent context may still be live.
`ended` could then emit a transcript error item before `Serve` cancelled the
context in its deferred stop. The original process watcher cancelled the
context on exit, suppressing that item. The new crash transcript depended on
scheduling. No corresponding transcript regression was confirmed in Codex,
which has no separate prompt-completion goroutine.

### Evidence

Ran original and restructured plugin binaries against the same stand-in ACP
CLI, which exited with status 3 during `session/prompt`. The original emitted
no prompt error in 50 runs; the restructured plugin emitted one in 14 of 50:
`session/prompt: opencode exited: exit status 3: boom`.

`TestExitDuringAPromptIsNotATurnError` waits until the stand-in receives the
prompt, then closes its output while leaving the agent context live. It
waits for the emitted idle event and checks that no error item was added. It
failed before the fix and passes after it, without depending on the race
between prompt completion and `Serve` stopping.

### Fix

The lead selected treating a closed `ag.rpc.Done()` as stopping when `ended`
decides whether to append an error item. `Serve` continues reporting the
process's exit reason. Startup calls retain their exit diagnostics instead
of competing with early context cancellation.

## F2. ACP error replies omit their former data fields

Low · API design · Accepted · `plugins/opencode/main.go:461` and
`plugins/opencode/main.go:480`

Replacing the ACP SDK removed optional `data` fields from two error replies.
An unknown method formerly received `data: {"method": "<method>"}`. A
withdrawn form formerly received
`data: {"error": "the form was not answered"}`. The error codes and messages
are preserved.

### Evidence

A wire comparison of the original and restructured plugin binaries confirmed
the omissions for `fs/read_text_file` and a withdrawn `elicitation/create`
request.

### Decision

The lead had already accepted dropping these optional fields because OpenCode
only logs them. This is an accepted behavior difference; no change was made.

## F3. Claude tests check private conversation state

Low · Testing · Fixed · `plugins/claude/main_test.go:157` and
`plugins/claude/main_test.go:480`

The audit's two named tests still asserted `h.conversation`. They could pass
when the private field was updated without reporting the conversation ID to
Hypercube. The streamed-text test only checked that the first event lacked
a conversation ID.

### Fix

Both tests now assert the expected conversation ID in emitted events. The
streamed-text test retains its check that the first event has no conversation
ID, and the unreadable-line test checks the result event after the bad line.

## Checks run

- `go test -race ./plugins/... ./internal/harness/...` — passed during the
  initial review with a temporary cache, which has since been deleted.
- Original/current OpenCode process and ACP wire comparison — confirmed F1
  and F2; temporary binaries, logs, source archive, and script were deleted.
- Focused OpenCode regression test — failed before the fix, passed after it.
- Focused Claude conversation tests — passed.
- `GOCACHE=/tmp/hypercube-gocache make check` — builds passed; Bun stopped on
  missing UI dependencies (`shiki/core` and `marked`). The lead directed this
  review to skip Bun and will run the full check after merging.
- `GOCACHE=/tmp/hypercube-gocache make build` — passed.
- `GOCACHE=/tmp/hypercube-gocache go vet ./...` — passed.
- The Makefile's `gofmt` check and `git diff --check` — passed.
- `GOCACHE=/tmp/hypercube-gocache go test -race ./...` — passed.

## Verdict

F1 and F3 are fixed. F2 is an accepted difference. No additional blocking
readability or concurrency defect was confirmed in the reviewed scope.
