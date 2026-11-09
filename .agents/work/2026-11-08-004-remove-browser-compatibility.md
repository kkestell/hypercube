# Current browser-state formats: implementation

- Task ID: `2026-11-08-004-remove-browser-compatibility`
- Status: complete.
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
- Code head: `d8f1a39dd44a2e02e92506061e3f0f0773bd221e`.
- Implementation commits: `013219a047d6d0579c3fe35cebedd4db6d30d35b`
  (`X-Stage: work`), `3beb54b94516c6ad46058387a7e2ed1022a88bd7` (`X-Stage: fix`,
  see Repair 1), `d8f1a39dd44a2e02e92506061e3f0f0773bd221e` (`X-Stage: fix`,
  see Repair 2). Verified tree `37bbb028f8bdb87b21b5033adbe0d57f9d3ea338`
  was written before the commit and is the committed tree; no hooks changed it.
- Author: `compat-impl`, Hypercube child of `2f4c7990`; native conversation
  `1a622741-22c7-4c26-9705-f8832685efee` (`CLAUDE_CODE_SESSION_ID`);
  Claude Code / `claude-opus-5-5` / `high`.
- Work review (repository-relative):
  `.agents/reviews/work/2026-11-08-004-remove-browser-compatibility.md`, Round 2; final verdict accept;
  reviewed range `3294d5aadfb3f7ae5285933f68928971183f0fa0..d8f1a39dd44a2e02e92506061e3f0f0773bd221e`;
  review committed in `bc0f339b5e96e1a8508b70be55560e73a80916e3`.

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
- Screenshot scenes in `docs/screenshots/` used the retired `exec_command` and
  `apply_patch` tool names. The parent assigned the fix to this unit; see
  Repair 1.

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

| Stage / name | Hypercube ID | Harness | Native conversation | Model / effort |
| --- | --- | --- | --- | --- |
| Original coordinator (planning, 003; stopped by the user) | `a58ee257` | OpenCode | `ses_e41aa9914ffeXSrMrW4OhBDxsQ` | `openai/gpt-6.1-sol` / `xhigh` |
| Planner / `compat-plan` | `d36e9339` | OpenCode | `ses_e41a1172bffeqvkaFBnCzmGxvc` | `openai/gpt-6.1-sol` / `xhigh` |
| Plan review / `compat-plan-review` | `3fdeced3` | OpenCode | `ses_e4195b6fcffe3Mncn7jH9SS1YY` | `openai/gpt-6.1-sol` / `xhigh` |
| Resuming coordinator (004–006, finalization) | `2f4c7990` | Claude Code | `bfa5e290-5355-47b7-b8a3-d81a92eca8cd` | `claude-opus-5-5` / not exposed |
| Implementation / `compat-impl` | child of `2f4c7990` | Claude Code | `1a622741-22c7-4c26-9705-f8832685efee` | `claude-opus-5-5` / `high` |
| Work review / `compat-review` | child of `2f4c7990` | Claude Code | `c805986c-9023-4e3e-a660-11912f975ea2` | `claude-opus-5-5` / `high` |

## Repair 1: current tool names in screenshot scenes

Requested by coordinator `2f4c7990` before review. Code base stays
`3294d5aadfb3f7ae5285933f68928971183f0fa0`. Fix commit and new Code head:
`3beb54b94516c6ad46058387a7e2ed1022a88bd7` (tree
`4c4105fa48ff925ac716f2b282434706f95b57e3`).

- `docs/screenshots/{attention,file-preview,first-session,overview,suggestion,worktrees}.json`:
  every retired call was in an OpenCode session. `exec_command` with `cmd`
  became `shell` with `command`, and now renders "Run command". `apply_patch`
  became `edit` with `path`, and now renders "Edit" with the file path. These
  are the OpenCode V2 tool names and inputs (https://opencode.ai/v2/docs/tools).
  "Edit files" no longer exists as a label, and the closest current title is
  "Edit". OpenCode gives some GPT models a `patch` tool with `patchText`
  instead of `edit`. `edit` was chosen because its `path` input gives the
  same one-line path summary the scenes showed before. Scene data only; PNG
  screenshots not regenerated.

Verification, run against the fix commit's tree:

| Command / inspection | Result |
| --- | --- |
| `bun /tmp/compat-impl-check/scene-titles.ts` (from `internal/ui`, not committed) | All 8 scenes parse and their views pass `readSavedViews`. No `exec_command`/`apply_patch` remain. All 16 changed calls get `toolTitle` "Run command" or "Edit" with the command or path as summary. |
| `git grep "docs/screenshots"` over code, `Makefile`, `make`, `scripts` | Only `make screenshots` (`scripts/screenshots/main.js`) reads the scenes, so `make check` does not cover them and was not rerun. Its earlier pass on `013219a…` still covers all code, because the fix changed only scene JSON. |
| `git diff --cached --check` | Passed; index held exactly the 6 scene files. |

## Repair 2: retired `cmd` summary key

Addresses finding F1 of Round 1 in
`.agents/reviews/work/2026-11-08-004-remove-browser-compatibility.md`
(review commit `f544ce62c399dd6267a1d0e743156505f382a86d`), accepted by
coordinator `2f4c7990`. Code base stays
`3294d5aadfb3f7ae5285933f68928971183f0fa0`. Fix commit and new Code head:
`d8f1a39dd44a2e02e92506061e3f0f0773bd221e` (tree
`240c94bc10e94148e9397d7b6cabf140ace0b419`).

- `internal/ui/web/transcript/transcript-tree.ts`: removed `'cmd'` from
  `summaryKeys`; only the retired Codex `exec_command` used it. No test used a
  `cmd` input.
- `docs/screenshots/details.json` `c1` (Claude `Read`): `cmd` → `file_path`.
- `docs/screenshots/suggestion.json` `c1`, `c3` (Pi `read`) and `c4` (Pi
  `edit`): `cmd` → `path`. The two `read` values drop their `cat ` prefix,
  leaving the file path.

Verification, run against the fix commit's tree (macOS/arm64, Bun `1.4.2`):

| Command / inspection | Result |
| --- | --- |
| `bun test web/transcript/transcript-tree.test.ts` (in `internal/ui`) | 27 pass, 0 fail. |
| `bunx tsc -p .` (in `internal/ui`) | Clean. |
| `bun /tmp/compat-impl-check/scene-titles.ts` | All 8 scenes parse and pass `readSavedViews`. |
| `bun /tmp/compat-impl-check/cmd-titles.ts` (not committed) | The 4 changed calls render "Read \| build.go", "Read \| content/guide/getting-started.md", "Read \| content/guide/sync.md", "Edit \| content/guide/getting-started.md". |
| `git grep '"cmd"' -- docs/screenshots` | No matches. |
| `git diff --cached --check` | Passed; index held exactly the 3 changed files. |

`make check` was not rerun. This fix changes one array literal in
`transcript-tree.ts`, which the focused tests and typecheck cover, plus scene
JSON that no check reads. The earlier `make check` pass on `013219a…` covers
the rest of the tree.

No blockers.
