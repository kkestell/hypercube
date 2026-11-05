# Profile settings review

## Scope and coverage

Reviewed the modified and new config, HTTP, settings, profile-selector, and UI component source files; their changed tests and fixtures; configuration documentation; and the session creation, selection, startup, and MCP callers. Used `agents/plans/2026-11-05-001-profile-settings.md` as the behavior contract. The scratch wireframe is context only.

Gaps:

- No visual review or screenshots, as required by AGENTS.md. The parent owns `make check` and the isolated live smoke test.
- Generated browser bundles were not reviewed as independent source.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Correctness | Fixed | Saving a removed profile leaves an open draft pointing at it |

The independent review reported findings only. The user subsequently approved fixing F1; the implementing session applied the fix below. Nothing was staged or committed.

## F1. Saving a removed profile leaves an open draft pointing at it

Medium · Correctness · Fixed · `internal/ui/web/app/settings-dialog.tsx:47`

Open a new session draft, then remove its selected profile in Settings and save. Saving only writes the configuration and closes the modal. The draft retains the removed profile name and its old selector label. Sending the draft submits that name, so the server refuses session creation with `no profile named "…"`. Renaming the selected profile has the same consequence because its old name disappears.

The plan requires an already open draft whose selected profile was removed to use the saved default. `ProfileSelect` only reconciles its choice when its config-fetch effect runs; successful settings saving changes none of that effect's dependencies. Opening the profile menu subsequently runs the effect and repairs the choice, but sending directly after saving fails.

### Evidence

A temporary mounted React behavior test loaded a draft with `lead` selected, deleted `lead` through the real Settings controls, and saved `worker` as default. Assertions confirmed that the saved config default was `worker`, `store.get().unsent.profile` was still `lead`, and `createSession` submitted `profile=lead`. The test passed all four assertions and was removed afterward. Server rejection follows directly from `internal/session/sessions.go:54`, which resolves the submitted name against the newly loaded config.

### Fix

After a successful save, publish the accepted config to the browser store and reconcile its unsent draft: preserve a choice that still exists, otherwise select the saved default. Profile selectors read the shared profiles so their labels update immediately as well. Existing sessions retain their stored selections. Regression tests cover removal, rename, and editing a retained profile, then sending directly without opening the menu.

## Checks run

- `bun test review-settings-repro.test.tsx` in `internal/ui`: one temporary reproduction passed, four assertions. React emitted `act` warnings from the temporary test; the assertions confirmed the stale stored and submitted profile independently of rendering. Removed the temporary file immediately afterward.
- Traced config validation and atomic replacement, HTTP decoding, draft sending, selector refresh, and stored session selection behavior.
- After the approved fix: the focused settings, profile-selector, and composer suite passes, including the save-to-send regression cases.
- `make check` passes after the fix, including the complete UI suite, generated asset check, Go vet, and race-enabled Go tests.

## Verdict

F1 is fixed with user approval. No other verified defects or blocking readability or scope problems were found in the independent review.
