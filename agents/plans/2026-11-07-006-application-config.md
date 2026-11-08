# Give application configuration its own package

## Goal

Move `config.json` preferences, validation, defaults, atomic replacement, and
upgrades from `internal/harness` to `internal/config`. Backend startup,
notifications, session guidance, and browser settings use the configuration
package directly. Preserve the file format and all existing behavior.

This is the first implementation plan in the
[runtime cleanup sequence](2026-11-07-005-runtime-boundaries.md). It has no
implementation prerequisite. The
[architecture review](../reviews/2026-11-07-007-runtime-boundaries.md#f4-shared-protocol-data-and-backend-operations-have-the-same-owner)
explains the ownership problem.

## Related code

- `internal/harness/config.go` and `config_test.go` own the complete current
  document, notification defaults, XDG path, validation, writing, and upgrades.
- `cmd/hypercube/main.go` upgrades before opening the backend and reads
  notification preferences; `integration_test.go` checks the upgraded result.
- `internal/session/launch.go` reads child-model guidance;
  `selection_test.go` saves preferences used by that behavior.
- `internal/ui/config.go`, `config_test.go`, and `ui_test.go` expose and test
  browser configuration reads and writes.

## Decisions

Application preferences include theme, terminal font size, notifications, and
worktree defaults as well as the default harness selection. They belong in
`config`; harness selection itself remains `harness.Settings`. Import that
shared type instead of copying it or adding conversion functions.

Use `config.Config`, `config.Notifications`, `config.DefaultNotifications`, and
the package functions `config.Load`, `config.Save`, and `config.Upgrade`.
Move existing private path and upgrade functions with the implementation.
Delete the old declarations; do not leave aliases or forwarding functions in
`harness`.

This task changes source ownership only. Keep JSON names, default values,
validation, error context, config location, upgrade order, no-change upgrade
behavior, and atomic document replacement identical. No migration is added.
Do not change protocol fields, helper commands, discovery, process launch,
HTTP routes, or notification delivery lifetime.

The new package still imports `harness` for `Settings`; that package remains
mixed until the next plan. Do not claim complete transitive isolation yet or
fold the process-client extraction into this task to obtain it sooner.

## Implementation plan

1. Move configuration implementation and its tests into `internal/config`.
   Qualify `Settings` and the embedded legacy-profile settings with `harness`.
   Update test fixtures to use the same shared type. Retain private helpers
   and tests at their new owner rather than exposing them for old callers.
2. Update startup upgrade, notification preferences, session guidance, and
   browser configuration callers. Update integration and fixture type
   references. Search all Go sources for the old configuration symbols to
   find callers beyond the inventory above.
3. Remove `internal/harness/config.go` and its old test file. Inspect whole
   touched files for obsolete imports, comments, and unrelated responsibilities
   exposed by the move. Keep the settings and notification behavior readable
   without introducing wrappers in session or UI.
4. Update the package list in `agents/development.md` and current architecture
   references that actually point to configuration's former owner. Read
   `agents/documentation.md` before changing current guides. No user-facing
   preference documentation or screenshots need changing because behavior and
   the persisted document are unchanged.

## Acceptance and verification

- Existing config tests run at the new owner and retain coverage for defaults,
  missing and malformed files, value validation, complete writes, legacy
  profiles, notification renaming, and preserving unchanged upgrade files.
  Move that coverage; do not duplicate it in `harness`.
- Existing browser configuration and session-guidance tests continue to
  observe the same settings. The backend integration still upgrades an old
  document before creating its first session.
- `internal/harness` contains no application preference document, path lookup,
  config writes, or config upgrades. No old API aliases remain.
- Use focused tests for `internal/config`, the affected session/UI behavior,
  and the backend integration while working. Run `make check` before this
  task's code commit, as required by `agents/development.md`. No bundle,
  installation, Linux container, native harness, or screenshot smoke test is
  needed for an unchanged file format and source-package move.
- Inspect the diff and current links. Record check results or omissions in the
  work log and stage this task's code, tests, documentation, and work log for
  the required fresh code review. Stop when the configuration owner and all
  callers are coherent; the protocol boundary is a separate plan.

Next plan: [Harness protocol boundary](2026-11-07-007-harness-protocol-boundary.md).
