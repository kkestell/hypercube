# Model entries in config.json

## Plan

`agents/plans/2026-11-09-001-models-in-config.md`

## Summary

Model entries are now the `models` array in `config.json`. Settings › Agents ›
Models edits the Settings draft, so entries apply on **Save changes** and
**Cancel** discards them. The `/models` routes, the store's entry code, and the
`model_entries` table are gone; `store.Open` moves rows from an existing table
into `config.json` and drops it. `docs/examples/config.json` holds the user's
configuration and models, and `TestExampleConfig` keeps it in the saved format.
The plan's goal is met.

## Departures from the plan

- `EntryDialog` takes an `adding` prop besides `{entry, others, save, close}`,
  so its title says **Add Model** or **Edit Model**. The old dialog decided by
  the entry's ID, which entries no longer have.
- The failed-save store test proves that the row stayed in the table by
  fixing `config.json`, reopening, and finding the row moved, rather than
  reading the table between the two opens.

## Decisions

- The entry dialog shows its local validation errors in the dialog's existing
  alert line, which also shows a failed harness list.
- `reopen_test.go` gained `createCurrentDatabase`, `writeConfig`, and
  `hasModelEntriesTable` helpers, shared by the preserved-state test and the
  new invalid-config test.
- `internal/ui/models_test.go` keeps its name; it holds `TestHarnesses`, the
  `get` helper, and an unrelated session test.

## Checks run

- `go test -race ./internal/config ./internal/store ./internal/ui` — passed.
- `bun test` in `internal/ui` — 403 passed, five full runs in a row.
- `make check` — passed: 403 browser tests, 28 desktop tests, Go build, vet,
  formatting, and all Go tests under `-race`.
- `git diff --check` — passed.

## Manual verification

`scripts/dev` sets both `XDG_STATE_HOME` and `XDG_CONFIG_HOME` under its dev
folder. The run used a private folder and port so neither the user's real
folders nor the shared `~/projects/hypercube-dev` folder were touched.

1. Migration on startup.

   ```sh
   D=/tmp/hc-config-models-dev
   mkdir -p $D/state/hypercube $D/config/hypercube
   sqlite3 $D/state/hypercube/hypercube.db < internal/store/testdata/current-schema.sql
   sqlite3 $D/state/hypercube/hypercube.db "INSERT INTO model_entries VALUES ('a','claude','haiku','Haiku','[\"low\",\"high\"]','low'); INSERT INTO model_entries VALUES ('b','opencode','provider/x','X','[]','');"
   echo '{"default_selection":{"harness":"claude","model":"haiku","effort":"low"}}' > $D/config/hypercube/config.json
   HYPERCUBE_DEV_DIR=$D HYPERCUBE_DEV_PORT=4297 scripts/dev up
   ```

   The dev `config.json` was rewritten with the default selection kept and
   both entries in `models`; `sqlite_master` no longer listed `model_entries`;
   `GET /config` returned the list and `GET /models` answered 404.

2. Saving through `POST /config` on the dev server: a body listing
   `claude` / `haiku` twice answered 400 `claude lists model "haiku" twice`;
   a body without the Haiku entry answered 204 and `GET /config` then listed
   only `provider/x`.

The Settings click-through (add, edit, delete, then **Save changes** or
**Cancel**) was not done in a browser; `settings-models.test.tsx` and
`settings-dialog.test.tsx` cover it through the real Settings dialog. The dev
session was stopped with `scripts/dev clean`, which removed its folder.

## Follow-up work

- `web/agent/model-select.test.tsx` "opening focuses search…" fails when run
  directly after `web/app/settings-dialog.test.tsx` "preferences save…"
  (`bun test <both files> -t …`). It fails intermittently in the same pairing
  at HEAD before this change, and adding any DOM read before its assertion
  makes it pass, which points at stale happy-dom query results. The full suite
  passes.
