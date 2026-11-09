# Redesign the Workspaces sidebar and make every session independent

## Goal

The Workspaces sidebar follows the three design images: a plain list of
workspaces, each with a header (name, caret, "+"), rounded session rows, nested
sessions under a guide line that ends in a rounded hook, faded long titles, and
one right-aligned status dot per row that turns into a "…" menu button on
hover. The busy spinner leaves the sidebar. A collapsed parent still lists its
nested sessions that are waiting on the user, then a summary row
"N more nested sessions · M unread".

Every session, top-level or nested, is independent for attention and replies.
A nested session that finishes a turn has an unseen reply, a blue dot, and a
reply card, and is marked seen when shown. A parent row shows only its own
state, and a nested session's request card shows whenever that nested session
is not the one shown.

## Related code

- `internal/session/event.go` — `applyEvent` sets `Unseen` only when
  `s.Parent == ""`.
- `internal/ui/snapshot.go` — `showSession` fills `Unseen`, `Reply`, and
  `ReplySeq` for top-level sessions only; `show` builds both kinds.
- `internal/ui/web/sidebar/sidebar.jsx` — the `vscode-tree`, workspace items
  with More Actions and New Session buttons, expansion state in `collapsed`,
  paging in `limits`, the frozen list while renaming, the context menu handler.
- `internal/ui/web/sidebar/session-item.jsx` — the row, its spinner, its
  aggregated waiting dot, its top-level-only unseen dot, the capture handler that
  works around `vscode-tree-item` toggling, and `RenameField`.
- `internal/ui/web/sidebar/sidebar-menus.js` — `sessionItems` (empty for an
  active nested session), `useSidebarMenu`.
- `internal/ui/web/sidebar/sidebar.css`, `session-item.css`,
  `sidebar.test.jsx`.
- `internal/ui/web/shared/status-dot.jsx` — `StatusDot` kinds `waiting` and
  `unseen`, used as is.
- `internal/ui/web/shared/use-tree-open.js` — still used by
  `files/files-panel.jsx`; it stays.
- `internal/ui/web/notifications/notifications.js` — `cards` builds reply
  cards from top-level sessions only and hides a nested session's request
  while its parent is selected.
- `internal/ui/web/sessions/session-view.jsx` — already marks the shown
  session seen through `currentSession`, which includes nested sessions.
- `internal/ui/web/sessions/selection.js` — `allSessions`, `sessionOf`.
- `docs/usage.md` — describes the sidebar's spinners, dots, and cards.

## Decisions

- **Plain markup replaces `vscode-tree` in the sidebar.** The tree item draws
  its own chevron before the label, its own indent guides, its own selection
  colors, and toggles on row clicks; the design puts carets after titles, a
  hooked guide, and a status column on the right. Plain elements make the
  design direct and remove the click-capture workaround. Rows are real
  `<button>` elements, so Tab, Enter, and Space work; arrow-key navigation
  goes away. `use-tree-open.js` and the `vscode-tree` imports in
  `elements.js` stay for the files panel.
- **Row structure.** Each session row is a `div.session-item` holding a title
  `<button class="session-item-title">` (selects the session), a caret
  `<button>` when the session has nested sessions, and a fixed-width status
  slot at the right. The slot holds the dot and, when the session has menu
  items, a "…" `<button>`; CSS shows the button instead of the dot while the
  row is hovered or has focus within. The button opens the same menu as
  right-click, positioned below it. A row with no dot and no menu leaves the
  slot empty.
- **One dot per row, own state only.** Orange (`waiting`) when
  `running && attention`; otherwise blue (`unseen`) when `unseen` and the row
  is not selected. A parent no longer reflects its nested sessions.
- **Workspace header.** Name, then a caret button; the whole name-and-caret
  part is one `<button>` that toggles the workspace. "+" (New Session, the lead
  picker) sits at the right and is always visible. More Actions ("…") appears
  beside "+" on hover or focus, matching session rows, and right-click still
  opens the workspace menu.
- **Collapsed parent.** It lists its running nested sessions with `attention`,
  in creation order, then a summary `<button>` when other nested sessions
  remain: "N more nested sessions", plus " · M unread" in the blue accent when
  M of those N are unseen. Clicking it expands the parent. A collapsed parent
  with every nested session listed shows no summary row. Expansion stays in
  the sidebar's `collapsed` state, keyed by session ID, open by default.
- **Measurements** (from the 2x images, halved): sidebar content inset 4px on
  each side; workspace header 32px with the name at 14px from the left; rows
  32px tall with 2px between them, 6px corner radius; titles 14px from the row's
  left at the top level and 34px for nested rows; the guide line 18px from the
  sidebar's left; the status slot 16px wide, centered 16px from the row's
  right edge, in line with "+". Carets are small filled triangles (▾ / ▸) in
  `--vscode-descriptionForeground`, 8px after the title.
- **Colors.** Hover: `--vscode-list-hoverBackground`. Selected:
  `--vscode-list-inactiveSelectionBackground` (gray, as in the design). Guide:
  `--vscode-tree-indentGuidesStroke`. Summary unread text: the unseen dot's
  `--vscode-notificationsInfoIcon-foreground`. Stopped rows and "Show more"
  keep `--vscode-descriptionForeground`.
- **Fade instead of ellipsis.** The title text sits in a span with
  `white-space: nowrap; overflow: hidden`, a 24px right padding, and
  `mask-image: linear-gradient(to right, #000 calc(100% - 24px), transparent)`.
  Overflowing text runs under the padding and fades; short text ends before
  it, so nothing visible fades. The caret follows with a negative left margin
  so it sits 8px after short text.
- **Guide line and hook.** The nested list (`div.sidebar-nested`) has one
  `::before` box at the guide's position, from its top to 16px above its
  bottom (the last row's middle), with a left and bottom border and a 6px
  bottom-left radius. Under a collapsed parent the summary row is the last row,
  so the hook ends there, as in image 4.
- **Unread for nested sessions.** `applyEvent` marks any session unseen when
  it goes from busy to idle. `snapshot.go` fills `Unseen`, `Reply`, and
  `ReplySeq` in `show`, so both kinds carry them; `show` takes the transaction
  and returns an error.
- **Cards.** Reply cards come from every session in sidebar order (each parent,
  then its nested sessions), carrying `parent` like request cards so the
  source line names it. A card hides only while its own session is selected.

## Test plan

Go:

- `internal/session/event_test.go`: replace
  `TestIdleParentAndFinishedChildDoNotMarkTheSessionUnseen` with
  `TestFinishedTurnsMarkOnlyTheirOwnSessionUnseen`: an already-idle image event
  marks nothing; a nested session's finished turn marks it unseen and leaves
  its parent seen; `Seen` clears the nested session.
- `internal/ui/ui_test.go`: alongside the existing reply checks near line 556,
  a nested session that finishes a turn appears in the snapshot with
  `unseen`, `reply`, and `replySeq`, and the `seen` action clears it.

Browser (`sidebar.test.jsx`, rewritten for plain markup; keep the paging,
rename, menus, delete dialogs, and lead picker tests, driving them by clicking
the new buttons):

- Clicking a workspace header collapses and expands its sessions; clicking "+"
  opens the lead picker and leaves the workspace open.
- Clicking a parent's title selects it and leaves it open; clicking its caret
  collapses it without selecting or posting.
- A collapsed parent lists its waiting nested sessions, then
  "2 more nested sessions · 1 unread"; with no unseen ones the unread part is
  absent; with all nested sessions waiting there is no summary row; clicking
  the summary expands the parent.
- A nested session's request shows a waiting dot on its own row and none on
  its parent; a nested session's unseen reply shows a blue dot, gone while it
  is selected; no row renders a `loading` icon.
- A row's "…" button opens its session menu; an active nested session has no
  "…" button; the workspace's "…" opens the workspace menu.
- Keys typed in the rename field do not reach the sidebar's handlers.

`notifications.test.js`:

- A nested session with an unseen reply gets a reply card after its parent's,
  naming the parent.
- A nested session's request card shows while its parent is selected and
  hides while the nested session itself is selected.

## Implementation plan

1. `internal/session/event.go`: drop the `s.Parent == ""` condition in
   `applyEvent` and update its comment. Update `event_test.go` as above.
2. `internal/ui/snapshot.go`: move the unseen and reply fill into `show(tx, s,
   running)`, returning `(shownSession, error)`; `showSession` uses it for the
   parent and each nested session. Add the `ui_test.go` check.
3. `internal/ui/web/notifications/notifications.js`: build reply cards from each
   parent and its nested sessions with `parent` set; filter shown cards only by
   `card.session.id !== state.selected`. Update the header comment and
   `notifications.test.js`.
4. `internal/ui/web/sidebar/sidebar-menus.js`: return `hasSessionMenu(session)`
   from `useSidebarMenu` (whether `sessionItems` is nonempty), for the "…"
   button.
5. `internal/ui/web/sidebar/session-item.jsx`: rewrite as the plain row
   described above: title button, optional caret button (`open`, `onToggle`
   props), status slot with `StatusDot` and the "…" button (`onMenu` prop).
   Delete the spinner, the aggregated waiting list, and the click-capture
   effect. `RenameField` replaces the title button while renaming and stops its
   key and click events from reaching the row. Keep `data-session` on the row
   and the `session-item-stopped` class.
6. `internal/ui/web/sidebar/nested-sessions.jsx` (new): `NestedSessions` renders
   a parent's nested rows in `div.sidebar-nested`: all of them when open;
   otherwise the waiting ones and the summary button.
7. `internal/ui/web/sidebar/sidebar.jsx`: replace the tree with a
   `div.sidebar-list` of workspaces, each a `div.sidebar-workspace` with
   `data-workspace`, its header (toggle button, "…", "+"), its top-level rows
   with their `NestedSessions`, and a "Show more" button. Remove `useTreeOpen`
   and the `vsc-tree-select` handler; the context menu handler finds
   `[data-session]` or `[data-workspace]`. Keep the frozen list while renaming.
8. CSS: rewrite `sidebar.css` (list, header, nested list and guide, summary,
   show more) and `session-item.css` (row, hover and selected backgrounds,
   title fade, caret, status slot with the hover swap, stopped color) to the
   measurements and colors above.
9. Rewrite `sidebar.test.jsx` as in the test plan.
10. Update `docs/usage.md`.

## Documentation updates

- `docs/usage.md`: the sidebar paragraphs. "+" beside a workspace's name opens
  the lead picker; its caret collapses it; a parent's caret collapses its
  nested sessions, and a collapsed parent still lists those waiting on you
  with a summary of the rest; hovering a row shows "…" for its menu. Remove
  the spinner sentence. An amber dot means that session awaits you; a blue
  dot marks a reply you have not seen, for any session. Reply cards come from
  any session, and a card hides only while you view its own session.
