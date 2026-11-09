# Holder and harness lifecycle fixes

## Goal

Resolve #9, #10, and #11: stop the held process when output copying fails,
preserve native Codex children across main-turn completion, and allow Pi
extension answers while prompt acceptance is pending.

## Decisions

- Keep the holder's normal final-output drain; watch the copier concurrently
  with the existing child wait and kill the process group on copying failure.
- Existing transcript item parents distinguish main items from child items.
  Retain those child items at main-turn completion. Record blocking requests'
  thread ownership alongside the ownership already used for async questions.
- Keep native input acceptance as the input reply boundary. Shared Serve runs
  one pending Input call while processing answers, retaining other commands and
  replies in received order. Serve stops the process and joins the pending call
  before returning. No additional plugin transport or process owner is added.

## Implementation plan

1. Update `internal/hold/holder.go` and add an isolated output-write failure
   regression in `internal/hold/hold_test.go`.
2. Limit Codex main-turn cleanup in `plugins/codex/events.go`; assign request
   thread ownership in `plugins/codex/requests.go`. Add late child output and
   child approval regressions in `plugins/codex/main_test.go`.
3. Update `plugins/internal/plugin/serve.go` so pending input cannot block
   answers. Add ordering and shutdown coverage in its tests, plus a Pi public
   serve regression using the existing stand-in in `plugins/pi/main_test.go`.

## Test plan

- Holder exits with the copying error after the output writer fails while its
  child is still producing output; normal exit still retains final output.
- Main Codex completion leaves child tools, streamed text, and approval items
  open; subsequent child completion and approval answers still reach the UI
  and app-server.
- Pi opens an extension dialog before replying to a prompt; an answer reaches
  Pi and releases prompt acceptance. Other commands and replies retain their
  order. Closing Serve while input is pending stops and joins its work.
- Focused package race tests; the coordinating agent runs the full checks.
