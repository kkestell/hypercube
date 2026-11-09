# Current browser-state formats: implementation

- Task ID: `2026-11-08-004-remove-browser-compatibility`
- Status: implemented; awaiting independent work review.
- Plan (repository-relative):
  `.agents/plans/2026-11-08-004-remove-browser-compatibility.md`
- Accepted plan blob: `781e49e77ac7d71b9ab3217e70fdefad60c2acf3`.
- Accepted review (repository-relative):
  `.agents/reviews/plans/2026-11-08-004-remove-browser-compatibility.md`, Round 2.
- Checkout: `/Users/kyle/src/hypercube/.hypercube/2f4c7990/compat-impl`
  (Hypercube-managed worktree; the single task checkout from this unit on).
- Branch: `hypercube/2f4c7990/compat-impl`, fast-forwarded from main
  `ae022eeebf9dc90c75a61527267db0682b194057` to
  `task/remove-backwards-compatibility` at
  `c4c466adccc8cb0129dd2295560d0d07b1000c47`, then the salvaged 003 work review
  was committed as `3294d5aadfb3f7ae5285933f68928971183f0fa0`. Clean tree and
  empty index confirmed before edits.
- Fixed Code base: `3294d5aadfb3f7ae5285933f68928971183f0fa0`.
- Code head: `013219a047d6d0579c3fe35cebedd4db6d30d35b`.
- Implementation commits: `013219a047d6d0579c3fe35cebedd4db6d30d35b`
  (`X-Stage: work`). Verified tree `37bbb028f8bdb87b21b5033adbe0d57f9d3ea338`
  was written before the commit and is the committed tree; no hooks changed it.
- Author: `compat-impl`, Hypercube child of `2f4c7990`; native conversation
  `1a622741-22c7-4c26-9705-f8832685efee` (`CLAUDE_CODE_SESSION_ID`);
  Claude Code / `claude-opus-5-5` / effort not exposed to the session.
- Work review (repository-relative, not yet written):
  `.agents/reviews/work/2026-11-08-004-remove-browser-compatibility.md`.

## Accepted guidance

These blobs matched the accepted Round 2 inputs before editing.

| Guidance path (repository-relative) | Blob |
| --- | --- |
| `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md` | `2ddcbac1570b7ef08da4015f76cb05ac06cfd42a` |
| `.agents/plans/2026-11-08-003-remove-backwards-compatibility.md` | `16ff0e39fb7c3c940d23526e1bb97aab5b156840` |
| `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md` | `cbb2b2eaf2c1cbba7b7d0dfa0fc60e79d400bf89` |
| `.agents/plans/2026-11-08-006-remove-install-compatibility.md` | `f6bf8eca928d621d7eb122d1f19a7bd86053896e` |

## Changes

All paths are under `internal/ui/web/` unless stated.

- `app/view-storage.ts`: removed the `hypercube.wrap`/`hypercube.fixedWidth`
  read, copy and removal and the eager conversion save. Missing storage keeps
  the initialized workbench; saving happens only on view changes, with the same
  error suppression and retry.
- `app/saved-views.ts`: removed global terminal-visibility conversion, missing
  `workspaceViews` and editor-size defaults, and `agent` tab pruning with its
  `tab-layout` import. `sizes.editor` and `workspaceViews` are now required; the
  validated value is returned unchanged.
- `agent/draft-storage.ts`: removed undefined-settings acceptance, the
  `drafts.new` rename and missing-map acceptance. `unsent` must be null or a
  context and `unsentDrafts` a record; null settings and worktree remain valid,
  and contexts without draft text are still dropped.
- `transcript/transcript-tree.ts`: maps `text` explicitly instead of a default
  branch, and drops the retired Codex labels (`exec_command`, `write_stdin`,
  `apply_patch`, `web_search`, `view_image`, `agent_*`). `shell`, `bash`,
  `subagent` and generic titles stay. Turn grouping is unchanged.
- `sessions/types.ts`: `SessionViewState` comment now describes the current
  composite shape.
- Tests: removed the legacy terminal-visibility and agent-tab migration tests;
  the quota test now starts from current state; the explicit-URL draft fixture
  includes `unsentDrafts: {}`; added missing-`workspaceViews` and
  missing-editor-size failure cases. The draft profile-name test became a
  current null-settings draft that takes the config default once; invalid-data
  cases add a missing `unsentDrafts` map and a context without `settings`. The
  unknown-kind transcript test now checks hidden state items with a text item.

## Deviations

- `scripts/screenshots/main.js` (outside the plan's file list): the screenshot
  tool wrote saved views in the old shape (global `workbench.panel`, no
  per-session `panel`, no `workspaceViews`) and depended on the removed
  conversion. It now writes `panel: false` in each session view (a scene may
  override it) and `workspaceViews: {}`, and its default workbench drops
  `panel`. This is the plan's hand-written-fixture update applied to the one
  non-test writer. Screenshots were not run (out of scope); a throwaway Bun
  script built each `docs/screenshots/*.json` scene's views as the tool does and
  passed all eight through `readSavedViews` without error.
- Not changed, for the parent: screenshot scenes in `docs/screenshots/` use the
  retired `exec_command` and `apply_patch` tool names. They now render with
  generic titles ("Exec command", "Apply patch") instead of "Run command" and
  "Edit files" if screenshots are regenerated. Scene and screenshot edits are
  outside this unit.

## Verification

Run in this checkout on macOS/arm64, Go `go1.27.3 darwin/arm64`, Bun `1.4.2`,
against tree `37bbb028f8bdb87b21b5033adbe0d57f9d3ea338` (Code head).

| Command / inspection | Result |
| --- | --- |
| `cd internal/ui && bun install --frozen-lockfile` | Installed the locked UI dependencies into the fresh worktree (ignored `node_modules`; lockfile unchanged), as `make check` also does. |
| `bun test web/app/view-storage.test.ts web/agent/draft-storage.test.ts web/transcript/transcript-tree.test.ts` and `bunx tsc -p .` (in `internal/ui`) | 50 pass, 0 fail; typecheck clean. Iteration check. |
| Throwaway scene check (`/tmp/compat-impl-check/screens.ts`, not committed) | All 8 screenshot scenes' views accepted by `readSavedViews`. |
| `make check` | Exit 0: browser and website builds, TypeScript, 401 browser tests, 28 desktop tests, `go vet`, gofmt gate, `go test -race ./...`. Full output: `/tmp/compat-impl-check/make-check-004.out`. |
| `git diff --check`, `git diff --cached --check` | Passed; index held exactly the 9 task paths. |
| `git hash-object` on plan and guidance | Matched accepted Round 2 blobs above. |
| `git grep` for retired tool aliases and storage keys | Remaining alias matches are only screenshot scene data (noted above) and historical `.agents`/`agents` records. |

No user browser profiles or state were read or changed; no installs of the app,
service restarts, pushes or merges.

## Session provenance

| Stage / name | Hypercube ID | Native conversation | Harness / model / effort |
| --- | --- | --- | --- |
| Coordinator (this phase) | `2f4c7990` | not supplied | not supplied |
| Earlier coordinator | `a58ee257` | `ses_e41aa9914ffeXSrMrW4OhBDxsQ` | OpenCode / `openai/gpt-6.1-sol` / `xhigh` |
| Planner / `compat-plan` | `d36e9339` | `ses_e41a1172bffeqvkaFBnCzmGxvc` | OpenCode / `openai/gpt-6.1-sol` / `xhigh` |
| Plan review / `compat-plan-review` | unavailable | `ses_e4195b6fcffe3Mncn7jH9SS1YY` | OpenCode / `openai/gpt-6.1-sol` / `xhigh` |
| Implementation / `compat-impl` | child of `2f4c7990`; own ID not exposed | `1a622741-22c7-4c26-9705-f8832685efee` | Claude Code / `claude-opus-5-5` / not exposed |

No blockers. Status stays implemented until independent `xreview work`.
