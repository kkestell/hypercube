# Work review: current browser-state formats

## Round 1 — 2026-11-08

**Verdict: changes-required.** One low-priority finding: the transcript
summary still reads the retired Codex `cmd` input, and four screenshot scene
calls still use it. Everything else in the range is correct and matches the
accepted plan. This is an independent review by a worker that did not plan or
implement the unit.

- Task ID: `2026-11-08-004-remove-browser-compatibility`; mode: `work`.
- Author: `compat-review`, Hypercube child of `2f4c7990`; native conversation
  `c805986c-9023-4e3e-a660-11912f975ea2` (`CLAUDE_CODE_SESSION_ID`);
  Claude Code / `claude-opus-5-5` / effort not exposed.
- Checkout: `/Users/kyle/src/hypercube/.hypercube/2f4c7990/compat-impl`,
  branch `hypercube/2f4c7990/compat-impl`.
- Fixed Code base: `3294d5aadfb3f7ae5285933f68928971183f0fa0` (ancestor of
  head, verified).
- Reviewed Code head: `3beb54b94516c6ad46058387a7e2ed1022a88bd7`, tree
  `4c4105fa48ff925ac716f2b282434706f95b57e3`. Implementation commits
  `013219a047d6d0579c3fe35cebedd4db6d30d35b` (tree
  `37bbb028f8bdb87b21b5033adbe0d57f9d3ea338`) and the fix commit at head.
- Checkout HEAD at review: `3dc2109`; Code head to HEAD changes only the
  work log. Working tree and index clean.
- Work log (repository-relative):
  `.agents/work/2026-11-08-004-remove-browser-compatibility.md`.
- Plan (repository-relative):
  `.agents/plans/2026-11-08-004-remove-browser-compatibility.md`, blob
  `781e49e77ac7d71b9ab3217e70fdefad60c2acf3`, matching the accepted Round 2 plan
  review. Shared design blob `2ddcbac1570b7ef08da4015f76cb05ac06cfd42a` matches.

### Findings

**F1 (low): retired Codex `cmd` input remains in the transcript summary and
screenshot scenes.**

- `internal/ui/web/transcript/transcript-tree.ts:19`: `summaryKeys` still
  lists `'cmd'`. It was added with the Codex tool polish (`42ae1d9`) for
  `exec_command`'s input; no current harness or plugin emits a `cmd` field
  (`git grep` over `plugins/` and `internal/` finds none). The unit removes the
  `exec_command` label but keeps the label's input mapping, so the code still
  carries a Codex-specific read path.
- `docs/screenshots/details.json:69` (Claude `Read`),
  `docs/screenshots/suggestion.json:63,91,107` (Pi `read`, `read`, `edit`):
  these calls still use `cmd`. The Repair 1 scene update replaced retired tool
  names but left these Codex-shaped inputs, so the scenes do not show current
  harness input either.
- Correction: drop `'cmd'` from `summaryKeys`, and give the four scene calls
  the current input fields (`file_path` for Claude `Read`, `path` for Pi
  `read`/`edit`, with the file path as value). Rendering is unchanged for old
  saved calls: with `cmd` absent from the list, `toolSummary` falls back to the
  first string input.

### Assessment

- **View storage** (`app/view-storage.ts`, `app/saved-views.ts`): the legacy
  `hypercube.wrap`/`hypercube.fixedWidth` reads, removals and eager save are
  gone. With no saved views, the initialized workbench (`state.ts:87`,
  `wrap: false, fixedWidth: true`) equals what the removed branch produced for
  missing keys, so fresh startup is unchanged. `readSavedViews` now requires
  `workspaceViews` and a positive `sizes.editor` and returns the validated
  value; `snapshot` always writes both, so current writer output round-trips.
  The `tab-layout` import went with the `agent` tab pruning.
- **Drafts** (`agent/draft-storage.ts`): `readSaved` requires `unsentDrafts` as
  a record and `unsent` as null or a context; the writer always emits both.
  `readContext` requires `settings` to be null or settings, keeping null as the
  "defaults not yet arrived" state. The drop of contexts without text and the
  workspace-key check are kept.
- **Transcript** (`transcript/transcript-tree.ts`): the exhaustive `case 'text'`
  replaces the default branch; the typecheck passing confirms `rowOf` covers
  the whole non-state union. `shell`, `bash` and `subagent` labels remain.
- **Tests**: migration tests are removed; the quota-retry test now starts from
  default state and still asserts one error, in-memory changes, and a later
  successful save. New missing-`workspaceViews`/editor-size cases and the
  added draft invalid cases exercise exactly the tightened validation. The
  rewritten null-settings draft test covers taking the config default once.
- **Deviation 1, `scripts/screenshots/main.js`**: correct. It is the only
  non-test writer of `hypercube.views`. Writing `panel: false` per session
  (scene view may override; no scene sets `panel`) and `workspaceViews: {}`
  gives the same result the removed conversion gave, since the tool saves
  `selected: null` and the conversion opened the panel only for the selected
  session. Removing `panel` from the default workbench matches `state.ts`.
- **Deviation 2, scene tool names**: correct for the changed calls. Every
  changed call is in an OpenCode session. `shell` with `command` is the name
  the OpenCode plugin's live native test drives
  (`plugins/opencode/native_test.go:85-93`). `edit` with `path` could not be
  checked locally; it relies on the log's cited OpenCode documentation and
  renders as "Edit" with the path. See F1 for the inputs this repair left.

### Verification

| Check | Status | Source |
| --- | --- | --- |
| `make check` on tree `37bbb028…` (013219a) | reused, exit 0, 0 failures | `/tmp/compat-impl-check/make-check-004.out`; log Verification |
| Scene views pass `readSavedViews`, changed calls title as "Run command"/"Edit" | reused | log Repair 1, `/tmp/compat-impl-check/scene-titles.ts` |
| Anchors, ancestry, commit trees, plan/design blobs, Code head→HEAD diff | run | `git merge-base --is-ancestor`, `git rev-parse`, `git hash-object`, `git diff --stat` |
| Origin and current use of `cmd` input | run | `git log -S"'cmd'"`, `git grep` over `plugins/`, `internal/`, `docs/screenshots/` |

The fix commit changes only scene JSON, which `make check` does not read, so the
earlier run still covers all code at head. No new test run was needed: the open
questions were answered by reading code and Git history.

Limitations: the OpenCode `edit` tool's input shape was not verified against a
running OpenCode. Screenshots were not regenerated, as the plan excludes them.
