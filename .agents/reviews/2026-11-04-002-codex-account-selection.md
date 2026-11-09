# Codex account selection

## Scope and coverage

Reviewed the unstaged changes to `plugins/codex/storage.go`, `storage_test.go`, `main_test.go` and `docs/harness-support.md` (`git diff HEAD`) against `agents/plans/2026-11-04-002-codex-account-selection.md` and the call path in `plugins/codex/main.go`. Traced the default, selected-account, explicit-override, private-resume and native-import paths through `prepareStorage`.

Gaps:

- The Activation section of the plan (plugin-only replacement) is reporting-only and was not reviewed.
- No live agent, installed app, or Neuron profile was exercised.

## Summary

No findings.

## Checks run

- `go test -race -count=1 ./plugins/codex` passed.
- `go vet ./plugins/codex` was clean.

## Verdict

The change follows the plan. Without Neuron the default stays `~/.codex` in the legacy home. An explicit nonempty `CODEX_HOME` wins. Each source gets its own home, and a changed selection never repoints links in an existing home. Private resumes find their original home before the selection is read, so an invalid selection file cannot break them. Native imports use only the selected account's history. The tests cover each of these behaviors.
