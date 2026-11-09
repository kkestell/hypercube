# Read only the current browser-state formats

- Task ID: `2026-11-08-004-remove-browser-compatibility`
- Status: ready; awaiting independent plan review
- Source HEAD: `ae022eeebf9dc90c75a61527267db0682b194057`
- Author: `compat-plan`, Hypercube `d36e9339`, native
  `ses_e41a1172bffeqvkaFBnCzmGxvc`; OpenCode / `openai/gpt-6.1-sol` / `xhigh`
- Artifact path (repository-relative): `.agents/plans/2026-11-08-004-remove-browser-compatibility.md`
- Shared guidance (repository-relative): `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md`,
  linked [here](../designs/2026-11-08-003-remove-backwards-compatibility.md).
- Technical prerequisites: none; scheduled after 003 for exclusive-checkout work.
- Request: remove historical browser storage/transcript adapters without losing
  current writer-format data or supported transcript behavior.

## Outcome and boundaries

View/draft readers validate and restore what their current writers emit; they do
not rename keys, fill missing historical fields, translate old profiles, or prune
obsolete chat tabs. Keep current storage keys and writer shapes. No browser
storage reset, versioning system, profile scan, installed-app interaction,
screenshots, or unrelated editor/attachment redesign.

This is a browser-boundary retirement, not a requirement to remove all defaults
or generic rendering. The shared scope map identifies the supported behavior
that must remain even when comments mention its history.

## Existing flow and decisions

`internal/ui/web/app/view-storage.ts:snapshot` saves selected/workbench/sessionViews/
workspaceViews. `editor/view-state.ts` saves only editor choices; attachment
combines a session's editor view with transcript disclosure flags. Keep this
current composite `SessionViewState`; its shape is not itself a compatibility
layer. Update its misleading historical-format comment in `sessions/types.ts`.

`app/saved-views.ts:readSavedViews` currently converts global terminal visibility,
supplies missing workspace views/editor size, then closes old `agent` tabs.
Delete those transformations and their imports. Require `workspaceViews` and
positive finite `sizes.editor`, alongside the existing current-shape validation,
and return the validated SavedViews unchanged. Do not add unknown-property bans,
storage versions or new old-format diagnostics. Existing invalid-storage errors
are enough; conversion retirement does not require rejecting harmless extra keys.

`agent/draft-storage.ts` writes `{drafts, unsent, unsentDrafts}`. Every saved draft
context has workspace, settings (object or null), and worktree (boolean or null).
Keep null as current semantics: settings/default worktree may not have arrived
or been chosen. Keep `withDraftDefaults` and transient-state reconstruction
(identity, sending=false, images=[], reading=0). Remove undefined-settings
acceptance for old profile names, the `drafts.new` rename, and missing-map
acceptance. Validate current `unsent` as null or a context and `unsentDrafts` as a
record, retaining matching workspace-key checks and ignoring contexts with no
remaining draft text as today. Do not add a key-migration or cleanup pass.

Transcript kinds are already the current discriminated union and validated at
the plugin boundary. Map `text` explicitly in `transcript/transcript-tree.ts`
instead of treating all unknown/future kinds as text. Use the exhaustive current
kind mapping, not another generic fallback or a new validation subsystem.

Native tool labels are not all historical. Keep `shell`, `bash` and `subagent`
labels used by supported harnesses. Delete the retired Codex-only aliases:
`exec_command`, `write_stdin`, `apply_patch`, `web_search`, `view_image`, and
`agent_spawn_agent`, `agent_send_input`, `agent_wait`, `agent_resume_agent`,
`agent_close_agent`. Generic human-readable tool titles still display old saved
tool identities/content, without maintaining bespoke mappings for them.

Keep turn rendering when state/final markers are absent: current start errors,
streaming, interruption/failure, and protocol-optional Final legitimately require
ungrouped rows. Keep ordinary message/tool nesting and optional fields. Likewise
raw harness/model names after entry deletion, optional submission IDs for
nonbrowser callers, and default uncommitted comparisons remain current behavior;
do not modify them in this unit.

## Execution sequence

1. In `view-storage.ts`, remove the legacy flag, reads/removals of
   `hypercube.wrap`/`hypercube.fixedWidth`, and eager migration save. Without
   `hypercube.views`, retain the already initialized workbench. Persist only on
   ordinary view changes, retaining error suppression/retry and subscription
   cleanup. Equality checking compares only the current serialized snapshot.
2. Simplify `saved-views.ts` to current-shape validation/return as above; preserve
   layouts, panels, splits, active panes, scroll, file selections and disclosures.
3. Simplify draft reading as above without changing writing or normal text
   removal after send/erase. Preserve both active and parked workspace drafts.
4. Adjust transcript label/kind handling only; keep grouping algorithms and
   supported-kind types intact. Correct directly affected comments, not unrelated
   presentation code or old documentation records.
5. Remove migration-only cases from `app/view-storage.test.ts` and
   `agent/draft-storage.test.ts`. Where migration and useful behavior share a
   test, retain the useful portion: in particular the view-storage quota-failure
   retry test currently starts with old wrap keys. Give it normal current state
   instead. Update hand-written fixtures (including the explicit-session URL
   test) to include current `unsentDrafts: {}` and required current fields.

## Verification

Keep current writer→reader round trips as the primary oracle, using the existing
Bun storage/store helpers. Preserve independent session and workspace panels,
editor size, file/diff tabs and splits, active pane, scroll and disclosure flags;
server file contents, live handles, and transcripts must not enter storage.
Retain explicit-URL precedence, removal of deleted owners, and fresh-default
startup when storage is missing. Add focused invalid-current-field cases for
missing workspaceViews/editor size and malformed draft context/map only where
needed to exercise changed validation; do not build a catalog of extinct formats.

Draft tests must cover active and parked multiworkspace text, current explicit
settings/worktree, null settings resolved once when config arrives, ordinary
session/request drafts, and cleared text. Invalid data or storage quota failure
must still report through existing errors and leave in-memory typing usable.
Never clear storage merely on startup failure.

In `transcript-tree.test.ts`, remove the artificial future/unknown-kind fixture
while retaining hidden state-marker and all current-kind coverage. Keep no-final,
in-progress, interruption/failure, response-only, nesting and current shell/bash
label checks. Remove assertions that mandate retired tool-label aliases; generic
tool naming/content remains independently covered. Do not rewrite all historical
transcript fixtures to demand complete markers.

Run `make check` before committing code. Its UI build/typecheck and Bun suite
cover this change; no visual/manual installed-app check is necessary. During
iteration, `bun test web/app/view-storage.test.ts web/agent/draft-storage.test.ts
web/transcript/transcript-tree.test.ts` from `internal/ui` is an available focused
check, not a routine extra full suite. Inspect the full touched files and record
commands/results in this unit's work log.

## Handoff and normalization

Ready; no unresolved decisions. The work is a normal boundary simplification
through three existing storage readers plus small obsolete renderer mappings,
with current-format regression coverage. It introduces no new persistence or
editor lifecycle and is distinct from the database/protocol units. Detail states
current accepted shapes and observable preservation without supplying function
bodies. Use the experimental 600–1,500-word band; no curated manifest exists.
This planning stage does not implement, stage or commit.
