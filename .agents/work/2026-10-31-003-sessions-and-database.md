# Sessions and one SQLite database

## Plan

`agents/plans/2026-10-31-007-sessions-and-database.md`

## Summary

Workspaces hold sessions; sessions are created, started, stopped, renamed, and
deleted through the app process's `/session` and `/workspace` endpoints, which
call the new `internal/session` package. All data lives in
`hypercube.db`; each running session has its own tmux session
`hypercube-<id>`. The view reads its data from `GET /state` and only terminals
from `/events?session=`. The CLI keeps `ui` and the internal commands. The
plan's goal is met.

## Departures from the plan

- The work started from `main` after merging `hypercube/4a3c98f7/kwork-006`
  into it, rather than from that branch, so the result lands on `main`.
- `scripts/dev up` no longer starts the scratch repository, since
  `hypercube start` is gone; the repository is added in the view with `+`.
- `workspace.Short` was deleted too: nothing uses it once tmux pane labels are
  gone.
- `Nudge` moved from `internal/mcp` to `internal/session`, because
  `session.AddWorker` launches workers and `mcp` imports `session`.
- `store` gained `DB.Read` (a read-only transaction, which modernc begins as
  deferred) for polling and snapshots, `Tx.Lead`, and `DB.Watch` for the
  `PRAGMA data_version` check, so waiters and the view take no write lock.
- `session.Seen` backs `POST /session?action=seen`.
- `report.Leads` lists a workspace's other leads for both the lead's start
  context and `list_agents`.

## Decisions

- Stop marks the session's agents exited, clears their panes, and clears
  attention, so a stopped session shows no spinners or dots.
- `stop` from a lead always sets `unseen`; the page clears it with `seen` while
  it shows that session, and never shows the dot on the selected session.
- `AddPane` finds the agent window by its marked panes rather than by window
  index, since a user's tmux `base-index` can change indexes.
- A new session is selected in the page from the ID that `action=new` returns.
- The page reconnects `/events` when the selected running session changes,
  disposing the previous session's terminals.
- Workspace rows carry the folder path as a tooltip, since two folders can
  share a name.

## Checks run

- `make check` — passed (build, vet, gofmt, `go test -race ./...`).
- `make ui` — regenerated `ui.css` for the new Start Session button classes.

## Manual verification

1. Live dev session in a fresh dev directory, since the existing
   `~/projects/hypercube-dev` config predates profile roles.

   ```sh
   HYPERCUBE_DEV_DIR=/tmp/hc-dev-smoke scripts/dev up
   sqlite3 /tmp/hc-dev-smoke/state/hypercube/hypercube.db \
     "INSERT INTO workspaces VALUES ('devws', '/tmp/hc-dev-smoke/repo', 0, 0)"
   curl -X POST -H 'Origin: http://localhost:4281' \
     'http://localhost:4281/session?action=new&workspace=devws&lead=claude-lead'
   ```

   A Claude Code lead ran in `hypercube-<id>`. Headless Chrome
   (`--use-angle=swiftshader --dump-dom`) showed the workspace row, the selected
   session, the lead's terminal, and the title `New Session — repo — Hypercube`,
   with no script errors. After `action=stop`, the row was dimmed, Start Session
   showed, and the lead was hidden. `action=start` relaunched it waiting for the
   user, and `scripts/dev up` (which kills the dev tmux server) restored it.
   `scripts/dev clean` removed everything.

   The folder picker, the terminal panel in two sessions, and clicking through
   the context menu were not exercised; the Chrome extension was not connected.

## Follow-up work

- `~/projects/hypercube-dev/config/hypercube/config.json` uses the old config
  format; `scripts/dev` writes the config only when it is missing.
