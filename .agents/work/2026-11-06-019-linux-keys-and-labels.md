# Keys and labels on Linux

## Plan

`agents/plans/2026-11-06-018-linux-keys-and-labels.md`

## Summary

On Linux, ⌘ is Ctrl and ⌥ is Alt in the menus' accelerators, the page's shortcut
handler, quick open, the shortcut reference, and the tooltips, with labels such
as `Ctrl+Shift+F`. Next and Previous Session, Next Change, and Previous Change
have Linux bindings. The panel terminal keeps its keys except the shortcuts
marked `terminal`, and Ctrl+Shift+C and Ctrl+Shift+V copy and paste; the
macOS editing keys apply only on macOS. `notifications.macos` is
`notifications.desktop`, and `UpgradeConfig` renames it. `test-desktop` has a
keys step that presses keys with `xdotool`. The plan's goal is met.

## Departures from the plan

- `desktop/accelerator.js` names `pagedown` and `pageup` as `PageDown` and
  `PageUp`. The plan said `desktop/` needs no change; the page sends the key
  lower case, and the new Linux bindings for Next and Previous Session use it.
  The Linux test confirmed the menu item's accelerator is `Control+PageDown`.
- Quick open now swallows a key through `matches(e, 'quickOpen')`, which also
  requires Shift to be up. ⌘⇧P was swallowed before and nothing is bound to it.
- The keys step's order differs from the plan's list: it prints a marker,
  clears it with Ctrl+L, then prints a second marker to select, copy, and paste.
  Selecting the first marker was impossible after the screen cleared.
- The menu opens with a mouse click at fixed offsets from the origin `xdotool`
  reports for the window (`MENU_VIEW_X`, `MENU_BAR_Y`). Electron's menu bar is
  not DOM and has no mnemonics, so no key opens View.
- Playwright's `dblclick` on a terminal row needs `force: true` because
  `.xterm-screen` covers the rows.

## Decisions

- The reference draws the Linux `+` as a plain `span` between keycaps, as
  shadcn's Kbd examples do, so only keys sit in `Kbd`.
- `UpgradeConfig` calls `upgradeProfiles` and `upgradeNotifications` on the
  decoded document. The profile upgrade reads `default_profile` and `profiles`
  from the document instead of decoding the file again.
- The keys step tests Ctrl+/ with the terminal focused; the reference opened,
  so xterm.js sends nothing for it.
- Ctrl+PageDown, F7, and the hidden second bindings are not pressed in
  `test-desktop`. The unit tests cover their matching, and the test reads the
  Next Session accelerator from the menu.

## Checks run

- `bunx tsc -p .` and `bun test` in `internal/ui` — passed (new:
  `keys.test.ts`, `shortcuts-dialog.test.tsx`, `quick-open.test.tsx`, and tests
  in `shortcuts.test.ts` and `app.test.tsx`).
- `go test ./internal/harness` — passed, including
  `TestUpgradeConfigNotifications`.
- `make check` — passed on macOS.
- `scripts/linux/run bash -c 'scripts/linux/test-bundle && scripts/linux/test-desktop'`
  — passed. `test-install` was not part of this run; the change does not touch
  the installer.
- macOS shortcuts and labels: `keys.test.ts` asserts the macOS parsing and
  labels equal the old ones, and the existing shortcut tests pass unchanged.
  `make install`, `make place`, `make quit-desktop`, `make install-service`, and
  `make uninstall` were not run on the Mac.

## Manual verification

Screenshots from `scripts/linux/run`, in `bin/linux-test/`.

- `keys-shortcuts.png`: the Keyboard shortcuts dialog over the window, with each
  binding as keycaps joined by `+`: Settings… `Ctrl + ,`, New Session `Ctrl + N`,
  Next Session `Ctrl + PageDown` and `Ctrl + Tab`, Previous Session
  `Ctrl + PageUp` and `Ctrl + Shift + Tab`, Select Session `Ctrl + 1–9`, Search
  Files `Ctrl + Shift + F`, Toggle Right Panel `Ctrl + Alt + B`. No ⌘, ⌥, or ⇧
  glyphs appear.
- `keys-menu.png`: the View menu open from the menu bar, with Toggle Sidebar
  `Ctrl+B`, Toggle Terminal `Ctrl+J`, Toggle Right Panel `Alt+Ctrl+B` (Electron's
  own order), Zoom In `Ctrl+=`, Zoom Out `Ctrl+-`, Actual Size `Ctrl+0`, and the
  stock Full Screen and Developer Tools items. The terminal below shows the
  pasted `echo pasted-ok` run and its output.

The keys step asserts: the View and Session accelerators use `Control` and none
uses `Command`; Ctrl+B hides and shows the sidebar and Ctrl+K opens session
search outside the terminal; in the terminal `cat -v` reads `^B^N`, the sidebar
stays and no session opens; Ctrl+C returns the prompt; Ctrl+L clears the screen;
Ctrl+P opens Quick Open; Ctrl+Shift+C puts the selected word on the clipboard;
Ctrl+Shift+V pastes `echo pasted-ok`, which runs; Ctrl+/ opens the reference.

## Follow-up work

- The View menu shows `Alt+Ctrl+B`, Electron's modifier order, while the
  reference and tooltips show `Ctrl+Alt+B`.
