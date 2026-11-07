# Desktop actions review

## Scope and coverage

Reviewed the staged changes against
`agents/plans/2026-11-06-013-desktop-actions.md` and the work log
`agents/work/2026-11-06-012-desktop-actions.md`, reading the whole touched
files: `desktop/actions.js`, `desktop/actions.test.js`, `desktop/preload.js`,
`desktop/window.js`, `internal/ui/actions.go`, `internal/ui/ui.go`,
`internal/ui/ui_test.go`, `internal/ui/test-render.tsx`,
`internal/ui/web/app/desktop-actions.ts`, `internal/ui/web/native.d.ts`,
`internal/ui/web/sessions/session-actions.ts`, `session-view.tsx`,
`internal/ui/web/sidebar/sidebar.tsx`, `sidebar-menus.ts`,
`internal/ui/web/files/path-menu.tsx`, their tests, `scripts/dev`, and the two
`agents/` documents.

The Linux paths (`code`, Show in Files) were not run on Linux; they are covered
by platform-stubbed tests. The app was not run in this review; the work log
records a `make dev-app` smoke test.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Low | Correctness | Fixed | A failed `open -a` blames the path |
| F2 | Low | Readability | Fixed | One launcher function mixes two launch modes through flags |

## F1. A failed `open -a` blames the path

Low · Correctness · Fixed · `desktop/actions.js:10`

On macOS a failed Open in VS Code or Open in Terminal showed "Could not open
<path>." whatever the cause. The likeliest failure, VS Code not installed,
therefore told the user the folder was the problem.

### Evidence

`open -a "No Such App Xyz" /tmp` exits 1 and prints "Unable to find
application named 'No Such App Xyz'"; `open -a Terminal /nonexistent/dir`
exits 1 and prints "The file /nonexistent/dir does not exist." The old code
ignored stderr and built its own message from the path.

### Resolution

`openWith` reads `open`'s stderr and returns it when `open` exits non-zero,
waiting for `close` so the output is complete. Running the action for Terminal
on `/nonexistent/dir` now returns "The file /nonexistent/dir does not exist."

## F2. One launcher function mixes two launch modes through flags

Low · Readability · Fixed · `desktop/actions.js:20`

A `launchers` table and a single `run` function covered both `open -a`, which
is waited on, and Linux's detached `code`, selected by `wait` and `missing`
flags that each applied to only one entry. A reader had to cross-reference the
table to learn which branch ran on which platform.

### Resolution

Replaced with `openWith(application, target)` for macOS and `startCode(target)`
for Linux; the `vscode` and `terminal` cases choose between them by platform.

## Other points checked

- IPC trust boundary: `window.js` registers `action` with the same `fromPage`
  check as `page` (main frame at the backend's origin), so the waiting page
  and subframes are refused; `closed` removes the handler. The window hides
  instead of closing until quit, so a second registration does not occur.
- Input validation: `open` requires an absolute string path, which also
  rules out option injection into `open` and `code`; unknown `app` and `type`
  return messages. Covered by `desktop/actions.test.js`.
- Spawning: `code` is detached and unreferenced after `spawn`; its `error`
  listener stays attached, ENOENT maps to the plan's message, and Node reaps
  the child. `open` is waited on and reaped. No unhandled `error` events.
- Hiding without the bridge: `hypercube()` gates the menu groups in
  `desktop-actions.ts` and the two Add Workspace buttons where they render.
  `ContextMenu` drops empty groups, so no stray separator appears.
- Nothing calls `/open` or relies on the backend's folder chooser; `POST
  /workspace?action=new` without `path` answers 400.
- `scripts/dev`: `add_repo` runs after `start_view` has confirmed the backend
  answers, and `|| echo …` keeps a failure from ending the script under
  `set -euo pipefail`.
- Work log departures: the `createActions({dialog, shell, platform})` seam
  keeps `actions.js` loadable under `bun test` without Electron and lets the
  tests set the platform; acceptable. `testBridge` replaces three hand-built
  bridge stubs whose type grew; acceptable. The "Could not open <path>."
  message is replaced by F1.

## Checks run

- `make check` — passed.
- `open -a` with a missing application and a missing path — confirmed F1.
- `createActions({platform: 'darwin'})` Terminal on `/nonexistent/dir` under
  `bun` — returns `open`'s message after the fix.

## Verdict

Ready to commit. The change meets the plan; no medium or high severity
findings. F1 and F2 are fixed.
