# Git error classification: implementation review

## Round 1

- Task ID: `2026-11-08-002-git-error-classification`; mode: work.
- Verdict: **changes-required**: R1 still allows a checkout command failure to
  become `ErrNotGit` through a symlinked subfolder.
- Independent author: `03a23830` / `git-errors-work-review` (underlying child hex
  unavailable from exposed metadata); native conversation
  `ses_e429a0a6bffe7d28QZnWLRQd3i`; OpenCode / `openai/gpt-6.1-sol` /
  default effort. Not the planner or implementer. Native ID obtained through
  documented `opencode.session_move` to the same confirmed absolute checkout.
- Checkout: `/Users/kyle/src/hypercube/.hypercube/03a23830`, branch
  `hypercube/session-03a23830`; starting/current HEAD
  `2fd5fcb2df4386ae15db4b7730c9aec5573ecf71`; initially clean.
- Fixed Code base: `5ac099bae76ca484d3a0996d32811f7c82b13706`.
- Reviewed Code head: `28cbbe71fe7c0c855990a86868c5ec62d1463fb4`.
  Both commits exist; base is an ancestor of head. The sole implementation
  commit is Code head. Later HEAD adds only the named work log; no later code,
  configuration, test, or untracked implementation differences were present.
- Inputs (repository-relative):
  `.agents/work/2026-11-08-002-git-error-classification.md`
  (blob `194561f9dd458454b0d6062af0cd02942836cb81`),
  `.agents/plans/2026-11-08-002-git-error-classification.md`
  (accepted blob verified: `6424f2dd339946ca4f58338405f208bc0c7f3861`),
  `.agents/reviews/plans/2026-11-08-002-git-error-classification.md`
  (round 1 accepted; blob `388d8272eb6b0ce642bd1df64ecadbd397c7dde9`).
  No linked design inputs. Issue #59 outcome supplied by coordinator and preserved
  in the plan. Repository guidance blobs match the accepted round.
- Implementation author: `03a23830` / `git-errors-work`, native
  `ses_e429faa09ffeXJi763YZQboz9a`; OpenCode / `openai/gpt-6.1-sol` / default.

### R1 — medium: inspect physical ancestors before declaring a folder outside Git

Location: `internal/workspace/workspace.go:152–163` (`Root` failure path), using
the lexical ancestor walk at lines 110–128; propagated through
`internal/session/sessions.go:23–48` and `internal/ui/changes.go:64–68`.

Create a real checkout with `repo/sub`, then a symlink in an unrelated directory
pointing to `repo/sub`. Successful `Root(alias)` correctly discovers the checkout.
Replace Git with the existing test-style wrapper emitting `test-git-refusal` and
exiting 128. `Resolve` preserves the alias, and `HasGitMetadata` walks its lexical
parents rather than the checkout's physical ancestors. It therefore misses
`repo/.git`: the actual `Root(alias)` returns `ErrNotGit`, and `At(alias)` returns
the alias with no error. Consequently AddWorkspace can still insert/persist
`NoGit=true`, while Changes/diff discard the genuine refusal for this valid input.
This violates the requested distinction, not a new compatibility guarantee.

Correction: after failed Git discovery and directory validation, resolve the
physical directory for Root's metadata inspection only, propagating resolution
errors. Keep successful Git discovery authoritative, retain the original resolved
input for saved-path matching/plain-folder returns, and leave mention detection's
existing helper behavior unchanged. Add a focused symlink-to-nested-checkout
regression proving the refusal remains an error and cannot become a saved
classification. This corrects an unhandled case in the plan's chosen failure
mechanism; the implementation otherwise follows its guidance. No path migration,
record merging, or broader symlink rewrite is needed.

### Independent assessment

Inspected the full eight-path implementation diff, complete touched files and
relevant callers, persistence/publication, HTTP error mapping, fixtures and test
assertions. Apart from R1, the difficult invariants hold: successful Root returns
before inspection; missing/invalid paths cannot become the sentinel; ordinary
metadata-backed diagnostics propagate; AddWorkspace discovers once before saving,
prefers exact saved input over root, and updates only `no_git` transactionally.
Existing IDs, paths, creation times and session references remain intact. Mention
detection is the original helper moved without behavioral changes. Both HTTP
callers now preserve errors through `fileError`. Whole touched files remain clear;
the focused session test split is proportionate. No other material findings.

### Verification

- **Reused:** implementer's `make check` on the final unchanged implementation
  committed as `28cbbe71fe7c0c855990a86868c5ec62d1463fb4`, macOS/arm64.
  Independently read the complete output at
  `/Users/kyle/.local/share/opencode/shell/803f5fc9179a0cdaf3cc1e7bbcca471e3cd3ceb8/sh_1bd62e5410014LCahDpoFuLpV6.out`;
  native `ses_e429faa09ffeXJi763YZQboz9a`, shell
  `sh_1bd62e5410014LCahDpoFuLpV6`. Browser/website and Go builds, TypeScript,
  393 browser tests, 28 desktop tests, vet, gofmt gate and all Go race tests passed.
  Makefile confirms the quiet gofmt gate precedes race tests. Only nonfatal React
  `act(...)` warnings. Coordinator separately confirmed final-code correspondence
  and command completion. This passing suite does not cover R1.
- **Run:** read-only Git identity/status, branch, full range/history, ancestry,
  later-record-only diff and input-blob checks; source/caller/test inspection.
- **Run, focused gap probe:**
  `go test -overlay=/private/var/folders/x3/s0dy9w311sncwf87160crpw80000gn/T/opencode/git-classification-review-joeazf8u/overlay.json ./internal/workspace -run '^TestReviewRootFailureThroughSymlink$' -count=1`.
  Failed as expected, exit 1: Root returned `"", folder is outside Git`; At
  returned the alias with `<nil>` instead of the original refusal. The probe first
  verifies real Git successfully discovers that same alias. Overlay/test source
  lives only in the named temporary directory; no repository code was edited.
  Exact command and output are in this reviewer's native conversation above.
  This new run addresses the concrete physical-versus-lexical ancestry gap; no
  broad suites were rerun.

Limitations: no Linux-container or live-app run; neither is needed to establish
R1. AddWorkspace/HTTP consequences follow the inspected sentinel branches; the
fresh probe directly exercised Root and At. No fixes, author log/plan edits,
staging, commits, delegation, installation, merge or push performed. Parent owns
the review commit and repair routing. Acceptance awaits R1 repair and review of
the full updated range against the original Code base.
