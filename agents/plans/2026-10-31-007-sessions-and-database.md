# Workspaces hold sessions; sessions start and stop; one SQLite database

Builds on `2026-10-31-005-start-at-login.md` and
`2026-10-31-006-lead-profiles.md`: start from branch
`hypercube/4a3c98f7/kwork-006` (commit `345f38e`), which contains both.

## Goal

Hypercube's model is what the user sees in the browser view, and tmux only runs
terminals.

- A **workspace** is a folder that holds sessions. It is added and deleted, and
  is never started or stopped.
- A **session** is a lead and its workers. The user creates, starts, stops,
  renames, and deletes it. A stopped session has no processes and stays listed
  and stopped until the user starts it.
- An **agent** is one harness in one pane of its session.
- Everything Hypercube knows lives in one SQLite database. tmux holds only live
  terminals, and the view reads Hypercube's data from the app process, never
  from tmux.
- At login, Hypercube starts the sessions that were running when it last
  exited. Each one resumes its conversations and waits for the user's prompt
  (plan 005's rule).

The JSON state files and their contents are dropped; no import.

## Lifecycles

- **App process** (`hypercube ui`, run by the login service with `--menu`). The
  only process that adds or deletes workspaces and creates, starts, stops, or
  deletes sessions, through the view's HTTP endpoints. At startup it starts
  every session marked running whose tmux session is missing. It pushes the
  database's contents to every open page whenever they change.
- **tmux server.** Started by the first session the app starts. Holds one tmux
  session per running Hypercube session, named `hypercube-<session ID>`, and
  outlives the app process.
- **tmux session of a Hypercube session.** Created by Start with the lead's
  pane, closed by Stop or Delete. Its first window holds the agents' panes; a
  second window, created by the view on demand, holds the terminal panel's
  shell.
- **Agent pane.** As today: `hypercube agent` replaces itself with the harness;
  `pane-died` runs `hypercube exited`, which relaunches it. The lead's MCP
  server adds and removes worker panes in its own running session.
- **Database** (`$XDG_STATE_HOME/hypercube/hypercube.db`). Holds workspaces,
  sessions, agents, and queued messages. Every Hypercube process opens it: the
  app process, MCP servers, hooks, and waiters.

## Related code

- `internal/store/store.go` — the JSON `State`, `Update`, `Load`,
  `SessionLock`, `Find`, `Take`, `Remove`, `Title`, `AwaitingUser`.
- `cmd/hypercube/main.go` — workspace commands (`start`, `stop`, `delete`,
  `status`, `running`, `window`), `openWorkspace`, `deleteTeam`, `retitle`,
  `restoreWorkspaces`, `serveUI`, `runAgent`, `exited`.
- `internal/tmux/tmux.go` — `NewPane` (session, window, or split),
  `WorkspaceSession`, `FindWindow`, `FindPane`, `Owns`, `Dead`, `Restart`,
  `ConfigureSession`, `Activity`, `markReply`, `RefreshActivity`,
  `RenameWindow`, `literal`.
- `internal/harness/harness.go` — `LaunchInput` (`Session`, `Env` with
  `HYPERCUBE_STATE`, `StateDir`), `Command`, `Launch`.
- `internal/mcp/mcp.go` — `Serve` reads `HYPERCUBE_STATE`; `createAgent` and
  `removeAgent` hold the session lock and use `tmux.FindWindow`/`FindPane`;
  `sendMessage` scopes lead-to-lead messages to one state file.
- `internal/report/report.go` — `Run` updates the state and calls
  `tmux.RefreshActivity`; `prompt` titles the session and renames its window;
  `logTool` logs `store.Workspace(path)`.
- `internal/wait/wait.go` — polls the state file.
- `internal/ui/ui.go` — `events` attaches to a tmux session picked by `pick`;
  `window` and `workspaceAction` shell out to `hypercube` commands; `leads`.
- `internal/ui/static/index.html` — `refresh` builds the sidebar, cards, and
  title from `list-sessions`, `list-windows -a`, and `list-panes` with
  `@hypercube_*` options; `@hypercube_open` keeps a card's open state;
  `renderSidebar`, `sessionRow`, `card`, the context menu, `leadMenu`,
  `createTerminal` and `renderPanel` (one terminal per workspace).
- `plugins/*/main.go` — read `in.Session` and report a `session` field.
- `internal/workspace/workspace.go` — `At`, `Name`, `Spell` (workspace naming
  for state files and tmux names), worktree functions.
- `scripts/dev` — `stop` runs `hypercube stop --all`; `clean` reads OpenCode
  conversation IDs from the JSON files.

## Decisions

### Names

- "Session" means only a Hypercube session. The harness's saved conversation
  is a **conversation** everywhere: `Agent.Conversation`,
  `LaunchInput.Conversation` (`"conversation"`), and the `conversation` field
  of `prompt` reports. `Team` and `SessionID` disappear.
- An agent's address stays `session/name`, using the session ID.

### Database

`internal/store` opens `hypercube.db` with `modernc.org/sqlite` (pure Go) through
`database/sql`, in WAL mode with a 5-second busy timeout and immediate
transactions (`_txlock=immediate`), so concurrent hooks wait instead of
failing. `Open` creates the tables if they are missing; a schema change means
deleting the file, since there is no backward compatibility.

```sql
CREATE TABLE workspaces (
  id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE, no_git INTEGER NOT NULL,
  created INTEGER NOT NULL);
CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  title TEXT NOT NULL DEFAULT '', running INTEGER NOT NULL,
  awaiting_user INTEGER NOT NULL DEFAULT 0, unseen INTEGER NOT NULL DEFAULT 0,
  created INTEGER NOT NULL);
CREATE TABLE agents (
  id TEXT PRIMARY KEY,
  session TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  name TEXT NOT NULL, role TEXT NOT NULL, harness TEXT NOT NULL,
  model TEXT NOT NULL, effort TEXT NOT NULL, dir TEXT NOT NULL,
  branch TEXT NOT NULL DEFAULT '', conversation TEXT NOT NULL DEFAULT '',
  pane TEXT NOT NULL DEFAULT '', activity TEXT NOT NULL,
  attention INTEGER NOT NULL DEFAULT 0, waiter INTEGER NOT NULL DEFAULT 0,
  launched INTEGER NOT NULL DEFAULT 0, quick_exits INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT '', status_at INTEGER NOT NULL DEFAULT 0,
  context INTEGER NOT NULL DEFAULT 0, window INTEGER NOT NULL DEFAULT 0,
  cost REAL NOT NULL DEFAULT 0, created INTEGER NOT NULL,
  UNIQUE (session, name));
CREATE TABLE messages (
  id INTEGER PRIMARY KEY,
  sender TEXT NOT NULL,
  recipient TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  text TEXT NOT NULL);
```

- `running` is what the user last chose: Start sets it, Stop clears it, and
  logout leaves it set, so the next login restores the session.
- `awaiting_user`, `unseen` (a lead reply the user has not seen), and `title`
  are session facts, so they move off the lead.
- The Go API is plain structs (`Workspace`, `Session`, `Agent`, `Message`,
  `Usage`) and functions on a transaction: `db.Tx(func(tx *store.Tx) error)`
  commits when the function returns nil. `Tx` has the reads and writes the
  callers below need (`Workspace`, `Workspaces`, `Session`, `Sessions`,
  `Agent`, `Agents`, `Find(session, address)`, `Add…`, `Save…`, `Delete…`,
  `Queue`, `Take`, `Queued`). `store.Snapshot(db)` reads everything the view
  shows in one transaction.
- Session lifecycle operations also take a per-session lock file,
  `$XDG_STATE_HOME/hypercube/locks/<session>.lock` (`store.Lock(id)`), so a
  slow launch never holds a database transaction while hooks wait.
- Agents get only `HYPERCUBE_AGENT_ID`. Every process finds the database
  through `store.Path()`, from the `XDG_STATE_HOME` it inherits.
  `HYPERCUBE_STATE` goes away.

### Sessions (`internal/session`, new)

The lifecycle moves out of `cmd/hypercube` into `internal/session`, so the view,
the MCP server, and the CLI's internal commands call the same functions. Each
takes the session lock.

- `AddWorkspace(db, dir) (store.Workspace, error)` — resolves the Git root
  with `workspace.At`, or uses the folder outside Git, and returns the existing
  workspace when the path is already added. It creates no session.
- `Create(db, workspaceID, lead string) (store.Session, error)` — adds a session
  with `running` set and a lead from `cfg.Lead(lead)`, then starts it.
- `Start(db, id)` — does nothing if its tmux session exists. Otherwise it sets
  `running`, sets `awaiting_user` unless the session is new, clears each
  agent's `pane`, `attention`, and `quick_exits`, records the lead's branch, and
  launches the lead (creating the tmux session) and then each worker, with no
  prompt. A failed launch closes the tmux session and leaves `running` set, so
  the view shows the error and the user can retry.
- `Stop(db, id)` — clears `running` and closes the tmux session. Conversations
  and queued messages stay.
- `Delete(db, id)` — the current worktree safety checks, then Stop, worktree
  removal, and deleting the session's rows.
- `DeleteWorkspace(db, id)` — deletes each session, then the workspace.
  Deleting a workspace's last session leaves the workspace.
- `Rename(db, id, title)` — database only.
- `AddWorker` and `RemoveWorker` — `createAgent` and `removeAgent`'s lifecycle
  halves, refusing a stopped session.
- `Restore(db)` — `Start` for each session with `running` set and no tmux
  session; errors are logged per session.
- `Exited(db, agentID, pane)` — today's `exited`, keyed by agent ID.

### tmux

`internal/tmux` knows tmux sessions and panes, not workspaces or titles.

- `Name(sessionID)` is `hypercube-<id>`. `Running(sessionID)` is true when that
  tmux session exists and its `@hypercube_session` option equals the ID, so a
  user's own tmux session with that name is never touched.
- `NewSession(sessionID, dir, env, argv)` creates the tmux session with the
  launch environment (`new-session -e`), sets `@hypercube_session`, and starts
  the first pane. `AddPane(sessionID, …)` splits the agent window. Both set
  the pane's `@hypercube_agent`, `remain-on-exit`, `extended-keys`,
  `main-pane-width`, the `main-vertical` layout, and the `pane-died` hook
  `hypercube exited #{@hypercube_agent} #{pane_id}`.
- `Owns(pane, agentID)` checks the pane's `@hypercube_agent` and its tmux
  session's name; `FindPane`, `KillPane`, `KillSession`, `Dead`, `Respawn`, and
  `Restart` keep their jobs.
- Deleted: `WorkspaceSession`, `FindWindow`, `ConfigureSession`, `Activity`,
  `markReply`, `RefreshActivity`, `RenameWindow`, `literal`, and every
  `@hypercube_*` option other than `@hypercube_session`, `@hypercube_agent`,
  and the view's `@hypercube_terminal`.

### Messages and turns

- `report`, `wait`, and `mcp` read and write the database only. Nothing copies
  activity, status, usage, attention, or titles into tmux.
- `prompt` titles the session (`sessions.title`) when it has none and clears
  `awaiting_user`. `stop` sets `sessions.unseen` when the session's lead becomes
  idle.
- `wait.Wait` polls its agent's row, `Queued`, and its session's
  `awaiting_user` every 250 ms, and delivers in one transaction, writing before
  committing, as today.
- Lead-to-lead messages stay limited to leads of sessions in the same
  workspace. `list_agents` lists those leads. `send_message` to a stopped
  agent says the message is delivered when its session starts.

### Browser view

The view has two connections: Hypercube's data from the app process, and the
selected session's terminals from tmux.

- `GET /state` is an event stream. On connect, and whenever the database
  changes, it sends the snapshot as JSON: workspaces sorted by folder name, each
  with its sessions in creation order, each with `running`, `unseen`, `title`,
  and its agents (`id`, `name`, `role`, `harness`, `model`, `effort`, `dir`,
  `activity`, `attention`, `status`, `statusAt`, `usage`). The app checks
  `PRAGMA data_version` every 250 ms on its own connection and sends only
  snapshots that differ from the last one.
- `GET /events?session=<id>` attaches a tmux control client to that session's
  tmux session, or fails with 409 when it is not running. The page uses that
  connection only for terminals: output, keys, pane sizes and layout, and the
  panel's window. It reads only `#{pane_id}`, geometry, `#{pane_active}`,
  `#{pane_dead}`, `#{pane_dead_status}`, `#{@hypercube_agent}`, and the
  windows' `#{@hypercube_terminal}`.
- Actions are `POST /workspace?action=new|delete` and
  `POST /session?action=new|start|stop|delete|rename|seen`, which call
  `internal/session` directly and answer 409 with the error text. `pick`,
  `window`, and the `hypercube` subprocess helper are deleted.
- The sidebar, document title, worker cards, spinners, and dots come from
  `/state`, so stopped sessions are listed. A stopped session's row is dimmed.
  The session context menu has Start or Stop, and Delete. Selecting a stopped
  session shows an empty center with one **Start Session** button. The URL
  hash is the session ID.
- A session shows a blue dot while `unseen` is set. The page posts `seen` for
  its session whenever it shows that session while the page is visible.
- A card's open or closed state, today `@hypercube_open`, is kept in the page's
  `localStorage` by agent ID.
- The terminal panel's shell is a window in the selected session's tmux session,
  so there is one terminal per session, started in the workspace folder and
  closed with the session.

### Command line

The browser view is the interface. `hypercube` keeps `ui [port] [--menu]` and
the internal commands `mcp`, `report <event>`, `wait`, `agent <prompt>`, and
`exited <agent> <pane>`. `start`, `stop`, `delete`, `status`, `running`, and
`window` are removed, with `workspaceAt`, `existingWorkspaceAt`,
`workspace.Name`, and `workspace.Spell`.

## Test plan

- `internal/store`: open creates the schema in a temporary directory; adding a
  duplicate workspace path or a duplicate agent name in a session fails;
  deleting a session removes its agents and their messages; `Take` returns a
  recipient's messages in order and leaves others; concurrent `Tx` calls from
  several goroutines and from a second process (the test binary) all commit;
  `Snapshot` returns workspaces by folder name and sessions in creation order.
- `internal/wait`: delivery when idle and queued; nothing while
  `awaiting_user` or busy; a newer waiter replaces an older one; a failed write
  leaves the messages queued.
- `internal/report`: `prompt` stores the conversation, titles an untitled
  session once, and clears `awaiting_user` for the whole session; `stop` from
  the lead sets `unseen`, and from a worker does not.
- `internal/mcp`: `send_message` across sessions works only lead to lead in one
  workspace; `create_agent` refuses a stopped session and a lead profile;
  `list_agents` shows the caller's session and its workspace's leads.
- `internal/ui`: `/state` sends a snapshot on connect and a new one after a
  database change; `/events` for a stopped session answers 409; each
  `/session` and `/workspace` action calls through to the database, checked by
  reading it; host and origin checks still refuse.
- `cmd/hypercube` `TestTmuxNativeLifecycle`, driven through a running
  `hypercube ui <port>` and its HTTP endpoints instead of CLI commands:
  - add a workspace (the test calls `session.AddWorkspace`, since the folder
    picker needs a person), create two sessions with different leads, and
    check that each has its own tmux session, named by ID, with the lead's
    model;
  - the existing worker, steering, wake-up, draft, relaunch, and stale-pane
    checks, adapted to agent IDs;
  - Stop one session: its tmux session is gone, the other keeps running, and
    `/state` lists it stopped; queue a message to its lead; restart the view
    process: the stopped session stays stopped;
  - Start it: it resumes its conversation, waits for the user, and delivers the
    queued message only after a typed prompt;
  - kill the view and the tmux server, as at logout, and start the view: only
    the session that was running starts again, waiting for the user;
  - rename and delete a session; deleting the last session leaves the
    workspace; deleting the workspace removes it and its worktrees.
- Plugin tests: launch passes `conversation` as each harness's resume argument,
  and reports carry `conversation`.
- Live, in `make dev`: add a workspace, create a session with each lead, stop
  and start a session, open the terminal panel in two sessions, and restart the
  view with `make dev` while sessions run.

## Implementation plan

1. `go get modernc.org/sqlite`; rewrite `internal/store` as above, with its
   tests.
2. `internal/tmux`: the session-and-pane API above; delete the rest.
3. `internal/harness`: `LaunchInput.Conversation`; `Env` with only
   `HYPERCUBE_AGENT_ID`; `Launch` returns the pane from `tmux.NewSession` for a
   session's first agent and `tmux.AddPane` otherwise.
4. `plugins/*`: read `in.Conversation` and report `conversation`.
5. `internal/session` (new): the functions above, moved and reshaped from
   `openWorkspace`, `deleteTeam`, `retitle`, `restoreWorkspaces`, `exited`,
   `createAgent`, and `removeAgent`.
6. `internal/report`, `internal/wait`, `internal/mcp`: the database, the new
   names, and no tmux copies; `logTool` logs the workspace path and session ID.
7. `cmd/hypercube/main.go`: keep `ui`, `mcp`, `report`, `wait`, `agent`, and
   `exited`; `serveUI` opens the database and calls `session.Restore` before
   serving; `runAgent` records `pane` and `launched`.
8. `internal/ui/ui.go`: `/state`, `/events?session=`, the `/workspace` and
   `/session` actions, and `/leads`.
9. `internal/ui/static/index.html`: data from `/state`, terminals from
   `/events`, stopped sessions, the Start Session button, Start/Stop in the
   context menu, `seen`, card state in `localStorage`, and a terminal per
   session.
10. `scripts/dev`: `stop` kills the dev tmux server
    (`TMUX_TMPDIR="$DEV" tmux kill-server`); `clean` reads OpenCode
    conversation IDs with
    `sqlite3 "$DEV/state/hypercube/hypercube.db" "SELECT conversation FROM agents WHERE harness = 'opencode'"`.
11. `workspace.go`: delete `Name` and `Spell`; `At` returns the root only.
12. Tests from the test plan; delete tests of removed commands.

## Documentation updates

- `docs/usage.md`: workspaces hold sessions; add a workspace with the sidebar's
  `+` and delete it from its context menu; create a session with a workspace's
  `+`; start, stop, and delete a session from its context menu, or start a
  selected stopped session with Start Session; stopping keeps conversations and
  queued messages; at login, sessions that were running start again and wait
  for your prompt; the terminal panel is per session; the command table lists
  only `hypercube ui`.
- `docs/architecture.md`: Ownership and Persistence describe one database, one
  tmux session per running session, the app process as the owner of session
  lifecycles, and the view reading data from the app and terminals from tmux.
  The source table gains `internal/session`.
- `docs/installation.md`: the login paragraph says sessions that were running
  start again; state lives in `~/.local/state/hypercube/hypercube.db`.
- `AGENTS.md`: the package list gains `session`; `cmd/hypercube/` is the command
  line and the app process.
