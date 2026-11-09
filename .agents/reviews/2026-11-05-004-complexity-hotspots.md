# Complexity hotspots

## Scope and coverage

Reviewed the working tree at `b0f0fc0`, focusing on the ten application files
identified in the preceding complexity measurements:

- `internal/session/{event,removal,alerts,commands,grants,restore}.go`
- `plugins/codex/events.go`
- `internal/ui/web/agent/composer.tsx`
- `internal/ui/web/transcript/markdown.tsx`
- `internal/ui/web/app/shortcuts.ts`

Read those files in full and traced their immediate session, persistence,
delivery, history, and keyboard-handling dependencies. Also inspected the
three highlighted test hotspots: `cmd/hypercube/integration_test.go`,
`internal/store/migrate_agents_test.go`, and
`internal/session/removal_test.go`. Used the user guides and existing restart
and child-coordination plans as behavioral context.

The user's follow-up overrides the skill's fix phase: this is a review only.
No application code or tests were changed. Every finding is classified
`Trivial`, meaning its recommendation preserves established behavior and needs
no new user decision; no recommendation has been applied.

Gaps:

- This is a focused review, not a review of the entire repository.
- Native harnesses and the installed Electron app were not exercised. Go
  behavior checks used the repository's isolated harness stand-in.
- Frontend checks exercised DOM interactions, not browser appearance or visual
  rendering. The full application-process integration test and migration tests
  were inspected but not run.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Correctness | Trivial; not applied | Automatic child reports enter user prompt history |
| F2 | Medium | Correctness | Trivial; not applied | Starting alerts during a question can announce an old reply |
| F3 | Medium | Readability | Trivial; not applied | Event application mixes live updates and durable effects |
| F4 | Medium | Readability | Trivial; not applied | Family cleanup hides its lifecycle phases in one function |
| F5 | Medium | Readability | Trivial; not applied | Composer history transitions are embedded in keyboard dispatch |
| F6 | Medium | Readability | Trivial; not applied | Codex events combine lifecycle handling and payload conversion |

## F1. Automatic child reports enter user prompt history

Medium · Correctness · Trivial; not applied ·
`internal/ui/web/agent/history.ts:8`

After a child report arrives, ArrowUp recalls the automatic coordination
message instead of the user's last prompt. Enter can then resend that report
as a user-authored instruction. The history predicate excludes messages with
`from` but does not exclude `automatic` messages.

### Evidence

`internal/session/reports.go:102` produces reports with `kind: user`,
`automatic: true`, and no sender. They therefore satisfy `promptHistory`'s
predicate. The composer uses that result at `composer.tsx:69`.

A temporary mounted-composer test supplied a real user prompt followed by an
automatic report, typed a draft, and pressed ArrowUp. Its assertion expected
`Investigate the bug` but received `All children are idle or have stopped.
Continue with their reports: ...`. The test failed on the current code.

### Recommended fix

Exclude automatic items from user prompt history, alongside the existing
sender exclusion. Protect the distinction with a composer interaction test.

## F2. Starting alerts during a question can announce an old reply

Medium · Correctness · Trivial; not applied ·
`internal/session/alerts.go:100`

Restarting the backend while an agent is asking a question can cause an old
reply to be announced when the question is answered. `startApp` starts the
watcher after restoring the agents. This violates `WatchAlerts`'s contract
that replies already present at startup are not announced.

### Evidence

The initial `read(nil)` skips busy sessions before calling `LastText`, so it
does not initialize their reply positions. When such a session next becomes
idle without producing new text, its old reply position is greater than the
map's zero value and is treated as a new reply.

A temporary Go overlay test used existing fixture operations: create a session
with reply `ack`, issue `!ask`, initialize the watcher, answer the question,
and read alerts after the session becomes idle. It failed because the watcher
emitted `Text:ack` for the earlier turn. The overlay did not modify repository
files.

### Recommended fix

Seed reply positions for every session during the initial snapshot, including
busy sessions. Keep later reply eligibility and the existing busy-to-idle
handling for replies already marked seen. No stronger delivery guarantee is
needed.

## F3. Event application mixes live updates and durable effects

Medium · Readability · Trivial; not applied ·
`internal/session/event.go:82`

`eventLocked` handles item namespacing, grant lookup, streaming accumulation,
feed publication, state-marker ordering, persistence, child reports,
checkpointing, and delivery scheduling in one 123-line function. A reader
changing one concern must retain the other concerns' ordering and state in
mind. Its measured cognitive complexity is 76.

The problem is the interleaving, not the score: item handling at lines 93–140
and the transaction at lines 153–191 are distinct responsibilities with an
important boundary between them. The streaming-only early return sits between
them and determines whether a checkpoint may advance.

### Recommended fix

Extract the per-item live update and the transaction's event effects into
direct helpers, leaving the entry point to express their order. Preserve one
transaction for durable effects, reports, and checkpointing, and preserve the
streaming-only path. A new event framework or state owner is unnecessary.

## F4. Family cleanup hides its lifecycle phases in one function

Medium · Readability · Trivial; not applied ·
`internal/session/removal.go:27`

`removeFamily` combines membership admission, child/archive validation, Git
preflight, writer shutdown, repeated safety checks, incremental child cleanup,
and final archive/delete persistence. The archive flag changes behavior in
several distant blocks. Its measured cognitive complexity is 59.

The claim transaction occupies lines 33–65; process shutdown starts at line
94; partial-progress writes start at line 123; final persistence starts at
line 132. Those phases carry different lock and failure rules, but all remain
embedded in the same function. That makes maintaining the existing cleanup
safety rules unnecessarily difficult.

### Recommended fix

Give membership claiming and completed-member persistence named helpers while
keeping the cleanup sequence visible in one short orchestration function.
Preserve the preflight before stopping, the checks after stopping, and each
completed child's recorded progress. Keep the existing deletion claim and
worktree representation rather than introducing another lifecycle mechanism.

## F5. Composer history transitions are embedded in keyboard dispatch

Medium · Readability · Trivial; not applied ·
`internal/ui/web/agent/composer.tsx:59`

The key handler combines IME handling, mention priority, caret eligibility,
transcript filtering, recalled-copy edits, history-index arithmetic, draft
restoration, mention dismissal, and submission. History transitions are buried
four conditions deep, including a nested conditional expression for the next
index. Its measured cognitive complexity is 35.

The existing `history.ts` already owns prompt extraction, while the rest of
the history rules live among DOM event handling and component rendering.
Changing recall behavior consequently requires reasoning about unrelated
keyboard policies at the same time.

### Recommended fix

Move the pure calculation of a history step and its displayed text into the
existing history module. Keep state owned by the composer, and retain the
visible dispatch order: IME, mentions, history, submission. Existing multiline,
edited-copy, and mention-priority interaction tests are useful protection.

## F6. Codex events combine lifecycle handling and payload conversion

Medium · Readability · Trivial; not applied ·
`plugins/codex/events.go:135`

This 558-line file contains notification routing and turn ownership alongside
disk image reads, MIME detection, base64 conversion, image-result fallback,
and tool-specific presentation. The reader moves from image loading at lines
135–200 to turn transitions at lines 203–360, then back to payload translation
at lines 455–558.

Those concerns have different inputs and failure behavior. Keeping them in
one file obscures the main/child turn state machine and makes unrelated payload
changes compete with lifecycle code. The notification and tool functions
score 37 and 35 respectively, but the finding is the file's mixed ownership.

### Recommended fix

Move image loading/decoding and tool payload conversion into files named for
those concerns within the same plugin package. Keep notification routing,
turn ownership, and completion ordering together. Use the existing concrete
data and direct functions; no shared plugin interface is needed.

## Checks run

- Focused session checks passed under `-race`: alerts, event state and markers,
  grants, restart and restore, family cleanup, child ending, partial removal,
  and acceptance during stop. Command:
  `go test -race ./internal/session -run 'Test(RequestAlerts|ReplyAlerts|SeenRepliesStillAlert|ExistingRequestsAndRepliesAreNotAlerted|EventsWithoutStateKeepTheAgentWorking|StateMarkersBracketTheTurnsItems|AlwaysAllow|Restart|Restore|FamilyRemoval|EndChild|RefusedRemoval|PartialFamilyRemoval|StopAppliesAcceptance)'`.
- Focused Codex checks passed under `-race`:
  `go test -race ./plugins/codex -run 'Test(.*Turn.*|UnknownThreadIsIgnored|.*Image.*|.*Async.*|.*Subagent.*)'`.
- `bun test web/agent/composer.test.tsx web/transcript/markdown.test.tsx web/app/shortcuts.test.ts`
  in `internal/ui`: 37 passed, zero failed.
- The temporary composer regression probe failed as expected for F1. Its
  source and output are in
  `/tmp/hypercube-hotspot-review.Dx0bco/history-probe.test.ts` and
  `history-probe.log`.
- `go test -race -overlay /tmp/hypercube-hotspot-review.Dx0bco/overlay.json ./internal/session -run '^TestReviewWatcherStartsDuringQuestion$' -count=1 -v`
  failed as expected for F2. Source and output are in the same temporary
  directory as `alerts_probe_test.go` and `alerts-probe.log`.
- Inspected the review diff and confirmed staging adds only this review.
  No `make check` was run because application code and tests were unchanged.

## Verdict

Six medium findings remain open: two reproduced behavior defects and four
readability defects. The review is staged; unrelated staged and untracked
files were left untouched. No fixes were applied, committed, or installed.
