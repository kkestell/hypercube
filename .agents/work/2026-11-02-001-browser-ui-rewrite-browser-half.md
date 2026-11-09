# Browser UI rewrite: browser half

## Plan

`agents/plans/2026-11-01-003-browser-ui-rewrite.md`, "Browser half".

## Summary

The browser view is a Preact app in `internal/ui/web/`, built by
`internal/ui/build.js` into the committed `internal/ui/dist/`, which
`assets.go` embeds. It follows the plan's store, modules, controls, theme, and
data socket protocol, and adds notification cards. The old page, Tailwind
stylesheet, bundles, and their tests are deleted, and their cases are ported.
`make check` passes, including the stale-`dist/` check, and the build is
deterministic across checkouts.

## Departures from the plan

- `build.js` does not run `bun install`. `make ui` and `make check` run
  `bun install --frozen-lockfile` before it. Bun's resolver caches the
  directory listing when the script starts, so a `node_modules` that the
  script installed went unseen, and `Bun.build` failed in a fresh checkout.
  The version check still comes before the script imports any package.
- License files match `/^licen[cs]e/i`. Many bundled packages (`ccount`,
  `vfile`, and others) ship a lowercase `license`.
- Added `transcript/transcript-images.jsx`, the images row shared by text,
  message, and tool items. Keeping it in `transcript-item.jsx` would create an
  import cycle with `message-item.jsx`. `tool-items.jsx` and
  `transcript-item.jsx` still import each other, because tool calls draw
  nested rows.
- Added `internal/ui/test-render.jsx`, the test helper that mounts a component
  under a store as `App` does, and `notifications/notification-card.test.jsx`
  for the 15-second reply timer, which lives in the card component.
- `sidebar-menus.js` exports `useSidebarMenu`, a hook that holds the open
  menu and the lead picker's stale-request guard. The menu is still state
  local to the sidebar.
- Session rows put their indicators in `vscode-tree-item`'s `decoration`
  slot, which truncates the title. There is no gradient fading the title
  under the indicators.
- `rememberScroll` takes the session ID, so a view that unmounts because the
  page attached to another session does not write its scroll into the new
  one.
- `setVisible` is in `app/workbench.js`.
- CSS uses a few literal sizes besides the variables: 1px borders, VS Code's
  11px view title, and the 3px tab strip scrollbar. No literal colors appear.

## Decisions

- The store's context value is `[state, store]`, so every `useStore` caller
  draws from the current state. Memoized transcript items take their item
  and a stable `setOpen` as props and do not call `useStore`.
- `vscode-tree-item` turns itself into a leaf when its children slot empties.
  The Files tree therefore keeps a listed folder's children rendered while it
  is collapsed, and folders are marked `branch`.
- The context menu mounts only while open, because `vscode-context-menu`
  closes itself without an event.
- The rename field also stops clicks, so clicking in it does not select, and
  so start, the session.
- `EditorTabs` moves the focus in a `requestAnimationFrame` after the store
  update, because the store redraws after the handler returns and a closed
  pane's strip is gone.
- `markSeen` goes through `api.post`, so a refusal alerts like every other
  action.

## Checks run

- `make check` — passes: 99 bun tests, `go vet`, gofmt, and `go test -race`.
  The Go tests pass against the unchanged server.
- `bun test` in `internal/ui` — 99 pass.

## Manual verification

1. Task 2, the split check. `vscode-split-layout` keeps both slotted panes in
   the light DOM and moves only the handle and the panes' flex sizes, so
   `Split` hides a pane by moving the handle to its edge with a zero handle
   size and no minimum. `shared/split.test.jsx` checks that both panes stay
   mounted. Whether it looks right is for the user to judge after the merge.

2. A changed source makes `dist/` stale. A comment-only edit leaves the
   minified output unchanged, so this used a string change.

   ```sh
   sed -i '' "s/'Toggle Panel'/'Toggle The Panel'/" internal/ui/web/app/title-bar.jsx
   tmp=$(mktemp -d); (cd internal/ui && bun build.js "$tmp" && diff -rq dist "$tmp")
   ```

   `diff` listed `main.js` and the highlight chunk. The recipe printed
   `internal/ui/dist is stale; run make ui` and exited 1. The edit was then
   reverted.

3. Another bun version fails the build.

   ```sh
   sed -i '' 's/bun@1.4.2/bun@1.4.1/' internal/ui/package.json && (cd internal/ui && bun build.js /tmp/x)
   ```

   Printed `internal/ui needs bun 1.4.1; this is bun 1.4.2` and exited 1. The
   edit was then reverted.

4. Determinism. Two detached worktrees of the commit, at `$tmp/one` and
   `$tmp/deeper/path/two`, each ran
   `bun install --frozen-lockfile && bun build.js dist`. `diff -r` between
   them, and between each and the committed `dist`, printed nothing. Both
   worktrees were removed.

5. `make dev` built the app and served the page; `index.html`, `main.js`,
   `main.css`, `theme.css`, `codicon.css`, and `favicon.svg` answered 200.
   `scripts/dev clean` stopped it and removed its state. The page needs the Go
   half's data socket (`build` and `requests` in state messages), so it runs
   only after the merge.

## Follow-up work

- Bundling `codicon.css` as a second entry of the same split build makes Bun
  write an empty `main-<hash>.js` chunk that nothing loads. It is
  deterministic and harmless.
