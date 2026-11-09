# Model entries in config.json

## Goal

The model picker list (Settings › Agents › Models) is saved in the
`model_entries` SQLite table and changes through its own immediate-apply
routes (`GET /models`, `POST /models`, `DELETE /models/{id}`). Every other
preference is in `config.json` and applies on **Save changes**.

This task moves the list into `config.json` as a `models` array, so all user
preferences live in one file and follow one load and save path:

- Adding, editing, and deleting a model entry change the Settings draft only.
  **Save changes** writes them with the rest of the config; **Cancel** or
  closing Settings discards them.
- The `/models` routes, the store's model entry code, and the
  `model_entries` table are deleted. The page reads the list from the config it
  already loads.
- On first start after the upgrade, the backend moves existing
  `model_entries` rows into `config.json` and drops the table.
- `docs/examples/config.json` holds a complete example configuration in the
  saved format, linked from `docs/configuration.md`.

The picker's behavior, the entry fields, and their validation rules are
unchanged. There are no prerequisites.

## Related code

Backend:

- `internal/config/config.go` — `Config`, `defaults`, `Load`, `Validate`,
  `Save`. Gains `ModelEntry` and the `models` field.
- `internal/config/config_test.go` — `TestSave`, `TestLoadDefaults`.
- `internal/store/model_entries.go` — `ModelEntry`, `Check`,
  `ErrDuplicateModel`, `Tx.ModelEntries`, `Tx.SaveModelEntry`,
  `Tx.DeleteModelEntry`. Deleted; `Check` moves to `config`.
- `internal/store/model_entries_test.go` — `TestModelEntryCheck` (moves to
  `config`) and `TestModelEntries` (deleted).
- `internal/store/store.go:157-160` — `schema` creates `model_entries`.
  `Open` (line ~170) runs `schema` in one transaction.
- `internal/store/reopen_test.go` — `TestOpenCurrentDatabasePreservesState`
  opens a database built from `testdata/current-schema.sql` and saves and
  reads back a model entry through the store.
- `internal/store/testdata/current-schema.sql` — the installed schema,
  including `model_entries`. Unchanged: it is the schema users have now, and
  the migration test starts from it.
- `internal/ui/models.go` — `server.harnesses` (kept), `listModelEntries`,
  `saveModelEntry`, `deleteModelEntry` (deleted), `writeJSON` (kept, used by
  four other handlers).
- `internal/ui/ui.go:29-31` — the three `/models` routes.
- `internal/ui/models_test.go` — `TestHarnessesAndModels` covers `/harnesses`
  and the `/models` routes.
- `internal/ui/config.go` — `getConfig`, `saveConfig` (decodes with
  `DisallowUnknownFields`, answers 400 for a failed `Validate`). Unchanged.
- `internal/ui/config_test.go` — `TestSaveConfig`.

Browser interface (`internal/ui/web/`):

- `agent/types.ts` — `ModelEntry` with `id`.
- `app/types.ts` — `Config`.
- `state.ts` — `State.modelEntries`, `initialState`.
- `api.ts:87-100` — `fetchModelEntries`, `saveModelEntry`, `deleteModelEntry`.
- `main.tsx:32` — fetches `/models` at startup.
- `app/settings-models.tsx` — `SettingsModels` (reads and writes
  `state.modelEntries` through the routes) and `EntryDialog`.
- `app/settings-preferences.tsx` — `DefaultSelection` renders `ModelSelect`;
  `SettingsAgents` renders `<SettingsModels />`.
- `app/settings-dialog.tsx` — `SettingsContent` owns the `Config` draft,
  `edit`, and `save`. Unchanged.
- `agent/model-select.tsx` — reads `state.modelEntries`; keys and compares
  entries by `id`.
- `agent/selection-name.tsx`, `agent/session-selection.tsx:35`,
  `transcript/proposal-item.tsx:43,53` — pass `state.modelEntries` to
  `modelTitle` and `settingsText`.
- `test-state.ts` — `testConfig`.
- Tests: `agent/model-select.test.tsx`, `app/settings-models.test.tsx`,
  `app/settings-dialog.test.tsx:42,131`.

## Decisions

### Saved format

`config.json` gains a `models` array, written last:

```go
type Config struct {
	DefaultSelection *harness.Settings `json:"default_selection,omitempty"`
	ChildGuidance    string            `json:"child_model_guidance"`
	DefaultWorktree  bool              `json:"default_worktree"`
	Theme            string            `json:"theme"`
	TerminalFontSize int               `json:"terminal_font_size"`
	Notifications    Notifications     `json:"notifications"`
	Models           []ModelEntry      `json:"models"`
}

// ModelEntry is a model the user lists for the picker: a harness's native
// model ID, its title, its effort levels from lowest to highest, and the effort
// a session starts with. Sessions keep their own settings, not a reference to
// an entry.
type ModelEntry struct {
	Harness       string   `json:"harness"`
	Model         string   `json:"model"`
	Title         string   `json:"title"`
	Efforts       []string `json:"efforts"`
	DefaultEffort string   `json:"default_effort"`
}
```

Entries have no ID. Harness and model identify an entry and are unique, and
the browser edits the list by position in its draft. The list keeps the order
it is saved in; Settings appends new entries, and the picker shows harness
tabs and models in list order, as it does now.

`defaults()` sets `Models: []ModelEntry{}`, so a file without `models` loads
as an empty list. `Validate` sets a nil `Models` (`"models": null`) to an empty
slice, so `GET /config` always answers an array.

### Validation

`ModelEntry.Check` moves from `store` to `config` unchanged except that it no
longer trims an ID. Its rules stay: harness, model, and title required after
trimming; levels non-empty and not repeated; one level is its own default;
several levels need a listed default; no levels means no default.

`Config.Validate` checks each entry and rejects a repeated harness and model:

```go
seen := make(map[[2]string]bool)
for i := range c.Models {
	e := &c.Models[i]
	if err := e.Check(); err != nil {
		return fmt.Errorf("model %s %q: %w", e.Harness, e.Model, err)
	}
	key := [2]string{e.Harness, e.Model}
	if seen[key] {
		return fmt.Errorf("%s lists model %q twice", e.Harness, e.Model)
	}
	seen[key] = true
}
```

`Validate` runs in `Load`, `Save`, and `saveConfig`, so a hand-edited file
with a bad entry fails to load with the file's path, and a bad save answers
400 with the reason. `ErrDuplicateModel` is deleted.

### Routes

`GET /models`, `POST /models`, and `DELETE /models/{id}` are deleted with their
handlers. `GET /config` and `POST /config` carry the list. `internal/ui/models.go`
keeps `harnesses` and `writeJSON`; rename it to `harnesses.go`, since that is
all it holds.

### Migration

`store.Open` moves the old table's rows to `config.json` after creating the
schema. New file `internal/store/model_entries.go` replaces the old one:

```go
// moveModelEntries moves the model list from the model_entries table, where
// earlier builds saved it, to config.json, then drops the table.
func (d *DB) moveModelEntries() error
```

In one `d.Tx`:

1. Query `sqlite_master` for the `model_entries` table. Without it, return nil.
2. Read `harness, model, title, efforts, default_effort` ordered by harness,
   then model, decoding `efforts` from JSON into `config.ModelEntry`.
3. With any rows: `config.Load()`, append each row whose harness and model the
   config does not already list, and `config.Save(cfg)`.
4. `DROP TABLE model_entries`.

The config is written before the drop commits. If the save fails, the
transaction rolls back and the table stays for the next start. If the commit
fails after the save, the next start finds the rows already listed and skips
them, so the move never duplicates or loses an entry. The rows were checked
when saved, so they are trusted.

`Open` calls it after the schema transaction and returns its error wrapped as
`moving model entries to config.json: %w`, closing the database first, as it
does for a schema failure. An unreadable or invalid `config.json` therefore
stops startup until the file is fixed; the error names the file.

`schema` drops its `CREATE TABLE IF NOT EXISTS model_entries` statement, so a
new database never has the table and `moveModelEntries` does nothing.

`store` importing `config` adds no cycle (`config` imports only `harness`).

### Page state

- `agent/types.ts`: `ModelEntry` loses `id`.
- `app/types.ts`: `Config` gains `models: ModelEntry[]`; its comment adds
  "models lists the model picker's entries".
- `state.ts`: `modelEntries` is deleted. Readers use
  `state.config?.models ?? null`, which is null until the config loads, as
  `modelEntries` was.
- `api.ts`: `fetchModelEntries`, `saveModelEntry`, and `deleteModelEntry` are
  deleted; `ModelEntry` leaves its imports. `main.tsx` drops the `/models`
  fetch.
- `test-state.ts`: `testConfig` gains `models: []`.

After **Save changes**, `receivedConfig(state, draft)` already stores the
saved config, so the composer's picker shows the new list at once. Other open
windows see it after a reload, as for every other preference.

### The picker

`ModelSelect` gains an optional prop:

```ts
// entries is the list to choose from; the saved list when omitted.
entries?: ModelEntry[];
```

and uses `entries ?? state.config?.models ?? null`, showing "Loading models…"
while that is null. Settings › General's `DefaultSelection` passes
`entries={draft.models}`, so a model added in the same Settings visit can be
chosen as the default before saving, and a deleted one cannot.

`SelectionName` gains the same optional `entries?: ModelEntry[] | null` prop,
defaulting to `state.config?.models ?? null`; `ModelSelect` passes its list to
it and to `settingsText`, so the trigger shows the draft's title.

Entries are keyed and compared by model within the selected harness tab:
`CommandItem key={entry.model}`, `current?.model === entry.model` for the
check and `aria-selected`, and the `Effort` key
`${current.model}:${current.efforts.join(',')}:${value.effort}`. The opening
comment stays accurate ("saved model entries").

`session-selection.tsx`, `proposal-item.tsx`, and `selection-name.tsx`
replace `state.modelEntries` with `state.config?.models ?? null`.

### Settings › Agents › Models

`SettingsModels` becomes a draft editor like the other sections:

```ts
export function SettingsModels({models, onChange}: {models: ModelEntry[]; onChange: (models: ModelEntry[]) => void})
```

`SettingsAgents` renders
`<SettingsModels models={draft.models} onChange={models => onChange({models})} />`.

- The file's opening comment becomes "The model picker's entries, edited in the
  Settings draft." The paragraph "Model changes apply immediately, without Save
  changes." and the list's error line are deleted.
- The list renders `models` with key `` `${entry.harness}\n${entry.model}` ``
  and the same rows, buttons, and `aria-label`s. "Loading models…" is deleted:
  the section renders only once the draft has loaded. With no entries it shows
  "No models."
- Component state is `editing: {index: number | null; entry: ModelEntry} | null`.
  **Add Model** opens the dialog with `index: null` and a blank entry; **Edit**
  passes the row's index. **Delete** calls
  `onChange(models.filter((_, i) => i !== index))`.
- The list sits inside `SettingsContent`'s `<fieldset disabled={saving}>`, so
  its buttons are disabled while the config saves.

`EntryDialog` takes `{entry, others, save, close}`, where `others` is the list
without the entry being edited and `save(entry)` replaces or appends it in the
draft:

```ts
save={entry => { onChange(editing.index === null ? [...models, entry] : models.with(editing.index, entry)); setEditing(null); }}
```

The dialog keeps its harness fetch (`fetchHarnesses`), fields, unavailable
harness option, and default-effort behavior. It drops `saving`, the async
request, and `store`. Its **Save** button validates locally, so a mistake
shows in the dialog beside the field instead of in the Settings footer after
**Save changes**:

- Levels are split on commas, trimmed, and blanks dropped (`low, high,` is
  `low, high`). Model and title are trimmed.
- No harness chosen: "Choose a harness."
- Model or title empty after trimming: "Enter a model ID." or "Enter a
  title." The inputs keep `required`.
- A level repeats: `Effort level "${level}" repeats.`
- `others` lists the harness and model:
  `${displayName(harness)} already lists ${model}.`

On success it calls `save` with the trimmed entry and the computed default.
`Config.Validate` stays the authority for every save, including hand edits.

### Example configuration

New file `docs/examples/config.json` holds the user's current configuration
and models in exactly the form `config.Save` writes (two-space indent,
trailing newline):

```json
{
  "default_selection": {
    "harness": "claude",
    "model": "opus",
    "effort": "medium"
  },
  "child_model_guidance": "",
  "default_worktree": false,
  "theme": "system",
  "terminal_font_size": 13,
  "notifications": {
    "requests": true,
    "replies": true,
    "desktop": true,
    "sound": true
  },
  "models": [
    {
      "harness": "claude",
      "model": "haiku",
      "title": "Haiku",
      "efforts": [
        "low",
        "medium",
        "high",
        "xhigh",
        "max"
      ],
      "default_effort": "medium"
    },
    {
      "harness": "claude",
      "model": "opus",
      "title": "Opus",
      "efforts": [
        "low",
        "medium",
        "high",
        "xhigh",
        "max"
      ],
      "default_effort": "medium"
    },
    {
      "harness": "claude",
      "model": "sonnet",
      "title": "Sonnet",
      "efforts": [
        "low",
        "medium",
        "high",
        "xhigh",
        "max"
      ],
      "default_effort": "medium"
    },
    {
      "harness": "opencode",
      "model": "openai/gpt-6-luna",
      "title": "openai/gpt-6-luna",
      "efforts": [
        "none",
        "low",
        "medium",
        "high",
        "xhigh",
        "max",
        "default"
      ],
      "default_effort": "default"
    },
    {
      "harness": "opencode",
      "model": "openai/gpt-6.1-sol",
      "title": "openai/gpt-6.1-sol",
      "efforts": [
        "low",
        "medium",
        "high",
        "xhigh",
        "max"
      ],
      "default_effort": "high"
    },
    {
      "harness": "opencode",
      "model": "openai/gpt-6.1-terra",
      "title": "openai/gpt-6.1-terra",
      "efforts": [
        "none",
        "minimal",
        "low",
        "medium",
        "high",
        "xhigh"
      ],
      "default_effort": "high"
    },
    {
      "harness": "opencode",
      "model": "opencode-go/deepseek-v4.1-flash",
      "title": "opencode-go/deepseek-v4.1-flash",
      "efforts": [
        "low",
        "high",
        "max"
      ],
      "default_effort": "max"
    },
    {
      "harness": "opencode",
      "model": "opencode-go/muse-spark-1.3-contributor",
      "title": "opencode-go/muse-spark-1.3-contributor",
      "efforts": [
        "minimal",
        "low",
        "medium",
        "high",
        "xhigh"
      ],
      "default_effort": "medium"
    }
  ]
}
```

The website mounts only `docs/*.md` and `docs/images/`, and its rules link
repository files outside the guides by GitHub URL, so `docs/configuration.md`
links it as
`https://github.com/kkestell/hypercube/blob/main/docs/examples/config.json`.
A config test keeps the file valid and in the saved format.

## Test plan

Go (`go test -race ./...`):

- `internal/config/config_test.go`:
  - Move `TestModelEntryCheck` here from `store`, dropping ID handling; keep
    its cases.
  - `TestSave`: `cfg` gains two models (one with levels and a default, one with
    none) and round-trips. Invalid cases add a repeated harness and model and
    an entry without a title; each leaves the file unchanged.
  - `TestLoadDefaults`: expected defaults gain `Models: []ModelEntry{}`; a file
    with `"models": null` loads as an empty list.
  - New `TestExampleConfig`: read `../../docs/examples/config.json`, decode it
    with `DisallowUnknownFields`, `Validate` it, and require
    `json.MarshalIndent(cfg, "", "  ")` plus `"\n"` to equal the file's bytes.
- `internal/store/reopen_test.go`, `TestOpenCurrentDatabasePreservesState`:
  - Set `XDG_CONFIG_HOME` to a temporary directory and write a `config.json`
    with `"theme": "dark"` and one model, `opencode` / `provider/model`.
  - Append to the raw schema setup two `INSERT INTO model_entries` rows:
    `opencode` / `provider/model` (a different title) and `claude` / `opus`
    with levels `["low","high"]`, default `high`.
  - After the first `reopen`: `sqlite_master` has no `model_entries`;
    `config.Load()` keeps theme `dark` and lists the config's own
    `provider/model` entry unchanged followed by `claude` / `opus` with its
    levels in order. Remove the `SaveModelEntry` / `ModelEntries` steps and the
    final model comparison. The second reopen leaves `config.json` unchanged.
- New `internal/store` test: with the table holding a row and `config.json`
  invalid (`{"theme":"blue"}`), `Open` fails with an error naming the file and
  the table still holds the row; after fixing the file, `Open` moves it.
- Delete `internal/store/model_entries_test.go`.
- `internal/ui/models_test.go`: `TestHarnessesAndModels` becomes
  `TestHarnesses`, keeping only the `/harnesses` assertion.
- `internal/ui/config_test.go`, `TestSaveConfig`: `cfg` gains one model;
  invalid bodies add "repeated model" (two equal entries) and "entry id"
  (`"id"` inside an entry, rejected as an unknown field), each answering 400
  and leaving the saved config unchanged.

Browser (`bun test` and `bunx tsc -p .` in `internal/ui`):

- `agent/model-select.test.tsx`: build state with
  `testState({config: testConfig({models: entries})})`, drop `id` from
  `entries`, and replace `modelEntries` updates with config updates. Existing
  cases otherwise stay.
- `app/settings-models.test.tsx`, rewritten through `SettingsDialog` opened at
  `'Agents'`, with fetch stubbed for `config` (a config with one saved entry),
  `harnesses?`, and `POST config`:
  - Adding an entry (harness, model, title, levels `low, medium, high`,
    default `medium`) shows it in the list and sends no request besides the
    harness list; `state.config.models` is unchanged. **Save changes** posts a
    config whose `models` is the saved entry followed by the new one, and
    `state.config.models` then equals it.
  - Adding, editing a title, and deleting, then **Cancel**, posts nothing and
    leaves `state.config.models` unchanged; reopening Settings shows the saved
    list.
  - The entry dialog refuses a harness and model already listed with
    "… already lists …" and keeps the form; a repeated level is refused; the
    default-effort control appears only with two or more levels and follows
    the last level until chosen.
  - Editing an entry whose harness is not installed keeps its harness; deleting
    an entry leaves `state.workspaces` unchanged.
- `app/settings-dialog.test.tsx`: drop `modelEntries` from `open` and the
  default-model test. In "the default model is chosen without a workspace…",
  the fetched config carries the `opencode` / `sol` entry; the rest of the test
  is unchanged. Add to it, or a sibling test, that a model added in Agents can
  be chosen in General's default picker before **Save changes**, and both are
  posted together.

Checks: `make check`. In `make dev`, add, edit, and delete entries and confirm
the picker changes only after **Save changes**; confirm **Cancel** discards.
For the migration, copy a database with `model_entries` rows into the dev state
folder (never the user's real `~/.local/state` or `~/.config`), start
`make dev`, and confirm the rows appear in the dev `config.json` and the table
is gone.

## Implementation plan

1. **Config.** In `internal/config/config.go`, add `ModelEntry` with `Check`
   (moved from `store`), the `Models` field, the `defaults()` value, and the
   `Validate` loop. Update the `Config` doc comment to mention `Models`. Move
   and extend the config tests.
2. **Example.** Add `docs/examples/config.json` with the contents above and
   `TestExampleConfig`.
3. **Store.** Remove `model_entries` from `schema`. Replace
   `internal/store/model_entries.go` with `moveModelEntries` and call it from
   `Open`. Delete `model_entries_test.go`. Update `reopen_test.go` and add the
   failed-save test.
4. **Routes.** Delete the three handlers and routes; rename `models.go` to
   `harnesses.go`; drop the now unused `errors` and `store` imports. Trim
   `models_test.go`; extend `config_test.go`.
5. **Browser types and state.** Update `agent/types.ts`, `app/types.ts`,
   `state.ts`, `api.ts`, `main.tsx`, and `test-state.ts`.
6. **Picker and labels.** Update `model-select.tsx` (prop, keys, comparisons),
   `selection-name.tsx`, `session-selection.tsx`, and `proposal-item.tsx`.
   Update `model-select.test.tsx`.
7. **Settings.** Rewrite `settings-models.tsx` as the draft editor; update
   `settings-preferences.tsx` (`SettingsAgents`, `DefaultSelection`). Rewrite
   `settings-models.test.tsx`; update `settings-dialog.test.tsx`.
8. Update the documentation below and run `make check`.

## Documentation updates

- `docs/configuration.md`:
  - In **Models**, describe the `models` field: a list of objects with
    `harness`, `model`, `title`, `efforts` (lowest first), and
    `default_effort`; an empty list by default.
  - Replace "Entries are saved globally in Hypercube's database, not
    `config.json`. Adding, editing, and deleting apply immediately, without
    **Save changes**." with: entries are saved in `config.json` and apply on
    **Save changes**, like other settings. Keep the sentences on checking
    against the harness, existing sessions, and agents' own choices.
  - Add one sentence after the file location paragraph:
    "[An example configuration](https://github.com/kkestell/hypercube/blob/main/docs/examples/config.json)
    shows every field."
- `docs/getting-started.md` step 2 — after adding the model, choose
  **Save changes**, then choose its harness tab and model below the message
  box.
- `agents/architecture.md:35` — "The browser lists model entries the user saves
  in SQLite" becomes "the user saves in `config.json`".
- `agents/glossary.md` — **model entry**: "saved in Settings" becomes "saved
  in `config.json` from Settings".
- `docs/harness-support.md` and `docs/usage.md` say models come from
  **Settings › Agents › Models** and do not mention where they are saved;
  unchanged.
