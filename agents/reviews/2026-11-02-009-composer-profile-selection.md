# Composer profile selection

## Scope and coverage

Reviewed request 1, commit `072adc8` against `bfc510e`, in the original checkout. Read the whole changed source and test files, deleted sidebar picker, associated documentation, and the session selection, creation, draft/image, editor, store, and API callers. Inspected the installed `@vscode-elements/elements` 2.5.1 single-select, option, option controller, and styles to verify their actual API and lifecycle. The implementation uses the supported option `value`/`selected` properties and native `change` event.

The baseline is to open the existing unsent session immediately and move profile choice into its composer, retaining the existing creation and draft paths. This change follows that baseline without introducing another creation path. Profile changes preserve drafts and image state through the existing immutable update. Successful creation returns through the existing selected-session path and renders the static configuration label.

Gaps:

- No browser rendering, screenshots, or visual validation, per instruction. F1 is established from component positioning code and the existing bottom-composer layout.
- Generated bundles were not reviewed line by line or rebuilt. The implementation author's reported `make check` pass was not independently repeated; this review ran focused checks.
- Excluded unrelated parent changes for requests 2/3/4, the child guide, and `mux/`.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | High | Correctness | Fixed | Profile options open below the window |
| F2 | Medium | Concurrency | Trivial, unapplied | Replacement sessions retain selectable stale profiles |

F1 was fixed after the user explicitly requested upward opening. F2 remains unapplied under the review-only override. Nothing was staged.

## F1. Profile options open below the window

High · Correctness · Fixed · `internal/ui/web/agent/profile-select.jsx:30`

The reviewed select omitted `position="above"`. The installed component defaults to `position = 'below'` and positions its popover at the bottom edge of the select, with no viewport adjustment. With the terminal panel hidden, the composer sits at the bottom of the window. The dropdown therefore extended below the viewport, leaving later profiles unavailable to pointer selection in an ordinary layout.

### Evidence

`vscode-select-base.js:120` sets the default position. Its `_renderDropdown` at lines 569–613 uses `top = host.top + host.height` for that position and a scroll region of `min(optionCount * 22, 220)` pixels, followed by the active option's description. The component stylesheet supplies no automatic flip or viewport constraint. `app.jsx` allows the lower terminal panel to collapse, and the agent pane places the composer beneath its flexible transcript. The composer's lower padding and margin do not provide room for the options list.

The unit tests treat VS Code controls as plain elements, so the passing composer tests cannot detect this positioning behavior.

### Fix

Set `position="above"` on the composer select, using the component's supported placement API, after the user explicitly requested that the select open up. Tests, build, and generated outputs are owned by the parent session.

## F2. Replacement sessions retain selectable stale profiles

Medium · Concurrency · Trivial, unapplied · `internal/ui/web/agent/profile-select.jsx:14`

Opening another unsent session while one is already open keeps the same `ProfileSelect` instance. Its effect refetches profiles for the new identity but retains the previous session's selectable `profiles` until the response arrives. If configuration has removed a profile, the user can select that old profile during the fetch. Line 19 then keeps the selected name because it is truthy, even when the fresh options no longer contain it. Sending submits the removed profile and creation fails, while the dropdown lists only the current profiles.

This regresses the existing behavior of reading current configuration for each new session choice. The server deliberately rereads profiles, and configuration changes are documented to affect subsequent creations.

### Evidence

A focused headless reproduction mounted `Composer`, loaded `[claude, codex]`, called `openUnsent` again, selected `codex` before the second fetch completed, and resolved that fetch with `[claude]`. It printed:

```text
pending replacement options: [ "claude", "codex" ]
after fresh profiles: { profile: "codex", options: [ "claude" ] }
```

`unsentReady` accepts any truthy profile name. `createSession` sends that name unchanged, and `Runtime.Create` calls `cfg.Profile(profileName)`, which rejects the removed profile. The existing replaced-session test covers late responses from an old fetch, but does not cover already-loaded choices retained across replacement.

### Recommended fix

Give the select's local choices the same lifetime as the unsent identity, such as remounting it for each identity, so old profiles are unavailable while fresh choices load. No change applied.

## Checks run

- `bun test web/agent/composer.test.jsx web/sessions/selection.test.js web/sessions/session-actions.test.js web/sessions/session-view.test.jsx web/sidebar/sidebar.test.jsx` in `internal/ui`: 51 passed, 0 failed, 163 assertions.
- Temporary in-memory composer reproduction using the existing test DOM and mounting helpers: confirmed F2 without editing implementation or test files.
- Static inspection of the installed select API and CSS: confirmed F1 and verified `value`, `selected`, `description`, and `change` integration. Descriptions use one active-option area; no per-row description requirement was imposed.

## Verdict

F1 is fixed. F2 remains unapplied pending user authorization. The profile-select source change and this review report remain unstaged; the parent session owns validation and generated outputs.
