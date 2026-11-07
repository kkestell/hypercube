# Keys and labels on Linux

## Goal

Key bindings are written with ⌘ and ⌥ and always become `metaKey` and
Electron's `Command`. On Linux that is the Super key, which the desktop owns, so
no shortcut works and the menus, tooltips, and shortcut reference show macOS
glyphs. The saved notification setting is named `notifications.macos`, and
Settings calls it "Show macOS notifications".

When the work is done:

- On Linux, ⌘ becomes Ctrl and ⌥ becomes Alt in the menus' accelerators, the
  page's shortcut handler, quick open, the shortcut reference, and the tooltips
  that show keys. Labels take the Linux form: `Ctrl+Shift+F`. macOS does not
  change.
- Bindings that collide with GNOME's defaults after mapping have Linux
  bindings that do not.
- In the panel terminal on Linux, every key reaches the shell except the
  shortcuts marked `terminal: true` and Ctrl+Shift+C and Ctrl+Shift+V, which
  copy and paste as in GNOME Terminal.
- `xterm.ts`'s macOS editing keys apply only on macOS.
- The setting is `notifications.desktop`, labeled "Show desktop notifications".
  The backend renames the old key in `config.json` at startup, keeping the
  user's choice.

## Related code

- `internal/ui/web/app/shortcuts.ts`: the binding table (`shortcuts`), the
  notation parser `binding()`, `matches()`, `firstKeys()`, the page's capture
  handler `shortcut()`, and the rule that the terminal keeps keys not marked
  `terminal`.
- `internal/ui/web/app/app.tsx`: posts the `menu: true` shortcuts to the desktop
  app as `{key, metaKey, altKey, ctrlKey, shiftKey}` bindings.
- `desktop/accelerator.js`, `desktop/menu.js`: turn those bindings into
  Electron accelerators (`metaKey` → `Command`, `ctrlKey` → `Control`) and
  build the menus. Neither needs to know the platform: once the page sends
  `ctrlKey`, Linux menus get `Control+B`, and Electron labels it `Ctrl+B`.
- `internal/ui/web/app/shortcuts-dialog.tsx`: the shortcut reference; splits
  each binding into ⌘⌥⌃⇧ glyphs, one `Kbd` each.
- `internal/ui/web/app/layout-toggles.tsx`, `internal/ui/web/editor/view-options.tsx`,
  `internal/ui/web/agent/composer.tsx`: show a binding in a tooltip or menu
  (`firstKeys()`, or `keys[0]` in the composer).
- `internal/ui/web/files/quick-open.tsx`: swallows ⌘P inside the dialog by
  testing `metaKey`.
- `internal/ui/web/changes/diff-view.tsx`, `internal/ui/web/shared/find.tsx`:
  use `matches()`, so they follow the new rule without changes.
- `internal/ui/web/terminal/xterm.ts`: creates the terminal; its custom key
  handler sends the macOS editing keys.
- `internal/ui/web/terminal/terminal-panel.tsx`: creates the terminal for the
  panel.
- `internal/ui/web/app/window.ts`: `hypercube()`, the desktop bridge, whose
  `platform` is `process.platform`.
- `internal/harness/config.go`: `Notifications.MacOS`, `UpgradeConfig()` (runs at
  startup from `cmd/hypercube/main.go` before anything reads the config).
- `cmd/hypercube/main.go`: `sendAlert` checks `n.MacOS`.
- `internal/ui/web/app/types.ts`, `internal/ui/web/app/settings-preferences.tsx`,
  `internal/ui/test-state.ts`, `internal/ui/web/notifications/notifications.ts`:
  the UI's notification type, the Settings toggle, test state, and a comment.
- `scripts/linux/test-desktop`, `scripts/linux/Dockerfile`: the Linux desktop
  test and its container image.

## Decisions

### The platform rule

A new module, `internal/ui/web/app/keys.ts`, holds the platform rule and
nothing else:

- `linux()`: the desktop bridge's `platform` is `'linux'`. Without a bridge the
  page uses the macOS rule. The desktop app is the only supported client, and
  unit tests set the platform through `testBridge({platform})`.
- `binding(text)`: parses the existing notation into
  `{key, metaKey, altKey, ctrlKey, shiftKey}`. On macOS it is today's parser. On
  Linux ⌘ and ⌃ set `ctrlKey`, ⌥ sets `altKey`, and `metaKey` is never set.
  `binding()` moves here from `shortcuts.ts`.
- `keyNames(text)`: the label's parts. On macOS, the glyphs and key as today
  (`['⌘', '⇧', 'F']`). On Linux, `Ctrl`, `Shift`, `Alt` in VS Code's Linux
  order, then the key: letters upper case, `↓`/`↑` as `Down`/`Up`, and other
  names (`Tab`, `PageDown`, `F7`, `Escape`, `,`, `+`, `1–9`) as written.
  GNOME Terminal shows zoom in the same way, `Ctrl++`.
- `keysLabel(text)`: the parts as one string, joined with nothing on macOS
  (`⌘B`) and with `+` on Linux (`Ctrl+B`), for tooltips and menu text.

The binding table keeps the macOS notation. A shortcut may add `linux` keys
that replace `keys` on Linux; `shortcuts.ts` gets `keysOf(shortcut)`, used by
`matches()`, the menu message, the reference, and `firstKeys()`. `firstKeys()`
returns `keysLabel()` of the first binding, and the composer's tooltip uses
`firstKeys('composer')`.

### Conflicts

After mapping, no two Hypercube bindings resolve to the same keystroke, and
none uses the Edit menu's Ctrl+C, Ctrl+V, Ctrl+X, Ctrl+A, Ctrl+Z, or
Ctrl+Shift+Z, or the terminal's Ctrl+Shift+C and Ctrl+Shift+V. ⌃Tab and ⌃⇧Tab
stay Ctrl+Tab and Ctrl+Shift+Tab and collide with nothing.

The container's GNOME 48 schemas (`org.gnome.desktop.wm.keybindings`,
`org.gnome.mutter.*`, `org.gnome.settings-daemon.plugins.media-keys`) and
KDE's KWin and Plasma source give these collisions:

| Shortcut | Mapped | Desktop default | Linux keys |
| --- | --- | --- | --- |
| Next Session | Ctrl+Alt+Down | GNOME: switch to the workspace below | `Ctrl+PageDown`, `Ctrl+Tab` |
| Previous Session | Ctrl+Alt+Up | GNOME: switch to the workspace above | `Ctrl+PageUp`, `Ctrl+Shift+Tab` |
| Next Change | Alt+F5 | GNOME: unmaximize | `F7` |
| Previous Change | Alt+Shift+F5 | none, kept beside Next Change | `Shift+F7` |

Ctrl+PageDown and Ctrl+PageUp switch tabs in GNOME Terminal, Firefox, and VS
Code on Linux. F7 and Shift+F7 are VS Code's "Go to Next/Previous Difference"
in its diff editor. KDE's defaults use Meta, Ctrl+F1–F4, and Alt+F3/F4, which
no binding uses. Nothing else in the table collides with either desktop.

### Which handler sees a key first on Linux

The container answered this with Electron 45.2.3 under Xvfb and openbox, with
real X key events from `xdotool`:

- The page's `keydown` runs first. A menu accelerator, including the Edit
  menu's copy, paste, select all, and undo roles, fires only if the page did not
  call `preventDefault()`.
- Playwright's `page.keyboard` sends keys through the DevTools protocol, which
  never triggers menu accelerators. The Linux test therefore presses keys with
  `xdotool`, added to the test image.

xterm.js 6 calls `preventDefault()` on every key it sends to the shell:
Ctrl+letters (Ctrl+C, Ctrl+D, Ctrl+L, Ctrl+B, Ctrl+K, Ctrl+P, Ctrl+N, Ctrl+J,
Ctrl+A, Ctrl+V, Ctrl+Z), Ctrl+Tab, Ctrl+PageUp/PageDown, Ctrl+Alt+keys, and
function keys. Those keys reach the shell, and no menu item takes them. This
needs no `registerAccelerator: false` and no change to `desktop/menu.js`.

Keys xterm.js sends nothing for (Ctrl+=, Ctrl+-, Ctrl+0, Ctrl+/, and
Ctrl+Shift+letters) fall through to the menu, so zoom works in the terminal as
it does in GNOME Terminal.

### Panel terminal keys

`createTerminal` takes a `passes(event)` predicate from the panel. The custom key
handler, on keydown:

1. Returns `false` when `passes(event)`, so xterm.js leaves the key to the page
   or the menu. The panel passes `terminalShortcut(event)`, a new export from
   `shortcuts.ts`: some shortcut marked `terminal` matches. `shortcut()` uses
   the same function for its terminal rule. This makes Ctrl+, reach the File
   menu without depending on xterm.js ignoring it.
2. On Linux, copies the selection on Ctrl+Shift+C with
   `navigator.clipboard.writeText(term.getSelection())`, and pastes on
   Ctrl+Shift+V with `term.paste(await navigator.clipboard.readText())`, which
   keeps bracketed paste. Both call `preventDefault()`. The page is served from
   localhost, a secure context, and the desktop app sets no permission handler,
   so Electron allows clipboard reads.
3. On macOS, sends the editing keys as today.

On macOS, step 1 changes nothing: xterm.js ignores ⌘ keys other than ⌘A.

### Notification setting

`Notifications.MacOS` becomes `Desktop` (`json:"desktop"`), and the UI type
field becomes `desktop`. `UpgradeConfig` becomes a sequence of upgrades on the
decoded document: the existing profile conversion, and a new one that moves
`notifications.macos` to `notifications.desktop` when only the old key is
present. When no upgrade changes the document, the file is left as it is;
otherwise it is decoded, validated, and written as the typed `Config`, which
holds only the new key. Each upgrade is its own function, so `UpgradeConfig`
reads as a list of steps. The user's config migrates when the new build first
starts.

The Settings toggle becomes "Show desktop notifications" with id
`notify-desktop`. Chrome and Slack use the term on both platforms, and it
describes the setting on both.

## Test plan

Unit tests (`bun test`):

- `keys.test.ts`: for each platform, `binding()` and `keysLabel()` of `⌘,`,
  `⌘⇧F`, `⌘⌥B`, `⌘⌥↓`, `⌃⇧Tab`, `⌥Z`, `⌘+`, `⌘1–9`, `F3`, and `Escape`; macOS
  results equal today's; Linux results set no `metaKey` and read `Ctrl+,`,
  `Ctrl+Shift+F`, `Ctrl+Alt+B`, `Ctrl+Alt+Down`, `Ctrl+Shift+Tab`, `Alt+Z`,
  `Ctrl++`, `Ctrl+1–9`.
- `shortcuts.test.ts`: on each platform, no two bindings of the table (with
  `1–9` expanded) resolve to the same keystroke, and none equals an Edit menu
  key or, on Linux, Ctrl+Shift+C or Ctrl+Shift+V. On Linux, Ctrl+K runs session
  search, Ctrl+Alt+Down and Alt+F5 run nothing, Ctrl+PageDown and F7 match
  their shortcuts, and in the terminal panel Ctrl+B is left alone while Ctrl+P
  and Ctrl+, pass. On macOS the existing tests keep passing.
- `app.test.tsx`: the menu message on Linux carries `ctrlKey` bindings and the
  Linux keys for Next and Previous Session.
- The shortcut reference renders `Ctrl`, `+`, `Shift`, `+`, `F` for Search Files
  on Linux and `⌘`, `⇧`, `F` on macOS.
- Quick open swallows Ctrl+P on Linux and ⌘P on macOS.
- `settings-dialog.test.tsx`, `notifications.test.ts`: use `desktop` and
  `#notify-desktop`.

Go (`go test ./internal/harness`), in `config_test.go`:

- A config with `"notifications": {"macos": false, ...}` upgrades to
  `"desktop": false` and no `macos` key, with the other fields unchanged.
- A config with both keys, or only `desktop`, is left byte for byte.
- A config with profiles and `macos` gets both upgrades in one write.
- `LoadConfig` reads `desktop`, and `SaveConfig` writes it.

Linux (`make linux-test`), a new step in `scripts/linux/test-desktop` after the
open actions, while the session is open. Keys are pressed with `xdotool` on the
focused window:

- Outside the terminal, Ctrl+B hides the sidebar through the menu, and Ctrl+K
  opens session search through the page.
- The View and Session menus' items carry `Control` accelerators
  (`MenuItem.accelerator` through `electronApp.evaluate`). The test clicks the
  View menu open with `xdotool` and saves a screenshot.
- Ctrl+J opens the panel terminal. With the terminal focused, the test runs
  `cat -v`, presses Ctrl+B and Ctrl+N (Ctrl+N is New Session's menu key), then
  Enter, and reads `^B^N` from the terminal's rows. No unsent session opened.
  Ctrl+C ends `cat` (the prompt returns), and after the test prints a marker,
  Ctrl+L clears it from the screen.
- Ctrl+P opens Quick Open from the terminal.
- Ctrl+Shift+C with the marker selected puts it on the clipboard, read with
  Electron's `clipboard.readText()`; after the test writes `echo pasted-ok` to
  the clipboard, Ctrl+Shift+V and Enter print `pasted-ok`.
- Ctrl+/ opens the shortcut reference, and the test saves a screenshot.

Screenshots `keys-menu.png` and `keys-shortcuts.png` land in `bin/linux-test/`.
Inspect them: Ctrl labels in the menu, the Linux keycaps in the reference.

## Implementation plan

1. Add `internal/ui/web/app/keys.ts` with `linux()`, `binding()`, `keyNames()`,
   and `keysLabel()`, and its unit tests.
2. In `shortcuts.ts`, import `binding` from `keys.ts`, add `linux` keys to Next
   Session, Previous Session, Next Change, and Previous Change, add `keysOf()`
   and `terminalShortcut()`, use them in `matches()` and `shortcut()`, and make
   `firstKeys()` return the label. Add the duplicate and Linux tests.
3. In `app.tsx`, build the menu message from `keysOf()`. Use `firstKeys()` in
   the composer's tooltip.
4. Render the reference from `keyNames()`: one `Kbd` per part, with a `+`
   between parts on Linux.
5. In `quick-open.tsx`, swallow the key when `matches(e, 'quickOpen')`.
6. In `xterm.ts`, take `passes`, add the Linux copy and paste keys, and limit
   the editing keys to macOS. Update the file's opening comment. Pass
   `terminalShortcut` from `terminal-panel.tsx`.
7. Rename the setting in `config.go`, `cmd/hypercube/main.go`, `types.ts`,
   `settings-preferences.tsx`, `test-state.ts`, and the tests. Split
   `UpgradeConfig` into its upgrades and add the notification one, with Go
   tests.
8. Add `xdotool` to `scripts/linux/Dockerfile`, add the keys step and its
   screenshots to `scripts/linux/test-desktop`, update its opening comment, and
   update the description of `test-desktop` in `agents/development.md`.
9. Update the documentation below. Run `make check` and `make linux-test`, and
   inspect the new screenshots.

## Documentation updates

- `docs/configuration.md`: the `notifications` row names `desktop` ("shows
  desktop notifications while the window is not focused") and the example uses
  `"desktop": true`.
- `docs/usage.md`: the notifications paragraph names "desktop notifications"
  and the Settings label. The shortcuts paragraph adds that on Linux shortcuts
  use Ctrl for ⌘ and Alt for ⌥, that the terminal keeps every key it uses
  except Ctrl+P, Ctrl+Shift+F, and Ctrl+,, and that Ctrl+Shift+C and
  Ctrl+Shift+V copy and paste there.
- `README.md`: the notification preferences bullet says "desktop
  notifications".
- `agents/development.md`: `test-desktop` also checks shortcuts, the panel
  terminal's keys, and the Ctrl labels.
- `agents/research/linux.md`: the menu accelerator fact records the measured
  order: the page's keydown first, and its `preventDefault()` suppresses the
  accelerator.
