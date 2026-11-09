# Use the current database and configuration directly

- Task ID: `2026-11-08-003-remove-backwards-compatibility`
- Status: ready; awaiting independent plan review
- Source HEAD: `ae022eeebf9dc90c75a61527267db0682b194057`
- Author: `compat-plan`, Hypercube `d36e9339`, native
  `ses_e41a1172bffeqvkaFBnCzmGxvc`; OpenCode / `openai/gpt-6.1-sol` / `xhigh`
- Artifact path (repository-relative): `.agents/plans/2026-11-08-003-remove-backwards-compatibility.md`
- Shared guidance (repository-relative): `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md`,
  linked [here](../designs/2026-11-08-003-remove-backwards-compatibility.md).
- Technical prerequisites: none. First in the parent's serial four-unit cycle.
- Request: retire all historical migration/compatibility machinery now that both
  installations run the latest version, while preserving current saved state.

## Outcome and boundaries

Database opening initializes the current schema without inspecting or upgrading
historical layouts or rewriting saved rows. Backend startup does not convert
config files. Fresh installations and current databases/configurations remain
usable, including adding sessions after reopening. No actual user state is
modified during the task; no reset, installation, restart, push, or merge.

This unit owns store/config/startup and their tests/documentation, not browser
conversion, protocol aliases, or installation cleanup. Follow the shared scope
map's preservation decisions. Do not create a new schema-version policy or an
old-format rejection/recovery subsystem.

## Current behavior and chosen baseline

`internal/store/store.go:Open` secures the state directory, opens SQLite with
the existing busy timeout/WAL/foreign-key/immediate-transaction settings, then
calls `migrate`. `internal/store/migrate.go` combines current table creation with
old tables/columns, data conversions, `user_version` steps and attention repair.
The two transcript search indexes exist only in its version-3 step, not `schema`.

Delete `migrate.go` and execute the current `schema` in one `DB.Tx` from Open.
Move both partial-expression search index definitions into that schema. Retain
`IF NOT EXISTS` for all tables/indexes: it makes first-run initialization and
ordinary reopen the same operation. Preserve existing error wrapping and closing
the database on initialization failure. Do not query schema versions/columns or
ALTER/DROP/UPDATE/DELETE data during Open. Leave existing user_version values
alone; a fresh database needs no version assignment.

The current sessions table itself requires `last_active INTEGER NOT NULL` without
a default. Retain that inert column and `AddSession`'s initial `created` value;
otherwise inserts into today's installed databases fail. Keep one identical
baseline for new and existing databases, with a short factual schema comment
explaining its required inert field rather than an old-schema branch. Removing
the constraint from actual state would require an unauthorized replacement
migration/reset. No active last-activity feature is added.

Keep `MkdirAll`/`Chmod`: state-directory privacy is an active security invariant
on every open, not an old-format conversion. Keep normal foreign keys, indexes,
transactions, queued commands and transcript APIs unchanged. Runtime lifecycle
owners already clear attention when launches end or families archive; removing
the store's historical attention sweep must not introduce another sweep.

`internal/config/config.go:Upgrade` is the only legacy profile/notification
converter, invoked by `cmd/hypercube/backend.go:startApp`. Remove it, its helper
types/functions and that invocation. Keep Load/Save/Validate, atomic write,
unknown-field tolerance on disk, and defaults for missing files or hand-written
partial config; `{}` is a current supported configuration, not necessarily an
old version. Rewrite only comments that describe removed upgrading.

## Execution sequence

1. Delete the store migration implementation and migration-only tests:
   `migrate_agents_test.go`, `migrate_upgrade_test.go`, `migrate_test.go`.
   Their old-schema builders/legacy conversion helpers have no current callers.
   Initialize the current schema/indexes as above and keep required current
   constraints. Do not preserve obsolete migration helpers for test convenience.
2. Replace `TestSearchIndexesMigrateSavedEntries` in `search_test.go` with a
   current-data reopen/search regression: remove index dropping and version
   rewinding. Keep normal search behavior and partial-index coverage.
3. Add a small static current-schema SQL fixture under `internal/store/testdata/`
   based on this source HEAD's schema plus its two search indexes. It represents
   the baseline already installed, particularly the no-default last_active
   constraint; it must not be generated from the newly edited schema at test
   runtime. A store test loads it into isolated state before calling public Open,
   then uses public transaction APIs to populate/reopen/read/write current data.
   This prevents a fresh-only test from missing installed-schema insert failures.
4. Delete config conversion implementation and `TestUpgrade` /
   `TestUpgradeNotifications`, retaining current config/default/write-failure tests.
   Add a current-config Load test proving values and file bytes remain unchanged.
   Remove startup conversion in `backend.go`.
5. In `cmd/hypercube/integration_test.go:newWorld`, replace the legacy profiles
   fixture with current `default_selection` and explicit child guidance, retaining
   its test harness/model choices. Assert those current values at startup instead
   of generated profile guidance. Preserve all app/process/lifecycle checks and
   the bridge checks until unit 005 edits them.
6. After reading `agents/documentation.md`, update `agents/development.md` and
   `docs/upgrades.md` only where they promise the retired conversion chain.
   Explain this retirement's current baseline and preserve the responsibility to
   protect actual saved state in future changes; do not impose a permanent ban
   on migrations. Keep app replacement, holder survival and restart instructions.

## Verification

The current-schema fixture test should preserve workspaces, a root/child session,
conversation/fork/launch fields, a queued message, a recorded current command,
transcript JSON/sequence, and configured model entries through close/Open. Compare
saved values, not SQLite file bytes (WAL is legitimate). Add another session
after reopen to exercise last_active's constraint. Existing tests cover mutable
session fields, grants, limits, cascading deletion and state-directory privacy;
reuse them instead of duplicating their suites in a new fixture framework.

For a fresh database, assert both named search indexes exist and existing search
tests still return current user/text/send_message results. Structural inspection
must show no schema probes, user_version handling, historical table conversion
or startup row repair remains. Do not manufacture invalid state merely to demand
new rejection behavior; the supported baseline and normal error paths suffice.

Config tests retain missing-file and `{}` defaults, explicit false notification
choices, current selection/guidance, and malformed JSON failure without file
rewrite. The app integration fixture still exercises current startup settings.

Run `make check` before the unit's code commit; it builds the browser/assets and
programs, typechecks, and runs all Go race/Bun checks. Focused Go tests during
iteration are optional when answering a concrete issue, not additional mandatory
full-suite passes. Inspect Go formatting and the full diff, including live doc
links. No full packaging build, real-state probe, install, or live smoke test is
required. Record exact checks/results and anchored code in the unit's work log.

## Handoff and normalization

Ready; no open engineering decisions. This is one bounded persistence-baseline
retirement across initialization, configuration startup and meaningful reopen
coverage. It is larger than a diagnostic-and-test brief but reuses existing
storage rather than designing migration/recovery machinery. Browser/protocol and
platform integration are deliberately separate plans. Decision-level detail
settles the installed-schema trap without transcribing SQL or function bodies.
Use the experimental 600–1,500-word band; no curated reference manifest was
provided. Planner writes records only; parent owns commits and independent
review before execution.
