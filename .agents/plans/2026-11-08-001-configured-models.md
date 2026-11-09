# Configured models

## Goal

The model picker below the message box, and the default-model picker in
Settings, list models by asking each harness's plugin (`models` command) for a
catalog in the workspace's directory, saving it in SQLite, and refreshing it
whenever the picker opens. Labels elsewhere read the same saved catalogs.

This task replaces that with a model list the user writes by hand. Each
**model entry** is saved globally in the database and names a harness, an exact
native model ID, its effort levels from lowest to highest, and a default
effort. Settings › Agents adds, edits, and deletes entries. The picker reads
only these entries: it starts no plugin, asks for nothing, and has nothing to
refresh. The plugin still checks the model and effort when it starts or
configures a session, as it does now.

The agent tool `list_models` keeps asking plugins directly. It stops saving
what it finds, because nothing reads the saved catalogs any more.

When the work is done:

- Settings › Agents has a **Models** list with **Add Model**, and **Edit** and
  **Delete** on each entry. Changes apply immediately, like Permissions.
- The picker shows harness icon tabs at the top (one per harness with
  entries), a search field beneath them that has focus when the picker opens,
  the selected tab's models beneath that, and the effort control at the
  bottom.
- With no entries, or none for a running session's harness, the picker says so
  and links to Settings › Agents.
- A model shows as its native ID everywhere. Hypercube shows no discovered
  labels.

There are no prerequisites.

## Related code

Backend:

- `internal/store/store.go` — `schema` creates the `catalogs` table, keyed by
  harness, directory, and model.
- `internal/store/catalogs.go` — `Tx.Catalog` and `Tx.SaveCatalog`.
  `catalogs_test.go` tests them. Both files are deleted.
- `internal/store/migrate.go` — `DB.migrate`, with `PRAGMA user_version`
  steps up to 3. Gains step 4.
- `internal/store/grants.go` — the model for a small saved list
  (`Grant`, `Tx.Grants`, `Tx.DeleteGrant`) that the new file follows.
- `internal/session/selection.go` — `Runtime.Models` asks the plugin and saves
  the catalog; `Runtime.CachedModels` reads the saved catalog and asks the
  plugin when none is saved. `ScanHarnesses`, `Select`, and `childSettings`
  are unchanged.
- `internal/mcp/models.go` — `listModels` calls `rt.ScanHarnesses` and
  `rt.Models`. Unchanged.
- `internal/ui/models.go` — `server.harnesses` (`GET /harnesses`, kept) and
  `server.models` (`GET /models`, which reads or refreshes a catalog; replaced),
  plus `writeJSON`.
- `internal/ui/ui.go:28-29` — the routes.
- `internal/ui/actions.go:142-163` — `server.grants` and `server.deleteGrant`,
  the pattern for the new handlers.
- `internal/ui/models_test.go` — `TestHarnessesAndModels` and
  `TestModelsCacheAndRefresh`.
- `internal/harness/harnesstest/harnesstest.go:38,164-174` —
  `HYPERCUBE_TEST_MODELS_LOG`, used only by `TestModelsCacheAndRefresh`.
- `internal/config/config.go` — `Config.DefaultSelection` stays in
  `config.json` as `harness.Settings`. Unchanged.

Browser interface (`internal/ui/web/`):

- `agent/catalogs.ts` — the catalog cache in `State.catalogs`, `loadCatalog`,
  and `useModels`, which label displays call. Deleted with
  `agent/catalogs.test.tsx`.
- `agent/model-select.tsx` — `ModelSelect`: fetches `/harnesses` when opened,
  loads and refreshes catalogs, picks a harness from a `Select`, a model from a
  `Command` list, and an effort from a `Select`. Rewritten.
- `agent/model-select.test.tsx` — tests built on stand-in catalogs. Rewritten.
- `agent/session-selection.tsx` — `SessionSelection` passes `workspace` to
  `ModelSelect`; `SettingsDiffer` calls `useModels` for hover text.
- `agent/selection-label.ts` — `displayName`, `modelLabel`, `settingsText`,
  `sameSettings`.
- `agent/selection-name.tsx` — `SelectionName` takes `models` for labels.
- `agent/types.ts` — `Settings`, `Harness`, `Model`, `Catalog`.
- `transcript/proposal-item.tsx:32` — calls `useModels` for proposal labels.
- `app/settings-dialog.tsx` — `SettingsDialog`, `SettingsContent`, with the
  section in local state.
- `app/settings-preferences.tsx` — `DefaultSelection` (picks a workspace for
  the catalog) and `SettingsAgents` (child guidance).
- `app/settings-permissions.tsx` — the immediate-apply list the Models list
  follows.
- `app/app.tsx:51,64,108,121` — `settingsOpen` local state, opened by the
  shortcut and the sidebar.
- `app/types.ts` — `Config`, `Grant`.
- `api.ts:84-89` — `fetchHarnesses`, `fetchModels`.
- `state.ts` — `State.catalogs`, `initialState`.
- `main.tsx:31` — fetches the config at startup.
- `components/ui/` — stock shadcn components; `tabs.tsx` exists, a slider does
  not.
- Tests that stub `models?` requests: `app/settings-dialog.test.tsx:120-170`,
  `transcript/transcript.test.tsx:67-77`,
  `transcript/submissions.test.tsx:26,122`.

## Decisions

### Saved data

Model entries live in a new table, created by `schema`:

```sql
CREATE TABLE IF NOT EXISTS model_entries (
  id TEXT PRIMARY KEY, harness TEXT NOT NULL, model TEXT NOT NULL,
  efforts TEXT NOT NULL, default_effort TEXT NOT NULL,
  UNIQUE (harness, model));
```

`efforts` holds a JSON array of strings in the user's order, lowest first.

New file `internal/store/model_entries.go`:

```go
// ModelEntry is a model the user listed for a harness: its native ID, its
// effort levels from lowest to highest, and the effort a session starts with.
// A model without levels leaves the effort to the harness.
type ModelEntry struct {
	ID            string   `json:"id"`
	Harness       string   `json:"harness"`
	Model         string   `json:"model"`
	Efforts       []string `json:"efforts"`
	DefaultEffort string   `json:"default_effort"`
}

// Check trims the entry's fields and validates them. One level is its own
// default.
func (e *ModelEntry) Check() error

// ModelEntries lists the entries by harness, then model.
func (t *Tx) ModelEntries() ([]ModelEntry, error)

// SaveModelEntry adds an entry without an ID, giving it one, or replaces the
// entry with its ID.
func (t *Tx) SaveModelEntry(e ModelEntry) error

// DeleteModelEntry removes an entry.
func (t *Tx) DeleteModelEntry(id string) error
```

`Check` rules, after trimming spaces from every string:

| Field | Rule |
| --- | --- |
| `Harness` | Not empty. Need not be installed. |
| `Model` | Not empty. Not checked with the harness. |
| `Efforts` | Each level is non-empty; no level repeats. May be empty. |
| `DefaultEffort` | No levels: empty. One level: set to that level. Several: one of them. |

`SaveModelEntry` returns `ErrNotFound` when an ID names no entry, and an error
such as `claude already lists model "opus"` when another entry has the same
harness and model. A nil `Efforts` is saved as `[]`, and `ModelEntries` returns
`[]` for none, so the JSON is always an array.

The entry ID is `NewID()`. It exists only so Settings can edit and delete an
entry; sessions never store it.

### Transition from saved catalogs

Saved catalogs are discovered data, not something the user wrote, so they are
not converted. Migration step 4 drops them:

```go
if version < 4 {
	// Model choices are now model entries the user writes; discovered
	// catalogs are no longer saved.
	if err := tx.exec(`DROP TABLE IF EXISTS catalogs; PRAGMA user_version = 4;`); err != nil {
		return err
	}
}
```

After upgrading, the model list is empty. Existing sessions, saved proposals,
new-session drafts, and `default_selection` keep their harness, model, and
effort and keep showing them. The picker's empty state links to Settings, so
the first open after the upgrade says what to do.

### Backend routes

`internal/ui/models.go` keeps `GET /harnesses` and replaces `GET /models`:

| Route | Body | Answer |
| --- | --- | --- |
| `GET /models` | — | `[]ModelEntry` |
| `POST /models` | one `ModelEntry` as JSON; empty `id` adds | the full `[]ModelEntry` after saving |
| `DELETE /models/{id}` | — | the full `[]ModelEntry` after deleting |

`POST` decodes with `DisallowUnknownFields`, as `saveConfig` does, and answers
400 for malformed JSON or a failed `Check`, 404 for `ErrNotFound`, and 409 for
a duplicate. Returning the list lets the page replace its copy without a
second request. The handlers read and write in one `s.db.Tx` each.

`GET /harnesses` is now used only by the entry form in Settings, to offer
harness choices. It runs `describe`, which looks for an executable and starts
no native agent.

### Runtime and agent discovery

`Runtime.Models` becomes a plain call to `harnessclient.Models` and saves
nothing. `Runtime.CachedModels` is deleted. `list_models`, `create_session`
overrides, `childSettings`, and the plugins' `models` commands are unchanged:
agents still discover models on their own, and their choices are not limited to
model entries. The child-model guidance text in Settings already says it does
not limit agents' choices; that stays true.

### Validation at start and configure

Nothing new. The plugin already rejects an unknown model or effort when it
starts a session or applies a model change, and the error appears in the chat
(`docs/harness-support.md`, Requirements). An entry for an uninstalled harness
or a model the harness no longer offers stays saved and fails the same way when
used.

### Page state

`State` loses `catalogs` and gains:

```ts
// modelEntries is the saved model list, null until it loads.
modelEntries: ModelEntry[] | null;
// settings is the open Settings section, null while Settings is closed.
settings: SettingsSection | null;
```

`ModelEntry` mirrors the Go type in `agent/types.ts`; `Model` and `Catalog`
are deleted there, and `Harness` stays for the entry form. `SettingsSection`
(`'General' | 'Agents' | 'Notifications' | 'Permissions'`) moves to
`app/types.ts` with the `sections` array, so the picker can open Settings at a
section.

`main.tsx` fetches `/models` at startup beside the config and stores the list.
Each save or delete in Settings stores the list the server returns. Other open
windows see changes after a reload, as they do for the config today.

`app.tsx` drops `settingsOpen`; the shortcut and sidebar set
`settings: 'General'`, and `SettingsDialog` is open while `state.settings` is
not null. `SettingsContent` reads its section from the store and its nav sets
it, so the picker's link works both from the composer and from the General
section's default-model picker inside Settings.

### Labels

There are no labels. The user types the model ID and sees it back.
`modelLabel` is deleted; `settingsText(settings)` and
`SelectionName({settings})` lose their `models` argument and show
`settings.model`. `SettingsDiffer` and `ProposalItem` stop calling
`useModels`. A session whose model has no entry still shows its harness icon,
model ID, and effort.

### The picker

`ModelSelect` keeps its trigger (harness icon, model ID, dimmed effort,
chevron; `aria-label` "`{label}`: …") and its props, minus `workspace`:

```ts
interface ModelSelectProps {
  value: Settings | null;
  onChange: (settings: Settings) => void;
  fixedHarness?: boolean;
  disabled?: boolean;
  label?: string;
  className?: string;
}
```

It reads `state.modelEntries` and makes no requests. The popover contents,
top to bottom:

1. **Harness tabs.** shadcn `Tabs` with a `TabsList` of `TabsTrigger`s, one per
   harness that has entries, in entry order (harness ID). Each trigger shows
   `harnessIcon(id) ?? displayName(id)` with `aria-label={displayName(id)}`
   and the same text as its `title`. The initial tab is `value.harness`
   when it has entries, otherwise the first. With `fixedHarness` (a running
   session) the tabs are omitted and the list is the session's harness.
2. **Search.** `CommandInput` with placeholder "Search models". It has focus
   when the popover opens: `PopoverContent`'s `onOpenAutoFocus` prevents the
   default and focuses the input. Search matches the model ID within the
   selected tab.
3. **Models.** `CommandList` of the tab's entries, each showing the model ID
   and a check on the current model (`aria-selected`). `CommandEmpty` reads
   "No models match."
4. **Effort.** Shown only when the current value's model has an entry under the
   selected tab:
   - No levels: nothing.
   - One level: a row with the label "Effort" and the level name.
   - Several levels: the row with "Effort" and the selected level's name above
     a shadcn `Slider` with `min=0`, `max=levels-1`, `step=1`, so it snaps to
     levels. The thumb's `aria-valuetext` is the level name. While dragging,
     the name follows the thumb (component state); `onValueCommit` calls
     `onChange` with the new effort, so a running session receives one
     configure command per change, not one per step.
   - A current effort that is not one of the entry's levels (blank, or the
     entry was edited): the row shows that effort, or "Default" when blank,
     and the thumb rests on the entry's default until the user moves it.

Choosing a model calls `onChange({harness: tab, model, effort: entry.default_effort})`
at once (for no levels, `default_effort` is empty). Choosing the current model
again changes nothing. The popover stays open so the effort can be adjusted.

Empty states replace items 2–4:

- `modelEntries` is empty: "No models configured." with a link-style
  **Open Settings** button.
- `fixedHarness` and no entries for the session's harness:
  "No `{displayName}` models configured." with the same button.

The button closes the popover and sets `settings: 'Agents'`.

The picker never edits sessions or entries by itself: a removed entry leaves
the trigger showing the saved selection, and the list simply lacks it.

### Settings › Agents

New file `app/settings-models.tsx`, rendered by `SettingsAgents` above the
child guidance as a **Models** group. Its edits apply immediately and do not
use the dialog's draft or **Save changes**, like Permissions; the group's
description says so in one sentence.

- **List**: the bordered, divided list Permissions uses. Each row shows the
  harness icon (or name) and the model ID, and beneath it the levels joined by
  " · ", with the default marked "(default)" when there are several. Rows have
  outline **Edit** and **Delete** buttons with `aria-label`s naming the
  harness and model. With no entries, "No models." The group header has an
  **Add Model** button.
- **Entry dialog**: a nested shadcn `Dialog` titled "Add Model" or "Edit
  Model", with:
  - **Harness**: `Select` of the plugins from `GET /harnesses`, fetched when the
    dialog opens, showing `harnessIcon` and `name || displayName(id)`. When
    editing an entry whose harness is not listed, its ID is added as an option
    so the entry can be saved unchanged.
  - **Model ID**: `Input`, required.
  - **Effort levels**: `Input` for levels separated by commas, lowest first
    (placeholder `low, medium, high`). The form splits on commas and trims.
  - **Default effort**: `Select` of the levels, shown only with two or more.
    Adding defaults it to the last level typed; changing the levels keeps it
    when still listed, otherwise resets it to the last level.
  - Footer **Cancel** and **Save**. Save posts the entry, stores the returned
    list, and closes; a refused save shows the server's reason in the dialog
    and keeps the form.
- **Delete** sends `DELETE /models/{id}` and stores the returned list. No
  confirmation, matching Permissions' **Remove**. A failure shows the reason
  above the list.

`api.ts` replaces `fetchModels` with `fetchModelEntries(store)` (through
`request`, like `fetchConfig`), and adds `saveModelEntry(entry)` and
`deleteModelEntry(id)`, which throw `HttpError` like `saveConfig` and resolve
to the returned list.

### Settings › General default model

`DefaultSelection` drops the workspace lookup and its "Add a workspace" text.
It renders `ModelSelect` with `label="Default model"`; the explanation text
loses its workspace condition. Its empty state links to the Agents section
inside the same dialog.

### Slider component

Add `components/ui/slider.tsx` from the shadcn registry (the `radix-ui`
variant matching the other components). The only change is an optional
`thumbProps` spread onto each `SliderPrimitive.Thumb`, so the picker can set
`aria-valuetext`.

## Test plan

Go (`go test ./...` passes under `-race`):

- `internal/store/model_entries_test.go` (new), table-driven `Check` cases:
  empty harness or model, blank or repeated level, missing default with several
  levels, default not listed, one level setting its own default, no levels with
  a default rejected, surrounding spaces trimmed. Store round trip: add gives an
  ID; list order is harness then model; levels keep their order; replace by ID;
  replacing an unknown ID gives `ErrNotFound`; a duplicate harness and model is
  refused, including on edit; delete removes; an empty list encodes as `[]`.
- `internal/store/migrate_test.go`: a database at `user_version = 3` with a
  `catalogs` row opens with no `catalogs` table, an empty `model_entries`
  table, and `user_version = 4`; sessions are unchanged.
- `internal/ui/models_test.go`: keep the `/harnesses` assertion. Replace the
  rest with one test through the HTTP server: `GET /models` is `[]`; `POST`
  adds and returns the list; `POST` with the ID edits; malformed JSON, unknown
  fields, and failed checks are 400; an unknown ID is 404; a duplicate is 409;
  `DELETE` returns the shortened list. Saving an entry for a harness with no
  plugin succeeds. Delete `TestModelsCacheAndRefresh`.
- `internal/harness/harnesstest`: delete the `HYPERCUBE_TEST_MODELS_LOG`
  branch and its doc line. Keep `exitLogFailed`, which other logs use.
- `internal/mcp` `list_models` tests stay as they are and must pass.

Browser (`bun test` in `internal/ui`), rewriting `agent/model-select.test.tsx`
around `testState({modelEntries: [...]})` with entries for two harnesses: one
model with levels `low, medium, high` and default `medium`, one with one level,
one with none. Each picker test also asserts that fetch was never called.

- Opening shows one tab per harness with entries, the value's harness selected,
  and focus in the search field.
- Searching narrows the selected tab's models.
- Choosing a model sets the draft to its default effort without losing the
  draft's text; switching models resets the effort to the new model's default.
- Several levels: the slider shows the selected level's name, a keyboard step
  commits the next level, and a running session posts one `selection` action
  per commit.
- One level shows the level with no slider; no levels show no effort control
  and set an empty effort.
- A running session shows no tabs and only its harness's models.
- A selection whose model has no entry still shows in the trigger.
- With no entries, the picker shows the empty state; **Open Settings** sets
  `state.settings` to `'Agents'`. The same for a running session whose
  harness has none.

Settings (`app/settings-dialog.test.tsx` and a new
`app/settings-models.test.tsx`):

- Adding an entry posts harness, model, levels in order, and default; the list
  and `state.modelEntries` show it; the dialog's **Save changes** is not
  needed.
- The default-effort control appears only with two or more levels.
- A refused save shows the server's reason in the entry dialog.
- Editing an entry whose harness is not installed keeps its harness.
- Deleting removes the row and leaves the sessions in state unchanged.
- Rewrite "the default model is chosen from the workspace…" to choose from
  model entries with no workspace, then save and start a draft with it. Delete
  "the default model needs a workspace".

Other browser tests:

- `transcript/transcript.test.tsx`: replace the proposal label test with one
  that shows the model ID and makes no `models?` request.
- `transcript/submissions.test.tsx:26,122`: drop the `models?` stubs.
- Delete `agent/catalogs.test.tsx`.

Checks: `make check`. In `make dev`, add entries for two harnesses, start a
session from the picker, change effort on a running session, delete its entry,
and confirm the session keeps its settings; save an entry with a misspelled
model and confirm the start error appears in chat.

## Implementation plan

1. **Store.** In `store.go`, replace the `catalogs` table with
   `model_entries`. Add migration step 4 in `migrate.go`. Add
   `model_entries.go` and its test. Delete `catalogs.go` and
   `catalogs_test.go`. Add the migration test.
2. **Runtime.** In `internal/session/selection.go`, reduce `Models` to the
   `harnessclient.Models` call and delete `CachedModels`; remove the now unused
   `encoding/json` and `store` imports if nothing else uses them.
3. **Routes.** In `internal/ui/models.go`, replace `server.models` with
   `listModelEntries`, `saveModelEntry`, and `deleteModelEntry`. In `ui.go`,
   route `GET /models`, `POST /models`, and `DELETE /models/{id}`. Rewrite
   `models_test.go`. Remove the models log from `harnesstest`.
4. **Types and state.** In `agent/types.ts`, delete `Model` and `Catalog` and
   add `ModelEntry`. In `app/types.ts`, add `SettingsSection` and the section
   list. In `state.ts`, replace `catalogs` with `modelEntries` and add
   `settings`. In `api.ts`, replace `fetchModels` with the three entry calls.
   In `main.tsx`, load the entries at startup.
5. **Labels.** Delete `agent/catalogs.ts` and its test. Simplify
   `selection-label.ts` (delete `modelLabel`, drop `models` from
   `settingsText`), `selection-name.tsx`, `session-selection.tsx` (no
   `workspace`, no `useModels`), and `transcript/proposal-item.tsx`.
6. **Slider.** Add `components/ui/slider.tsx` as described.
7. **Picker.** Rewrite `agent/model-select.tsx` as described, removing
   `fetchHarnesses`, catalog loading, the pending-efforts flow, the harness
   `Select`, and the effort `Select`. Update the file's opening comment. Rewrite
   its tests.
8. **Settings.** Move the open section to the store (`app.tsx`,
   `settings-dialog.tsx`). Add `app/settings-models.tsx` and render it from
   `SettingsAgents`. Simplify `DefaultSelection`. Update
   `settings-dialog.test.tsx` and add `settings-models.test.tsx`.
9. **Other tests.** Update the transcript and submissions tests.
10. Update the documentation below and run `make check`.

## Documentation updates

- `agents/architecture.md:29-39` — `describe` runs at startup, for the Settings
  entry form, and for `list_models`. `models` serves `list_models` only and
  saves nothing. The browser lists model entries the user saves in SQLite and
  never asks a plugin for models. The plugin still checks the model and effort
  at start and configure.
- `agents/glossary.md` — add **model entry**: a harness, native model ID,
  ordered effort levels, and default effort the user saves in Settings for the
  model picker; sessions save their own settings, not a reference to an entry.
- `docs/configuration.md` — add a **Models** section: entries are edited in
  Settings › Agents, saved in Hypercube's database rather than `config.json`,
  apply immediately, and are not checked against the harness until a session
  starts or changes model. Replace "Valid model and effort values come from the
  installed harness…" with: values are the harness's native IDs; the plugin
  rejects invalid ones at start or model change.
- `docs/usage.md` — **Start a session** step 2 and **Change the model or
  effort**: choose a harness tab, a model, and an effort level with the
  slider; models come from Settings › Agents › Models.
- `docs/getting-started.md` step 2 — add a model in Settings › Agents first
  (harness, model ID as the harness names it, effort levels), then choose it
  below the message box.
- `docs/harness-support.md:20-26` — remove "discovers … when the selector
  opens" and "Models and efforts are discovered in the session's folder";
  say the selector lists configured models and that each harness's row in the
  requirements table gives the form of its model ID and effort. The sentences
  about reported efforts and listing models without a conversation stay; they
  describe the plugins' configure replies and `list_models`.
- `docs/teamwork.md:75` — "not a restriction on the model selector" becomes
  "not a restriction on the models agents can choose".
- Screenshots are unchanged: the scripted plugin's models have no labels, so
  the composer trigger already shows model IDs.
