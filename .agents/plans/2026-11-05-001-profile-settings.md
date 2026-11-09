# Editable profile settings

## Goal

Use the settings wireframe's header, section navigation, content area, and save footer for real configuration editing. Users can select a default agent profile and create, edit, and delete profiles. Preserve saved tool permissions and build information in their own sections; leave proposed appearance and notification preferences out.

## Decisions

- The least costly complete implementation keeps `harness.LoadConfig` as the existing read path, adds validated file saving, and serves the complete config through GET/POST `/config`. Replace the browser's profiles-only request with that config request; no server cache, watcher, new service, or database state is needed. The browser store shares the last fetched or saved config with composers so a save updates their choices immediately.
- Add `default_profile` naming a configured profile. An omitted value in an existing file resolves to its first profile, preserving current behavior. Saving persists an explicit default. At least one complete profile with a unique name must remain, and the default must exist.
- Write a temporary file beside the config and rename it into place so existing concurrent readers see a complete document. Direct file edits while running are unsupported; do not add external-edit conflict detection or merge behavior. Browser saves replace the configuration; the last successful save wins.
- The modal owns one draft config. Save validates and writes the complete draft; failed saves keep it open. Closing discards unsaved edits. Renaming the default profile updates its reference; deleting it selects the first remaining profile. Removing the final profile is disabled.
- Saved settings apply to subsequent profile choices and newly opened session drafts. Existing sessions retain their stored harness, model, and effort, including after removal of the original profile. An already open session draft retains its choice unless that profile was removed; it then uses the saved default. Reopening a profile menu reads saved settings rather than keeping an outdated list.
- Keep model and effort as text fields because their accepted values belong to each harness. Use stock shadcn controls and layout utilities; do not turn wireframe styles into a new component theme.
- This fits existing config and HTTP boundaries, so no architectural review is needed. Obtain an independent code review after implementation. Do not stage, commit, merge, or install.

## Implementation plan

1. Extend `internal/harness/config.go` with the optional default, shared validation, and saving. Add config tests for legacy loading, defaults, complete profile validation, save/reload, and refused writes leaving the original intact.
2. Replace GET `/profiles` with GET/POST `/config` in `internal/ui/ui.go` and a small config handler file. Test persisted edits, rejected input, and saved selections being used by session operations without changing stored selections.
3. Add the frontend config type and API calls. Update the composer selector to use the saved default and refresh its choices when opened, preserving deliberate choices and current-session labels.
4. Rebuild `settings-dialog.tsx` around section navigation and a shared save footer. Put the profile list/editor and existing permission/build content in small components with one concern each. Test create/edit/delete, default reference changes, cancel/reopen, validation, failed saves, and default selection in a new session draft.
5. Update `docs/configuration.md` to explain modal saves, the optional default, and unsupported direct editing while running. Leave the example's valid legacy config unchanged. Regenerate the browser bundle.

## Test plan

- Go tests exercise public config loading/saving and HTTP routes, including persistence across subsequent reads, invalid defaults/duplicates/missing fields, invalid JSON, and write failure.
- Browser behavior tests cover the actual form controls, save payloads and failures, profile editing, keeping existing sessions intact, and new drafts selecting the configured default.
- Run `make check`. Run an isolated `make dev` smoke test and open its URL for the user's visual assessment; do not inspect screenshots or renderings. Stop the dev process started for this work after the smoke test.
