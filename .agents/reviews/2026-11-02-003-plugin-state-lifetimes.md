# Plugin state lifetimes

## Scope and coverage

Reviewed all 12 Go files touched by 7575443 in full, as present at a7536c9,
plus the shared RPC, process, and serve paths they use. The Go sources match
7575443; its accidental `claude` build artifact was already absent. Read
`AGENTS.md` and the harness sections of `agents/reviews/go-audit.md`.

No model-backed harness sessions were started. Event lifetimes were checked
against the installed Claude Code 2.1.316 implementation and pinned upstream
Codex source, then reproduced through plugin events. Tests use isolated
stand-ins. No further correctness or readability findings were confirmed.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | High | Correctness | Fixed | Main results discard active Claude subagent state |
| F2 | Medium | Correctness | Fixed | Codex forgets completed items before late shell events |
| F3 | Low | Testing | Fixed | Pi cancellation test bypasses stop |

Locations below refer to the files before the review fixes.

## F1. Main results discard active Claude subagent state

High · Correctness · Fixed · `plugins/claude/main.go:616`

A main turn's result does not end its background subagents. Clearing `blocks`
restarts a child message's block index, so a later block replaces an earlier
one with the same item ID. Clearing `tools` makes `toolResults` discard a
later result for a child tool already shown as running.

### Evidence

Claude Code 2.1.316's background subagent loop writes assistant and user
messages to the active output directly, independently of the main turn.
The SDK documents that background reports arrive in later turns and that
complete blocks of one response share a message ID.
See [background subagents](https://code.claude.com/docs/en/agent-sdk/subagents#choose-whether-claude-waits-for-subagents)
and [message flow](https://code.claude.com/docs/en/agent-sdk/streaming-output#message-flow).

Temporary Go overlays reproduced both failures: two child blocks straddling
a main result became one transcript item, and a child tool result after a
main result left its displayed tool unfinished. The permanent tests check
those emitted transcript items without reading either map.

### Fix

The lead approved removing both result-time clears and retaining message
block counters. Unfinished tools remain available for their results;
`toolResults` already deletes each entry when its result arrives. No new
cleanup mechanism was added.

## F2. Codex forgets completed items before late shell events

Medium · Correctness · Fixed · `plugins/codex/events.go:244`

A background shell can finish after another main turn starts. `endTurn`
already marked its displayed tool finished. Clearing `finished` in
`startTurn` allows the old command's completion to emit again. If the same
completion arrives before the next turn, it is ignored. This changes the
previous behavior and makes output depend on unrelated turn timing.

### Evidence

Codex's [background exit watcher](https://github.com/openai/codex/blob/67f0994b532b8740f14b5b738af7b7b2a915b1d6/codex-rs/core/src/unified_exec/async_watcher.rs#L155)
retains the original turn context while waiting for the process to exit.
Its [tool emitter](https://github.com/openai/codex/blob/67f0994b532b8740f14b5b738af7b7b2a915b1d6/codex-rs/core/src/tools/events.rs#L561)
then completes that turn's command item; the app-server translates canonical
item completions to `item/completed` notifications.

A temporary overlay reproduced the extra emission after the next
`turn/started`. The permanent test checks that receiving the old command's
completion emits nothing further.

### Fix

The lead approved restoring process-lifetime `finished` tracking. Removed
the clear and corrected its comments. Late completion handling retains the
previous behavior; no turn filter or additional state was added.

## F3. Pi cancellation test bypasses stop

Low · Testing · Fixed · `plugins/pi/main_test.go:429`

`TestStopCancelsPendingCall` called `ag.cancel` directly and did not establish
that input was pending. It could pass even if `stop` stopped cancelling calls.

### Fix

With the lead's approval, the existing stand-in records an unanswered prompt
and waits until the test releases it. The test waits for that record, calls
`stop`, and checks that input returns `context.Canceled` before allowing the
stand-in to exit. Removing cancellation from `stop` through a temporary Go
overlay makes the revised test fail with a process-exit error instead.

## Checks run

- `go vet ./plugins/...`: passed before changes.
- `go test -race ./plugins/...`: passed before changes.
- Temporary event reproductions: all three state-loss/re-emission checks
  failed before their fixes, as expected; repository files were unchanged.
- Focused regression tests with `-race`: passed after fixes.
- Temporary Pi mutation omitting `stop` cancellation: revised test failed,
  as expected.
- `make check`: passed, including builds, generated-UI comparison, all 113
  browser unit tests, Go vet, formatting, and the complete Go race suite.
- `git diff --check`: passed.

## Verdict

Pass after the accepted fixes. F1, F2, and F3 are fixed. Claude block counters
and Codex completed-item IDs intentionally remain tracked for the process
lifetime, as the lead requested.
