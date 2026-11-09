# Linux keys and labels review

## Scope and coverage

The staged changes for
`agents/plans/2026-11-06-018-linux-keys-and-labels.md`, judged as whole files:
`internal/ui/web/app/keys.ts`, `shortcuts.ts`, `shortcuts-dialog.tsx`,
`app.tsx`, `agent/composer.tsx`, `files/quick-open.tsx`,
`terminal/xterm.ts`, `terminal/terminal-panel.tsx`, `desktop/accelerator.js`,
`internal/harness/config.go`, `cmd/hypercube/main.go`, the notification
rename in the UI, their tests, `scripts/linux/test-desktop`,
`scripts/linux/Dockerfile`, and the documentation. The work log's departures
were checked against the plan.

- macOS: every binding in the table was compared with `HEAD`'s parser. A
  throwaway test asserted that `binding()` equals the old parser's result,
  `keysLabel()` equals the notation, `keysOf()` returns `keys`, and
  `firstKeys()` returns `keys[0]` for all 23 shortcuts. The table's `keys`
  arrays are unchanged; only `linux` arrays were added. The menu message is
  `keysOf(s).map(binding)`, so the macOS accelerators are those of `HEAD`.
- The config migration ran against a copy of the user's real
  `~/.config/hypercube/config.json` (in a temporary `XDG_CONFIG_HOME`). The
  only change was `"macos": true` → `"desktop": true`; field order and every
  other value were kept. Unit tests cover the old key alone, both keys, the new
  key alone, no notifications, and profiles plus the old key in one write.
- Departures in the work log: the `PageDown`/`PageUp` accelerator names are
  required by the new Linux bindings and tested; quick open's `matches()` no
  longer swallows ⌘⇧P, which nothing binds; the reordered keys step and the
  forced double click are sound. The fixed-pixel menu click is F1.
- Not covered: Wayland, and real GNOME or KDE sessions; the container runs
  openbox on Xvfb. Ctrl+PageDown and F7 are covered by unit tests only, as the
  work log says.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Tests | Fixed | The View menu screenshot clicks fixed pixels and never checks the menu opened |
| F2 | Low | Consistency | Fixed | The menu shows Alt+Ctrl+B while the reference and tooltips show Ctrl+Alt+B |
| F3 | Low | Correctness | Fixed | Ctrl+Shift+C without a selection empties the clipboard |
| F4 | Low | Maintainability | Fixed | The shortcut reference repeats the Linux separator rule outside `keys.ts` |

## F1. The View menu screenshot clicks fixed pixels and never checks the menu opened

Medium · Tests · Fixed · `scripts/linux/test-desktop`

The keys step moved the mouse to the window origin plus `MENU_VIEW_X = 92`,
`MENU_BAR_Y = -7` and clicked. The offsets depend on the font, the window
manager's frame, and the widths of the File and Edit labels. A miss opens
another menu or none, and the step still passes and saves a screenshot, so the
check meant to show the Ctrl labels can silently show nothing.

### Evidence

The step had no assertion after the click. In the container, pressing Alt,
Right, Right, Down with `xdotool` opened View (Electron's menu bar takes focus on
a lone Alt), and the View submenu's `menu-will-show` event fired for both the
keyboard and the click, so the opening can be observed.

### Resolution

The step registers a `menu-will-show` listener on the View submenu, opens the
menu with Alt, Right, Right, Down, and waits for the event before the
screenshot. The pixel constants are gone. The key path depends only on the menu
order that `desktop/menu.js` sets (File, Edit, View), noted in a comment.

## F2. The menu shows Alt+Ctrl+B while the reference and tooltips show Ctrl+Alt+B

Low · Consistency · Fixed · `internal/ui/web/app/keys.ts`

Neither `desktop/accelerator.js` nor GTK decides the menu's order. Electron's
Linux menu bar is drawn by Chromium views, and Chromium's
`Accelerator::GetLongFormModifiers()` (`ui/base/accelerators/accelerator.cc`)
lists Alt, then Ctrl, then Shift, whatever order the accelerator string uses.
`keyNames()` used VS Code's Ctrl, Shift, Alt order, so the two differed for
Toggle Right Panel.

### Evidence

`keys-menu.png` showed `Alt+Ctrl+B`, `keys-shortcuts.png` `Ctrl + Alt + B`, and
the Chromium source gives the order above.

### Resolution

`keyNames()` on Linux orders Alt, Ctrl, Shift, matching the menu. Of the
current Linux bindings only Toggle Right Panel changes, to `Alt+Ctrl+B`; the
unit tests were updated. This departs from the plan's VS Code order in favor
of one order across the app.

## F3. Ctrl+Shift+C without a selection empties the clipboard

Low · Correctness · Fixed · `internal/ui/web/terminal/xterm.ts`

`copyPaste` wrote `term.getSelection()` unconditionally, so Ctrl+Shift+C with
nothing selected replaced the clipboard with an empty string. GNOME Terminal
leaves the clipboard alone.

### Evidence

`getSelection()` returns `''` without a selection, and the handler passed it to
`navigator.clipboard.writeText`.

### Resolution

Copy writes only when `term.hasSelection()`. The key is still consumed.

## F4. The shortcut reference repeats the Linux separator rule outside `keys.ts`

Low · Maintainability · Fixed · `internal/ui/web/app/shortcuts-dialog.tsx`

`keysLabel()` joined parts with `'+'` on Linux, and the reference tested
`linux()` itself to draw the same `+`, a second copy of the platform rule.

### Resolution

`keys.ts` exports `keySeparator()`, used by `keysLabel()` and the reference.
The remaining `linux()` callers choose platform behavior, not notation:
`keysOf()` selects a shortcut's Linux bindings, and `xterm.ts` chooses copy and
paste or the macOS editing keys.

## Checks run

- `bunx tsc -p .` and `bun test` in `internal/ui` — passed, 338 tests.
- `go test ./internal/harness` — passed.
- `bun test accelerator` in `desktop` — passed.
- macOS table comparison against `HEAD` (throwaway test, removed) — passed.
- `UpgradeConfig` on a copy of the real config — only the key renamed.
- `scripts/linux/run` with `test-bundle` and `test-desktop`, with the
  keyboard and click menu paths logged side by side — both opened View.
- `make check` — passed.
- `make linux-test` — passed (`test-bundle`, `test-install`, `test-desktop`).
  `keys-menu.png` shows the View menu opened from the keyboard with Ctrl
  labels and `Alt+Ctrl+B`; `keys-shortcuts.png` shows the Linux keycaps with
  `Alt + Ctrl + B` for Toggle Right Panel. No containers remain; the
  `hypercube-linux` image and cache volume were kept.

## Verdict

The change meets the plan. macOS bindings, labels, and menu accelerators are
unchanged, and the config migration keeps the user's choice and nothing else.
F1 made the menu screenshot step verify what it shows; F2–F4 were small fixes.
No medium or high finding is open.
