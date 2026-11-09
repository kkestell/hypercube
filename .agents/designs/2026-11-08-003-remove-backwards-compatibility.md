# Retire historical Hypercube formats

- Root task: `2026-11-08-003-remove-backwards-compatibility`
- Status: ready; shared execution guidance for the four plans below
- Revision: 2; corrects Round 1's live grant producer and current owned-link scope
- Source HEAD: `ae022eeebf9dc90c75a61527267db0682b194057`
- Checkout: `/Users/kyle/.local/share/opencode/worktree/803f5f/remove-backwards-compatibility`
- Branch: `task/remove-backwards-compatibility`; clean index/worktree confirmed before planning
- Author: planner `compat-plan`, Hypercube `d36e9339`, native conversation
  `ses_e41a1172bffeqvkaFBnCzmGxvc`, OpenCode / `openai/gpt-6.1-sol` / `xhigh`
  (effort confirmed by the parent's creation metadata).
- Coordinator: Hypercube `a58ee257`, native `ses_e41aa9914ffeXSrMrW4OhBDxsQ`,
  OpenCode / `openai/gpt-6.1-sol`; parent effort not supplied with the assignment.
- All paths in tables are repository-relative.

## Request and preservation boundary

Remove all migration and backwards-compatibility machinery throughout Hypercube.
Both computers ever to run it are now on the latest version. Preserve the current
database, configuration, browser-state writer formats, and supported process and
protocol behavior. The older-format inventory is evidence to classify, not a
requirement to delete every fallback or every historically introduced field.

No reset, deletion of actual user state, installation, service/app restart, push,
or merge is authorized. Tests use isolated temporary state and disposable Linux
containers, never the installed application's state or processes. Retiring this
historical chain does not establish a permanent prohibition on future migrations.
Leave historical research, plans, reviews, and Codex-retirement records intact.

## Bounded units and integration

| ID / execution plan | Responsibility | Technical prerequisite |
| --- | --- | --- |
| [003](../plans/2026-11-08-003-remove-backwards-compatibility.md) | Store/config current baseline, startup and related current documentation | None |
| [004](../plans/2026-11-08-004-remove-browser-compatibility.md) | Browser saved-state conversions and obsolete transcript aliases | None; current writer formats stay unchanged |
| [005](../plans/2026-11-08-005-remove-protocol-compatibility.md) | Plugin aliases, live grant-answer encoding, old CLI commands and runtime exceptions | None; surviving latest plugins already accept array answers |
| [006](../plans/2026-11-08-006-remove-install-compatibility.md) | Build/install/dev historical cleanup and integrated Linux verification | Execute after 005 so Linux verifies the final CLI/protocol |

The parent schedules 003 → 004 → 005 → 006 in the same exclusive checkout.
That is mostly administrative ordering, not a dependency between independent
formats. Each intermediate commit must work with its current peers: no schema,
writer format, holder format, or executable location changes. Unit 005 changes
new grant answers to the already-supported array form, without rewriting saved
command lines. Units 003 and 005 both edit different portions of
`cmd/hypercube/integration_test.go`; preserve the preceding unit's changes.
Each unit owns its same-basename `.agents/work/` and plan/work-review records.

Four units are warranted by the separate persistence, browser, protocol/runtime,
and platform-test responsibilities. Do not combine them into one execution of
the whole inventory. These are retirement slices through existing mechanisms,
not four new subsystems.

## Settled baseline decisions

**Database:** use the present sessions schema directly, including both transcript
search indexes. `CREATE ... IF NOT EXISTS` is ordinary idempotent initialization
for first runs and reopens, not an old-schema adapter. Execute it transactionally
without table/column probes, `user_version` reads/writes, or data rewrites.
Existing `user_version=4` remains inert; fresh databases need no version marker.

`sessions.last_active` is currently `INTEGER NOT NULL` without a default, including
in databases created by this HEAD. Keep that inert column and AddSession's
required initial value in the single current baseline. Removing it from inserts
breaks current databases; removing it from actual databases requires precisely
the new migration/reset the task excludes. Do not introduce schema variants,
conditional inserts, a replacement timestamp feature, or a new version gate.

**Saved browser data:** keep the shapes emitted by the existing view/draft writers.
Readers validate those shapes without converting obsolete ones. Missing storage
still means fresh defaults. Invalid storage still uses the existing error path;
do not eagerly clear it, scan real profiles, or add a versioned storage system.
Null draft settings/worktree remain valid current values while defaults arrive.
Partial hand-written config remains supported through ordinary config defaults.

**Live agents:** current starts contain `mcp`, and ordinary answers use `choices`,
but `internal/session/grants.go:recordAllow` still produces scalar `choice` for
AlwaysAllow and automatic granted approvals. Unit 005 must change that producer
to `Choices: []string{item.Choices[0]}` in the same change that removes the scalar
field/decoder. Already-recorded command lines stay byte-for-byte unchanged:
`commands.go:send` sends their saved bytes to the holder, and surviving latest
plugins run the existing Serve that understands both scalar and array answers.
Those plugins also accept newly produced arrays. Do not decode/re-encode queued
commands or add migration/recovery machinery. Keep replay order, duplicate
suppression, durable commands, launch adoption, SIGTERM detachment, queued inputs,
per/all-session grants, automatic approval, and holder-owned shells. No session
restart is part of implementation or validation against user state.

## Complete candidate disposition

| Candidate / paths | Decision and owner |
| --- | --- |
| `internal/store/migrate.go`: agents→sessions conversion, launch/base/fork/input_pending/report/title upgrades, running/awaiting_user removal | Delete implementation and old-schema fixtures/tests; 003 |
| Same file: version 1 transcript cleanup, 2 profiles→settings, 3 search indexes, 4 catalogs removal, unconditional attention cleanup | Delete upgrade/data-repair paths; place index definitions in current schema; 003 |
| `internal/store/store.go`: migration call, `IF NOT EXISTS`, `last_active`, directory chmod | Replace migration call with schema initialization; keep idempotence, required baseline column/insertion, and active permission protection; 003 |
| `internal/config/config.go`: legacyProfile, Upgrade, upgradeProfiles, upgradeNotifications; `cmd/hypercube/backend.go` | Delete converters/startup invocation and migration tests; retain Load/Save/defaults/validation; 003 |
| `cmd/hypercube/integration_test.go` old config fixture | Use current selection/guidance config without weakening lifecycle coverage; 003 |
| `internal/ui/web/app/view-storage.ts` old wrap/fixedWidth keys | Delete read/copy/delete and eager conversion-write path; 004 |
| `app/saved-views.ts`: global terminal visibility, missing workspaceViews/editor size, agent-tab pruning | Delete conversion/default/pruning, require current fields, return validated current structure; 004 |
| `agent/draft-storage.ts`: profile-name defaults, drafts.new remap, missing parked draft map | Delete adapters, require current settings-or-null and unsentDrafts map; keep current null/default and text restoration; 004 |
| `sessions/types.ts` historical SessionViewState shape | Composite editor choices plus transcript disclosures are still emitted/consumed by current attachment/storage; retain representation, correct comment; 004 |
| `transcript/transcript-tree.ts` native labels | Remove retired Codex-only aliases (`exec_command`, `write_stdin`, `apply_patch`, `web_search`, `view_image`, `agent_*`); keep current `shell`, `bash`, `subagent` and generic tool titles; 004 |
| Same file: missing state/final, unknown kind | Keep ungrouped current startup/in-progress/failed/interrupted rows and optional Final handling; remove explicit future-kind-as-text fallback in favor of the current exhaustive kind mapping; 004 |
| Raw harness/model display, optional submission IDs, changes comparison defaults | Keep: deleted model entries and custom harness IDs still exist; current nonbrowser callers lack provisional IDs; uncommitted changes are a current default. No old-shape conversion exists here. |
| `internal/harness/protocol.go` Choice/StartArgs.Hypercube, `plugins/internal/plugin/serve.go` scalar answer, `plugin.go` MCPCommand and four callers; `internal/session/grants.go:recordAllow` | Change live grant producer to a one-element Choices array in the same unit as deleting aliases/helper; direct bridge command and array answers, with required MCP validation at shared Serve boundary. Keep saved-line replay unchanged; 005 |
| `cmd/hypercube/main.go` mcp/hold commands; old integration bridge tests | Delete compatibility dispatch, keep dedicated executable tests and latest lifecycle/replay coverage; 005 |
| `internal/session/offers.go` two-choice Dismiss exception | Validate against saved current item choices; delete legacy two-choice test variant, retain current Dismiss behavior; 005 |
| `internal/session/child_options.go` explicit saved fork dirs | A fork takes no dir: reject nonempty dir, retain checkout/non-Git sharing and worktree-parent placement; 005 |
| `internal/session/sessions.go` exact-input workspace identity | Keep: a current plain subfolder can become part of a newly initialized parent Git checkout. Existing identity/reclassification tests protect current behavior, not solely old-version repair. |
| `plugins/opencode/configure.go` default effort with no controls | Keep: current models can have zero variants. Provider default adds no variant; this is current selection normalization, not version detection. Existing configure tests cover it. |
| Makefile/make platform named Codex executable/link deletion | Delete named historical cleanup; keep current bundle replacement/link creation/uninstallation; 006 |
| `scripts/linux-install` generic owned broken-plugin-link pruning | Retain unchanged: current custom bundle plugins can disappear on stock-bundle reinstall. Keep its ownership restriction; replace the Codex-named test fixture with a neutral additional plugin and retain replacement reconciliation coverage; 006 |
| `make/macos.mk` earlier-backend executable distinction | Replace with current desktop-running check before quit; do not remove normal desktop quit or touch service SIGTERM behavior; 006 |
| `scripts/dev` old `hypercube hold` match | Delete that alternative, retain current holder cleanup scoped to dev state; 006 |
| Generic JSON tolerance, external-input validation, MCP SDK negotiation, signing and AppArmor | Keep current supported boundaries/security/platform integration; no historical-version machinery to remove. |
| `agents/development.md`, `docs/upgrades.md`, `agents/architecture.md` | Correct only live statements affected by retirement; 003 owns saved-state descriptions, 005 CLI description. No historical-record rewriting or website/screenshot restructuring. |

## Verification ownership

Every implementation code commit requires `make check` beforehand, as repository
instructions require. Prefer one code commit per unit; use focused tests during
iteration only for a concrete uncertainty, not routine duplicate package suites.
Reviewers reuse matching recorded evidence instead of automatically rerunning it.
Unit 006 additionally runs Linux bundle/install behavior in a disposable container;
the selected scripts cover current helper layout, replacement/owned custom-link
reconciliation, service restart, shell environment and holder survival, without
requiring desktop screenshots or release packaging. Unit 005 keeps grant scope,
automatic approval and restart coverage alongside byte-for-byte command replay.
Each work log records exact commands/results and code anchors.

Planning ran no build/test suite. `docker info --format
'{{.OSType}}/{{.Architecture}}'` returned `linux/aarch64`; Go, Bun and Docker are
available. External contract checks used current OpenCode V2
[tools](https://opencode.ai/v2/docs/tools) and
[models](https://opencode.ai/v2/docs/models): current native tools and models
without effort variants must not be mistaken for historical adapters.

At the end, audit executable source/build/install paths for migration symbols and
old names, classify remaining matches using this table, and inspect all task
diffs. A grep hit in historical prose, a current test's ordinary word "older", or
a current third-party protocol is not a defect. A newly discovered actual
historical mechanism belongs to the appropriate unit if it remains bounded;
otherwise report it to the parent before expanding the accepted scope.
