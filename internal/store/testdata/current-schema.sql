-- Installed baseline at ae022eeebf9dc90c75a61527267db0682b194057.
-- Keep independent of the schema used by Open so current-database tests cover
-- its existing constraints, including last_active without a default.
CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE, no_git INTEGER NOT NULL,
  created INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  parent TEXT REFERENCES sessions(id) ON DELETE CASCADE,
  name TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '',
  ended INTEGER NOT NULL DEFAULT 0, unseen INTEGER NOT NULL DEFAULT 0,
  harness TEXT NOT NULL, model TEXT NOT NULL, effort TEXT NOT NULL,
  dir TEXT NOT NULL, branch TEXT NOT NULL DEFAULT '',
  conversation TEXT NOT NULL DEFAULT '', fork TEXT NOT NULL DEFAULT '', activity TEXT NOT NULL,
  attention INTEGER NOT NULL DEFAULT 0, input_pending INTEGER NOT NULL DEFAULT 0,
  context INTEGER NOT NULL DEFAULT 0, window INTEGER NOT NULL DEFAULT 0,
  cost REAL NOT NULL DEFAULT 0, created INTEGER NOT NULL,
  launch TEXT NOT NULL DEFAULT '', position INTEGER NOT NULL DEFAULT 0,
  ready INTEGER NOT NULL DEFAULT 0, quick INTEGER NOT NULL DEFAULT 0,
  base TEXT NOT NULL DEFAULT '',
  last_active INTEGER NOT NULL, UNIQUE (parent, name));
CREATE TABLE IF NOT EXISTS commands (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  command TEXT NOT NULL, line TEXT NOT NULL, message INTEGER, seq INTEGER, item TEXT);
CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY,
  sender TEXT NOT NULL,
  recipient TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  text TEXT NOT NULL, report INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS transcript (
  session TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL, item TEXT NOT NULL, data TEXT NOT NULL,
  PRIMARY KEY (session, seq), UNIQUE (session, item));
CREATE INDEX IF NOT EXISTS transcript_root_text_search ON transcript (
  session, seq, lower(json_extract(data, '$.text')))
WHERE json_extract(data, '$.kind') IN ('user', 'text')
  AND json_extract(data, '$.parent') IS NULL;
CREATE INDEX IF NOT EXISTS transcript_message_text_search ON transcript (
  session, seq, lower(json_extract(data, '$.input.text')))
WHERE json_extract(data, '$.kind') = 'tool'
  AND json_extract(data, '$.server') = 'hypercube'
  AND json_extract(data, '$.tool') = 'send_message';
CREATE TABLE IF NOT EXISTS grants (
  session TEXT REFERENCES sessions(id) ON DELETE CASCADE,
  harness TEXT NOT NULL, tool TEXT NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS grants_unique ON grants (COALESCE(session, ''), harness, tool);
CREATE TABLE IF NOT EXISTS model_entries (
  id TEXT PRIMARY KEY, harness TEXT NOT NULL, model TEXT NOT NULL, title TEXT NOT NULL,
  efforts TEXT NOT NULL, default_effort TEXT NOT NULL,
  UNIQUE (harness, model));
CREATE TABLE IF NOT EXISTS limits (
  harness TEXT NOT NULL, minutes INTEGER NOT NULL, used INTEGER NOT NULL,
  resets INTEGER NOT NULL, at INTEGER NOT NULL, PRIMARY KEY (harness, minutes));
PRAGMA user_version = 4;
