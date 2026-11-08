# Remove the Codex harness integration

## Scope and coverage

Independent review of the staged implementation of
[plan 012](../plans/2026-11-07-012-remove-codex-integration.md) and its
[work log](../work/2026-11-07-009-remove-codex-integration.md). I inspected the
whole touched files, the deleted plugin and shared helpers, their remaining
callers, discovery and restore paths, build and installer upgrade behavior,
saved JSON compatibility, UI labels and transcript display, scenes, and
documentation.

I also checked that runtime code and plugin discovery contain no Codex
reference, that build and install code names it only to remove retired
artifacts, and that no deleted shared API has a remaining caller. Stored Codex
data and historical `agents/` records are not removed or rewritten. The review
covers the complete staged change, including the binary diagram.

Material gaps: no full macOS bundle was built, existing raster screenshots
were not regenerated (as the plan directs), and no live harness CLI was
exercised during this review. I relied on the recorded `make check` and Linux
packaging results for those paths.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | --- |
| F1 | Low | Documentation | Fixed | Attention item definition still describes asynchronous questions |

No medium or high severity findings.

## F1. Attention item definition still describes asynchronous questions

Low · Documentation · Fixed ·
`agents/glossary.md:76`

The glossary still defined an attention item as one "which can leave the agent
working when marked asynchronous." The protocol no longer has an asynchronous
question, no current harness emits one, and `waitsForUser` now treats every
open main-conversation attention item as waiting. The definition described a
concept that no longer exists and contradicted the updated protocol comment.

### Evidence

After the staged removal, a repository search for current (non-historical)
`async` wording found only this glossary line. The protocol now says the agent
waits for the answer, and the stand-in and report tests no longer model an
asynchronous question.

### Resolution

Changed the definition to "which the agent waits for." No other current guide
describes asynchronous questions.

## Checks run

- Temporary `go test ./internal/harness -run TestOldAsyncItemStillReads`
  — passed. It decoded a representative saved Codex item containing
  `"async": true` and verified its identity, question fields, and free-text
  flag; the temporary test was removed afterwards.
- Inline shell exercises of the Linux installer's prune rule and the macOS
  `place` removal condition — an owned dangling Codex link was removed and an
  unrelated custom link was kept.
- `git grep`/`rg` for current `codex`, `async`, `ReadLines`, `Requests.Find`,
  `ParseForm`, and RPC references — only intentional cleanup, historical
  records, generic test sample IDs, and preserved Codex-data documentation
  remain.
- Inspected the architecture PNG (1440×2432) and screenshot scenes; no Codex
  settings remain in the scenes.
- Reused the implementation's recorded `make check`, focused race tests, and
  Linux bundle/install/release results. No production code changed during
  review, so the full checks were not repeated.
- `git diff --cached --check` — passed.

## Verdict

Pass. The Codex support surface is removed coherently, stored Codex JSON still
reads, and build and install upgrades clean only owned artifacts. F1 was a low
documentation inconsistency and is fixed. No unresolved findings.
