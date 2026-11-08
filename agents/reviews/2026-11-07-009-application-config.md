# Application configuration package

## Scope and coverage

Reviewed the staged implementation of
[the plan](../plans/2026-11-07-006-application-config.md) and its
[work log](../work/2026-11-07-006-application-config.md), including the complete
touched Go files, the development reference, and relevant callers. Compared
the moved implementation and tests with their originals, and traced
`harness.Settings.Check`, startup upgrades, notification preferences, launch
guidance, and the configuration HTTP routes.

The move preserves the configuration path, JSON fields, defaults, validation,
error handling, atomic replacement, and upgrade order. `defaults()` returns
the same value previously constructed in both readers. Qualifying the
embedded `harness.Settings` preserves the legacy profile fields. The
`getConfig` rename preserves the route and handler behavior. The complete
touched files remain straightforward to follow.

There are no material coverage gaps for this extraction. The work log and
parent report a successful `go build ./...` and `make check`, including
`TestAppProcess` and the full Go race suite. This review independently ran
the focused checks below. Linux packaging and live installation were not
repeated; neither changes in this task. Later runtime boundary work is outside
this review.

## Summary

No findings. No code fixes were needed.

## Checks run

- `go test -race ./internal/config ./internal/ui ./internal/session -run
  '^(TestSave.*|TestLoadDefaults|TestUpgrade.*|TestConfig|TestInstructionsIncludeGuidance)$'
  -count=1` — passed in all three packages. Covers the transferred configuration
  tests, GET/POST behavior and failures, and guidance reaching a launch.
- `rg -n
  'harness\.(Config|LoadConfig|SaveConfig|UpgradeConfig|Notifications|DefaultNotifications)'
  --glob '*.go' .` — no matches.
- `go list -deps ./internal/harness` — succeeds, with no dependency on
  `internal/config`; the existing `hold` dependency remains as planned.
- Inspected all current configuration API callers — all use the new package;
  startup still upgrades before opening state and starting the runtime.
- `git diff --cached --check` — passed.

## Verdict

Pass. The staged change meets the plan and preserves existing behavior.
Nothing remains unresolved. Only this review was added and staged by the
reviewer; no commit was made.
