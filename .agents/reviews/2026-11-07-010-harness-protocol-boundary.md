# Harness protocol boundary

## Scope and coverage

Reviewed the staged implementation of
[the plan](../plans/2026-11-07-007-harness-protocol-boundary.md) and its
[work log](../work/2026-11-07-007-harness-protocol-boundary.md), including the
complete touched Go files, development reference, and relevant session,
browser, MCP, and holder callers and tests. The initial review was completed
before writing this report; no implementation edits were needed.

Compared the moved client and discovery functions with their originals using
an exact text comparison after applying the expected package qualifications
and parameter rename. Their bodies and the retained discovery protocol data
match. `DecodeRecord` preserves the original decoder and its validation;
session call sites change only the owning package. The executable-directory
fallback, holder launch, environment, cancellation, errors, and record
positions remain unchanged. The complete touched files are easy to follow.

Independent dependency assertions confirm that `harness` imports only the
standard library; each shipping plugin depends internally only on `harness`;
the store depends internally only on itself and `harness`; and the client
depends internally only on itself, `harness`, and `hold`.

There are no material coverage gaps for this behavior-preserving extraction.
The work log and parent report successful `go build ./...` and full
`make check`, including 368 browser tests, 28 desktop tests, and all Go race
tests. This review independently ran the focused checks below. Live native
harnesses, Linux packaging, and installation were not exercised; their
behavior and packaging do not change in this task. Later executable boundary
changes remain outside this review.

## Summary

No findings. No code fixes were needed.

## Checks run

- `go test -race ./internal/harness ./internal/harnessclient
  ./internal/session ./internal/ui -run
  '^(TestDecodeChecksEvents|TestDescribe|TestModels|TestModelsCancel|TestSessionStartsAndDeliversInOrder|TestRestartAppliesOutputAndStreamingOnce|TestDetachReleasesBlockedPluginSender|TestHarnessesAndModels|TestModelsCacheAndRefresh|TestNewSessionAndSelection)$'
  -count=1` — passed in all four packages, using the existing temporary Go
  build cache. Covers decoding, discovery errors and cancellation, launch,
  ordered delivery, reattachment, blocked-sender cleanup, and model routes.
- Assertions over `go list -json ./internal/harness` and `go list -deps` for
  the four shipping plugins, store, and client — passed with the dependency
  sets described above. Go emitted nonfatal module-cache write warnings under
  the sandbox; the commands and assertions succeeded.
- Exact comparison of transferred client and discovery code against `HEAD`
  after expected name qualifications — passed.
- `rg -n 'harness\.(Client|Launch|Attach|Plugins|Find|Describe|Models)\b'
  --glob '*.go' .` — no matches.
- `git diff --cached --check` — passed.

## Verdict

Pass. The extraction meets the plan and preserves existing behavior. Nothing
remains unresolved. Only this review was added and staged by the reviewer;
the implementation remains as received, and no commit was made.
