# Replace leads and workers with nested sessions

## Goal

A workspace has sessions, and each session has exactly one agent. A top-level
session's agent creates and ends child sessions, one level deep, in place of
today's workers. The lead/worker roles and the agents table go away from the
store, runtime, MCP tools, launch instructions, and browser view. A child's
requests and status updates also appear in its parent's transcript, for the
user only. The sidebar nests children under their parent, and selecting a child
shows the same view as any session. The worker stack goes away.

## Related code

- `internal/store/store.go` — `sessions` and `agents` tables, `Agent`, `Find`,
  `Lead`, queues, transcripts keyed by agent.
- `internal/session/sessions.go` — `Create`, `start`, `Stop`, `Delete`,
  `Restore`, `Shell`, `detachLocked`. Starting, stopping, and deleting loop
  over a session's agents; they will loop over a session and its children.
- `internal/session/workers.go` — `AddWorker`, `RemoveWorker`, `SetStatus`.
  Replaced by `children.go`.
- `internal/session/launch.go` — `live`, `register`, `startArgs`, `end`,
  `markEnded`, `note`.
- `internal/session/event.go`, `deliver.go`, `input.go`, `messages.go`,
  `inactivity.go`, `feed.go`, `transcript.go` — agent-keyed events, delivery,
  the session-wide `AwaitingUser` hold, failure reports to the lead, idle stop,
  `Change`, `Request`.
- `internal/mcp/mcp.go` — the five tools and the `X-Hypercube-Agent` identity.
- `internal/report/report.go` — instructions, `Format`, `Leads`.
- `internal/harness/config.go` — `Workers`/`Worker` profile lookups;
  `internal/harness/protocol.go` — `StartArgs.Agent` and the unused
  `StartArgs.Role`; `harnesstest/harnesstest.go` logs `Role`.
- `internal/workspace/workspace.go` — `WorkerBranch`, `WorktreeDir`, worker
  wording.
- `internal/ui/snapshot.go`, `socket.go`, `actions.go`, `files.go`, `ui.go` —
  the view's data, transcript messages, `/session` and `/agent` routes, and
  the Files root, which is the workspace folder today.
- `plugins/opencode/main.go:108` writes `<StartArgs.Agent>.md` in a state
  directory, so session IDs must stay free of `/`.
- `internal/ui/web/sessions/selection.js` — attaches only to running sessions
  and deselects a session that stops; `shownAgents`, `sessionOfAgent`, `failed`.
- `internal/ui/web/sidebar/` — rows, indicators, menus, five-at-a-time limit.
- `internal/ui/web/sessions/session-view.jsx`, `internal/ui/web/workers/` — the
  lead column and worker stack.
- `internal/ui/web/notifications/notifications.js` — request cards, which name
  the worker.
- `internal/ui/web/transcript/` — transcripts keyed by agent, attention items,
  message headings from `item.from`, hypercube tool titles.

## Decisions

### Model and schema

The `agents` table folds into `sessions`. `store.Agent`, `Agents`, `Lead`,
`Find`, `AddAgent`, `SaveAgent`, `DeleteAgent`, and the `Lead`/`Worker`
constants are deleted.

```sql
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  parent TEXT REFERENCES sessions(id) ON DELETE CASCADE,
  name TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '', running INTEGER NOT NULL,
  ended INTEGER NOT NULL DEFAULT 0,
  awaiting_user INTEGER NOT NULL DEFAULT 0, unseen INTEGER NOT NULL DEFAULT 0,
  harness TEXT NOT NULL, model TEXT NOT NULL, effort TEXT NOT NULL,
  dir TEXT NOT NULL, branch TEXT NOT NULL DEFAULT '',
  conversation TEXT NOT NULL DEFAULT '', activity TEXT NOT NULL,
  attention INTEGER NOT NULL DEFAULT 0,
  context INTEGER NOT NULL DEFAULT 0, window INTEGER NOT NULL DEFAULT 0,
  cost REAL NOT NULL DEFAULT 0,
  created INTEGER NOT NULL, last_active INTEGER NOT NULL,
  UNIQUE (parent, name));
```

`messages.recipient` and `transcript.session` (renamed from `agent`) reference
`sessions(id)`. `parent` is NULL for a top-level session; Go reads it as `""`
through `COALESCE` and writes it through `NULLIF`. The status columns are
deleted: status now lives only as transcript items.

`store.Session` gains `Parent`, `Name`, `Ended`, and the agent fields
(`Harness`, `Model`, `Effort`, `Dir`, `Branch`, `Conversation`, `Activity`,
`Attention`, `Usage`).

Family state (a top-level session and its children) has one owner:

- `Running`, `AwaitingUser`, `LastActive`, and `Unseen` are stored only on
  top-level sessions. A child's columns are never read or written.
- `Tx.Root(id)` returns the top-level session of `id`: the session itself, or
  its parent. Code reads and writes family state only through it. That code is
  `Input`, `deliver.next`, `recordActivity`/`TouchSession`, `applyEvent`,
  `start`, `stopLocked`, and the idle stop.
- `Tx.Running(s)` is `Root(s).Running && !s.Ended`, the one definition of
  whether a session's agent should run. The snapshot, `end`'s relaunch check,
  `Shell`, and `SendMessage`'s reply use it. The browser reads the snapshot's
  `running`.
- Prompting any session in a family releases the family's held messages, as
  prompting any agent of a session does today. Child activity keeps the family
  from the idle stop.
- `applyEvent` sets `Unseen` only when the session is its own root. Children
  report to their parent by message, so their turns raise no blue dot or reply
  card.

`Ended` is set only on children, and it is the single fact that a child was
ended. `Branch` and `Dir` stay as a record after the worktree is removed.

Store API:

- Kept: `AddSession`, `SaveSession` (every mutable field), `TouchSession`,
  `Session(id)`, `DeleteSession`, and the queue and transcript functions,
  keyed by session ID.
- `Sessions(workspace)` returns only top-level sessions. An empty workspace
  means every workspace.
- New: `Root`, `Running`, `Children(parent)` in creation order,
  `FindSession(caller Session, to string)`, and `DropQueued(id)`.

### Addressing and naming

- Session IDs stay random hex, and `Session.Address()` does not exist. Plugins
  put the ID in file names.
- `FindSession(caller, to)`:
  - For a top-level caller, it returns the caller's own child named `to`, or
    else the top-level session in the caller's workspace whose ID is `to`.
  - For a child caller, it returns only its parent, whose ID is `to`.
  - Anything else is not found. This one function is the messaging scope.
    `SendMessage` adds only the refusals for messaging yourself and messaging
    an ended child.
- The text the model reads names each session the way the recipient would
  address it: a child by its name to its parent, and every other session by
  its ID. That text is `report.Format`'s "Message from X … use send_message to
  X", `list_sessions`, and the instructions.
- A child's `name` follows today's rule (lowercase letters, digits, hyphens) and
  is unique among its parent's children, ended ones included. The refusal says
  that a child session with that name already exists and to choose another.
  The child's title starts as its name and can be renamed like any title; the
  name does not change.
- Worktree and branch come from `workspace.ChildBranch(parent, name)`
  (`hypercube/<parent>/<name>`) and `workspace.WorktreeDir(root, parent,
  name)` (`.hypercube/<parent>/<name>`). These are today's worker paths.
  `WorkerBranch` is deleted, and comments say "child session" where they now
  say "worker".
- A transcript item's `From` is the sender's session ID, on delivered
  messages, mirrored requests, and status items. The browser shows the
  sender's title, or the ID if the session is gone.
- The identity env var and header become `HYPERCUBE_SESSION_ID` and
  `X-Hypercube-Session`. `StartArgs.Agent` becomes `StartArgs.Session`, and
  `StartArgs.Role` is deleted. No plugin reads `Role`.
- In `harness.Config`, `Workers()`/`Worker(name)` become
  `ChildProfiles()`/`ChildProfile(name)`. Config roles and the file format do
  not change. The missing-profile error names "child profile (plan, work, or
  review)".

### Runtime ownership

- `live` keeps `id` (the session) and `root` (its top-level session's ID,
  which is `id` for a top-level session). `register(id, root, quick)`.
  `sessionLive`, `kickSessionLocked`, and `detachLocked` match on `root`, and
  `kickSessionLocked` becomes `kickFamilyLocked`.
- `detachLocked`'s `ended` flag becomes `exited`, and `markEnded` becomes
  `markExited`. "Ended" now means only a child that its parent ended.
- `Start(id)` and `Stop(id)` refuse a child ("a child session starts and stops
  with its parent"). `start` sets the top-level session's `Running`, then
  launches it and its children that have not ended, as today it launches a
  session's agents. `stopLocked` clears that one `Running` and detaches the
  family. No child row is written. `Stop` and the idle stop end the shells of
  the whole family.
- `Delete(id)`:
  - Top-level: checks `Removable` for children with `!Ended && Branch != ""`,
    then detaches the family and removes each of those children's worktrees,
    deleting that child's row after its removal succeeds. This keeps today's retry behavior. Then it deletes
    the session, which deletes its remaining children through the foreign key.
  - Ended child: deletes its row.
  - Child that has not ended: refused.
- `CreateChild(parentID, name, profile, dir, message)` and
  `EndChild(parentID, name)` in `children.go` replace `AddWorker` and
  `RemoveWorker`, with the same checks and cleanup. `EndChild` removes the
  worktree first (Git refuses dirty or unmerged), then detaches the child with
  `exited` set, saves `Ended`, and drops its queued messages. The row,
  transcript, `Dir`, and `Branch` stay.
- `ViewSession(id)` holds the family's top-level session, so viewing a child
  keeps its family from the idle stop. `StopIdleSessions` checks top-level
  sessions, and treats a family as busy if any member is busy, starting, or
  delivering.
- `Shell` and the Files routes open `Session.Dir`. A child's shell and Files show
  its worktree, and a top-level session's `Dir` is the workspace folder, as
  today.
- `Input`, `Interrupt`, and `Answer` take a session ID. `Input` clears
  `AwaitingUser` on `Root` and kicks the family.
- A child's failed turn is queued to its parent (`queueFailure`), as a worker's
  is to its lead today.

### Mirrored requests and status items

Both are ordinary items in the parent's saved transcript, written by the
runtime. The parent's agent never receives them, because transcripts are not
sent to plugins.

- **Request.** The runtime writes a copy of a child's attention item into the
  parent's transcript twice. The first time is when the item opens, which is
  its first event in `event`. The second time is when the child's item is
  saved: answered, finished, or drained when the child stops or exits.
  Streaming changes in between are not copied.
  - The copy has the same item ID, `From` set to the child's session ID, and
    `Parent` cleared, so `transcript-tree.js` does not file it under a missing
    tool call.
  - Item IDs are already unique across launches, because each launch has a
    unique prefix.
  - Each write takes its seq from `r.nextSeq(parent)`. `PutEntry`'s upsert
    keeps the first write's seq, so the record keeps its place, and no
    per-item seq map is needed.
  - The writing lives in one helper, `mirrorLocked(tx, id, item)`, which
    does nothing for a top-level session. It is called from `event` for a new
    attention item and from `write.putItem` for a saved one. To make that
    possible, `putItem` takes the session ID instead of `l`.
- **Answering.** Both the child's item and the parent's mirrored item answer
  through `Answer(child session ID, item ID)`. The browser answers a mirrored
  item at `item.from`. Answering in either place updates both, because the
  mirror follows the child's item.
- **Status.** `SetStatus(child, text)` collapses whitespace, refuses empty text
  and top-level callers, and writes an item of a new kind, `status`
  (`harness.KindStatus`, written only by Hypercube like `KindUser`), with
  `From` set to the child's ID, into the parent's transcript. Nothing is
  stored on the child.
- An app crash between a request opening and closing leaves its mirror open.
  Answering it then reports that the request is finished. This is accepted;
  shutdown and plugin exits already drain open items as finished.

### MCP tools

Each tool runs on behalf of the session named in the header. `newServer`
registers only the tools that session can use: create and end for top-level
sessions, set_status for children. The runtime still refuses wrong callers.
`to` and every listed name follow `FindSession` and the naming rule above.

- `send_message {to, text}`. Top-level description: "Send a message to one of
  your child sessions by its name, or to another top-level session in this
  workspace by its ID. This is the only way to reach other Hypercube
  sessions." Child description: "Send a message to your parent session at
  <parent ID>. This is the only way to reach it."
- `list_sessions` (was `list_agents`): for a top-level session, its children
  by name, and the workspace's other top-level sessions by ID; for a child, its
  parent by ID. Fields: `to` (what send_message takes), `title`, `harness`, `activity`, `ended`, `directory`, `branch`.
- `create_session {name, profile, dir?, message?}` (was `create_agent`):
  "Create a child session and launch its agent. …", with today's worktree,
  `dir`, and profile text, saying "child session" for "worker" and "your
  directory" for "the lead's directory". `dir` is required outside Git. The
  profile enum lists `ChildProfiles()`.
- `end_session {name}` (was `remove_agent`): "End a child session: its agent
  stops for good, and a child with a worktree also has its worktree and branch
  removed; Git refuses dirty worktrees and unmerged branches. The ended child
  stays listed for the user and cannot be messaged."
- `set_status {text}`: "Tell the user in a sentence or two what you are doing
  now. It appears in your parent session's transcript for the user; your
  parent's agent does not see it."

### Launch instructions

`report.Instructions(tx, s, profiles)` writes one of two texts. `report.Leads`
becomes `report.Peers` (the other top-level sessions in the workspace).

Top-level:

- "You are the agent of Hypercube session <id>, titled <title>. Hypercube runs
  one coding agent per session."
- You can create child sessions with create_session to work in parallel, and
  address each by its name. Children can message only you. You end them with
  end_session. You can also message the other top-level sessions in this
  workspace by their IDs, and list_sessions lists the current ones.
- Today's lines about Hypercube tools versus native subagent tools, and about
  not waiting for messages, with "sessions" for "agents".
- The working directory and branch, then `Children:` (name, harness, directory,
  branch, "ended") or "none yet", then "Other top-level sessions: id (title)".
- Today's shared-checkout paragraph, the Git and non-Git paragraphs, and the
  teamwork guidance and profile list. Use "child session" for "worker", "end"
  for "remove", and create_session for create_agent.

Child:

- "You are <name>, a child session of Hypercube session <parent>, titled
  <parent title>. Your parent created you and is the only session you can
  message: use send_message to <parent>."
- The same tools and waiting lines, and the working directory.
- With a branch: "Commit your work on your branch. Your parent merges it from
  its checkout." Without one: "You work directly in your working directory.
  Handle branches and commits as your parent tells you."
- Today's report-back paragraph, saying that set_status reaches the user in
  your parent's session, not your parent's agent, and that you send your
  parent a short summary when you finish or need something.

### Browser view

- **Snapshot.** Each workspace lists top-level sessions, ordered as today.
  Each `shownSession` is `{id, parent, title, running, ended, unseen, reply,
  replySeq, harness, model, effort, dir, activity, attention, usage, children}`,
  with `children` in creation order and empty for children. `allSessions`
  flattens parents and children.
- **Routes.** `/agent` is deleted. `input`, `interrupt`, and `answer` become
  `/session` actions on a session ID. `new` returns `{"session": id}`. The
  socket's transcript message drops `agent`, and `Change` drops `Agent`: a
  change with an empty `Session` is a data change.
- **Page state.** The attached session holds one `transcript` instead of
  `transcripts` keyed by agent, and `withoutDeparted` is deleted. `shownAgents`
  becomes `shownSession` (the unsent session's placeholder, or the attached
  session), and `sessionOfAgent` becomes `sessionOf`, which returns
  `{workspace, session, parent}`. Components that took an agent take the
  session (`session` prop); drafts and scroll are keyed by session ID. The
  `agent/` folder keeps its name: it draws the session's agent.
- **Attaching.** The page attaches to the selected session, whatever its
  state. So an ended child opens with its transcript, and stays shown when it
  ends. The existing rule still applies to sessions that have not ended: one
  that stops while shown is deselected. This includes children when their
  parent stops.
- **Choosing.** `chooseSession` on a top-level session works as today. On a
  child that has not ended, it starts the parent when the child is not running.
  On an ended child, it only selects it. `failed` applies only to top-level
  sessions.
- **Center.** `SessionView` shows the shown session's `AgentPane` and the
  editor area in one `Split`. The `workers/` folder is deleted.
- **Composer.** The composer shows `AgentStatus` for every session. `canSend`
  and `AgentStatus` check `ended` explicitly: an ended child's send button is
  disabled, and `AgentStatus` shows nothing for it, since its red dot means a
  crash.
- **Files.** `loadFolder` turns a 404 into an empty tree, with no text. When
  the root listing is missing (an ended child's removed worktree), the Files
  panel says "This session's folder no longer exists." Changes stays blank.
- **Sidebar.**
  - A top-level row with children is a branch item holding its children's rows,
    open by default. Its open or closed state is tracked like workspace rows
    (`data-key`, `useTreeOpen`).
  - Clicking a parent row selects and opens the session and leaves its
    children as they are. Only the chevron expands or collapses them.
    `vscode-tree` 2.5.1 toggles a branch on any click of its row, so the
    parent row takes over its own clicks. It adds a capture-phase click
    listener that acts only when the click's nearest `vscode-tree-item` is the
    row itself, not a child row, and stops the event before the tree sees it.
    If the event's composed path includes the row's `arrow-icon-container`
    part, it toggles the row's open state (`useTreeOpen` records it).
    Otherwise it calls `chooseSession`. Keyboard navigation, double-click
    rename, and the context menu are unchanged.
  - The five-at-a-time limit counts top-level sessions only; a shown parent
    shows all its children.
  - Rows that are not running (stopped families and ended children) are
    dimmed.
- **Row indicators.** Each row shows its own spinner. The blue dot shows only
  on top-level rows. The amber dot shows while the session,
  or one of its running children, awaits the user. A collapsed parent therefore
  still shows a waiting child.
- **Menus.** A top-level session keeps Start/Stop and Delete. Delete's detail
  says its children and their worktrees and branches are deleted too. An ended
  child offers Delete Session ("Its chat is deleted."). A child that has not
  ended has no menu, but rename still works.
- **Notification cards.** A request card names the workspace, the parent's
  title, and the child's title. It is hidden while the page shows the
  requesting session or its parent, since the parent's transcript shows the
  mirrored card. Reply cards stay top-level only. The page title counts running
  sessions with attention.
- **Transcript.**
  - A `status` row shows a pulse icon, the sender's title, the text, and its
    time.
  - An attention item with `from` names the sender's title above the question
    and answers at `item.from`.
  - Message headings show the sender's title.
  - The tool titles become Create session, End session, and List sessions.

## Test plan

- Store: the schema creates; `Sessions` returns only top-level sessions;
  `Children` returns creation order; `FindSession` gives a top-level caller its
  own child by name or a peer in its workspace by ID, and gives a child only
  its parent, refusing everything else; `Root` and `Running` give a child its
  parent's state, and `Running` is false for an ended child; a duplicate
  child name is refused; deleting a parent deletes its children, their
  messages, and their transcripts.
- Runtime, children (`children_test.go`, replacing `workers_test.go`):
  - A child gets a worktree and branch `hypercube/<parent>/<name>`, or runs in
    `dir`; outside Git `dir` is required; a child cannot create children; a
    first message is queued from the parent.
  - `EndChild` refuses a dirty worktree and keeps the child running. On
    success it removes the worktree and branch, stops the agent, keeps the row
    and transcript with `Ended`, and drops queued messages.
  - Ended children cannot be messaged, started, or relaunched.
- Runtime, family lifecycle:
  - Stopping a parent stops its children and their shells.
  - Starting it relaunches only children that have not ended, holding messages
    until a prompt to any family member releases the whole family.
  - `Start` and `Stop` refuse a child.
  - Restore starts families.
  - The idle stop counts a busy child and a viewed child.
- Runtime, delete:
  - Deleting a parent refuses, changing nothing, when a child that has not
    ended has a dirty worktree or unmerged branch. It skips ended children.
  - Otherwise it removes the children's worktrees and deletes everything.
  - Deleting an ended child removes only it.
  - Deleting a child that has not ended is refused.
- Messaging: a parent and its children, and top-level sessions in one
  workspace, can message each other. A child cannot message another child, a
  peer, or another parent. Top-level sessions in other workspaces cannot. A
  delivered message's item has `From` set to the sender's ID. The model text
  names a child by its name to its parent, and every other sender by ID.
- Mirrors:
  - An open child request appears in the parent's transcript with the child's
    ID in `From` and no `Parent`, even when the child's item is nested under
    a tool call.
  - Answering it at the child updates both, and the record stays answered in
    the parent.
  - A request drained by stopping the family closes the mirror.
  - Status writes a `status` item to the parent and nothing to the child;
    top-level callers and empty text are refused.
  - No mirror or status input reaches the parent's plugin log.
- Instructions: top-level text lists children, peers, and child profiles; child
  text names its parent, its branch rule, and set_status; neither says lead or
  worker.
- MCP:
  - A top-level session sees send_message, list_sessions, create_session, and
    end_session; a child sees send_message, list_sessions, and set_status.
  - `create_session` requires `dir` outside Git and lists child profiles.
  - `list_sessions` shows the right scope for each caller.
  - The handler refuses an unknown session header.
- UI Go:
  - The snapshot nests children with `ended`.
  - `/session` handles input, interrupt, and answer; `/agent` is gone.
  - Files serve a child's worktree and return 404 after it is removed.
  - The socket sends one transcript for the selected session.
- Browser:
  - The sidebar nests children in creation order, dims ended rows, and
    propagates the amber dot to the parent.
  - Clicking a parent row selects it without toggling its children, and
    clicking its chevron toggles them without selecting it.
  - Show more counts only parents.
  - Menus match the session kind.
  - Choosing a child of a stopped parent starts the parent.
  - An ended child attaches, shows its transcript, and cannot send.
  - A request card is hidden while the parent is shown.
  - Mirrored attention answers at `from`, and status rows render.
  - The worker stack is gone, and session-view shows one pane with files.
- Integration (`cmd/hypercube/integration_test.go`): the MCP tools create,
  message, and end a child through the stand-in; deleting and restoring keep
  working with children.
- Plugins: update `StartArgs` fixtures (`Session`, no `Role`) and the identity
  env var in the Claude, Codex, and Pi tests; OpenCode writes `<session>.md`.

## Implementation plan

1. `internal/store/store.go`, `store_test.go`: new schema, merged `Session`,
   `Root`, `Running`, `Children`, `FindSession`, `DropQueued`, session-keyed queues and
   transcripts; delete the agent API and role constants.
2. `internal/workspace/workspace.go`, `workspace_test.go`:
   `ChildBranch(parent, name)`, `WorktreeDir(root, parent, name)`, wording.
3. `internal/harness/protocol.go` (`StartArgs.Session`, no `Role`,
   `KindStatus`), `config.go` (`ChildProfiles`, `ChildProfile`),
   `harnesstest/harnesstest.go` (drop `Role`, log by session), their tests.
4. `internal/report/report.go`, `report_test.go`: `Peers`, the two instruction
   texts, and a `Format` that returns only text.
5. `internal/session/`: `launch.go` (`live.root`, `register`, `startArgs` with
   `HYPERCUBE_SESSION_ID`, `markExited`); `feed.go` (`Change` without `Agent`,
   `putItem(id, root, …)`, `mirrorLocked`); `event.go`, `deliver.go`,
   `input.go`, `messages.go`, `transcript.go`, `inactivity.go`; `sessions.go`
   (family start, stop, delete, restore, view, shell); add `children.go` and
   delete `workers.go`. Update `fixture_test.go` and every session test, and
   replace `workers_test.go` with `children_test.go`.
6. `internal/mcp/mcp.go`, `mcp_test.go`: tool set per caller, renamed tools and
   descriptions, and `SessionHeader`.
7. `internal/ui/snapshot.go`, `socket.go`, `actions.go`, `files.go`, `ui.go`,
   `ui_test.go`, `files_test.go`.
8. `plugins/opencode/main.go` and the plugin tests' `StartArgs` and env
   fixtures; the comment in `plugins/codex/main.go:107`.
9. `cmd/hypercube/integration_test.go`, plus any wording in
   `cmd/hypercube/main.go`.
10. Browser data and state: `data-socket.js`, `api.js`,
    `sessions/selection.js`, `sessions/session-actions.js`,
    `transcript/transcripts.js`, `agent/drafts.js`, `editor/editor.js`, and
    their tests.
11. Browser views: `sessions/session-view.jsx`, `agent/agent-pane.jsx`,
    `agent/composer.jsx`, `agent/focus-composer.js`, `shared/agent-status.jsx`,
    `editor/editor-pane.jsx`, `editor/editor-tabs.jsx`,
    `transcript/transcript.jsx`, `transcript-item.jsx`, `tool-items.jsx`,
    `attention-item.jsx`, `attention-form.jsx`, `message-item.jsx`,
    `transcript-tree.js`, a new `transcript/status-item.jsx` and `.css`,
    `files/files-panel.jsx`, `sidebar/sidebar.jsx`, `session-item.jsx`,
    `sidebar-menus.js`, `notifications/notifications.js`,
    `notification-card.jsx`; delete `workers/`; update the tests.
12. Docs and skill (below).

## Documentation updates

- `docs/usage.md`: sessions and child sessions instead of leads and workers;
  addressing (a parent names its children by name, everything else by ID);
  who can message whom; child ending, ended rows, and deleting ended children; family start, stop, and
  delete; sidebar nesting and indicators; mirrored requests and status in the
  parent's transcript; one-session center view; Files showing the session's
  directory; notification cards naming the child; terminal in the session's
  directory.
- `docs/configuration.md`: "worker profile" becomes "child profile"; plan,
  work, and review profiles are offered to top-level sessions for children;
  "Files needed by worker worktrees" becomes child worktrees. The JSON example
  is unchanged.
- `docs/architecture.md`: worktree and scoping paragraphs, the database
  contents (no agents table), and the source table's link to `children.go`.
- `docs/harness-support.md`: failure reports go to the parent, and the Codex
  paragraph says a child session can commit.
- `README.md`: "lead and child profiles".
- `AGENTS.md`: "Leads must use the hypercube skill" becomes "Top-level session
  agents must use the hypercube skill".
- `.agents/skills/hypercube/SKILL.md`: the description and body say "top-level
  session" for "lead" and "child session" for "worker", including planning,
  architectural review, implementation, review, and integration.
