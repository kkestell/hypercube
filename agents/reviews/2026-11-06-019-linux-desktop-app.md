# Linux desktop app review

## Scope and coverage

The staged changes for
[the Linux desktop app plan](../plans/2026-11-06-017-linux-desktop-app.md),
with [its work log](../work/2026-11-06-018-linux-desktop-app.md): `desktop/`
(`window.js`, `menu.js`, `attention.js`, `setup.js`, `setup.test.js`,
`main.js`, the deleted `dock.js`), the waiting page and `SetupStatus`,
`scripts/linux-install`, `scripts/linux-uninstall`, `make/linux.mk`,
`scripts/linux/` (`run`, `Dockerfile`, `file-manager`, `test-desktop`), the
`Makefile`, the CI workflow, and the documentation updates.

The macOS paths of `window.js`, `menu.js`, `attention.js`, `setup.js`, and
`main.js` were compared line by line with `HEAD`. The macOS template, hidden
title bar and traffic lights, hide-on-close with the Dock calls, tray, About
options, `launchctl` check and `kickstart` command, AppTranslocation check, and
installer path and environment are unchanged. Two changes are visible on
macOS, both decided by the plan: the Dock badge comes from
`app.setBadgeCount`, which sets the same text (`''` for zero), and the waiting
page says "The service is installed but not answering" where it said "The
login service is loaded but not answering". The macOS app was not run.

Coverage gaps: the `quit_desktop` timeout path is untested, as the work log
says. CI's `linux-test` job and its artifact upload were not run; the action
`actions/upload-artifact@v7` exists (v7.0.2 is the latest release).

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Testing | Fixed | The notification check passes for any notification |
| F2 | Medium | Testing | Fixed | Desktop test waits on fixed sleeps and reads the menu before it opens |
| F3 | Low | Testing | Fixed | Settings… in the Linux File menu is not checked |
| F4 | Low | Readability | Fixed | The tray is optional at each use instead of created in one place |
| F5 | Low | Readability | Open | `quit_desktop` is copied in two scripts |

## F1. The notification check passes for any notification

Medium · Testing · Fixed · `scripts/linux/test-desktop:215`

The step that checks the notification names the workspace tested
`/Notify[\s\S]*string "project"/.test(notify) || notify.includes('project')`.
The second clause is true for any bus log containing `desktop-project`
anywhere, so the check never tested the Linux-specific behavior in
`attention.js`: the subtitle folded into the body because Linux drops
`subtitle`.

### Evidence

`bin/linux-test/bus.log` from the run shows the `Notify` call with summary
`"first"` and body `"desktop-project\nsecond done"`. Removing the subtitle from
the Linux body in `attention.js` would still have passed the old check.

### Resolution

The check now requires the summary to be the session's title and the body to
be the workspace, then the reply, as `Notify` carries them.

## F2. Desktop test waits on fixed sleeps and reads the menu before it opens

Medium · Testing · Fixed · `scripts/linux/test-desktop:152,233,274`

- After starting openbox, dunst, the bus monitor, and the stand-in file
  manager, the test slept one second and assumed all four were ready. A slow
  start would make a later step fail for an unrelated reason (a frameless
  window, a missing `Notify`, or `show()` blocking for D-Bus's timeout without
  a notification server).
- After right-clicking the workspace, it read `[role=menuitem]` with
  `allInnerTexts()`, which does not wait, so a menu that had not rendered yet
  read as empty and failed the check.
- After reinstalling, it slept three seconds before the screenshot instead of
  waiting for the reopened app to load the backend, which the test also never
  checked.

### Evidence

Read against Playwright's API: `locator.allInnerTexts()` returns the current
matches without waiting. The sleeps had no condition behind them.

### Resolution

The test waits for openbox through `wmctrl -m`, for the notification server
and file manager to own their bus names (`NameHasOwner`), and for the bus
monitor's `NameLost` line, which it prints once it is monitoring. It waits for
the workspace menu's last item before reading the menu, and for the menu's
animations to finish before the screenshot. After reinstalling, it waits for
the new window's title to name the workspace, which the backend page sets.
The settle before the notification screenshot remains; nothing observable
marks the X server's repaint.

## F3. Settings… in the Linux File menu is not checked

Low · Testing · Fixed · `scripts/linux/test-desktop:195`

The plan's step 4 puts Settings… first in File. The test checked only that
Quit ends File and printed the menus. It now checks Settings… first and Quit
last, and the debugging output is gone.

## F4. The tray is optional at each use instead of created in one place

Low · Readability · Fixed · `desktop/attention.js:11`

`createAttention` built the tray menu on Linux and dropped it through
`tray?.setContextMenu`. The macOS menu bar icon is now made by `createTray`,
called only on macOS; `setBadge` still updates the tray's title and tooltip
where it exists.

## F5. `quit_desktop` is copied in two scripts

Low · Readability · Open · `scripts/linux-install:19`, `scripts/linux-uninstall:17`

The two copies differ only in their message prefix. The plan chose the copies
because `make uninstall` runs `scripts/linux-uninstall` from the checkout and
`install` runs from the bundle, so a shared file would need to ship under the
same name in both places. The function itself is safe: it signals only
processes whose `exe` is exactly the installed bundle's `hypercube-desktop`,
which excludes other users' processes and other bundles, skips Chromium's
child processes, and fails the install or removal if the app has not exited
after ten seconds, with the staged copy cleaned up by the installer's `EXIT`
trap.

## Departures in the work log

All are sound. The badge is checked with a question because the page counts
sessions waiting on a question; the substring match for `--type=` follows
Chromium's rewritten child `cmdline`; `createSetup`'s `execPath` argument
exists for the tests; `scripts/linux/run` keeps the last screenshots when a
command makes none.

## Checks run

- `make linux-test` before the fixes — passed; screenshots inspected.
- `make check` after the fixes — passed.
- `make linux-test` after the fixes — passed; the new screenshots match.
- Screenshots in `bin/linux-test/`: the openbox frame and File, Edit, View,
  Session, Help menu bar; each waiting page with the expected command; the
  backend page under the menu bar with browser spacing; the dunst notification
  reading "first", "desktop-project", "second done"; the workspace menu with
  Open in VS Code and Show in Files and no Open in Terminal; the reopened app
  on the same session after reinstalling.

## Verdict

The Linux desktop work matches the plan, keeps platform differences in the
owners the plan names, and leaves macOS behavior as it was apart from the two
plan-decided changes above. The IPC trust checks are unchanged: the setup
handlers accept only the waiting page's main frame, and setup runs only when
the status is `setup`. F1 and F2 made the desktop test check what it claimed
and wait on conditions; F5 stays as the plan decided.
