import { Database } from "bun:sqlite";
import { mkdirSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import type { Agent, Model, Session, TranscriptItem, Workspace } from "../model";

/**
 * The SQLite database behind the model and the transcripts. Each row is one
 * record as JSON. Until `openStore` is called nothing is stored, so tests and
 * a server that hasn't started yet never touch a file.
 *
 * There are no migrations: a database from another SCHEMA_VERSION is dropped.
 */

const SCHEMA_VERSION = 1;

const globals = globalThis as typeof globalThis & { __hypercubeDb?: Database };

/** What of an agent is kept. Status, error, queue and steer belong to a running server, not to the record. */
export type StoredAgent = Omit<Agent, "status" | "error" | "queue" | "steer">;

export function databasePath() {
  return process.env.HYPERCUBE_DB || join(homedir(), ".local/share/hypercube/hypercube.db");
}

export function openStore() {
  if (globals.__hypercubeDb) return;
  const path = databasePath();
  mkdirSync(dirname(path), { recursive: true });
  const db = new Database(path, { create: true });
  db.exec("PRAGMA journal_mode = WAL");
  db.exec("PRAGMA synchronous = NORMAL");
  if ((db.query("PRAGMA user_version").get() as { user_version: number }).user_version !== SCHEMA_VERSION) {
    for (const table of ["workspaces", "sessions", "agents", "items"]) db.exec(`DROP TABLE IF EXISTS ${table}`);
    // `seq` keeps insertion order, which is the order workspaces and transcript items are shown in.
    for (const table of ["workspaces", "sessions", "agents"]) {
      db.exec(`CREATE TABLE ${table} (seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE, data TEXT NOT NULL)`);
    }
    db.exec("CREATE TABLE items (seq INTEGER PRIMARY KEY AUTOINCREMENT, agent_id TEXT NOT NULL, id TEXT NOT NULL, data TEXT NOT NULL, UNIQUE (agent_id, id))");
    db.exec(`PRAGMA user_version = ${SCHEMA_VERSION}`);
  }
  globals.__hypercubeDb = db;
}

/** Everything stored, in the order it was added. */
export function loadStore() {
  const db = globals.__hypercubeDb!;
  const all = <T>(table: string) => (db.query(`SELECT data FROM ${table} ORDER BY seq`).all() as { data: string }[]).map(row => JSON.parse(row.data) as T);
  // Left behind only if the server died between removing an agent and its transcript.
  db.exec("DELETE FROM items WHERE agent_id NOT IN (SELECT id FROM agents)");
  const items = new Map<string, TranscriptItem[]>();
  for (const row of db.query("SELECT agent_id, data FROM items ORDER BY seq").all() as { agent_id: string; data: string }[]) {
    const list = items.get(row.agent_id);
    if (list) list.push(JSON.parse(row.data));
    else items.set(row.agent_id, [JSON.parse(row.data)]);
  }
  return { workspaces: all<Workspace>("workspaces"), sessions: all<Session>("sessions"), agents: all<StoredAgent>("agents"), items };
}

/** Makes the table hold exactly these records, writing only the rows that changed. */
function sync(db: Database, table: string, records: { id: string }[]) {
  const upsert = db.query(`INSERT INTO ${table} (id, data) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET data = excluded.data WHERE data != excluded.data`);
  for (const record of records) upsert.run(record.id, JSON.stringify(record));
  db.query(`DELETE FROM ${table} WHERE id NOT IN (SELECT value FROM json_each(?))`).run(JSON.stringify(records.map(record => record.id)));
}

export function saveModel(model: Model) {
  const db = globals.__hypercubeDb;
  if (!db) return;
  db.transaction(() => {
    sync(db, "workspaces", model.workspaces);
    // A session's terminal belongs to a running server, like an agent's status.
    sync(db, "sessions", Object.values(model.sessions).map(({ terminal: _terminal, ...stored }): Session => stored));
    sync(
      db,
      "agents",
      Object.values(model.agents).map(({ status: _status, error: _error, queue: _queue, steer: _steer, ...stored }): StoredAgent => stored),
    );
  })();
}

/** Adds the item to the agent's transcript, or replaces the one with the same id in place. */
export function saveItem(agentId: string, item: TranscriptItem) {
  globals.__hypercubeDb
    ?.query("INSERT INTO items (agent_id, id, data) VALUES (?, ?, ?) ON CONFLICT (agent_id, id) DO UPDATE SET data = excluded.data")
    .run(agentId, item.id, JSON.stringify(item));
}

export function dropItems(agentIds: string[]) {
  const db = globals.__hypercubeDb;
  if (!db) return;
  const drop = db.query("DELETE FROM items WHERE agent_id = ?");
  db.transaction(() => {
    for (const agentId of agentIds) drop.run(agentId);
  })();
}
