# Always allow tool approvals

## Scope and coverage

Reviewed the implementation against
`agents/plans/2026-11-03-003-always-allow-tools.md`, AGENTS.md, and the accepted
architectural decisions. Compared task files with
`/tmp/hypercube-7bc17838-baseline`, using HEAD for files absent there, excluding
unrelated dirty changes. Reviewed the touched production files, associated
tests and documentation, and callers governing grants, durable answers,
publication, mirroring, alerts, restoration, and stopping.

Browser visuals were not assessed. The parent reported that `make ui` and
`make check` passed; this review did not repeat those checks.

## Summary

No findings.

The implementation keeps grants in the host database and reuses the existing
answer queue. Matching respects exact session, harness, and tool identity.
Automatically approved requests stay out of pending attention and publication;
completion records retain Grant and are dropped without additional live state.
Automatic answers remove their waiters and exclude stopping launches.

Accepted limitations: Codex MCP approvals lack a stable tool identity and keep
prompting, as do arbitrary permission, network host, and grant-root requests.
No grant revocation feature was requested or implemented.

## Checks run

- Passed focused tests in `internal/store`, `internal/session`,
  `internal/harness`, `plugins/claude`, and `plugins/codex` using
  `go test ./internal/store ./internal/session ./internal/harness ./plugins/claude ./plugins/codex -run 'Test(Grants|AlwaysAllow|OnlyCommandAndFileRequestsCanBeAlwaysAllowed|ToolPermissionPrompt|QuestionBecomesAttentionAndAnswerReachesClaude|DecodeChecksFinalItems)' -count=1`.

## Verdict

No confirmed correctness or readability defects. No implementation changes
were made during this review.
