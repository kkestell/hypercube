# Replace agent profiles with discovered harnesses and models

## Goal

Remove named agent profiles. Detect installed harnesses when Hypercube starts,
and let the user choose any advertised, valid harness, model, and effort
combination when creating a session. Existing sessions retain their saved
settings and conversations. A running session can change model and effort
within its harness, as it can today.

Children inherit their parent's current accepted settings unless the parent
passes an explicit override. An optional global guidance field tells agents
when to choose another model. Hypercube validates technical compatibility;
the guidance is an instruction, not an enforced restriction on spending or
model choice.

This plan is the requested deliverable in `agents/plans/`. It does not authorize
implementation, installation, or changes to the user's configuration now.

## Current implementation and what changes

| Concern | Current code | Result |
| --- | --- | --- |
| Configuration | `internal/harness/config.go` requires profiles and resolves `default_profile`. | Plain preferences, an optional default selection, and optional child-model guidance; legacy profiles exist only in the upgrade decoder. |
| Plugin boundary | `internal/harness/harness.go` finds a named executable; `protocol.go` defines start and configure. | Enumerate plugin executables and ask each plugin about installation and models. The core still names no native harness. |
| Native integration | Each `plugins/{claude,codex,opencode,pi}/main.go` handles only `serve`. | Each plugin also handles short-lived discovery commands using its native protocol. |
| Session creation | `internal/session/sessions.go` looks up a profile. | Accept and validate explicit settings. |
| Model changes | `internal/session/profile.go` sends a model/effort selection; `commands.go` stores the accepted reply. | Rename this operation around selection, retain its command ordering and partial-failure behavior. |
| Child creation | `child_options.go`, `children.go`, `proposals.go`, and `offers.go` pass a full profile. | Resolve inherited or overridden settings once, save concrete settings in proposals, and validate again when starting. |
| Agent tools | `internal/mcp/children.go` builds a profile enum from configuration. | Optional harness/model/effort strings and an on-demand model-discovery tool. |
| Instructions | `internal/report/report.go` injects profiles and their descriptions. | Short inheritance instructions plus the user's guidance; no full model catalog. |
| Browser | `internal/ui/web/agent/profile-select.tsx` and Settings read profiles. | Direct harness, searchable model, and effort controls, shared across creation and model changes. |
| Saved state | Sessions store actual settings, but proposal JSON embeds `profile` and browser drafts store profile names. | Preserve sessions, migrate embedded selections, and upgrade draft storage without losing prompt text. |
| Supporting tools | `scripts/dev`, `scripts/screenshots/main.js`, and `plugins/scripted` construct profiles. | Supply preferences and deterministic discovery responses without requiring native harness installations. |

Do not introduce a router, model-quality database, role system, approved-model
editor, or a second session-launch path. The new choices must flow through the
existing runtime, proposal, offer, and configure operations.

## Product decisions

### User selection

- New sessions choose **Harness**, **Model**, and **Effort**. Show provider
  identity in model search results where models have providers. Preserve full
  native IDs, including IDs whose model portion contains slashes.
- Use labels and effort order supplied by the plugin. Do not maintain model
  names, rankings, a universal effort enum, or translations such as “max means
  xhigh” in the core or browser.
- An effort control is absent when the selected model has no effort setting.
  “Use the harness default” is distinct from an explicit native effort named
  `default`. Use an advertised default effort when available. Otherwise an empty
  effort means no override: omit the native argument and display “Harness
  default” when the model has effort controls. Store a concrete effective level
  if the native protocol reports one; never invent a level it does not report.
- Keep a draft's own selection while it is edited or parked. For a newly
  opened draft, prefer the newest top-level session's saved settings in that
  workspace, then the configured default selection. Otherwise leave the
  selection empty until the user opens the picker. Do not fetch model catalogs
  merely to initialize a draft. Inside an open picker, use an advertised native
  default when present; catalog order is not a quality ranking.
- If an explicit draft or configured choice is unavailable, retain it visibly
  and require a new choice rather than silently replacing it. A default can
  initialize a new draft; it must not rewrite an existing one.
- Keep the existing global default preference as a direct combination,
  `default_selection`, instead of a profile name. Its Settings control uses
  discovery in the currently selected workspace, or the first workspace when
  none is selected. With no workspace, explain that a workspace is needed to
  choose a default; other preferences remain editable. Saving preferences
  validates their structure without requiring the default's provider to be
  available in every workspace.
- Settings replaces “Agent profiles” with a small “Agents” section containing
  the optional **Guidance for choosing child models** textarea. Explain that
  children inherit by default and guidance is not an enforced model limit.
  Do not add a separate child default in this change.
- No installed harness, no configured provider, and a discovery error are
  distinct states. Keep the app, saved transcripts, and Settings usable in all
  three. Disable new-session submission until a valid selection is available.
  If a harness exposes models but no default model, ask the user to select one;
  do not infer its recommended model from catalog order.

### Child selection

Use the following resolution rules for fresh children:

| Request | Effective selection |
| --- | --- |
| No overrides | Copy the parent's currently accepted harness, model, and effort. |
| Effort only | Keep the parent's harness/model and validate the requested effort for that model. |
| Model only | Use the parent's harness and the requested model. If the model differs, leave effort empty (the harness's default, which the plugin reports at start) unless effort is explicit. |
| Different harness | Require an explicit model; do not copy a model ID into another harness. An omitted effort is empty, the harness's default. |
| Explicit same harness/model, omitted effort | Retain the parent's effort. |

Keep the existing native-fork constraint: the harness cannot change. Preserve
the model/effort changes that a plugin supports for a fork; do not introduce a
new blanket restriction on all forks based on one harness's native subagent
API. Unsupported combinations fail clearly.

Inject this rule into top-level instructions and the child-creation tool:

> A child uses your current harness, model, and effort unless you pass others.
> Pass others only when the user requested that selection, or the user's
> guidance calls for it for this kind of task. Otherwise omit the overrides.

An ordinary “delegate this” inherits. “Ask Opus” can use an explicit override.
Do not treat a request for delegation itself as permission to choose any model.
The host enforces validity, not the truth of an agent's account of the user's
intent. No new approval step is introduced.

The effort rules above use the native-default representation when discovery
does not provide a concrete default. An omitted field inherits; it does not
implicitly clear an inherited effort. Selecting another model can reset effort
to that model's default. Do not add a separate override-clearing protocol just
for this change.

Resolve an `own_initiative` proposal when it is created and save its concrete
settings. Start uses exactly those saved settings even if the parent changes
models later. Revalidate availability on Start without changing the saved
choice. Validation happens in that child's normal native startup, not in a
separate preflight process. A rejected start leaves the proposal usable and
shows the failure.

Leave the offer lifecycle intact. `initiation: "offer"` still accepts only a
title. After the user chooses New session, the parent fills the child settings
and brief. That click approves delegation, not a model that has already been
shown. Do not add model controls or a second approval to offers in this change.

### Visibility

- Proposal cards show the resolved harness/model/effort before Start and retain
  those settings in their completed history.
- Every child's composer shows its accepted selection, including when it is
  not ready to accept a configuration change.
- Derive an accessible icon and hover text when a child's current selection
  differs from its parent's current selection. Place it by the selection and
  in the child sidebar row so a departure is visible without opening every
  child. Compare the existing session fields; add no persistent difference flag.
- Return the effective selection in successful child-creation tool results.
  Include model and effort in `list_sessions` results as well as the harness.
- Use plugin display labels when available and the saved native ID otherwise.
  Saved and unavailable selections must remain readable without a working
  catalog.

## Discovery boundary

### Plugin commands and data

Keep `serve` and its durable JSONL command stream. Add two one-shot commands:

1. `describe`: no request body; returns the plugin's display name and whether
   its native executable can be found in the backend's launch environment.
   This must not start a native agent. Installation does not imply authentication.
2. `models`: reads one JSON request with an absolute `dir` and optional `model`,
   writes one JSON result, and exits. Without `model`, return the current
   directory's available model choices and default selection. With `model`,
   also return that model's accepted effort choices and default effort.

Define the wire data in `internal/harness/discovery.go` alongside the client
functions. Model entries contain native ID, display label, optional provider
label, and any resolved alias identity the native API actually reports. The
response contains the default selection and, for a requested model, its effort
choices in native order. Do not synthesize effort lists for the whole catalog
by switching through hundreds of models. Preserve omitted/default metadata
explicitly rather than interpreting an empty list as a transport failure.

Use a plain `harness.Settings` value for a complete harness/model/effort
combination at configuration and creation boundaries. Keep the existing
`harness.Selection` for the model/effort accepted within a running harness.
Neither type carries a profile name or a description.

Discovery failures use a nonzero exit and useful stderr; valid results are
JSON on stdout. Treat external JSON as input to validate: nonempty IDs,
duplicate IDs, a requested model that is absent, and invalid defaults must not
become apparently usable choices. A missing native executable is an ordinary
`describe` result, not an app-start failure. Report a broken plugin separately
from an absent native executable.

### Core ownership and lifetime

- Enumerate executable `hypercube-harness-*` siblings in `PluginDir`, following
  the existing development override and packaged layout. Use the filename
  suffix as the harness ID. Resolve external harness requests through the
  discovered set; never interpolate unchecked input into a filesystem path.
- Scan descriptors during app startup in `cmd/hypercube/main.go`. Give each
  command a deadline and collect per-plugin failures so one broken plugin
  does not prevent other harnesses or saved sessions from working. Startup
  never waits for model-provider calls or native conversation initialization.
- Keep the installed-plugin snapshot with the runtime. A request to refresh
  harnesses can repeat the cheap descriptor scan, allowing installations made
  after startup to appear without rewriting configuration.
- Model-discovery entry points take context first; HTTP/MCP callers pass
  their request context. The caller owns the probe process, drains its output,
  and waits for its exit on success, cancellation, timeout, or error. Creation
  operations keep their existing signatures and durable launch ownership;
  they do not start separate discovery probes.
- Reuse `plugins/internal/plugin` process and native transport helpers where
  they fit. Extract connection/handshake code from conversation creation only
  where both discovery and `serve` need it. Do not build discovery by invoking
  normal Hypercube session startup, creating a holder, or supplying a prompt.
- Never perform discovery while holding `Runtime.mu` or a database
  transaction. Startup can supply installed-harness metadata from its existing
  snapshot without a subprocess inside `startArgs`. Keep child resolution pure
  and inside the existing lifecycle admission transaction.
- Each plugin validates requested settings during its normal startup handshake
  in the actual session directory, using the process it is already starting.
  Configure validates through that running process. Do not start a second
  native process to preflight a creation or a model change. Retain the existing
  cleanup of failed child launches and worktrees, including restoration of
  proposal/offer approvals. Inheritance can be unavailable in another directory.
- Catalog data belongs to the open picker's component state. Fetch it on open
  or when its directory/harness/model changes; cancel superseded requests.
  Reuse the same picker component for each consumer, not a global catalog
  cache. No persistent catalog, polling interval, background native process,
  or app-wide model cache is needed. Cancelling a query signals only the plugin
  first so its discovery context can unwind and delete temporary native state.
  Allow bounded cleanup before killing the remaining process group, and return
  a temporary-session deletion failure rather than claiming a successful catalog.
- A catalog is a statement of advertised availability, not proof of entitlement,
  quota, or provider health. Native startup/configuration is authoritative.
  Preserve explicit failures and do not fall back to another model or harness.

### Native implementations

| Plugin | Discovery implementation |
| --- | --- |
| Codex | Start app-server in the requested directory, perform the existing initialize/initialized exchange, and page through `model/list`. Read native display names, defaults, and supported reasoning efforts. Include catalog entries that are hidden only from the default native picker when the API permits and they are valid selections. Do not start, resume, or fork a thread just to list models. |
| Claude Code | Start the stream-JSON control process without a user prompt or Hypercube MCP server, read the models from the initialize response, and map `supportsEffort`, `supportedEffortLevels`, and resolved-model metadata. Use the process's native default when it is reported. Do not claim a default effort from a guessed family table; an omitted effort can remain a native default until startup reports the accepted value. |
| OpenCode | Reuse ACP initialization and the model/effort config-option decoding in `configure.go`. Open a discovery-only native session in the requested directory with no MCP servers, inspect model choices, and select only the requested model to obtain its effort options. Delete the temporary native session and close the process, including failure paths. This session must never receive a prompt. Keep native model metadata parsing in the plugin. |
| Pi | Start `pi --mode rpc --no-session` in the requested directory without the Hypercube extension. Use `get_available_models`, `get_state`, and `get_available_thinking_levels`. For a requested model, use explicit launch arguments or a verified nonpersisting model change; do not change global defaults to inspect it. Preserve full provider/model identity and let Pi supply supported thinking levels. |

Before extracting code, verify the installed native protocol's discovery and
cleanup behavior against the primary references below. Normal native logging
or caches are not Hypercube configuration. Hypercube must not write native
settings, credentials, trust decisions, `HOME`, or another conversation to make
discovery work. Do not treat human-formatted CLI tables as the model API.
If a required discovery method is unavailable, report an unsupported-version
error with an update instruction; do not fall back to baked-in model lists.

## Accepted settings and session lifecycle

`Selection` already travels in configure replies, and `commands.go` records
partial success even when a request fails. Extend that existing path to start
replies so the saved session also records the native startup's accepted model
and effort. Add an accepted selection to `plugin.Agent`, emit it in the start
reply, and apply it before the runtime declares the session ready.
Where the native API exposes only acceptance of an omitted effort, preserve the
explicit “Harness default” representation rather than claiming a measured
effective level. A start reply from an older adopted plugin can omit selection;
leave its saved settings intact. The launch counts as started only after the
transaction that saves the start reply (Ready and accepted settings) commits,
and `CreateChild` returns the child as saved, so its result reports the
accepted settings. Keep existing durable start/configure command
shapes and do not rewrite their queued JSON lines during the migration.

Use the accepted selection already available from native initialization
where it is reported (notably OpenCode's config result and Pi's `get_state`).
Where the API only acknowledges an explicit selection, keep that validated
selection; do not build a second alias/substitution resolver or pretend to know
an unreported native default. Preserve Pi's existing rejection of a substituted
model, and show native alias resolution in catalog labels when supplied.
Invalid explicit effort is rejected rather than deliberately relying on Pi's
clamping. If a native configure operation partly succeeds or reports changed
capabilities, continue storing its reported actual settings and show the error;
do not show requested settings as if they were accepted.

Claude and Codex currently pass effort unconditionally. At both start and
configure, distinguish a native default from an empty effort string. At start,
omit Claude's `--effort` and Codex's `model_reasoning_effort` override when empty.
At configure, use the native clear/default operation if omission would retain
the old explicit effort (for example a nullable flag reset); verify the actual
protocol semantics instead of assuming absence clears it. OpenCode must
continue accepting a saved `default` effort for
models without an effort control; keep that legacy interpretation inside its
plugin rather than adding harness-specific migration logic to the core.

Change top-level `Create` to take `harness.Settings`, and replace
`SelectProfile` with `Select` taking `harness.Selection`. Keep the prohibition
on changing a saved session's harness. User-created sessions, direct children,
accepted offers, and started proposals all use the same plugin-owned startup
validation; configure validates through its running native process. The core
validates request structure and lifecycle invariants. The MCP layer must not be
the only place that rejects malformed overrides.

Do not gate adoption of already-running holders or reading existing sessions
on successful discovery. Existing processes survive backend installation as
they do now. Restarting a saved session uses its saved selection and reports a
native failure if it no longer works; it does not select a new default.

## Saved-state migration

### Configuration

Add an explicit startup upgrade in `internal/harness/config.go`, called before
normal configuration reads can race with it. Use the existing temporary-file
and rename pattern; create the config directory when needed. After startup,
`LoadConfig` remains a reader and `SaveConfig` remains the normal writer.

The new configuration keeps appearance, terminal, location, and notification
preferences, and replaces `profiles` / `default_profile` with:

- `default_selection`: an optional concrete `harness.Settings` value.
- `child_model_guidance`: an optional string, empty on a new installation.

For legacy files, resolve the default exactly as the current loader does,
including the first-profile fallback. Copy that profile's settings into
`default_selection`. Seed guidance with the existing descriptions, prefixed
by their explicit harness/model/effort so their meaning survives removal of
profile names. Preserve profile order and the original wording; do not ask
another model to summarize or reinterpret the user's preferences. If a file
already contains an explicit new guidance field, preserve it, including an
intentionally empty value. A second startup must not duplicate the guidance.

Persist the converted document without the legacy fields. Keep the legacy
profile struct private to the upgrade decoder; delete `Config.Profile`, the
public `Profile` type, and profile-required validation. A missing configuration
file is now a fresh installation with ordinary preference defaults, not an
error requiring the user to construct a model list. Malformed files and failed
upgrade writes are explicit errors; leave the original file intact.

### Database and proposals

Session columns already store harness/model/effort and need no replacement.
Add a versioned migration in `internal/store/migrate.go` for transcript JSON:
replace the embedded `input.profile` in proposal records and saved host offer
records with concrete `input.settings`, dropping only profile name/description.
Preserve item IDs, sequence, completion, answer, brief, folder, and context.
Handle both pending and completed cards so historical rendering uses one shape.
Do not rewrite arbitrary recorded MCP tool-call inputs; historical calls using
`profile` remain historical evidence, not executable legacy requests.

Use the existing `PRAGMA user_version` transaction and add an upgrade test
covering a pending proposal that can still be started. Add no import from store
to session and no profile lookup during proposal startup.

### Browser drafts

Change `Unsent` from a profile name to a complete optional settings value.
Upgrade `hypercube.drafts` in `agent/draft-storage.ts` when reading it, then save
the new shape through the existing persistence path. Always preserve typed
text, workspace, and worktree choice.

An old draft only stored a profile name, not the selected settings. After the
configuration has been converted, that reference is not always recoverable.
Keep the text and location and drop the obsolete profile name. Initialize
selection through the same new-draft defaults above, with no special unresolved
state flag; if no default exists the user must choose before sending. Do not
discard the entire draft, pretend its old selection was preserved, or introduce
a permanent profile-alias store just for this one-time upgrade. New-format
drafts preserve their complete settings across reloads, including unavailable
choices.

## Implementation sequence

### 1. Add the discovery contract and native implementations

- Add `internal/harness/discovery.go` and focused boundary tests for executable
  enumeration, descriptor results, JSON validation, and cancellation.
- Add `plugins/{claude,codex,opencode,pi}/discovery.go` and protocol stand-in
  tests, and dispatch `describe` / `models` from each plugin's `main.go`.
- Reuse `opencode/configure.go`'s grouped-option parsing. Extract native
  connection concerns from the plugin main files only when discovery and
  normal sessions both use them; keep conversation creation and events separate.
- Extend `internal/harness/harnesstest` with deterministic discovery results
  and a blocking probe for cancellation tests. Tests use native stand-ins,
  never the user's installed account or workspaces.

### 2. Upgrade preferences and embedded saved selections

- Implement the config conversion and new preferences in
  `internal/harness/config.go`, `config_test.go`, and `harness_test.go`.
- Add the transcript JSON migration and upgrade assertions in
  `internal/store/migrate.go` and its migration tests.
- Call the config upgrade and descriptor scan from `cmd/hypercube/main.go`.
  Keep restore/adoption independent of provider discovery failures.

### 3. Replace profile-based runtime entry points

- Add `internal/session/selection.go` for resolution and selection operations;
  remove `profile.go`, and replace `profile_test.go` with behavior tests for
  direct settings and inheritance.
- Update `sessions.go`, `child_options.go`, `children.go`, `proposals.go`, and
  `offers.go` to use resolved settings. Keep `prepareChild` pure with respect
  to external discovery and preserve its existing directory/fork checks.
- Keep discovery contexts confined to descriptor/model queries. Session
  creation and configuration use their existing launch/command ownership;
  do not thread new preflight contexts through every child-start operation.
- Update `launch.go`, `commands.go`, `plugins/internal/plugin/serve.go`, and
  plugin startup paths to record accepted start selections through the same
  durable reply mechanism as configure. Keep command replay and output
  checkpointing intact.
- Update fixtures and existing lifecycle/creation/configure tests to pass
  settings directly. Remove assertions that only protect profile machinery.

### 4. Expose selection and discovery to agents

- Change `create_session` in `internal/mcp/children.go` to optional `harness`,
  `model`, and `effort` overrides. Remove the profile property and enum, config
  lookups, and profile-specific tool text. Keep initiation/offer/name/brief
  validation and direct-vs-worktree semantics.
- Add `list_models` in `internal/mcp/models.go`. Without a harness, list the
  discovered harnesses and their installation status. With a harness, list
  models in the caller's directory; optional `model` returns effort choices,
  and optional `query` narrows returned IDs/labels. Use the caller's directory;
  add no arbitrary-directory argument. A child started in another directory
  validates there through its own native process. This tool starts no session and mutates no
  selection. It uses the same discovery functions as the browser and runtime.
- Invalid selection errors identify the rejected field and point to
  `list_models`; do not dump hundreds of unrelated models into an error.
- Update `internal/mcp/mcp.go` to register discovery and return model/effort in
  session listings. The existing parent-only creation permission stays intact.
- Update `internal/report/report.go` and `internal/session/launch.go` to inject
  the inheritance rule, names of installed harnesses, and optional guidance.
  Delete profile-selection wording and “choose the cheapest profile” advice.
  Do not inject discovered model catalogs into instructions or tool enums.

### 5. Replace browser controls and draft state

- Add `internal/ui/models.go` for `GET /harnesses` and `GET /models`, register
  them in `ui.go`, and share runtime validation rather than duplicating it.
  Resolve model requests from a workspace/session identity; do not trust a
  browser-supplied path in place of that identity.
- In `actions.go`, replace `action=profile` with `action=selection`. New-session
  requests carry the concrete settings; preserve prompt/image body handling.
  Model/effort changes keep using the existing configure command.
- Update `web/api.ts` and the agent/session/app types. Keep the draft's
  concrete selection in the store; native catalog data and request handles
  belong to the picker and its effects. Extract pure selection transitions
  only where their complexity warrants a separate module.
- Replace `profile-select.tsx` with `model-select.tsx`, and rename
  `profile-name.tsx` / `profile-label.ts` around selection display. Use stock
  shadcn controls, searchable model choices, accessible labels, and native ID
  details sufficient to distinguish providers. Remove profile-only fallback
  strings and assumptions that model names can be derived perfectly from IDs.
- Reuse the selection control for the configured new-session default and
  running sessions. A running session's harness is fixed; its current settings
  remain visible while controls are disabled. Changing the selected model
  loads its effort choices before committing a valid complete selection.
- Replace draft/profile reconciliation in `app/config.ts` and
  `sessions/selection.ts`; update `agent/drafts.ts`, `draft-storage.ts`, and
  `sessions/session-actions.ts`. Do not let late catalog responses replace
  another draft's selection, another workspace's data, or newer user input.
  Abort superseded requests and ignore stale completions by request identity.
- Delete `app/settings-profiles.tsx`, remove its `ProfileDraft` / default-index
  machinery, and update `settings-dialog.tsx`, `settings-preferences.tsx`, and
  `app/types.ts` for the direct default and guidance field.
- Update `transcript/types.ts` and `proposal-item.tsx` for saved concrete
  settings. Add the derived difference indicator in selection display and
  `sidebar/session-item.tsx` without new persistent state or custom CSS.

### 6. Update development tools, fixtures, and documentation

- Replace the seeded profile document in `scripts/dev` with the new preference
  shape. Keep its isolated environment and native harness launch behavior.
- Change `scripts/screenshots/main.js`, `plugins/scripted/main.go`, and the
  scripted plugin tests to provide concrete scene settings and discovery
  responses. Use the invoked plugin filename to emulate the linked harness
  IDs. Scene model data belongs to the scene fixtures, not the production core.
- Update the existing JSON scenes in `docs/screenshots/`, including embedded
  proposal settings and example tool calls. Do not generate or hand-edit image
  outputs as part of implementation validation.
- Update the behavior descriptions in `docs/configuration.md`,
  `docs/installation.md`, `docs/usage.md`, `docs/harness-support.md`, and the
  affected sentence in `docs/introduction.md`. Update `agents/architecture.md`
  with the discovery commands and selection ownership. Keep the existing
  documentation structure; create no additional guides.
- Do not convert the real Hypercube config while the installed old app is
  running: its `LoadConfig` rejects a document without profiles. Generate the
  `docs/configuration.md` example by applying the new conversion to a temporary
  copy of the user's current config (the source is read-only), preserving their
  descriptions and other preferences. When installation is later authorized,
  save the old config for rollback and let the new backend convert the real
  file on first startup, after the old backend has stopped. Then check that
  it matches the documentation example. This is the deployment sequence for
  AGENTS.md's requirement that the example and real file stay the same; do not
  load the new format into the old app or replace it with a generic example.
- Search live source, fixtures, and user docs for profile-specific references.
  Only the private upgrade decoder, migration tests, and historical developer
  records should still describe agent profiles. Browser/desktop application
  profiles are unrelated and should retain their names.

## Test plan

Prefer boundary behavior over repeated tests of every mapping. Extend the
existing fixture infrastructure rather than adding a second fake application.

### Discovery and native behavior

- Discover two plugin siblings with different native installation statuses;
  omit nonexecutables. One failing descriptor must not hide a usable plugin.
- Verify each plugin's discovery transcript using its native stand-in:
  pagination for Codex; model/effort metadata for Claude; grouped model choices
  and model-dependent efforts for OpenCode; full provider/model identity and
  nonreasoning models for Pi.
- Verify discovery sends no prompt, creates no Hypercube session or holder,
  leaves native configuration files unchanged, and cleans up temporary OpenCode
  sessions. Cover cancellation of a blocked probe through process exit, not a
  fixed sleep. Exercise default/absent effort without a universal effort table.
- Verify malformed discovery data, an unavailable provider, and an unsupported
  native method yield actionable errors and do not fall back to a model list.

### Runtime and MCP behavior

- A new session starts on a direct discovered selection without any profiles.
  Invalid selections fail in the native handshake before their first prompt;
  any created child worktree
  or proposal approval is cleaned up/restored through the existing lifecycle.
- A child inherits the parent's latest accepted configure result, not its
  original settings or global default. Exercise the override table, including
  model changes that require a new default effort and explicit child folders.
- A saved proposal retains the selection shown when proposed after the parent
  changes models. Availability is checked again on Start; a failed Start does
  not consume the proposal. Preserve concurrent-click protection.
- Offers remain title-only until accepted, and the accepted offer uses the same
  settings resolution as direct children. Forks reject a different harness.
- Startup reports and persists the accepted native selection. Configure partial
  failure still stores the actual accepted settings. Pi's existing model
  substitution rejection still happens before the first prompt. Empty efforts
  omit native overrides, and legacy OpenCode `default` remains resumable.
- Restoration of existing holders works with a failed discovery provider.
  Existing session settings survive config changes and backend restarts.
- `list_models` exposes current directory-specific choices without creating a
  session. Tool schemas have no profile enums, and instructions contain guidance
  and the inheritance rule without a full model catalog.

### Migration and browser behavior

- Upgrade an old config with an explicit default and one with an implicit
  first-profile default. Preserve descriptions and ordinary preferences; verify
  the new file, idempotence, explicit empty guidance, and failure-safe writes.
- Start from a missing config with zero harnesses: Settings and saved data are
  usable, and session creation explains what is missing.
- Upgrade pending/completed proposal and host-offer JSON without altering
  unrelated transcript entries. Start the migrated pending proposal.
- Restore legacy browser drafts without losing typed text or location. Apply
  ordinary new-draft defaults after removing a legacy profile name. Restore new-format
  selections unchanged, including choices no longer available.
- Browser interaction tests exercise harness → searchable model → effort,
  models with no effort setting, provider disambiguation, running-session model
  changes, keyboard operation/focus, loading and error states, guidance saving,
  and the proposal/current-child selection display.
- Exercise a delayed response after changing workspace or model: the new
  selection and draft must survive. A rejected create restores the prompt.
- Check the effective selection and difference indicator by accessible content,
  not styling or private React state. Keep the current offer buttons and flow.

Run focused Go and browser tests while implementing, then `make check`, which
includes the browser build, TypeScript checks, and Go race tests. Use the
isolated development server for one browser smoke test of discovery, direct
selection, guidance, and proposal display. Do not run native paid model prompts
or the documentation screenshot generator just to validate selectors.

## Review and delivery

Before implementation, obtain the independent architectural review required
by the Hypercube workflow, concentrating on the discovery/native-conversation
boundary, saved-state upgrades, and whether any catalog state can be deleted.
Use a temporary task folder for review artifacts; this plan stays at the
user-requested repository path. Verify findings before revising the plan.

After implementation, obtain one independent code review, resolve verified
findings, and complete the checks above. Do not commit, merge, or install
without the user's instruction. When offering installation, repeat that active
agents need restarting to receive the new plugin behavior, MCP schema, and
instructions. A backend restart alone adopts their existing processes.

## Primary references

These establish available mechanisms, not a universal quality or cost ordering.
Recheck native version details during implementation.

- [Codex model discovery](https://learn.chatgpt.com/docs/app-server#models) and
  [its inheritance instructions](https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/handlers/multi_agents_spec.rs).
- [Claude initialization and model metadata](https://code.claude.com/docs/en/agent-sdk/typescript#modelinfo)
  and [child defaults versus restrictions](https://code.claude.com/docs/en/sub-agents#choose-a-model).
- [OpenCode ACP model and effort choices](https://opencode.ai/docs/cli/acp/#config-options)
  and [project-specific availability](https://opencode.ai/docs/models/).
- [Pi RPC model and thinking queries](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/rpc-commands.md#model).
- [Third-party pi-subagents model selection and optional enforcement](https://github.com/nicobailon/pi-subagents/blob/main/docs/models.md).
