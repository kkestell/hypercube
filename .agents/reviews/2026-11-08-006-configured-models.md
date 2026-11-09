# Configured models review

## Scope and coverage

Reviewed commit `e6ccb83` against
`agents/plans/2026-11-08-001-configured-models.md` and the work log
`agents/work/2026-11-08-004-configured-models.md`. Read the whole touched
files: the store (`model_entries.go`, `migrate.go`, `store.go`), the routes
(`internal/ui/models.go`, `ui.go`), `internal/session/selection.go`, the picker
(`agent/model-select.tsx`, `components/ui/slider.tsx`), Settings
(`settings-models.tsx`, `settings-dialog.tsx`, `settings-preferences.tsx`),
page state, labels, the proposal card, their tests, and the documentation
changes. Agent discovery (`list_models`, `Runtime.Models`, the plugins'
`models` commands) and native model validation at start and configure are
unchanged, apart from `Runtime.Models` no longer saving catalogs.

Coverage gap: no live native-agent smoke test. Starting a session from the
picker, changing effort on a running session, and confirming that a
misspelled model's start error appears in chat were not exercised. The routes
were checked against a live isolated `make dev` backend over HTTP; the browser
interface was checked only through component tests.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Correctness | Fixed | Typing levels leaves the lowest level as the new entry's default |
| F2 | Low | Behavior | Open | Effort slider snaps back until a running session confirms the change |
| F3 | Low | Documentation | Fixed | Configuration guide says what the picker does not do |

## F1. Typing levels leaves the lowest level as the new entry's default

Medium · Correctness · Fixed · `internal/ui/web/app/settings-models.tsx`

The plan says adding an entry defaults its effort to the last level typed.
`changeLevels` kept the current default whenever it was still listed, so
typing `low, medium, high` one character at a time set the default to `low`
as soon as the first level was complete, and it stayed there. Saving without
touching **Default effort** gave every new multi-level entry its lowest effort.

### Evidence

The existing test typed `low` and then `low, medium, high` and asserted the
default was `low`, recording the behavior. Tracing `changeLevels` through
`l`, `lo`, `low`, `low,`, `low, m`, and so on gives the same result.

### Resolution

The dialog now derives the default: the level the user chose while it is still
listed, otherwise the last level. Edits start with the saved default as the
choice. This removes `changeLevels` and the one-level special case in `save`.
The test now asserts `high` after typing, and that a chosen `medium` survives
removing `low`.

## F2. Effort slider snaps back until a running session confirms the change

Low · Behavior · Open · `internal/ui/web/agent/model-select.tsx`

On release, `Effort` clears the dragged position and shows `value.effort`
again. For a running session, `value` changes only after the plugin's
configure reply arrives (`Runtime.Select` records a command and waits for
the reply), so the thumb and label return to the old level until then. Drafts
update at once. The previous effort `Select` behaved the same way, showing the
confirmed effort. Keeping the released position would show an effort the
harness has not yet accepted. Left open because choosing optimistic display
over confirmed display is a product decision outside the plan.

## F3. Configuration guide says what the picker does not do

Low · Documentation · Fixed · `docs/configuration.md`

"The picker does not discover models." describes something the picker lacks.
The surrounding sentences already say where entries come from. Removed it.

## Checks run

- `bun test web/app/settings-models.test.tsx web/app/settings-dialog.test.tsx`
  — 10 passed after the F1 fix.
- `bunx tsc -p .` in `internal/ui` — passed.
- `make check` — passed: 368 browser tests, 28 desktop tests, UI and website
  builds, Go build, vet, formatting, and all Go tests under `-race`.
- Isolated `make dev` (`HYPERCUBE_DEV_DIR=bin/cm-review-smoke`, port 4297),
  HTTP only: a new database answered `GET /models` with `[]`; `GET /harnesses`
  listed the three plugins; `POST` added an entry and returned the list; a
  duplicate was 409; a default outside the levels was 400; an unknown field was
  400; an unknown ID was 404; editing to one level set it as the default;
  `DELETE` returned `[]`. The server was stopped, `make dev-clean` run, and the
  state folder removed.
- Not run: the live native-agent smoke test (see Scope and coverage).

## Verdict

The implementation matches the plan: entries are saved and validated globally,
Settings edits them immediately, the picker reads only page state with harness
tabs, focused search, and a commit-based slider, and agent discovery and native
validation are unchanged. F1 is fixed and tested. F2 is a low, open product
choice. The live native-agent smoke test remains to be done.
