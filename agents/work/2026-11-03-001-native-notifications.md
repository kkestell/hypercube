# Native macOS notifications

## Plan

`agents/plans/2026-11-03-002-native-notifications.md`

## Summary

The app process posts a macOS notification for each new attention request and each finished turn's reply unless the window is key in the active app. Clicking one shows the window and selects the session. The app now builds and runs as a signed `Hypercube.app` (`Hypercube Dev` in `make dev`), which macOS requires. The plan's goal is met; the click's session selection in the real window and a request alert in the live app were not exercised (see Manual verification).

## Departures from the plan

- `startNativeNotifications` lives in the existing `internal/ui/web/notifications/notifications.js`, and its test in `notifications.test.js`, instead of a new file.
- `Makefile` has a `bundle` target (assemble and sign `$(APP)`) that `build` extends with the `bin/` symlinks; `scripts/dev` calls `bundle` so the dev build leaves `bin/hypercube` alone.
- The plan's Gate sentence was corrected: only a bundle started with `open` had got past the refusal, as a pending authorization, and no delivery or click had been seen.

## Decisions

- `WatchAlerts` keeps its state in a small `alertWatch` so tests can drive `read` directly for the two cases that must not depend on timing (seen reply, pre-existing state).
- `HCNotificationsStart` returns early when the main bundle has no identifier, and `hypercubeNotify` drops alerts when no notifier exists, so `go run` and tests show nothing.
- Existing installs: `make install` replaces the old regular files in `~/.local/bin` with symlinks (`ln -sf`). Not run.

## Checks run

- `make check` — passed (build, signing, stale-dist check, bun tests, vet, gofmt, `go test -race ./...`).
- `go test -race -count=5 -run Alert ./internal/session` — passed.

## Manual verification

1. Platform gate with a throwaway Go+cgo probe (deleted). A bare executable with an embedded Info.plist, with or without an ad-hoc signature identifier, is refused: `requestAuthorization not allowed` or `bundleProxyForCurrentProcess is nil`. A signed `.app` outside `/tmp` registered with `lsregister -f` and started through its inner executable from a shell got the permission prompt; after the user allowed it, `requestAuthorization` returned granted, `addNotificationRequest` returned no error, `getDeliveredNotifications` returned 1, and the user's click reached `didReceiveNotificationResponse`.
2. Permission persisted after rebuilding and re-signing the bundle at the same path: authorization status 2, alert style banner, no new prompt.
3. Real code in `scripts/dev up`: a haiku session created through the dev app's HTTP API produced `reply:<session>:4`, and usernoted logged it delivered "as banner" for `com.github.kkestell.hypercube.dev`.

   ```sh
   /usr/bin/log show --last 2m --predicate 'process == "usernoted" AND eventMessage CONTAINS "hypercube.dev"' --style compact
   ```

   Not exercised live: a request alert, the window-focused suppression, and a click selecting the session in the real window. The first two are one-line checks in `hypercubeNotify`; the click path is the same delegate verified in step 1 plus `hypercube-notification`, covered by the JS test.

## Follow-up work

- The first `make install-service` shows the permission prompt for `Hypercube`, and replaces `~/.local/bin/hypercube*` with symlinks into `~/Applications/Hypercube.app`.
