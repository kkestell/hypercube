# Start Hypercube at login

## Plan

`agents/plans/2026-10-31-005-start-at-login.md`

## Summary

Implemented the menu bar app and login restore, with restored sessions holding
their queued messages until the user prompts that session. Included the supplied
base patch, removed the old reinstall path, and updated the installation and
usage guides. Live behavior and the lifecycle test's new restore assertions
remain unverified here.

## Departures from the plan

- The lead requested a branch commit, overriding the skill's staging-only
  default, and authorized committing with the lifecycle test blocked in this
  environment. No implementation decisions changed.

## Checks run

- The first focused Go test command could not write the default build cache.
  Subsequent commands used `GOCACHE=/tmp/hypercube-005-go-cache`.
- `GOCACHE=/tmp/hypercube-005-go-cache go test ./internal/wait ./internal/report
  ./cmd/hypercube -run
  'TestAwaitingUserKeepsMessagesQueued|TestWorkerPromptReleasesOnlyItsSession|TestParseUIArgs'`
  — passed, including compilation and linking of the Cocoa implementation.
- `sh -n scripts/install-service` — passed.
- `git diff --check` — passed.
- `GOCACHE=/tmp/hypercube-005-go-cache make check` — build, vet, formatting,
  and all test packages except `cmd/hypercube` passed. Its
  `TestTmuxNativeLifecycle` failed at the existing third-window creation step
  (`integration_test.go:338`): `tmux respawn-pane ... fork failed: Device not
  configured`, before reaching the new restore assertions. The same failure
  occurred in focused runs, with elevated execution, and after the user enabled
  full access and requested a retry.

## Manual verification

Not run, as assigned: installing/restarting the service, menu appearance and
actions, retaining agent processes across upgrades and Quit, reboot restore,
and the login tmux environment's color settings. The user runs
`make install-service` from the original checkout and follows the plan's live
checklist. No live workspace or service was stopped or installed.

## Follow-up work

- The lead will run `TestTmuxNativeLifecycle` and `make check` independently.
- The user will perform the plan's live checks.
