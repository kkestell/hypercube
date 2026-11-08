# Git error classification: implementation

- Task ID: `2026-11-08-002-git-error-classification`
- Status: complete; independent review finding repaired locally as directed by user.
- Plan (repository-relative):
  `.agents/plans/2026-11-08-002-git-error-classification.md`
- Accepted plan blob: `6424f2dd339946ca4f58338405f208bc0c7f3861`.
- Accepted plan review (repository-relative):
  `.agents/reviews/plans/2026-11-08-002-git-error-classification.md`, round 1.
- Guidance blobs: no linked designs or transitive shared-design guidance.
  Repository inputs: `AGENTS.md`
  (`5511f174c0041550712d40f1b803b0a7fe617a21`),
  `agents/development.md` (`fe3a70054763a818cc82f6e13de8d7140a4bdcc8`).
  All accepted inputs matched before implementation and before committing.
- Checkout: `/Users/kyle/src/hypercube/.hypercube/03a23830`;
  branch: `hypercube/session-03a23830`. Starting checkout was clean and exclusive.
- Fixed Code base: `5ac099bae76ca484d3a0996d32811f7c82b13706`.
- Final Code head: `63825069cbca83d9d8aa87c09c968330e71ec784`.
- Independently reviewed Code head: `28cbbe71fe7c0c855990a86868c5ec62d1463fb4`.
- Implementation commits: `28cbbe71fe7c0c855990a86868c5ec62d1463fb4`
  (`X-Stage: work`), `63825069cbca83d9d8aa87c09c968330e71ec784`
  (`X-Stage: fix`, coordinator-owned). No hook changes or unrelated files.
- Author: implementation worker `git-errors-work`; native conversation
  `ses_e429faa09ffeXJi763YZQboz9a`; OpenCode / `openai/gpt-6.1-sol` /
  default effort.
- Work-review path (repository-relative):
  `.agents/reviews/work/2026-11-08-002-git-error-classification.md`.

## Changes

- `internal/workspace/workspace.go`: moved the ancestor metadata check from the
  UI into `HasGitMetadata`. `Root` preserves successful discovery and classifies
  failed commands as `ErrNotGit` only after validating the directory and finding
  no metadata. `At` falls back only for that sentinel.
- `internal/ui/mention_files.go`, `internal/ui/changes.go`: reuse the metadata
  check for mentions; propagate root-command failures through the existing HTTP
  error response rather than showing a plain-folder result.
- `internal/session/sessions.go`, `internal/store/store.go`: discover once before
  saving, select the exact resolved input before the discovered checkout root,
  and update only `no_git` on an existing record. Errors never enter the saving
  transaction; workspace identity, saved paths and session references remain.
- `internal/workspace/workspace_test.go`, `internal/ui/changes_test.go`,
  `internal/session/workspaces_test.go`: cover plain folders, nested and linked
  checkouts, refusal diagnostics, invalid metadata and executable-start failures;
  HTTP Changes/diff failures; no-write failures; duplicate identity; both
  reclassification directions; and saved-subfolder precedence without moving or
  merging records or changing sessions.

No consequential deviations. Session workspace regressions live in the focused
`workspaces_test.go` rather than adding more concerns to `sessions_test.go`.
No migration, global Git changes, background repair, live-app run, install, merge,
push or delegation occurred.

## Verification and evidence

The initial checks below were **run** on macOS/arm64 in the task checkout against
the tree committed unchanged as `28cbbe71fe7c0c855990a86868c5ec62d1463fb4`.
Final repair verification is recorded below.

| Command | Result | Evidence |
| --- | --- | --- |
| `make check` | Passed: browser build and website build; Go builds; TypeScript; 393 browser tests; 28 desktop tests; `go vet ./...`; gofmt gate; `go test -race ./...` including workspace, session, UI and existing mention regressions. | Native conversation `ses_e429faa09ffeXJi763YZQboz9a`, shell `sh_1bd62e5410014LCahDpoFuLpV6`, completed command/tool notification. Full output file below. |
| `git diff --check` | Passed before staging; final diff and new session test file inspected. | Same native conversation, shell result immediately after completed `make check`. |
| `git diff --cached --check` | Passed; index contained exactly the eight named implementation/test paths. | Same native conversation, staging/inspection shell result preceding the work commit. |

Full local validation output:
`/Users/kyle/.local/share/opencode/shell/803f5fc9179a0cdaf3cc1e7bbcca471e3cd3ceb8/sh_1bd62e5410014LCahDpoFuLpV6.out`.
React tests emitted non-fatal `act(...)` warnings; no check failures occurred.
No narrow suites or duplicate broad checks were needed. No installed user
workspaces were used for tests. Linux container validation was not run; this
change has no platform-specific branch or command.

## Session provenance

All stages use OpenCode / `openai/gpt-6.1-sol` / default effort. Worker addresses
are distinct from underlying Hypercube child hex IDs, which exposed metadata does
not supply. Coordinator supplied the earlier stages' native IDs; the implementer
obtained its own through documented `opencode.session_move` to the same confirmed
absolute checkout (no directory change).

| Stage | Hypercube coordinator / worker address | Native conversation |
| --- | --- | --- |
| Coordinator | `03a23830` | `ses_e42fd27dfffekQ56hXQYkGJZrg` |
| Plan | `03a23830` / `git-errors-plan` (child hex unavailable) | `ses_e42a503a4ffezrsVNqdzqGJDuM` |
| Plan review | `03a23830` / `git-errors-plan-review` (child hex unavailable) | `ses_e42a1a009ffegDdoyRUt3YjfK4` |
| Implementation | `03a23830` / `git-errors-work` (child hex unavailable) | `ses_e429faa09ffeXJi763YZQboz9a` |
| Work review | `03a23830` / `git-errors-work-review` (child hex unavailable) | `ses_e429a0a6bffe7d28QZnWLRQd3i` |

## Coordinator repair and completion

Independent review found R1: a symlink to a checkout subdirectory could hide the
physical `.git` ancestor on Git failure. The original finding remains intact in
the linked review. The user stopped further delegation; the coordinator repaired
this small correction directly rather than initiating more handoff rounds.

Repair `63825069cbca83d9d8aa87c09c968330e71ec784` resolves the physical directory
only for `Root`'s failed-command metadata inspection. Successful discovery, saved
input paths and the mention helper are unchanged. Existing workspace and session
regression tables now cover symlinked subfolders and failure without insertion.

**Run by coordinator:**
- Before the fix, `go test ./internal/workspace ./internal/session -run
  '^(TestRootFailures|TestAddWorkspaceFailureDoesNotSave)$' -count=1` reproduced
  the false non-Git classification and unintended saved workspace insertion.
- After the fix, `make check` passed against the unchanged final code tree:
  393 browser tests, 28 desktop tests, builds/TypeScript/vet/gofmt and all Go race
  tests, including both expanded regression tables. Existing React `act(...)`
  warnings remained non-fatal. `git diff --check` passed.
- Evidence: coordinator native conversation `ses_e42fd27dfffekQ56hXQYkGJZrg`,
  foreground shell results for those exact commands after the user's stop on
  further delegation. The repair was inspected and committed without code changes
  after validation.

R1 is resolved by the locally checked repair. The final repair was **not**
independently re-reviewed; the original review is not relabeled as accepting it.
This is a user-directed departure from xauto's mandatory re-review gate. No other
findings or blockers remain. No merge, push, installation or GitHub issue closure.
