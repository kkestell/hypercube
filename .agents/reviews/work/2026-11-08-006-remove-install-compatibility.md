# Work review: retired build and installation cleanup

## Round 1 — 2026-11-08

**Verdict: changes-required.** Unit 006's own changes are correct and have no
findings. The final whole-task sweep found no remaining migration or
compatibility code, but two low-priority leftovers in files owned by unit 003:
a test case built on the retired profiles format (F1), and retirement history
written into two documents (F2). This is an independent review by a worker that
did not plan or implement the unit.

- Task ID: `2026-11-08-006-remove-install-compatibility`; mode: `work`.
- Author: `compat-review`, Hypercube child of `2f4c7990`; native conversation
  `c805986c-9023-4e3e-a660-11912f975ea2` (`CLAUDE_CODE_SESSION_ID`);
  Claude Code / `claude-opus-5-5` / effort not exposed.
- Checkout: `/Users/kyle/src/hypercube/.hypercube/2f4c7990/compat-impl`,
  branch `hypercube/2f4c7990/compat-impl`.
- Fixed Code base: `ae546fee7dbb8a09a6edf4770011e23813193bb5` (ancestor of
  head, verified).
- Reviewed Code head: `fa4111e98ba559716f63a1bad98899d91590f1f6`, tree
  `fdedee42580802231bd2415bd669c8f9bf215311`; the single implementation commit.
- Checkout HEAD at review: `080105cf5b1a83f6b46b53b6b5c202e6d65ce72c`; Code
  head to HEAD changes only the work log. Working tree clean.
- Work log (repository-relative):
  `.agents/work/2026-11-08-006-remove-install-compatibility.md`.
- Plan (repository-relative):
  `.agents/plans/2026-11-08-006-remove-install-compatibility.md`, blob
  `f6bf8eca928d621d7eb122d1f19a7bd86053896e`, matching the accepted Round 2 plan
  review.
- Whole-task sweep range: main `ae022eeebf9dc90c75a61527267db0682b194057` to
  `fa4111e98ba559716f63a1bad98899d91590f1f6`.

### Findings

**F1 (low, unit 003's area): `internal/ui/config_test.go:37` still tests the
retired profiles format.** The `"legacy profiles"` case posts
`{"theme":"system","terminal_font_size":13,"profiles":[]}`, the shape of the
configuration that 003's converters used to upgrade (added in `444f60a` when
profiles were replaced). With the converters gone, it is a fixture for a
retired format. The rejection it checks is the generic unknown-field rejection
already covered by the `"unknown setting"` case. Correction: delete the case.
Verify with `go test ./internal/ui -run TestSaveConfig`.

**F2 (low, unit 003's area): two documents describe the retirement instead of
the current behavior.** `agents/development.md:243-248` says "Historical
conversion code has been retired now that both installations use that
baseline", and `docs/upgrades.md:18-22` says "conversion from historical
database and configuration formats has been retired". The user's documentation
rule is to describe what the project does as if the right way had been known
from the start, without history. Both sentences are history added by this
task. Correction, in substance:

- `agents/development.md`: "The current database and configuration are the
  supported baseline. Startup creates missing tables and indexes. A change to
  the database schema or other saved state includes a migration that preserves
  the user's state. Delete obsolete files when you find them, including
  leftovers from earlier builds and installs."
- `docs/upgrades.md`: "Normal upgrades preserve running agents and terminal
  shells. Follow the release notes for any saved-state changes. Replacing the
  app with an older version does not convert saved state back to that
  version's format."

Neither finding concerns unit 006's files; route both to the owner of 003's
area.

### Unit 006 assessment

- **Codex cleanup.** `Makefile:binaries`, `make/linux.mk:build`,
  `make/macos.mk:build` and `place` lose only the named
  `hypercube-harness-codex` deletions; the `PLUGINS` build and link loops are
  unchanged. `scripts/linux-install`'s generic pruning of owned links that no
  longer resolve is untouched, as plan review Round 1 required.
- **`quit-desktop` deviation: correct.** `lsappinfo find bundleid=APP_ID`
  prints the registered application for the bundle ID, or nothing. A read-only
  query on this Mac (no quit, no change) showed the only registered
  application for `com.github.kkestell.hypercube` is
  `.../MacOS/Hypercube Desktop`, although the backend `hypercube ui` was
  running from the same bundle. The backend therefore does not satisfy the
  guard, and the old executable-path match is no longer needed. When the app is
  not installed or not running, the output is empty and the recipe skips the
  quit without launching anything; the implementer showed that the plan's
  AppleScript `is running` form fails to compile for an unregistered ID, which
  would break a first `install-service`. `DESKTOP_EXE` is still used by the
  bundle rule (`make/macos.mk:26`); no variable became unused.
- **`scripts/dev:stop_holders`** keeps its PID-file scope and wait, matching
  only `hypercube-hold`.
- **Linux tests.** The neutral `hypercube-harness-extra` fixture copies a real
  plugin, links it, checks the link resolves, and asserts the stock reinstall
  prunes it, so current reconciliation stays covered. `test-bundle`'s exact
  comparison of discovered IDs with the bundle's `plugins` list is sound:
  `/harnesses` entries have only `id`, `name`, `installed` and `error`
  (`internal/harness/discovery.go:43,112`), so no nested `id` can disturb the
  match. `test-release` drops only the absent-Codex checks.

### Whole-task sweep

Searched executable Go, TypeScript, JavaScript, scripts and make files at
`fa4111e` for migration, upgrade, version, legacy, old-version and
compatibility terms, retired symbols and comments mentioning earlier builds,
then compared the results with every row of the shared design's candidate
table. Apart from F1 and F2, nothing remains to remove:

- `internal/store/testdata/current-schema.sql:56` `PRAGMA user_version = 4` is
  the current-database fixture; the design keeps that version as inert.
- `COALESCE` in `internal/store` handles columns and JSON fields that are
  nullable or optional in the current schema (top-level `parent`, all-session
  grants, optional item fields), not old rows.
- WebSocket `upgrade`, `make/macos.mk:74` in-place replacement, the completed
  `agents/todo.md:80` item and `agents/codex-retirement.md` are current
  behavior or historical records.

### Verification

| Check | Status | Source |
| --- | --- | --- |
| `make check` on tree `fdedee42…` | reused, exit 0 | `/tmp/compat-impl-check/make-check-006.out`; log Verification |
| `scripts/linux/run bash -c 'scripts/linux/test-bundle && scripts/linux/test-install'` | reused, exit 0 (`test-bundle: passed`, `test-install: passed`) | `/tmp/compat-impl-check/linux-006.out` |
| `lsappinfo find bundleid=com.github.kkestell.hypercube` and `lsappinfo info -only executablepath` on the result; `ps` for the backend | run (read-only) | This review. Needed to settle whether the running backend registers under the bundle ID, which would make the new guard true while the desktop app is closed. |
| Anchors, ancestry, tree, plan blob, Code head→HEAD diff; sweep searches | run | `git merge-base --is-ancestor`, `git rev-parse`, `git hash-object`, `git grep`, `git diff` from main |

Limitations: `make quit-desktop` was not executed and the installed app was not
quit or changed. `test-desktop`, `test-release` and host packaging were not run,
as the plan excludes them.

## Round 2 — 2026-11-08

**Verdict: accept.** F1 and F2 are resolved and the fix adds no new problems.
Same independent reviewer as Round 1.

- Author: `compat-review`; native conversation
  `c805986c-9023-4e3e-a660-11912f975ea2`; Claude Code / `claude-opus-5-5`.
- Fixed Code base: `ae546fee7dbb8a09a6edf4770011e23813193bb5`.
- Reviewed Code head: `672bfaaae8158bbd40d7a66325a96442abd54295`, tree
  `1c8bd54b22dc202e5eaaad997c66968378c8497e` (base is an ancestor, verified).
  New in the range since Round 1: fix commit `672bfaa` only.
- Checkout HEAD at review: `c2c581cdc8174ed975cf4a2693f9596e71c98653`; Code
  head to HEAD changes only the work log (Repair 1). Working tree clean.

### Resolution

| Finding | Status | Evidence at `672bfaa` |
| --- | --- | --- |
| F1 | resolved | `internal/ui/config_test.go`: the `"legacy profiles"` case is deleted; `"unknown setting"` still checks unknown-field rejection. The remaining map keys keep their alignment, so gofmt is unaffected. |
| F2 | resolved | `agents/development.md` "Backwards compatibility" and `docs/upgrades.md` now state current behavior only: startup creates missing tables and indexes, saved-state changes include a migration, and release notes cover saved-state changes. A search for "historical" and "has been retired" in `docs/`, `web/` and `agents/development.md` finds nothing. |

No new findings.

### Verification

| Check | Status | Source |
| --- | --- | --- |
| `go test ./internal/ui -run TestSaveConfig -count=1 -v` pass; gofmt clean | reused | work log Repair 1, run on the fix tree |
| `make check`, Linux bundle/install run | reused | Round 1; the fix changes only one test case and prose |
| Fix diff, ancestry, tree, Code head→HEAD diff, wording search | run | `git diff`, `git merge-base --is-ancestor`, `git rev-parse`, `git grep` |

No new test run: the one code change removes a test case, which the reused
focused run covers.
