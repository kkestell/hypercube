package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// migrate creates the current schema and upgrades older databases in one
// transaction, so a failed upgrade leaves the old database as it was.
func (d *DB) migrate() error {
	return d.Tx(func(tx *Tx) error {
		legacy, err := tx.exists(`SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'agents'`)
		if err != nil {
			return err
		}
		if legacy {
			if err := tx.checkLegacy(); err != nil {
				return err
			}
			if err := tx.exec(`
ALTER TABLE sessions RENAME TO old_sessions;
ALTER TABLE messages RENAME TO old_messages;
ALTER TABLE transcript RENAME TO old_transcript;`); err != nil {
				return err
			}
		}
		if err := tx.exec(schema); err != nil {
			return err
		}
		if legacy {
			if err := tx.convertLegacy(); err != nil {
				return err
			}
		} else if err := tx.addLaunchColumns(); err != nil {
			return err
		}
		// A session that is not running waits on nobody.
		return tx.exec(`
UPDATE sessions SET attention = 0
WHERE attention = 1 AND (ended = 1
  OR NOT (SELECT running FROM sessions AS root WHERE root.id = COALESCE(sessions.parent, sessions.id)))`)
	})
}

// addLaunchColumns upgrades databases from before durable launches.
func (t *Tx) addLaunchColumns() error {
	hasLaunch, err := t.exists(`SELECT 1 FROM pragma_table_info('sessions') WHERE name = 'launch'`)
	if err != nil || hasLaunch {
		return err
	}
	return t.exec(`
ALTER TABLE sessions ADD COLUMN launch TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN ready INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN quick INTEGER NOT NULL DEFAULT 0;`)
}

func (t *Tx) exists(query string) (bool, error) {
	var one int
	err := t.tx.QueryRow(query).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// checkLegacy requires what the conversion relies on: every session has one
// lead agent, and every other agent is a worker.
func (t *Tx) checkLegacy() error {
	var id string
	err := t.tx.QueryRow(`SELECT id FROM sessions s
 WHERE (SELECT COUNT(*) FROM agents WHERE session = s.id AND role = 'lead') != 1`).Scan(&id)
	if err == nil {
		return fmt.Errorf("session %s does not have exactly one lead agent", id)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	err = t.tx.QueryRow(`SELECT id FROM agents WHERE role NOT IN ('lead', 'worker')`).Scan(&id)
	if err == nil {
		return fmt.Errorf("agent %s has an unknown role", id)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

// convertLegacy copies a database that kept a session's agents in an agents
// table into sessions. A lead becomes its top-level session, keeping the
// session's ID; a worker becomes a child session, keeping its agent ID. The
// old tables are in old_* and agents, and are dropped after the copy.
func (t *Tx) convertLegacy() error {
	const columns = `id, workspace, parent, name, title, running, ended, awaiting_user, unseen,
 harness, model, effort, dir, branch, conversation, activity, attention,
 context, window, cost, created, last_active`
	err := t.exec(`
CREATE TEMP VIEW legacy_agents AS
 SELECT id AS agent, CASE role WHEN 'lead' THEN session ELSE id END AS session,
  session || '/' || name AS address FROM agents;

INSERT INTO sessions (` + columns + `)
 SELECT s.id, s.workspace, NULL, '', s.title, s.running, 0, s.awaiting_user, s.unseen,
  a.harness, a.model, a.effort, a.dir, a.branch, a.conversation, a.activity, a.attention,
  a.context, a.window, a.cost, s.created, s.last_active
 FROM old_sessions s JOIN agents a ON a.session = s.id AND a.role = 'lead';

INSERT INTO sessions (` + columns + `)
 SELECT a.id, s.workspace, s.id, a.name, a.name, 0, 0, 0, 0,
  a.harness, a.model, a.effort, a.dir, a.branch, a.conversation, a.activity, a.attention,
  a.context, a.window, a.cost, a.created, a.created
 FROM agents a JOIN old_sessions s ON s.id = a.session WHERE a.role = 'worker';

-- An item that delivers a message names its sender as session/name.
INSERT INTO transcript (session, seq, item, data)
 SELECT agent.session, t.seq, t.item,
  CASE WHEN sender.session IS NULL THEN t.data ELSE json_set(t.data, '$.from', sender.session) END
 FROM old_transcript t
 JOIN legacy_agents agent ON agent.agent = t.agent
 LEFT JOIN legacy_agents sender ON sender.address = json_extract(t.data, '$.from');

INSERT INTO messages (id, sender, recipient, text)
 SELECT m.id, COALESCE(sender.session, m.sender), recipient.session, m.text
 FROM old_messages m
 JOIN legacy_agents recipient ON recipient.agent = m.recipient
 LEFT JOIN legacy_agents sender ON sender.agent = m.sender;

DROP VIEW legacy_agents;
DROP TABLE old_messages;
DROP TABLE old_transcript;
DROP TABLE agents;
DROP TABLE old_sessions;`)
	if err != nil {
		return err
	}
	var table string
	err = t.tx.QueryRow(`SELECT "table" FROM pragma_foreign_key_check`).Scan(&table)
	if err == nil {
		return fmt.Errorf("converting agents left a dangling reference in %s", table)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}
