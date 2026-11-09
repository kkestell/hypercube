# Current database and configuration: implementation

- Task ID: `2026-11-08-003-remove-backwards-compatibility`
- Status: implemented; awaiting fresh independent work review.
- Plan (repository-relative):
  `.agents/plans/2026-11-08-003-remove-backwards-compatibility.md`
- Accepted plan blob: `16ff0e39fb7c3c940d23526e1bb97aab5b156840`.
- Accepted review (repository-relative):
  `.agents/reviews/plans/2026-11-08-003-remove-backwards-compatibility.md`, Round 2.
- Checkout: `/Users/kyle/.local/share/opencode/worktree/803f5f/remove-backwards-compatibility`.
- Branch: `task/remove-backwards-compatibility`; clean, exclusive checkout and
  empty index confirmed before edits.
- Fixed Code base: `71b91a2ffc41736ae95069889013bef3887b37e0`.
- Code head: `fcb65417c26e4b61776f16bab725516df55d1317`.
- Implementation commits: `fcb65417c26e4b61776f16bab725516df55d1317`
  (`X-Stage: work`). The separately committed log points backward to this code.
- Verified implementation tree: `737b5ca45d13da7260961f3dff58fd5aa5b59184`;
  staged before the check completed and identical after the code commit. No hook
  edits or generated tracked changes occurred.
- Author: `compat-store-work`; native conversation
  `ses_e4187f711ffeBZjK6i4CmNJItF`; OpenCode / `openai/gpt-6.1-sol` / `xhigh`
  (parent creation metadata). Own Hypercube numeric ID is unavailable: current
  context identifies the worker name, while `list_sessions` exposes only parent.
- Work review (repository-relative, not yet written):
  `.agents/reviews/work/2026-11-08-003-remove-backwards-compatibility.md`.

## Accepted guidance

All paths are repository-relative. These exact inputs matched accepted Round 2
before editing and again before committing; peer units are boundaries, not work
implemented here.

| Guidance path | Blob |
| --- | --- |
| `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md` | `2ddcbac1570b7ef08da4015f76cb05ac06cfd42a` |
| `.agents/plans/2026-11-08-004-remove-browser-compatibility.md` | `781e49e77ac7d71b9ab3217e70fdefad60c2acf3` |
| `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md` | `cbb2b2eaf2c1cbba7b7d0dfa0fc60e79d400bf89` |
| `.agents/plans/2026-11-08-006-remove-install-compatibility.md` | `f6bf8eca928d621d7eb122d1f19a7bd86053896e` |

Read repository instructions, development and documentation routing, xwork and
the shared xauto workflow. The user's selected xauto commit/review sequence
supersedes the older k-stage sequence in the development reference.

## Changes

- `internal/store/store.go`: initialize current tables and both partial search
  indexes in one existing `DB.Tx`. Preserve database settings, error cleanup,
  directory privacy, foreign keys, `last_active` and its required initial value.
- Delete `internal/store/migrate.go`, `migrate_agents_test.go`,
  `migrate_upgrade_test.go`, `migrate_test.go`, and
  `testdata/origin-schema.sql`. The removed origin fixture had no other caller.
- `internal/store/testdata/current-schema.sql` and `reopen_test.go`: independent
  installed-baseline fixture and public-API regression preserving two workspaces,
  root/child session fields, conversation/fork/launch values, attention, queued
  message/report flag, exact saved command and transcript bytes/sequences, model
  configuration, and insertion after reopen. The fixture's existing version-4
  marker is inert; production does not inspect or assign a version.
- `internal/store/search_test.go`: check fresh partial-index creation and current
  search after reopen, without dropping indexes or rewinding a schema version.
- `internal/config/config.go`, `config_test.go`: remove profile/notification
  conversion; retain normal defaults, validation and atomic saving. Cover current
  selection/guidance, explicit false notifications, partial documents, unknown
  disk fields and malformed JSON without rewriting file bytes.
- `cmd/hypercube/backend.go`, `integration_test.go`: remove startup conversion and
  use/assert current selection and explicit child guidance. Preserve bridge and
  process/session/holder lifecycle coverage for the later protocol unit.
- `agents/development.md`, `docs/upgrades.md`: correct only retired saved-state
  conversion promises; retain future state-preservation/migration responsibility,
  normal replacement and restart instructions.

No consequential deviations, replacement migration/version/recovery machinery,
or unrelated changes. No actual user-state inspection/modification, reset,
installation, app/service restart, push, merge, new checkout or delegation.

## Verification and evidence

All checks below were **run** in this checkout on macOS/arm64, Go
`go1.27.3 darwin/arm64`, Bun `1.4.2`, against the unchanged implementation tree
committed as Code head above. No earlier build pass was reused.

| Command / inspection | Result |
| --- | --- |
| `make check` | Passed browser/assets and Hugo website builds, program builds, TypeScript, 400 browser tests, 28 desktop tests, `go vet ./...`, gofmt gate and `go test -race ./...`, including store/config and app-process integration. |
| `git diff --check` and `git diff --cached --check` | Passed; full changes and new fixture/test inspected. Index contained exactly the 15 task-owned implementation/test/live-doc paths. |
| `git hash-object --` for the plan and four guidance paths above | Matched accepted Round 2 inputs before editing and before commit. |
| `git rev-parse HEAD 'HEAD^{tree}'` after commit | Confirmed full Code head and unchanged verified tree above; clean checkout. |
| Static fixture comparison below | Passed: fixture table/index definitions match the starting revision, independently of the new runtime schema. |

Full `make check` command/output evidence: native conversation
`ses_e4187f711ffeBZjK6i4CmNJItF`, shell
`sh_1be7c8e39001SDefFX0swjQYwC`; local output:
`/Users/kyle/.local/share/opencode/shell/803f5fc9179a0cdaf3cc1e7bbcca471e3cd3ceb8/sh_1be7c8e39001SDefFX0swjQYwC.out`.
The completed-shell notification records successful results. React `act(...)`
warnings and Hugo's duplicate `-lc++` linker warning were non-fatal. No focused
or repeated broad suites were needed. Packaging, live smoke testing and Linux
container runs are not required for this unit and were not run.

Structural inspection with the grep tool found no migration, legacy,
`user_version`, schema-column probes or ALTER/DROP in production store Go, and no
removed config converter or origin-fixture/helper references in Go. The sole
remaining ALTER/DROP-pattern match in store Go is the existing limits test's
ordinary missing-table error case. Its current error coverage stays. Inspected
the changed live-doc links; no links or procedure anchors changed.

Exact additional fixture-verification command (output is in the same native
conversation):

```sh
python3 - <<'PY'
import pathlib, re, subprocess
base = '71b91a2ffc41736ae95069889013bef3887b37e0'
def source(path):
    return subprocess.check_output(['git', 'show', f'{base}:{path}'], text=True)
store = source('internal/store/store.go')
migration = source('internal/store/migrate.go')
schema = re.search(r'const schema = `(.+?)`', store, re.S).group(1)
indexes = re.search(r'(CREATE INDEX IF NOT EXISTS transcript_root_text_search.+?send_message\';)', migration, re.S).group(1)
fixture = pathlib.Path('internal/store/testdata/current-schema.sql').read_text()
def statements(sql):
    sql = re.sub(r'--[^\n]*', '', sql)
    return sorted(' '.join(part.split()) for part in sql.split(';') if part.strip() and 'PRAGMA user_version' not in part)
assert statements(schema + indexes) == statements(fixture), 'Fixture differs from the installed current schema/indexes'
print(f'PASS: static fixture matches tables and indexes from {base}; inert user_version=4 is fixture-only')
PY
```

## Session provenance and handoff

All stages use OpenCode / `openai/gpt-6.1-sol` / `xhigh`. Parent supplied prior
stage metadata; own native ID came only from `OPENCODE_SESSION_ID`.
`list_sessions` confirmed the parent's model/effort and exposed no own numeric ID.

| Stage / name | Hypercube ID | Native conversation |
| --- | --- | --- |
| Coordinator | `a58ee257` | `ses_e41aa9914ffeXSrMrW4OhBDxsQ` |
| Planner / `compat-plan` | `d36e9339` | `ses_e41a1172bffeqvkaFBnCzmGxvc` |
| Plan review / `compat-plan-review` | unavailable; interface limitation | `ses_e4195b6fcffe3Mncn7jH9SS1YY` |
| Implementation / `compat-store-work` | unavailable; context/list interface exposes no own ID | `ses_e4187f711ffeBZjK6i4CmNJItF` |

No blockers or unresolved implementation decisions. Status remains implemented,
not complete, pending independent `xreview work` of this fixed-base Code-head
range. Units 004–006 and their verification remain the parent's separate work.
