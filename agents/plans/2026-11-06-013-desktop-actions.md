# Desktop actions

## Goal

The backend shows Add Workspace's folder chooser with `osascript` and opens
folders and files in VS Code, Finder, and Terminal with `open`. On Linux the
backend runs as a systemd user service and may not have the graphical
session's environment, so these actions fail there. Move them to the desktop
app, which runs in the graphical session on both platforms:

- Add Workspace shows the app's folder chooser (`dialog.showOpenDialog`) and
  sends the chosen path to `POST /workspace?action=new&path=…`.
- The workspace and session menus' items, and the file menu's Show in Finder
  item, ask the app to act: `shell.openPath` and `shell.showItemInFolder` for
  the file manager, VS Code, and Terminal on macOS only.
- Labels follow the platform: Finder labels on macOS, "Show in Files" on
  Linux.
- The backend's `osascript` and `open` calls and `POST /open` are deleted.

## Related code

- `internal/ui/actions.go` — `workspaceAction`; `new` runs `osascript` when
  no path is given.
- `internal/ui/open.go`, `internal/ui/open_test.go` — `POST /open`, which
  resolves a session's or workspace's directory, or a file in a session's
  directory, and runs `open`. Deleted.
- `internal/ui/ui.go` — registers `POST /open`.
- `internal/ui/ui_test.go` — posts `/workspace?action=new&path=…`.
- `desktop/preload.js` — the window's bridge: `postMessage` and `onEvent`.
- `desktop/window.js` — owns the trust check: it accepts messages only from the
  main frame at the backend's origin (`fromPage`).
- `desktop/main.js` — handles the page's `badge` and `shortcuts` messages.
- `internal/ui/web/native.d.ts` — the bridge's type.
- `internal/ui/web/app/window.ts` — `hypercube()` returns the bridge, or
  undefined in a browser.
- `internal/ui/web/sessions/session-actions.ts` — `addWorkspace` posts
  `workspace?action=new` with no path.
- `internal/ui/web/sidebar/sidebar.tsx`,
  `internal/ui/web/sessions/session-view.tsx` — the two Add Workspace buttons.
- `internal/ui/web/sidebar/sidebar-menus.ts` — the session and workspace
  menus' Open in VS Code, Reveal in Finder, and Open in Terminal items.
- `internal/ui/web/files/path-menu.tsx` — the file menu's Show in Finder item;
  it already computes a file's absolute path from `session.dir`.
- `scripts/dev` — `make dev`; creates the scratch repository.

## Decisions

### The bridge

`preload.js` adds three members to `hypercube`:

```js
platform: process.platform,
chooseFolder: () => ipcRenderer.invoke('action', {type: 'choose-folder'}),
open: (app, path) => ipcRenderer.invoke('action', {type: 'open', app, path}),
```

`process.platform` is available in a sandboxed preload. `chooseFolder`
resolves to the chosen folder, or an empty string when the user cancels.
`open` resolves to an error message, empty on success, as `hypercubeSetup.install`
does. `app` is one of:

| `app` | macOS | Linux |
| --- | --- | --- |
| `vscode` | `open -a "Visual Studio Code" <path>` | `code <path>` |
| `folder` | `shell.openPath(path)` | `shell.openPath(path)` |
| `reveal` | `shell.showItemInFolder(path)` | `shell.showItemInFolder(path)` |
| `terminal` | `open -a Terminal <path>` | refused |

`window.js` registers `ipcMain.handle('action', …)` beside its `page`
listener, applies the same `fromPage` check and throws `refused` otherwise,
and removes the handler when the window closes. It passes the request and its
`BrowserWindow` (the dialog's parent) to a new `desktop/actions.js`, which
owns the actions. `actions.js` checks its input: a known `type` and `app`, and
a path that is an absolute string. It returns an error message for a refused
request rather than throwing, so the page shows it.

The folder chooser is `dialog.showOpenDialog(win, {title: 'Add Workspace',
buttonLabel: 'Add', properties: ['openDirectory', 'createDirectory']})`.
Attached to the window, it is a sheet on macOS and a modal dialog on Linux.

### VS Code

A macOS app started from the Dock or at login gets launchd's default PATH,
`/usr/bin:/bin:/usr/sbin:/sbin` (`launchctl getenv PATH` is empty on this
machine). VS Code's "Install 'code' command in PATH" puts a link in
`/usr/local/bin`, and Homebrew's in `/opt/homebrew/bin`; neither is on that
PATH, so `code` is not found. macOS therefore keeps `open -a "Visual Studio
Code"`, which needs no PATH and is today's behavior, now run by the app.

On Linux, Microsoft's `.deb` and `.rpm` packages and Arch's packages install
`/usr/bin/code`, which is on every session's PATH. The snap installs
`/snap/bin/code`; snapd adds `/snap/bin` to graphical sessions through
`/usr/lib/environment.d/990-snapd.conf`. The app spawns `code` detached with
ignored output; when spawning fails with `ENOENT`, it returns "VS Code's code
command was not found."

### Paths come from the page

The page already has each workspace's `path` and each session's `dir`, and
`path-menu.tsx` computes a file's absolute path. The page passes these to
`open`; the backend no longer resolves them.

### Labels and items

A new `internal/ui/web/app/desktop-actions.ts` owns the page's side:

- `openItems(store, dir)` — the menus' group. macOS: Open in VS Code
  (`code`), Reveal in Finder (`folder-open`), Open in Terminal (`terminal`).
  Linux: Open in VS Code, Show in Files. Empty without the bridge.
- `revealItem(store, path)` — Show in Finder on macOS, Show in Files on
  Linux, for the file menu. Absent without the bridge.
- Each item calls `hypercube().open(app, path)` and shows a returned error with
  `showError`.

The icons stay the Lucide icons the items use now.

### The browser view in `make dev`

Without the bridge, the open items and both Add Workspace buttons are not
shown. `scripts/dev` adds the scratch repository as a workspace after the
backend answers, with `curl -X POST -H "Origin: http://localhost:$PORT"
"http://localhost:$PORT/workspace?action=new&path=<repo>"`. Adding an existing
workspace returns it unchanged, so this runs on every start.

### The backend

`workspaceAction`'s `new` requires `path` and answers 400 without it. It no
longer imports `os/exec`, `errors`, or `strings` for the chooser.

## Test plan

- `internal/ui/ui_test.go`: `/workspace?action=new` without a path answers
  400; the existing case with a path still adds the workspace.
- Delete `internal/ui/open_test.go`.
- `internal/ui/web/sidebar/sidebar.test.tsx`: with a bridge stub on `darwin`,
  the session menu lists Open in VS Code, Reveal in Finder, Open in Terminal;
  on `linux`, Open in VS Code and Show in Files; with no bridge, neither.
  Choosing Open in VS Code calls `open('vscode', <session dir>)`, and a
  workspace's item uses the workspace path.
- `internal/ui/web/files/path-menu.test.tsx`: Show in Finder calls
  `open('reveal', '/worktree with spaces/src/file with spaces.go')` and posts
  nothing; the menu closes and focus returns to the row.
- A test of `addWorkspace`: a chosen folder posts `workspace` with
  `action=new` and the path; a cancelled chooser posts nothing.
- `desktop/actions.test.js`: a relative path, an unknown `app`, and an unknown
  `type` return an error without running anything.
- Run `make check`. Smoke test with `make dev-app`: Add Workspace's sheet,
  each menu item, and cancelling the chooser. Check that `make dev` in a
  browser shows the scratch workspace and no open items.

## Implementation plan

1. Add `desktop/actions.js` and its test; register the `action` handler in
   `window.js`; extend `preload.js`.
2. Extend the bridge's type in `native.d.ts`.
3. Add `internal/ui/web/app/desktop-actions.ts`; use it in `sidebar-menus.ts`
   and `path-menu.tsx`.
4. Change `addWorkspace` to choose the folder through the bridge; hide both
   Add Workspace buttons without the bridge.
5. Require `path` in `workspaceAction`; delete `open.go`, `open_test.go`, and
   the `POST /open` route.
6. Add the scratch workspace in `scripts/dev`.
7. Update the tests above and the documentation below.

## Documentation updates

- `agents/development.md`: the browser view in `make dev` starts with the
  scratch repository as its workspace; adding workspaces and opening folders
  in other apps need `make dev-app`.
- `agents/architecture.md`, "Backend, desktop app, and terminals": the desktop
  app also shows the folder chooser and opens folders and files in other apps.
