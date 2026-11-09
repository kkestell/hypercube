# Model entries in config.json review

## Scope and coverage

Reviewed the staged changes on `config-model-entries` against
`agents/plans/2026-11-09-001-models-in-config.md` and
`agents/work/2026-11-09-001-models-in-config.md`. Read the whole touched files
for the config model, the migration in `store.Open`, the `/config` and
`/harnesses` handlers, the picker, the Settings model editor and dialog, the
state and API changes, the tests, and the documentation. Traced callers of
`store.Open`, `config.Load`, and `config.Save`, and how an open page reacts to
a new build.

The user's requirements are met:

1. Model entries are the `models` array in `config.json`; the `model_entries`
   table, its store code, and the `/models` routes are gone.
2. The model editor changes only the Settings draft. **Save changes** posts
   them with the rest of the config and **Cancel** discards them
   (`settings-models.test.tsx`, `settings-dialog.test.tsx`).
3. `docs/examples/config.json` holds a complete configuration with models, and
   `TestExampleConfig` keeps it in the saved format.

Existing rows move from `model_entries` into `config.json` on the first start
and the table is dropped (`TestOpenCurrentDatabasePreservesState`,
`TestOpenKeepsModelEntriesWhenConfigIsInvalid`, and the work log's dev run).

Gaps: the example's contents were not compared with the user's real
`~/.config/hypercube` and `~/.local/state/hypercube`, which this review does
not read. No browser click-through; the dialog tests drive the real Settings
dialog.

### Migration ownership and a running app

The migration in `store.Open` is acceptable where it is. `Open` already brings
the saved database to the current schema; the move must write `config.json`
before the drop commits, which is simplest inside one store transaction. A
version in the backend's `startApp` would need the store to expose the rows
and the drop separately and the backend to sequence them, which adds an
interface without changing the outcome. `store` importing `config` creates no
cycle, and the code is self-contained in `internal/store/model_entries.go`
for later deletion.

It is safe against a running app in normal use:

- `startApp` (`cmd/hypercube/backend.go`) is the only caller of `store.Open`,
  and it runs before the HTTP server serves, so no Settings save races the
  move in the same process.
- The listener is bound before the store opens, so a second backend on the
  same port stops before touching the state.
- `scripts/dev` sets both `XDG_STATE_HOME` and `XDG_CONFIG_HOME` under its
  folder, and Go tests use temporary folders; only `reopen_test.go` creates the
  old table, and it sets `XDG_CONFIG_HOME` first.
- An open page reloads when the server serves a new build
  (`web/data-socket.ts`), so a page from before the upgrade cannot post a
  config without `models` and erase the moved entries.
- The move runs only while the table exists, and rows the config already lists
  are skipped, so a repeat after an interrupted move neither loses nor repeats
  an entry.

### Entry dialog validation

`EntryDialog`'s checks are proportionate. They cover only what a user can get
wrong in the dialog: no harness chosen (the select has no native `required`),
a model ID or title that is blank after trimming (native `required` accepts
spaces), a repeated level, and a harness and model already in the list. The
default effort rules are not repeated because the dialog computes the
default. Without these checks a mistake would surface only after **Save
changes**, in the Settings footer and away from the entry. `Config.Validate`
remains the authority for every save.

### "opening focuses search…" failure

The failure is not caused by this change. With the two tests filtered as the
work log describes, ten runs failed nine times on this branch and nine times
on a detached checkout of `HEAD`. At `HEAD`, running any Settings dialog test
first ("guidance saves…", "cancel discards…") also makes it fail, and it passes
alone. At the failing assertion `querySelectorAll('[role=tab]')` returns
nothing while `document.body.innerHTML` contains both tabs, and reading
`innerHTML` first makes the test pass, which points to a stale happy-dom query
result rather than state left by a test. The full suite passes. It is outside
this task and left open.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Low | Documentation | Fixed | Glossary entry exceeds the wrap width |

## F1. Glossary entry exceeds the wrap width

Low · Documentation · Fixed · `agents/glossary.md:22`

The edited **model entry** line ran past the 80-column wrap the file uses.

### Evidence

`awk 'length > 80' agents/glossary.md` listed the line.

### Resolution

Rewrapped the entry.

## Checks run

- `go test -race ./internal/config ./internal/store ./internal/ui` — passed.
- `bun test` in `internal/ui` — 403 passed.
- `bun test web/app/settings-dialog.test.tsx web/agent/model-select.test.tsx
  -t "preferences save|opening focuses search"`, ten runs on this branch and
  ten on `HEAD` — nine failures each.
- `make check` was not repeated: the work log records it passing on this
  implementation, and this review changed only documentation.

## Verdict

Approved. The change meets the three requirements and the migration rule, with
no medium or high findings. F1 is fixed. The intermittent picker test failure
predates this change and remains open.
