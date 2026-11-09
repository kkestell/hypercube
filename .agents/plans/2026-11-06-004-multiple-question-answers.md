# Multiple question answers

## Goal

Resolve #3: questions that allow several selections show checkboxes and an
explicit submit action, and every selected label reaches the native harness.
Verify the existing #13 framing fix at the HTTP boundary.

## Decisions

- Extend attention items with `multi` and answer commands with `choices`.
  Preserve the old command `choice` field when replaying saved commands. No
  database migration is needed: transcript and command JSON accept added fields.
- Use repeated `choice` query parameters for answers, keeping the existing
  text body and single-choice HTTP requests unchanged. The session and plugin
  answer functions take a list of choices. Validate selections against the open
  item before recording a command.
- Claude's native answers map contains comma-separated labels, as documented
  in Anthropic's AskUserQuestion demo. OpenCode array fields and Codex MCP
  enumerated array fields receive actual arrays. Native single-choice questions
  and approvals keep their behavior; Pi select dialogs remain single-choice.
- Use the existing draft store for selected options as well as typed answers,
  so transcript and notification forms share one draft. Preserve the existing
  text-only draft persistence policy. No separate question subsystem.

## Related code

- `internal/harness/protocol.go`, `plugins/internal/plugin/serve.go` — command
  fields and dispatch, including saved command replay.
- `internal/session/input.go`, `internal/ui/actions.go` — external answer input.
- `plugins/{claude,opencode,codex,pi}/requests.go` — native question semantics.
- `internal/ui/web/transcript/attention-form.tsx`, `api.ts`, `agent/drafts.ts`
  — controls, transport, and draft ownership.

## Implementation plan

1. Extend protocol and list-based answer functions, retaining old saved command
   decoding. Update stand-ins and affected test calls without changing their
   responsibilities. The plugin agent owns shared dispatch and Codex/Pi changes.
2. Decode Claude multiSelect and OpenCode array fields; return every label in
   each native format, retaining typed answers and grouped-question completion.
3. Add checkbox rows and one submit action for multi questions. Single-choice
   rows still answer immediately. Keep controls disabled during submission and
   usable after a refused answer.
4. Add focused browser/native/HTTP regressions for list transport, selections,
   retry, single-choice behavior, and invalid external choices. Check #13's
   existing response-header test without altering its fix.

## Test plan

- Several labels, including labels containing punctuation, survive HTTP and
  plugin command serialization and arrive in the native answer.
- Unsupported or unknown selections are refused without consuming the request.
- Selected options and typed text survive a form remount and failed submission.
- Existing question/approval tests and the framing HTTP test continue to pass.
- The root session runs generated UI build, `make check`, and a dev smoke test.
