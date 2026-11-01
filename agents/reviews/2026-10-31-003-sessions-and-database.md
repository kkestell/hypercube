# Review: sessions and one SQLite database

Plan: `agents/plans/2026-10-31-007-sessions-and-database.md`. Work log:
`agents/work/2026-10-31-003-sessions-and-database.md`. Reviewed the staged
change with an Opus subagent; `make check` passed before and after the fixes.

## Findings

1. **A session whose start failed showed no way to retry** (medium). `start`
   leaves `running` set after a failed launch, so the page hid Start Session,
   retried `/events` forever, and offered only Stop. Fixed: the page marks a
   running session whose `/events` never connects as dead, shows Start
   Session for it, and offers Start in its context menu; the next successful
   connection clears the mark.
2. **Switching sessions could run two `/send` requests at once** (low).
   `attach` reset `sending` while an older request was in flight, so replies
   could reach the wrong callbacks. Fixed: `flush` stops when a newer
   attachment has begun.
3. **`docs/harness-support.md` named the old `session` report field** (low).
   Fixed: it names `conversation`, and a worker's failure goes to its
   session's lead.

## Checked and sound

Lock ordering (session lock before any transaction, none held across launches
or tmux calls), read transactions taking no write lock, `data_version`
watching on its own connection, the `pane-died` hook quoting, Start, Stop, and
relaunch, and the page's selection, seen, card state, per-session terminal,
and context menu.
