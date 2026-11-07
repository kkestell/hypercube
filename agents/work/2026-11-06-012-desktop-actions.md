# Desktop actions

## Plan

`agents/plans/2026-11-06-013-desktop-actions.md`

## Summary

All plan tasks are implemented. The desktop app runs the folder chooser and the
open actions; the backend's `osascript`, `open`, and `POST /open` are gone.
`make check` passes on macOS. The Linux paths were not run on Linux; they are
covered by platform-stubbed tests.

## Decisions

- `desktop/actions.js` exports `createActions({dialog, shell, platform})` so
  tests pass stubs; `window.js` supplies Electron's `dialog` and `shell` and
  `process.platform`.
- On macOS, `open -a` runs to completion and a failure ("Could not open
  <path>.") reaches the page, as `POST /open` did. `code` on Linux is detached,
  and only a failure to start is reported.
- `test-render.tsx` gained `testBridge`, a complete bridge stub, because the
  bridge's type gained members and three existing tests assigned the old shape.
- `hypercube()` is the only gate for the open items and both Add Workspace
  buttons, so the empty-state heading remains in a browser with no workspace.

## Checks run

- `make check` — passed.

## Manual verification

`make dev` and `make dev-app` used the existing dev state under
`~/projects/hypercube-dev` on port 4281. The Electron window was driven through
its remote debugging port (page) and inspector (main process), because
UI scripting and screen capture are not permitted here.

1. `make dev`: the scratch repository is a workspace; `POST /workspace?action=new`
   without a path answers 400; `POST /open` answers 405.
2. `make dev-app`: the workspace menu lists Open in VS Code, Reveal in Finder,
   Open in Terminal, Delete Workspace; the session menu lists the same group.
   Choosing Reveal in Finder opened a Finder window on the scratch repository,
   and Open in Terminal opened a Terminal window in it. Open in VS Code
   returned no error.
3. `hypercube.open` from the page: `reveal` of a file, a relative path
   (refused), an unknown app (refused), and VS Code on a missing folder
   (`Could not open /nonexistent/dir.`) returned as expected.
4. Add Workspace's button started the system's folder panel (an
   `openAndSavePanelService` process appeared). It could not be dismissed
   without accessibility access, so its look and native Cancel were not
   observed. With `dialog.showOpenDialog` replaced from the inspector, a
   cancelled result posted nothing and a chosen folder added the workspace;
   the dialog got title "Add Workspace", button "Add", and the
   `openDirectory` and `createDirectory` properties.
5. Not checked: the file menu's Show in Finder in the running app (covered by
   the page test and the `reveal` call above) and `make dev` in a real browser
   (covered by tests with no bridge).

Cleanup: the test workspace was deleted, `make dev-stop` ended the dev
backend and app; the installed app was untouched. The dev state directory was
kept because it held workspaces from earlier sessions.

## Follow-up work

- `agents/todo.md` still lists "Desktop actions".
- `docs/usage.md` and `README.md` name Finder and Terminal; Linux wording
  belongs to the Linux documentation task.
