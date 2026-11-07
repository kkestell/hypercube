# Rewrite the browser UI and give the runtime one change feed

## Goal

The browser view is one 1,046-line inline script in `static/index.html` that
builds HTML strings and keeps state in a dozen module-level variables, plus
helper scripts and a Tailwind stylesheet. The server learns of changes from two
sources: the runtime's `Changes` channel, and `store.Watch`, which polls SQLite
every 250 ms because some writes bypass the runtime. On every wake it re-reads,
re-marshals, and byte-compares whole transcripts.

After this change:

- **Browser.** A Preact app in `internal/ui/web/` keeps every feature and
  behavior of HEAD and adds notification cards.
  - One store holds the app's state. Socket messages, HTTP results, and user
    actions change it only through pure transition functions, and components
    draw from it.
  - Standard controls are `@vscode-elements/elements`. Icons are codicons and
    Seti file icons. Colors are VS Code's Dark Modern `--vscode-*`
    variables.
  - One deterministic bun build writes `internal/ui/dist/`, which is committed
    and embedded, so `go build` needs no bun. `make check` fails when `dist/`
    is stale.
- **Server.** `session.Runtime` is the only database writer, and it publishes
  one feed of changes: each write, and each changed transcript item with its
  agent and sequence number. The data socket sends the view data, open
  attention requests, and the changed items of the selected session. It no
  longer polls the database or re-diffs whole transcripts. The dev-reload
  script that `ui.go` injects is gone; a page reloads itself when the server's
  browser build changes.
- **The interface.** The data socket protocol below is the only contract
  between the two halves. A Go worker and a browser worker can build them in
  parallel, and no file is touched by both.

## Related code

Server:

- `internal/session/runtime.go`
  - `Runtime` claims to be "the only writer of the database" (`:43`).
  - `changed`, `Changes`, and `touchLocked` (`:105-118`) are the signal the UI
    waits on.
  - `event` (`:375-470`) applies plugin items: pending items are merged and
    kept in `live.pending`, and done items are written with `entry`.
  - `note` (`:183`), `ended` (`:332-337`, drained items), `deliver`
    (`:518-529`, a delivered message), `Input` (`:632`), and `detachLocked`
    (`:754`) also write items with `tx.PutEntry`. `Transcript` merges saved
    and pending items.
  - `report.Apply`, `report.Failure` (`:443,453`), and `report.Prompt`
    (`:628`) are called here.
- `internal/session/session.go`
  - `AddWorkspace`, `Rename`, `Seen`, and `update` take a raw `*store.DB`.
  - `Create` returns only the session.
  - The rest of the file writes through `r.db.Tx`.
- `internal/session/inactivity.go` — `recordActivity` writes through `r.db.Tx`.
  `ViewSession` keeps a selected session from idling.
- `internal/report/report.go` — `Apply`, `Prompt`, and `Failure` (`:14-49`).
  `failure` (`:200`) formats the failure text.
- `internal/mcp/mcp.go` — `sendMessage` (`:267`) and `setStatus` (`:311`)
  write the database directly.
- `internal/store/store.go`
  - `Watch` (`:206-232`).
  - `ShownWorkspace`, `ShownSession`, `ShownAgent`, and `Snapshot`
    (`:461-534`).
  - `Tx.LastSeq` and the `transcript` table (`agent`, `seq`, `item`, `data`).
- `internal/ui/ui.go`
  - `//go:embed static` and `Icon`.
  - `Handler` starts the `Watch` goroutine and the `Changes` goroutine.
  - `devReload`.
  - `refresh`, `sentItems`, `sendTranscript`, and `state`: the socket.
  - `workspaceAction`, `sessionAction`, and `agentAction`. The `new` action
    reads the lead back after `Create`.
  - `terminal`, `localOnly`, `leads`, and `refused`.
- `internal/ui/ui_test.go` — `TestStateStreamsSnapshots` writes the database
  directly. `TestTranscriptStreamsToTheSelectedSession` and `TestDevReload`
  test the current socket and dev reload.
- `cmd/hypercube/main.go` (calls `ui.Handler`) and
  `cmd/hypercube/integration_test.go` (calls `session.AddWorkspace` and
  `store.Snapshot`).
- `agents/reviews/go-audit.md` — step 1, "One writer, one change feed", is in
  scope. Its other items are not.
- `backup/orchestrator` commits `9640733` and `bc1fd9c` — the notification
  cards, `Runtime.Requests`, `Tx.LastText`, and the reply fields. They are a
  reference for behavior; the orchestrator parts do not apply.

Browser (a reference for behavior only):

- `internal/ui/static/index.html`
  - The data socket (`watch`, `announce`) and selection (`changed`, `select`,
    `attach`).
  - The unsent session, sidebar, and menus.
  - Cards and composer, drafts, and images.
  - Tab rendering, file tabs and polling, and the terminal panel.
  - Sashes and drop guards.
- `internal/ui/static/tabs.js`, `files.js`, and `hypercube.js`.
- `internal/ui/transcript.js`, `terminal.js`, `viewer.js`, `style.css`, and
  `*.test.js`.
- `Makefile` (`ui`, `check`) and `scripts/dev` (`sources`,
  `HYPERCUBE_DEV_RELOAD`).
- `docs/usage.md`.

## Work split

**Step 0, done by the lead before both workers start, in one commit.** Create
`internal/ui/assets.go`, which owns the embedded browser files:

- the `//go:embed static` variable, moved from `ui.go`, and `Icon()`, moved
  from `ui.go`;
- `assets() fs.FS`, the embedded files rooted at the page;
- `buildID`, the hex SHA-256 of every file's path and contents in
  `fs.WalkDir` order, computed once.

In `ui.go`, `Handler` serves `assets()`, and `devReload` reads `index.html`
from `assets()`. Tests still pass.

After step 0, each worker owns only the files listed for its half.

| Go half | Browser half |
| --- | --- |
| `internal/session/**` | `internal/ui/web/**` (new) |
| `internal/report/**` | `internal/ui/dist/**` (new, generated) |
| `internal/store/**` | `internal/ui/assets.go` (embed `dist` instead of `static`) |
| `internal/mcp/**` | `internal/ui/build.js`, `jsconfig.json`, `bunfig.toml`, `test-dom.js` (new) |
| `internal/ui/ui.go`, `ui_test.go` | `internal/ui/package.json`, `bun.lock` |
| `internal/ui/socket.go`, `snapshot.go`, `actions.go`, `terminal.go` (new, split from `ui.go`) | the old browser files under `internal/ui/` (deleted) |
| `cmd/hypercube/main.go`, `integration_test.go` | `Makefile`, `scripts/dev`, `docs/usage.md` |

Nobody changes `internal/ui/files.go`, `files_test.go`, `plugins/`, or
`internal/harness/`. Another session is changing the last two.

The lead merges both halves, then runs `make check` and `make dev`.

## Data socket protocol

The page opens one WebSocket at `/state`. All messages are JSON text.

### Page → server

```json
{"select": "<session ID>"}
```

`select` names the session whose transcripts the page wants, or `""` for
none. The page sends it when the socket opens and whenever its attached
session changes. While a session is selected by an open page, it does not
stop for idleness (`ViewSession`).

### Server → page: state

```json
{"type": "state", "build": "<hex>", "workspaces": [...], "requests": [...]}
```

The server sends this first on connect. After that it sends it whenever the
workspaces or requests change, at most once per 50 ms batch, and only when its
bytes differ from the last state sent.

- `build` is `buildID`. A page reloads itself when a later state message
  carries a different `build` than the first one it received.
- `workspaces` are sorted by name. Each workspace is:

  ```
  {id, path, name, sessions: [
    {id, title, running, unseen, reply?, replySeq?, agents: [
      {id, name, role, harness, model, effort, dir, activity, attention,
       status, statusAt, usage: {context?, window?, cost?}}]}]}
  ```

  Sessions come running first, then by last activity, newest first. Agents are
  in creation order. `reply` and `replySeq` are the lead's latest top-level
  `text` item and its `seq`. They are present only while the session is unseen
  and its lead is idle.
- `requests` are `[{agent, item}]`: every running agent's open attention item,
  in all workspaces, newest first (by `item.at`, then `item.id`, both
  descending). The page shows request cards in this order.

### Server → page: transcript

```json
{"type": "transcript", "session": "<ID>", "agent": "<ID>", "reset": true, "items": [...]}
```

Each item is a `harness.Item` object with one more field, `seq`, its position
in the agent's transcript.

- After each `select` of a session, the server sends one message with
  `reset: true` for every agent of that session. It holds all of the agent's
  items in `seq` order, saved and still streaming. The list may be empty.
- After that, every new or changed item of an agent in the selected session
  arrives in a message with `reset: false`. The message holds the item whole,
  with streamed text merged. Messages are batched per agent at most once per
  50 ms, and their items are in `seq` order. A worker added to the session
  later sends all its items this way.
- The page keeps one item per `id` and orders items by `seq`. It ignores a
  message whose `session` is not its attached session.

The terminal socket and every HTTP route keep their current form. The one
exception is `POST /session?action=new`, which returns `{session, lead}` from
`Create`'s results.

## Go half

### Decisions

- **The feed.** It lives in a new file, `internal/session/feed.go`:

  ```go
  // Change is one change the runtime made. A transcript change names the
  // session, agent, item, and the item's position. Any other write, to
  // workspaces, sessions, agents, or messages, leaves Agent empty.
  type Change struct {
  	Session, Agent string
  	Seq            int64
  	Item           harness.Item
  }
  ```

  The item is a copy, because `event` merges streamed text into the pending
  item in place.

  - `(*Runtime).Subscribe() (<-chan Change, func())` returns a channel with a
    1,024-change buffer and a function that ends the subscription.
  - Publishing never blocks. A subscriber whose buffer is full is dropped and
    its channel closed, as `terminal` does for slow readers. Its page
    reconnects and gets a fresh state and transcripts.
  - A `feed` type with its own mutex holds the subscribers. Publishing works
    whether or not the caller holds `r.mu`.
- **Every write publishes.** `feed.go` also holds the write path:
  - `write` is one runtime transaction: it embeds `*store.Tx` and collects
    the items written in it.
  - `(*Runtime).save(fn func(*write) error) error` is the only place the
    package commits a transaction. After a commit it publishes `Change{}`,
    then each collected item. Nothing is published for a transaction that
    fails.
  - `(*write).putItem(l *live, seq int64, item harness.Item) error` is the
    only caller of `tx.PutEntry`. It fills `item.At` as `entry` does, writes
    the item, and collects its change. `note`, `Input`, `deliver`, `ended`,
    `detachLocked`, and `event`'s done items use it. `markEnded` and the
    other helpers that take a `*store.Tx` and write items take a `*write`
    instead.
  - `event` publishes its still-pending items itself, merged, before the
    early return. Their done versions are published by `putItem`.
  - Every caller of `putItem` holds `r.mu`, and so does `event`, so each
    agent's changes are published in order.
  - Every `r.db.Tx` in `session.go`, `runtime.go`, and `inactivity.go`
    becomes `r.save`. Reads keep `r.db.Read`.
  - `changed`, `Changes`, and `touchLocked` are deleted.
- **One writer.** These become `Runtime` methods:
  - In `session.go`, the session and agent record edits: `AddWorkspace(dir)`,
    `Rename(id, title)`, `Seen(id)`, and `SetStatus(agent, text string) error`
    (from `mcp.setStatus`). `update` becomes a method that uses `save`.
  - In a new `internal/session/messages.go`, the messages queued between
    agents:
    - `SendMessage(from, to, text string) (recipient store.Agent, running
      bool, err error)` takes the checks and the queueing from
      `mcp.sendMessage`, then calls `Deliver`.
    - `queueFailure(w, worker, detail)` (was `report.Failure`) queues a
      worker's failure for its lead. `event` calls it only for workers.

  The MCP tools keep only argument handling and the reply text.
- **`Create`** returns `(store.Session, store.Agent, error)`. The agent is the
  lead it created.
- **Event application.** `applyEvent(a, s, ev)` (was `report.Apply`) goes in
  `runtime.go` beside `event`, its only caller. `report.Prompt` sets one
  field, so `Input` sets `s.AwaitingUser = false` itself. `report` keeps the
  failure text as the exported `FailureMessage(detail)`, and its package doc
  drops "It applies protocol events to the stored agent."
- **Requests.** `(*Runtime).Requests() []Request` goes in `runtime.go`, with
  `Request{Agent string; Item harness.Item}` (JSON `agent`, `item`). It reads
  `live.pending` under `r.mu` and sorts the requests newest first, as the
  protocol says.
- **Store.**
  - Delete `Watch` and its test.
  - Delete `ShownWorkspace`, `ShownSession`, `ShownAgent`, and `Snapshot`.
  - Add `Tx.LastText(agent) (string, int64, error)`: the latest top-level
    `text` item's text and `seq`, or `""` and 0. It is the query from
    `9640733`.
- **ui.go splits by concern.** Every handler changes, so `ui.go` is split as
  part of this change:
  - `ui.go`: package doc, `Handler(db, rt)` with routes, `localOnly`,
    `refused`, and `leads`. `ctx` is dropped, because no server-wide
    goroutine remains, and `cmd/hypercube/main.go` is updated.
  - `snapshot.go`: unexported `shownWorkspace`, `shownSession`, `shownAgent`
    (with `reply`, `replySeq`), and `snapshot(db)`, moved from `store`.
  - `socket.go`: the `/state` route.
  - `actions.go`: `workspaceAction`, `sessionAction`, `agentAction`, and
    `readInput`. Writes call `rt` methods, and `new` answers with
    `Create`'s session and lead IDs.
  - `terminal.go`: the `/terminal` route, moved unchanged.
  - Delete `devReload`, `HYPERCUBE_DEV_RELOAD`, `refresh`, `sentItems`,
    `sendTranscript`, and the `server` fields `snapshot`, `shown`, and
    `changed`.
- **The socket loop** (`socket.go`), one goroutine per connection besides its
  reader:
  1. `Subscribe`, then send the first state.
  2. On `select`:
     1. Swap the `ViewSession` hold.
     2. Read the session's agents.
     3. Send a reset message for each from `rt.Transcript`.
     4. Clear the pending batch.

     Subscribing comes first, so a change during the reads arrives again
     afterwards and replaces its item in place.
  3. On a change:
     - An item of the selected session goes into the batch, keyed by agent
       and item ID, keeping the latest.
     - A change with no agent marks the view data stale.
     - Any change starts the 50 ms timer if it is not running.
  4. When the timer fires:
     - If the view data is stale, rebuild `snapshot(db)`.
     - Always rebuild `rt.Requests()`.
     - Send the state if its bytes changed.
     - Send one transcript message per agent in the batch, items sorted by
       `seq`.
  5. A closed change channel or a failed write ends the connection.

  Each socket builds its own snapshot. Pages are few, and this needs no
  shared cache.

### Tests

- `internal/session`, new `feed_test.go`:
  - a write (for example `Rename`) publishes a change with no agent;
  - streamed items of one agent arrive in order, with `seq`, `Session`, and
    merged text, and a published item keeps its text when a later delta
    arrives;
  - a delivered message, a launch that ends (its drained items and the error
    note), and a stopped session's drained items publish their items;
  - a failed transaction publishes nothing;
  - a subscriber that stops reading is dropped and its channel closes;
  - the cancel function stops delivery.
- `internal/session`: `AddWorkspace`, `Rename`, `Seen`, `SendMessage` (its
  cross-session rules), and `SetStatus` (workers only) through the runtime.
  `Create` returns the lead. `Requests` lists open attention items newest
  first and drops answered ones. `Input` releases queued messages (moved from
  `TestPromptReleasesQueuedMessages`).
- `internal/store`: `LastText` ignores items with a parent and returns
  `""`, 0 when there is none. Delete the `Watch` test.
- `internal/report`: delete `TestPromptReleasesQueuedMessages`.
- `internal/mcp`: the `send_message` and `set_status` tests pass through the
  runtime methods. Keep the existing test style; moving to `callTool` is a
  separate audit item.
- `internal/ui/ui_test.go`:
  - `TestStateStreamsSnapshots` makes its changes with `rt.AddWorkspace` and
    `rt.Create`, and checks `build` against `buildID`.
  - `TestTranscriptStreamsToTheSelectedSession` checks:
    - one reset per agent after `select`, carrying `session` and increasing
      `seq`;
    - update messages hold only the changed items;
    - streamed text replaces itself in place;
    - a second page gets the whole transcript;
    - `select: ""` stops the items.
  - Port `TestRepliesAndRequestsReachEveryPage` from `9640733` and `bc1fd9c`,
    without the orchestrator:
    - requests reach every page in the state message and go when answered;
    - `reply` and `replySeq` appear when the lead finishes and clear while it
      works again.
  - `TestNewSessionTakesItsFirstPrompt` checks the returned lead ID.
  - Delete `TestDevReload`.
- `cmd/hypercube/integration_test.go`: update the call sites
  (`rt.AddWorkspace`, and read sessions through `store` instead of
  `store.Snapshot`). The audit's restructuring of `TestTeams` is out of scope.
- Checks:
  - `grep -rn 'db.Tx(' cmd internal` lists only `save` and
    `internal/store`.
  - `grep -rn 'PutEntry' cmd internal` lists only `putItem` and
    `internal/store`.

### Tasks

1. `feed.go`: the feed, `write`, `save`, and `putItem`. Replace every
   `r.db.Tx` and `tx.PutEntry`. Delete `Changes`.
2. Move the writes into the runtime: `AddWorkspace`, `Rename`, `Seen`, and
   `SetStatus` in `session.go`; `messages.go`; and `Create`'s lead. Move
   `applyEvent` beside `event` and update `report`.
3. `Requests`, `Tx.LastText`. Delete `Watch` and `Shown*`/`Snapshot` from
   `store`.
4. Split `ui.go` and write `snapshot.go`, `socket.go`, `actions.go`, and
   `terminal.go`. Delete dev reload. Update `cmd/hypercube/main.go`.
5. Update the MCP tools, the tests, and the integration test call sites. Run
   `make check`.

## Browser half

### Dependencies

All versions are exact in `package.json` and frozen in `bun.lock`.

- Runtime, bundled: `preact` 11.1.1, `@vscode-elements/elements` 2.5.1 (Lit
  web components that copy VS Code's controls), `@vscode/codicons`
  0.0.46-24, `marked` 18.1.2, `shiki` 4.5.0, `@xterm/xterm` 6.0.0,
  `@xterm/addon-fit` 0.11.0, `@xterm/addon-webgl` 0.19.0.
- Build only: `@vscode-elements/webview-playground` 1.9.0. Its
  `dist/themes/dark-v2.js` exports VS Code's Default Dark Modern theme as VS
  Code itself resolves it for webviews: 783 `--vscode-*` variables, including
  the color registry defaults that the theme JSON files leave out.
- Test only: `@happy-dom/global-registrator` 21.0.4.
- Removed: `tailwindcss` and `@tailwindcss/cli`.
- Bun is pinned by `"packageManager": "bun@1.4.2"` in `package.json`.

JSX uses Preact's automatic runtime. It is configured once in
`internal/ui/jsconfig.json` (`"jsx": "react-jsx"`, `"jsxImportSource":
"preact"`), which both `bun build` and `bun test` read.

### Controls

Components use vscode-elements as custom elements from JSX. Preact sets
properties on them and listens for their events with `on<event-name>`, as in
`onvsc-tree-select`. `web/elements.js` imports only the elements in this table,
and `main.jsx` imports it. Components never import element definitions, so in
tests the tags are plain elements.

| Control | Element | Reason for a custom component |
| --- | --- | --- |
| Title bar layout toggles | `vscode-toolbar-container`, `vscode-toolbar-button` (`toggleable`, `checked`) | |
| Sidebar view title and New Workspace | custom title, `vscode-toolbar-button icon="add"` | VS Code's view title is not an element |
| Workspaces and sessions | `vscode-tree`, `vscode-tree-item` (`actions` slot for More Actions and New Session, `decoration` slot for indicators); Show more is a leaf item | |
| Session rename field | `vscode-textfield` | |
| Context menus and the lead picker | `vscode-context-menu` (`data`, `show`, `vsc-context-menu-select`) | |
| Resizable regions and pane splits | `vscode-split-layout` | |
| Right panel tabs | `vscode-tabs`, `vscode-tab-header`, `vscode-tab-panel` | |
| Files tree | `vscode-tree`, `vscode-tree-item` (`icon-leaf` slot for the Seti icon) | |
| Every icon and spinner | `vscode-icon` (`name`, `spin` for `loading`) | |
| Composer buttons (attach, stop, send) | `vscode-button icon-only secondary` | `vscode-toolbar-button` cannot be disabled |
| Attention choices and Send | `vscode-button`; typed answer in `vscode-textfield` | |
| Notification card actions (View Session, close) | `vscode-toolbar-button` | |
| Editor tab strip | custom `EditorTabs` | `vscode-tabs` has no close buttons, dragging, preview or deleted states, or split targets |
| Composer text box | custom `textarea` | VS Code's chat input is a box of several controls that grows with its text, and `vscode-textarea` does neither |
| Tool calls, tool runs, Thinking | native `details` with a `chevron-right` icon | `vscode-collapsible` is a view section header, not the chat's inline disclosure |
| Worker cards, notification cards, message bubbles, status dots, usage ring, image thumbnails | custom | vscode-elements has no card, toast, bubble, or dot |
| Terminal, highlighted code | custom, wrapping xterm and Shiki through refs | |

`vscode-tree-item` opens and closes itself and fires no event when it does.
`shared/use-tree-open.js` watches the reflected `open` attribute of a tree's
items with a `MutationObserver` and calls `onToggle(key, open)`, so the owner
of the expansion state learns of every change, by mouse or keyboard. The
component passes `open` from that state when it renders. The sidebar and the
Files tree both use it.

The session rename field stops `keydown` from propagating, so the tree does
not treat Space or the arrow keys as navigation while the user types.

### Icons

Codicons come from `@vscode/codicons`. `web/codicon.css` imports its
`codicon.css`, and the build writes it to `dist/codicon.css` with the font.
`index.html` links it with `id="vscode-codicon-stylesheet"`, which is how
`vscode-icon` finds it. The names follow VS Code: `add`, `ellipsis`,
`new-session`, `folder`, `folder-opened`, `layout-sidebar-left(-off)`,
`layout-panel(-off)`, `layout-sidebar-right(-off)` for the right panel toggle,
`organization` for Workers, `files` for Files, `source-control` for Changes,
`attach`, `send`, `stop-circle`, `close`, `chevron-right`, `loading`, `error`,
`check`, `eye`, `info`, `warning`, and `layout-sidebar-left` for Open
Transcript.

The Seti font, `vs-seti-icon-theme.json`, and their license move unchanged to
`web/files/seti/`. `file-icon.jsx` imports the JSON, so the theme is bundled
and loads with no fetch.

### Theme and styles

- `build.js` writes `dist/theme.css` as one `:root` rule from the `theme`
  array in `@vscode-elements/webview-playground/dist/themes/dark-v2.js`.
  Nothing is copied by hand.
- `index.html` links `codicon.css`, `theme.css`, and `main.css`, in that
  order.
- `web/base.css` is the first import of `main.jsx`, so it comes first in
  `main.css`. It holds the reset, `body` font and colors, VS Code's
  scrollbars (`--vscode-scrollbarSlider-*`), the VS Code macOS font settings
  that the theme omits or sets for another platform
  (`--vscode-font-family`, `--vscode-editor-font-family`,
  `--vscode-editor-font-size: 12px`), and the shared sizes:
  `--size-space` (4px, used in multiples), `--size-row-height` (22px, list
  rows), `--size-bar-height` (35px, title bar and tab bars), `--size-icon`
  (16px), `--size-dot` (8px), and `--size-radius` (4px).
- Each component has one stylesheet next to it with the same base name, and
  the component imports it. Its class names start with the component's name,
  as in `.editor-tabs-tab`. Rules use only `--vscode-*` variables, the
  `--size-*` variables, and `calc()` of them. No literal colors appear.
- Where colors come from:
  - Title bar: `titleBar-*`. Sidebar: `sideBar-*` and `sideBarTitle-*`.
    Panel: `panel-*`. Editor area: `editor-background`.
  - Tabs: `editorGroupHeader-tabsBackground`, `tab-*`, and
    `editorGroup-dropBackground`. Sashes come with `vscode-split-layout`.
  - Composer: `input-*` and `focusBorder`. Message bubbles: `chat-request*`.
  - Errors: `inputValidation-error*`. Open requests:
    `inputValidation-warning*`. Muted text: `descriptionForeground`.
  - Markdown: `textLink-*`, `textCodeBlock-background`, `textPreformat-*`,
    `textBlockQuote-*`, and `textSeparator-foreground`.
  - Status dots: waiting is `editorWarning-foreground`, exited is
    `errorForeground`, and unseen is `notificationsInfoIcon-foreground`.
  - Usage ring: `progressBar-background` on a `descriptionForeground` track.
  - Notifications: `notifications-*`, `notificationToast-border`,
    `notificationsInfoIcon-foreground`,
    `notificationsWarningIcon-foreground`, and `widget-shadow`.
  - File view: `editor-*` and `editorLineNumber-foreground`.
- Colors outside CSS also come from the theme. `xterm.js` builds the terminal
  theme from the computed `--vscode-terminal-*` variables, as VS Code does:
  the background falls back to `--vscode-panel-background`, and the cursor
  falls back to the terminal foreground. The tesseract reads its canvas's
  computed background color. Its phosphor color is part of the artwork and
  stays in the shader. Shiki's token colors and Seti's icon colors come from
  their vendored theme data.

### Store

`web/store.js` exports `createStore(initial)`, which returns
`{get, update, subscribe}`. `update(fn)` replaces the state with
`fn(state)` and notifies subscribers once. It also exports `StoreContext` and
`useStore()`, which returns `[state, store]`. `App` subscribes once and
provides the context, and components read from it. Components that draw one
transcript item take their item as props and are wrapped in `memo`, so an
update redraws only the items that changed.

There are no action objects. Each concern module exports:

- **transitions**: pure functions `(state, ...args) => state`;
- **selectors**: pure functions of state, such as the shown agents or the
  page title;
- **commands**: functions `(store, ...args)` that call the server through
  `api.js` and apply transitions to the results.

The store is the only owner of shared state. Component `useState` holds only
state that one component reads: the sidebar's collapsed workspaces, page
limits, rename, and open menu; and a split's remembered size. The data
socket, file poller, and terminal keep their connections in their own
closures.

State shape:

```js
{
  workspaces: [],      // from the last state message
  requests: [],        // from the last state message
  selected: '',        // see "Selection" below
  pending: null,       // a session ID to select once a state message lists it
  unsent: null,        // {workspace, lead, harness, model, effort, sending}
  visible: true,       // document.visibilityState === 'visible'
  drafts: {},          // draft key → {text, images, reading}
  dismissed: {},       // notification key → true
  workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'workers'},
  session: null,       // the attached session, below
}

session = {
  id,
  transcripts: {},     // agent ID → {order: [item IDs by seq], items: {ID: item}}
  expanded: {},        // tool, run, message, and Thinking keys the user opened
  tabs: {root: null, preview: null, dragging: null, nextId: 1},
  scroll: {},          // tab or agent ID → {top, left, following}
  files: {listings: {}, expanded: {}, open: {}},
}
```

- A draft key is an agent ID for a composer, `new` for the unsent session's
  lead, or `<agent>/<request>` for a typed answer to an attention request.
  This keeps a half-typed answer when its tab closes, and the transcript and
  the notification card share it.
- A pane is `{id, tabs: [tab IDs], active}`. A split is
  `{id, axis: 'x' | 'y', ratio, children: [node, node]}`. A tab ID is a worker
  agent ID or `file:<path>`. Pane and split IDs come from `nextId`, so
  Preact's keys stay stable when the tree changes.
- `files.listings[dir]` is the sorted entries of a folder, `files.expanded`
  is keyed by folder path, and `files.open[path]` is
  `{text, message, etag, deleted}`.
- An expansion key is an item ID, or `run:<first tool ID>` for a tool run.

Session-scoped state lives in `session`. When the attached session changes,
`session` is replaced, which clears the transcripts, tabs, files, scroll, and
expansion together. Components bound to a session (`SessionView`,
`TerminalPanel`, `FilesPanel`) are keyed by its ID, so their DOM state starts
over too.

#### Selection

`selected` is a session ID. An ID that names no listed session, such as the
`''` of a page opened without a hash, moves to a running session when one is
listed. `null` shows nothing. `receivedWorkspaces` applies these steps in
order:

1. Discard `unsent` and its draft if its workspace is gone.
2. Select `pending` once it is listed.
3. If the shown session stopped while attached, set `selected` to `null`.
   Otherwise, if `selected` is not `null` and names no session, move it to the
   first running session, or `''`.
4. Attach. The attached ID is the selected session's ID when it runs, or
   none. If it differs from `session?.id`, replace `session`. If not, drop
   the transcripts, tabs, and scroll of agents that left the session.

The unsent session is not attached. `SessionView` draws its lead pane from
`unsent` under the draft key `new`. It has no transcript, files, or terminal.

#### Focus

The store holds no focus state.

- A composer focuses its text box when it mounts. Mounts happen when a session
  attaches, when an unsent session opens, and on Open Transcript, because a
  pane draws only its active tab. A mount skips focusing while the focus is in
  an editor tab strip, so clicking a tab or moving between tabs with the
  arrow keys keeps the focus on the tab.
- `EditorTabs`' close handler moves the focus to the pane's newly active tab.
  When it closes the last tab, it focuses the lead's text box, which it finds
  by the composer's `data-agent` attribute.

#### How each message and action updates the store

| Source | Function | Change |
| --- | --- | --- |
| state message | `selection.receivedWorkspaces`, then `notifications.receivedRequests` | the selection steps above; then `requests`, and prune `dismissed` |
| state message with a new `build` | `data-socket.js` | `location.reload()` |
| transcript message | `transcripts.receivedTranscript` | ignored unless `session` matches `session.id`. Otherwise replace (reset) or upsert the items, keep `order` sorted by `seq`, and add the expansion keys around each open attention item. |
| socket open, or `session.id` changes | `data-socket.js` | sends `{select: session?.id ?? ''}` |
| `visibilitychange` | `main.jsx` → `setVisible` | `visible` |
| Click a session or View Session | `session-actions.chooseSession` | `selectSession` (discard unsent; select, or set `pending` if the session is not listed yet), then POST `start` if the session is stopped or `failed` |
| Choose a lead profile | `selection.openUnsent` | `unsent` set; `selected` and `pending` cleared |
| Type in a composer or answer field | `drafts.typed` | `drafts[key].text` |
| Attach images | `drafts.attachImages` | `reading` counted up and down, images appended, limits checked |
| Send a prompt | `drafts.sendPrompt` | draft cleared, POST `input`, or `createSession` for the unsent session. If refused, `restored`. |
| Send the first prompt | `session-actions.createSession` | `unsent.sending`, POST `new`. When accepted, move the `new` draft to the returned lead ID, then `chooseSession` the returned session. |
| Answer a request | `api.answerRequest` | POST `answer`. The new state arrives over the socket. |
| Open Transcript | `editor.openWorkerTab` | tab opened in the first pane and made active |
| Click a file | `editor.openFileTab` | replaces the preview tab, or selects the file's open tab. `files.open[path]` is created and read. |
| Double-click a file or preview tab | `editor.keepOpen` | `preview` cleared |
| Select, close, or drag a tab | `editor.selectTab`, `closeTab`, `moveTab`, `setDragging` | `tabs` |
| Drag a split's divider | `editor.resizeSplit` | `ratio` |
| Leave or remove a tab view | `editor.rememberScroll` | `scroll[id]` |
| Expand or collapse a disclosure | `transcripts.setExpanded` | `expanded[key]` |
| Toggle a folder | `files.setFolderOpen` | `files.expanded`. The first open lists the folder. |
| Poll result | `files.listed`, `listingMissing`, `fileRead` | dropped unless its session ID matches `session.id` |
| Title bar toggle | `workbench.toggle` | `workbench.sidebar`, `rightPanel`, or `panel` |
| Right panel tab | `workbench.selectRightTab` | `workbench.rightTab` |
| Terminal exits or cannot connect | `workbench.closePanel` | `workbench.panel = false` |
| Close a notification card | `notifications.closeCard` | `dismissed[key]`. For a reply, also POST `seen`. |
| A reply card's 15 s timer | `notifications.dismiss` | `dismissed[key]` |

### Transcripts

- `transcript-tree.js` is a pure function from a transcript's ordered items to
  rows. It nests an item under its parent when the parent is present. It
  groups consecutive tool items within one container into a run, which closes
  at the next non-tool item. It classifies `send_message` calls as messages,
  and it computes run durations ("Worked for 32s", "<1s", or "Worked"). It
  also holds `toolTitle`, `toolSummary`, `isLong`, and `clip` (from
  `transcript.js`). Because the tree is recomputed from the whole list, an
  item that arrives before its parent moves under the parent when the parent
  arrives.
- `Transcript` follows the end until the user scrolls up and follows again at
  the end, using HEAD's rules: a 2px end tolerance, and a `ResizeObserver` on
  the list. It restores `{top, following}` from `session.scroll` when it
  mounts and saves it when it unmounts.
- Each pane draws only the body of its active tab. A tab that is hidden,
  moved, or rebuilt by a split remounts its view. Scroll, expansion, and
  drafts live in the store, so nothing visible is lost.
  `editor/use-scroll-memory.js` records the scroll in a ref on each `scroll`
  event and writes it to the store on unmount.
- Expansion is controlled. `details` gets `open` from `session.expanded`, and
  `onToggle` writes it back. `receivedTranscript` opens the tool calls and
  runs around an open attention item, as HEAD's `reveal` does.
- An open attention item keeps the warning style and shows `AttentionForm`.
  A finished one is quiet: its question in `descriptionForeground`, then the
  answer beside a `check` icon. `AttentionForm` sends through
  `api.answerRequest`, the one path for answers. It disables its controls
  while the answer is in flight and enables them again if the server refuses.
  The transcript and the notification card both render it.

### Editor area

`tab-layout.js` keeps `TabLayout`'s behavior as pure functions that return new
trees: open into the first pane, close and prune, move to a pane's center
before a tab, split at an edge, replace the preview in place, and the no-op
rules for a sole tab. A split adds `ratio: 50`. Splits render as nested
`Split` components with `handle-position="<ratio>%"`.
`vsc-split-layout-change` writes the new `positionInPercentage` back through
`resizeSplit`, keeping the ratio clamped to 15–85.

### Regions

`shared/split.jsx` wraps `vscode-split-layout`. It is used for the sidebar
(fixed start), the right panel and the terminal panel (fixed end), the lead
column and editor area, and editor splits. Its `collapsed` prop hides one
pane without unmounting either side: it sets `handle-size` to 0 and the
handle position to the pane's edge, and restores the last size from
`vsc-split-layout-change` when shown. Hiding a region must keep the lead
transcript, terminal, and file views mounted, as HEAD does. Minimum sizes use
`min-start` and `min-end`. If the element cannot collapse a pane this way
during implementation, stop and report to the lead instead of building a
different mechanism.

### Files

`files.js` holds the listing and file state, `loadFolder`, and `readOpenFile`.
It sorts folders first, then by `Intl.Collator` with numeric order. `api.js`
holds the HTTP mapping from `files.js` `files`, unchanged: 304 is unchanged,
404 is deleted, 413 and 415 give VS Code's messages, and other failures give
their error text. `file-polling.js` runs once a second while `visible` and
`session` are set and the previous poll has finished. It works only from the
store:

- When the right panel is open on Files, it lists the root and each expanded
  folder whose ancestors are all expanded.
- When the editor area is shown, it rereads the active file tab of each pane
  with `If-None-Match`.

Requests go one at a time. A listing that returns 404 removes the folder and
its expansion. Any other poll failure leaves the view as it is.

`FileView` draws `files.open[path]`:

- a centered message for large, binary, or unreadable files;
- the Markdown preview, with code blocks highlighted;
- highlighted source with a line-number gutter at least 5 digits wide.

`HighlightedCode` loads `highlight.js` (Shiki, a separate chunk) with a
dynamic import on first use. It writes Shiki's escaped HTML through a ref, and
nothing else writes HTML.

### Notifications

- `notifications.cards(state)` returns request cards in the order of
  `requests`, which the server sends newest first, without sorting again. Reply
  cards follow (sessions with `replySeq`) in sidebar order. It leaves out cards
  for the selected session and cards in `dismissed`. A request card's key is
  `request:<agent>:<item>`, and a reply card's is `reply:<session>:<replySeq>`.
  Each card carries the workspace name, the session title, the worker's name
  when a worker asks, and the request item or reply text.
- Pruning: a request's dismissal is forgotten once the request is gone. A
  reply's dismissal is forgotten once its session is no longer unseen, so the
  same reply does not return after the lead's busy spells.
- `NotificationCard` follows VS Code's toast: an `info` or `warning` icon, the
  details, the View Session (`eye`) and close buttons, and the source line
  below. A reply shows its opening lines, clamped to three. A request shows
  `AttentionForm`. A reply card dismisses itself after 15 s without marking
  the reply seen. The close button on a reply card posts `seen`, which
  persists across reloads.
- `NotificationStack` stacks the cards in the top right corner, below the
  title bar.

### Build, determinism, and staleness

`internal/ui/build.js <outdir>` is the only build entry. `make ui` runs it with
`dist`, and `make check` runs it with a temporary directory. It:

1. Fails with `internal/ui needs bun <pinned>; this is bun <found>` unless
   `Bun.version` equals the version in `packageManager`. It does this before
   it imports anything from `node_modules`.
2. Runs `bun install --frozen-lockfile`.
3. Empties `<outdir>`, then runs one `Bun.build`:
   - entry points `web/main.jsx` and `web/codicon.css`;
   - `target: 'browser'`, `minify: true`, `splitting: true`,
     `sourcemap: 'none'`, `metafile: true`;
   - naming `entry: '[name].[ext]'`, `chunk: '[name]-[hash].[ext]'`,
     `asset: '[name]-[hash].[ext]'`.

   Hashes come from content.
4. Writes `theme.css` from `dark-v2.js` and copies `web/index.html` and
   `web/favicon.svg`.
5. Ships licenses. For every input in the metafile under `node_modules`, it
   copies the `LICENSE*` files of that package to
   `<outdir>/licenses/<package name>/`, and fails if the package has none. For
   an input under `web/` whose folder has a `LICENSE` (the Seti folder), it
   copies it to `<outdir>/licenses/<folder name>/LICENSE`. It adds
   `@vscode-elements/webview-playground`'s license for `theme.css`.

Nothing in the output depends on time or the checkout path. A trial with this
configuration on bun 1.4.2 (Preact, codicons, and Shiki, split into a chunk)
gave identical output from two directories. `make check` builds into a
temporary directory, then runs `diff -r dist <tmp>`. On a difference it prints
`internal/ui/dist is stale; run make ui` and fails. Then it runs `bun test`.

`assets.go` changes `//go:embed static` to `//go:embed dist` and roots
`assets()` at `dist`. `scripts/dev` prunes `internal/ui/dist` from
`sources()` instead of excluding `static/ui.css`, and no longer passes
`HYPERCUBE_DEV_RELOAD`. Its revision checksum still decides when to rebuild.

### Modules

All paths are under `internal/ui/web/`. Each `.jsx` component has a `.css`
file with the same base name next to it, unless the table says otherwise.

| Module | Owns |
| --- | --- |
| `index.html` | Markup only: the three stylesheet links, `<div id="app">`, and the `main.js` module script |
| `main.jsx` | Startup: imports `base.css` and `elements.js`; creates the store with the hash as `selected`; starts the data socket and file polling; tracks visibility; renders `App` |
| `elements.js` | Registers the vscode-elements in the Controls table |
| `store.js` | `createStore`, `StoreContext`, `useStore` |
| `api.js` | Every HTTP call: `post(path, params, body)` (alerts the refusal, returns text or null), `fetchLeads`, `answerRequest`, `listFiles`, `readFile` |
| `data-socket.js` | The `/state` socket: applies state and transcript messages; reloads on a new build; announces the attached session on open and on change; reconnects after 1 s |
| `images.js` | Prompt image rules: accepted types, the 10 MiB total, `imageSrc`, `imageBytes`, `readImage` |
| `base.css` (no component) | Reset, body, scrollbars, font settings, `--size-*` |
| `codicon.css` (no component) | Imports `@vscode/codicons/dist/codicon.css` |
| `favicon.svg` | The Hypercube icon, also the menu bar icon through `Icon()` |
| `app/app.jsx` | Page frame: title bar, sidebar/center/right panel splits, center/terminal split, notification stack; sets `document.title` and the URL hash from the store; refuses tab drags outside the editor area and file drops outside agent panes |
| `app/title-bar.jsx` | The three layout toggles |
| `app/right-panel.jsx` | Workers, Files, and Changes tabs (Changes is empty) |
| `app/workbench.js` | Transitions for region visibility, the right panel tab, and closing the panel |
| `shared/split.jsx` | `vscode-split-layout` with a collapsible pane |
| `shared/context-menu.jsx` | `vscode-context-menu` at a point; closes on select, Escape, a press outside, or window blur |
| `shared/status-dot.jsx` | A titled dot (waiting, exited, unseen), used by `AgentStatus` and the sidebar's session rows |
| `shared/agent-status.jsx` | An agent's indicator: exited dot, waiting dot, or busy spinner |
| `shared/use-tree-open.js` (no CSS) | The tree expansion observer |
| `sessions/selection.js` | Workspaces, selection, pending, unsent, attach; selectors `allSessions`, `currentSession`, `sessionOfAgent`, `failed`, `shownAgents`, `pageTitle` |
| `sessions/session-actions.js` | `chooseSession`, `startSession`, `stopSession`, `deleteSession` (confirm), `renameSession`, `markSeen`, `addWorkspace`, `deleteWorkspace` (confirm), `createSession` |
| `sessions/session-view.jsx` | The center: the tesseract when nothing is shown; otherwise the lead pane beside the editor area (collapsed with no tabs); posts `seen` while the shown session is unseen and the page visible |
| `sidebar/sidebar.jsx` | The Workspaces view: tree, collapse, five-at-a-time paging, menus, New Workspace |
| `sidebar/session-item.jsx` | A session row: title, dimmed when stopped, indicators with names in their titles, double-click rename |
| `sidebar/sidebar-menus.js` (no CSS) | Session and workspace menu items, and the lead picker (fetch, stale-request guard, `openUnsent`) |
| `agent/agent-pane.jsx` | One agent: transcript, composer, attention border, image drop target |
| `agent/composer.jsx` | Text box, thumbnails, attach/stop/send, config line, usage, Enter to send, focus on mount |
| `agent/drafts.js` | Drafts: transitions, `attachImages`, `sendPrompt` with restore on refusal |
| `agent/usage.jsx` | Context ring, `845 / 84k / 1.2M` tokens, cost |
| `transcript/transcripts.js` | Transcript state: `receivedTranscript`, `setExpanded` |
| `transcript/transcript-tree.js` | Items to rows, tool titles and summaries, clipping |
| `transcript/transcript.jsx` | The scrolling list and following the end |
| `transcript/transcript-item.jsx` | Chooses a row's component; draws text and error items |
| `transcript/message-item.jsx` | User prompts and agent messages, Show more/less |
| `transcript/reasoning-item.jsx` | Thinking: six-line preview and expansion |
| `transcript/tool-items.jsx` | Tool runs and tool calls with input, output, images, and children |
| `transcript/attention-item.jsx` | An attention request, open or answered |
| `transcript/attention-form.jsx` | Choices, typed answer, and sending through `answerRequest` |
| `transcript/markdown.jsx` | Marked tokens to Preact elements; `safeHref`; `chat` and `preview` styles; preview code blocks use `HighlightedCode` |
| `editor/tab-layout.js` | The pure pane tree |
| `editor/editor.js` | Tab transitions on `session.tabs` and `session.scroll`; worker and file tab entries |
| `editor/editor-area.jsx` | Renders the pane tree as splits and panes |
| `editor/editor-pane.jsx` | A pane: tab strip, active body, edge drop targets |
| `editor/editor-tabs.jsx` | The tab strip: select, close, keys, drag, double-click to keep open, preview and deleted styles |
| `editor/use-scroll-memory.js` | Saves a view's scroll to the store on unmount and restores it on mount |
| `files/files.js` | Listing and open-file state, `loadFolder`, `readOpenFile` |
| `files/file-polling.js` | The one-second sequential poller |
| `files/files-panel.jsx` | The Files tree |
| `files/file-view.jsx` | One open file: message, Markdown preview, or code with gutter |
| `files/highlighted-code.jsx` | Lazy Shiki highlighting through a ref |
| `files/highlight.js` | Shiki setup (from `viewer.js`) |
| `files/language.js` | File name to VS Code language ID |
| `files/file-icon.jsx` | Seti icon lookup and glyph (`@font-face` in its CSS) |
| `files/seti/` | Vendored `seti.woff`, `vs-seti-icon-theme.json`, `LICENSE` |
| `workers/workers-panel.jsx` | Worker cards: name, indicator, Open Transcript, report clamped to four lines, its age |
| `terminal/terminal-panel.jsx` | The shell socket: connect, replay reset, reconnect, exit, resize messages, fit on resize, focus |
| `terminal/xterm.js` (no CSS) | xterm setup: WebGL renderer and its repaint fix, fit, theme from `--vscode-terminal-*`, macOS editing keys |
| `notifications/notifications.js` | `requests`, `dismissed`, `cards`, `closeCard`, `dismiss` |
| `notifications/notification-stack.jsx` | The stack in the top right |
| `notifications/notification-card.jsx` | One card and its reply timer |
| `tesseract/tesseract.jsx` | The WebGL tesseract backdrop, drawn while mounted |

Files outside `web/` under `internal/ui/`: `assets.go`, `build.js`,
`jsconfig.json`, `bunfig.toml` (preloads `test-dom.js` for tests),
`test-dom.js` (registers happy-dom), `package.json`, `bun.lock`, and the
generated `dist/`.

### Behavior inventory

Every behavior of HEAD, and the new cards, with the module that owns it.

| Behavior | Module |
| --- | --- |
| Data socket reconnects after 1 s; announces the attached session on open; the server resends whole transcripts | `data-socket.js` |
| A page reloads when the server's browser build changes (replaces dev reload) | `data-socket.js` |
| Selection from the URL hash; the hash follows selection | `main.jsx`, `app/app.jsx` |
| Selection rules, pending, unsent discard, attach and its resets, pruning departed agents | `sessions/selection.js` |
| Selecting a stopped or failed session starts it | `sessions/session-actions.js` |
| Page title `(waiting) title — workspace — Hypercube` | `selection.pageTitle`, `app/app.jsx` |
| Mark seen while shown, unseen, and visible | `sessions/session-view.jsx` |
| Tesseract when nothing is shown; it stops when hidden | `tesseract/tesseract.jsx` |
| Workspace rows: folder icons, path tooltip, collapse | `sidebar/sidebar.jsx` |
| Five sessions per workspace, Show more adds five | `sidebar/sidebar.jsx` |
| Session row: dimmed when stopped, selected, busy spinner and amber dot naming agents, blue dot when unseen and not selected, fade over the title | `sidebar/session-item.jsx` |
| Rename: double-click; Enter or blur saves a trimmed, nonempty, changed title; Escape cancels; the row holds still while renaming | `sidebar/session-item.jsx`, `session-actions.renameSession` |
| Session menu: Start or Stop (running and not failed), Delete with confirm | `sidebar/sidebar-menus.js` |
| Workspace menu, by right-click or More Actions: Delete with confirm | `sidebar/sidebar-menus.js` |
| New Session lead picker; an unsent session replaces any other | `sidebar/sidebar-menus.js`, `selection.openUnsent` |
| New Workspace button | `sidebar/sidebar.jsx`, `session-actions.addWorkspace` |
| Menu keys, closing, and ignoring a stale lead fetch | `shared/context-menu.jsx`, `sidebar/sidebar-menus.js` |
| Refused actions alert their reason | `api.post` |
| Lead in its own column; editor area appears with the first tab and goes with the last; divider | `sessions/session-view.jsx`, `shared/split.jsx` |
| Unsent lead pane with the profile's harness, model, and effort; first prompt creates and selects the session; text typed meanwhile moves to the new lead | `agent/drafts.js`, `session-actions.createSession` |
| Attention border on a pane | `agent/agent-pane.jsx` |
| Enter sends; Shift+Enter, modifiers, and IME composition do not | `agent/composer.jsx` |
| Send disabled without content, while images are read, when exited, while the unsent session sends | `agent/composer.jsx` |
| Stop shown while busy (not unsent); interrupts | `agent/composer.jsx` |
| Images by button or by drop anywhere on the pane; type, 10 MiB, and read-error alerts; thumbnails with remove | `agent/drafts.js`, `agent/composer.jsx`, `agent/agent-pane.jsx`, `images.js` |
| Drafts survive session switches and arriving messages; a refused prompt returns unless another was started | `agent/drafts.js` |
| Config line and usage (ring, tokens, `<$0.01`) | `agent/composer.jsx`, `agent/usage.jsx` |
| Lead indicator beside the config; worker indicators in tabs and cards | `shared/agent-status.jsx` |
| Items replace in place by ID, ordered by `seq`; reset replaces all | `transcript/transcripts.js` |
| User prompts (From, images), messages to agents (To, spinner, failure), text with images, errors | `transcript/message-item.jsx`, `transcript/transcript-item.jsx` |
| Long agent messages clamp to six lines or 600 characters; Show more and less | `transcript/message-item.jsx` |
| Thinking shows its last six lines; expands and collapses | `transcript/reasoning-item.jsx` |
| Tool calls: title, spinner or failed icon, input and output clipped at 20,000 characters, images, nested items | `transcript/tool-items.jsx`, `transcript-tree.js` |
| Tool runs: the latest call, then Worked for Ns; spinner while running; nested runs | `transcript/transcript-tree.js`, `transcript/tool-items.jsx` |
| Hypercube tool titles; other servers as `server - tool` | `transcript/transcript-tree.js` |
| Follow the end; stop on scrolling up; resume at the end | `transcript/transcript.jsx` |
| Open requests open their enclosing tool calls and runs | `transcript/transcripts.js` |
| Open request: warning style, choices, typed answer, disabled while sending, enabled again on refusal | `transcript/attention-item.jsx`, `transcript/attention-form.jsx` |
| Answered request: muted question, answer beside a check | `transcript/attention-item.jsx` |
| Safe Markdown: no raw HTML; only http, https, and mailto links, opened in a new tab; tables, task lists, list starts; streamed code blocks | `transcript/markdown.jsx` |
| Open Transcript opens and focuses the worker; new tabs use the first pane | `editor/editor.js`, `workers/workers-panel.jsx`, `agent/composer.jsx` |
| The lead's composer is focused when a session attaches | `agent/composer.jsx` |
| Drag a tab to a strip (before a tab) or an edge (within 25%) to split; drop overlay; sole-tab no-ops; moves stay in the area | `editor/editor-pane.jsx`, `editor/editor-tabs.jsx`, `editor/tab-layout.js` |
| Close a tab, leaving its agent running; focus the next selected tab; the last close returns focus to the lead | `editor/editor-tabs.jsx` |
| Tab keys: arrows, Home, End | `editor/editor-tabs.jsx` |
| Tab contents: agent indicator; file Seti icon, path title, italic preview, struck-through `(deleted)` | `editor/editor-tabs.jsx` |
| Scroll kept per tab across hides and moves | `editor/use-scroll-memory.js` |
| Resizable splits | `shared/split.jsx`, `editor/editor.js` |
| Refuse tab drags outside the editor area and file drops outside panes | `app/app.jsx` |
| Files tree: folders first, numeric names, listed on first expand, no children when unreadable | `files/files-panel.jsx`, `files/files.js` |
| Click opens a preview; double-click on the file or preview tab keeps it open; an open file selects its tab | `editor/editor.js`, `files/files-panel.jsx`, `editor/editor-tabs.jsx` |
| Tree keys | `vscode-tree` in `files/files-panel.jsx` |
| One-second polling of visible listings and file tabs; sequential; 404 and failure handling | `files/file-polling.js`, `files/files.js` |
| File view: messages, Markdown preview, highlighted code with gutter, deleted file keeps its contents, scroll kept across refresh | `files/file-view.jsx`, `files/highlighted-code.jsx` |
| Languages and Seti icons | `files/language.js`, `files/file-icon.jsx` |
| Right panel tabs with keys | `app/right-panel.jsx` |
| Worker cards, No workers yet., No update yet., age as now/3m/2h/1d | `workers/workers-panel.jsx` |
| Title bar toggles with VS Code's titles and icons | `app/title-bar.jsx`, `app/workbench.js` |
| Terminal: starts when the panel first shows; one per session; reconnect and replay; exit or failed connect closes the panel; hiding keeps the shell; fit and resize; editing keys; focus; panel divider | `terminal/terminal-panel.jsx`, `terminal/xterm.js`, `shared/split.jsx` |
| Cards for requests and unseen replies outside the viewed session, with workspace, session, and worker | `notifications/notifications.js`, `notifications/notification-card.jsx` |
| Answer from a card through the transcript's path | `transcript/attention-form.jsx`, `api.answerRequest` |
| View Session; close marks a reply seen and hides a request; replies close after 15 s and when the lead works again; answered requests go | `notifications/notification-card.jsx`, `notifications/notifications.js` |

### Tests

`bun test` runs from `internal/ui` with happy-dom preloaded. Component tests
render with Preact into happy-dom and simulate events. They read DOM,
attributes, and the store, and they never depend on vscode-elements
internals.

Ported cases (same behavior, new modules):

- `tabs.test.js` → `editor/tab-layout.test.js`: first-pane opening, closing,
  moves, reordering, edge splits, sole-tab no-ops, nested first pane, preview
  replaced in place. Also check that each function returns a new tree and
  leaves its input unchanged.
- `transcript.test.js` →
  - `transcript/markdown.test.jsx`: blocks, streamed code, lists, tables,
    `safeHref`, and no raw HTML;
  - `transcript/transcript-tree.test.js`: `toolSummary`, `toolTitle`,
    `isLong`, grouping, durations, replay, nesting, message tools, mixed
    failures;
  - `transcript/transcript.test.jsx`: in-place replacement, tool and error
    text, attention flow and refusal, reveal, following, long messages,
    Thinking, images;
  - `images.test.js`: `imageSrc`;
  - `agent/composer.test.jsx`: `submitsOnKey`.
- `files.test.js` → `api.test.js` (list and read mapping),
  `files/file-icon.test.jsx`, `files/language.test.js`,
  `files/highlight.test.js`.

New cases:

- `sessions/selection.test.js`:
  - a page without a hash selects a running session;
  - pending waits for its state;
  - a session that stops while shown is deselected;
  - a removed selection moves to a running one;
  - unsent is discarded with its workspace;
  - attaching replaces `session`;
  - a departing agent loses its tab, transcript, and scroll;
  - `pageTitle` counts waiting agents.
- `transcript/transcripts.test.js`:
  - ignores messages for another session;
  - reset and upsert;
  - orders by `seq`, including an update that arrives out of order;
  - an open request expands its ancestors.
- `agent/drafts.test.js`:
  - type and size limits;
  - the send button waits for reads;
  - a refused prompt restores unless a new draft exists;
  - the unsent draft moves to the created lead.
- `editor/editor.test.js`: the preview tab is replaced, kept open, and selected
  if already open; Open Transcript makes the worker's tab active;
  `resizeSplit` clamps.
- `files/file-polling.test.js`, with a fake clock and `api` stubs:
  - nothing while hidden or unattached;
  - only visible folders and active file tabs;
  - one request at a time, skipped while a poll runs;
  - a 404 listing collapses its folder;
  - results for an old session are dropped.
- `notifications/notifications.test.js`:
  - none for the selected session;
  - order;
  - keys;
  - dismissal pruning for requests and replies;
  - closing a reply posts `seen` and closing a request does not;
  - a reply card closes after 15 s without `seen`.
- `data-socket.test.js` with a fake `WebSocket`:
  - state and transcript messages reach the store;
  - `select` is sent on open and when the attached session changes;
  - it reconnects after close;
  - a changed `build` reloads the page and the first one does not.
- `sidebar/sidebar.test.jsx`:
  - Show more;
  - collapse through the `open` attribute;
  - rename with Enter, Escape, and blur;
  - menu items for running, stopped, and failed sessions;
  - Space in the rename field does not reach the tree.
- `editor/editor-tabs.test.jsx`: close focuses the next tab, and closing the
  last focuses the lead's text box; the arrow keys work and keep the focus on
  the tabs; double-click keeps a preview open; deleted and preview classes.
- `agent/composer.test.jsx`: a composer focuses its text box when it mounts,
  except while the focus is in a tab strip.
- `shared/split.test.jsx`: collapsing keeps both children mounted.
- `agent/usage.test.js` and `workers/workers-panel.test.jsx`: token and age
  formats.

Build:

- `make check` passes.
- Edit any file under `web/` without running `make ui`, and `make check` fails
  with the stale message.
- A different bun version fails `make ui` with the version message.
- Determinism, checked once by the implementer and recorded in the work log:
  `git worktree add` two checkouts of the branch at different paths, run
  `bun build.js dist` in each, and `diff -r` the two `dist` directories
  (expect no output). Also diff each against the committed `dist`.

The user judges how the view looks with `make dev` after the lead merges both
halves.

### Tasks

1. **Dependencies and build.**
   - Update `package.json`: dependencies, `packageManager`, and no
     Tailwind. Regenerate `bun.lock`.
   - Add `jsconfig.json`, `bunfig.toml`, `test-dom.js`, and `build.js`.
   - Replace the `ui` target in `Makefile`. Add the staleness check to
     `check`, before `bun test`.
   - Update `scripts/dev` and `assets.go`.
2. **Split check.** Build a minimal `web/` with `index.html`, `base.css`,
   `codicon.css`, `elements.js`, `store.js`, `shared/split.jsx`, and
   `app/app.jsx` with placeholder regions. Run `make dev` and confirm that
   `Split` collapses and restores the sidebar, right panel, and terminal panel
   without remounting their content. Stop and report to the lead if it cannot.
3. **State.** `store.js`, `api.js`, `images.js`, `sessions/selection.js`,
   `transcript/transcripts.js`, `transcript/transcript-tree.js`,
   `agent/drafts.js`, `editor/tab-layout.js`, `editor/editor.js`,
   `files/files.js`, `notifications/notifications.js`, and
   `app/workbench.js`, with their tests.
4. **Effects.** `data-socket.js`, `files/file-polling.js`,
   `sessions/session-actions.js`, and `main.jsx`, with their tests, against
   the protocol above.
5. **Components**, each with its stylesheet and tests, in this order:
   - shared controls;
   - title bar, sidebar, and menus;
   - session view, agent pane, composer, and usage;
   - transcript components and Markdown;
   - editor area, panes, and tabs;
   - files panel, file view, and highlighting;
   - workers panel and right panel;
   - terminal;
   - notifications;
   - tesseract.
6. **Cleanup.** Delete the files below. Run `make ui`, then `make check`.
   Then run the two-checkout determinism check.
7. **Documentation.** Update `docs/usage.md` (below).

### Files to delete

- `internal/ui/static/`: the whole directory. `seti.woff`,
  `vs-seti-icon-theme.json`, and `seti-LICENSE` (as `LICENSE`) move to
  `web/files/seti/`, and `favicon.svg` moves to `web/`. The `*-LICENSE` copies
  are replaced by `dist/licenses/`.
- `internal/ui/style.css`, `transcript.js`, `terminal.js`, and `viewer.js`.
- `internal/ui/files.test.js`, `tabs.test.js`, and `transcript.test.js`.
  Their cases are ported.

### Documentation updates

`docs/usage.md`:

- Add the notification cards after the session row indicators. Use
  `bc1fd9c`'s wording without the orchestrator: cards stack in the top right
  for unseen lead replies and for questions and approval requests outside the
  viewed session. Each shows the workspace, the session, and the worker when
  a worker asks. The eye opens the session. Closing a reply's card marks it
  seen; closing a request's card only hides it. A reply's card closes by
  itself after 15 seconds or when its lead works again. A request's card
  stays until the request is answered or the card is closed.
- In the chat paragraph, add that an answered request shows its question and
  the answer beside a check.
