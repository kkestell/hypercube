package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// migrate upgrades older schemas and updates saved state in one transaction.
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
		} else {
			if err := tx.addLaunchColumns(); err != nil {
				return err
			}
			if err := tx.addBaseColumn(); err != nil {
				return err
			}
			if err := tx.addChildColumns(); err != nil {
				return err
			}
			if err := tx.dropRunColumns(); err != nil {
				return err
			}
		}
		if err := tx.addModelTitle(); err != nil {
			return err
		}
		var version int
		if err := tx.tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
			return err
		}
		if version < 1 {
			if err := tx.exec(`
DELETE FROM transcript WHERE json_extract(data, '$.kind') = 'status';
-- A child's requests belong to its own transcript alone.
DELETE FROM transcript WHERE json_extract(data, '$.kind') = 'attention' AND json_extract(data, '$.from') IS NOT NULL;
PRAGMA user_version = 1;`); err != nil {
				return err
			}
		}
		if version < 2 {
			// Proposals and accepted offers saved the agent profile they
			// started with; they now save only its settings.
			if err := tx.exec(`
UPDATE transcript SET data = json_set(json_remove(data, '$.input.profile'), '$.input.settings', json_object(
	'harness', coalesce(json_extract(data, '$.input.profile.harness'), ''),
	'model', coalesce(json_extract(data, '$.input.profile.model'), ''),
	'effort', coalesce(json_extract(data, '$.input.profile.effort'), '')))
WHERE json_type(data, '$.input.profile') = 'object'
	AND (json_extract(data, '$.kind') = 'proposal' OR json_extract(data, '$.kind') = 'attention' AND json_extract(data, '$.offer') = 1);
PRAGMA user_version = 2;`); err != nil {
				return err
			}
		}
		if version < 3 {
			if err := tx.exec(`
CREATE INDEX IF NOT EXISTS transcript_root_text_search ON transcript (
  session, seq, lower(json_extract(data, '$.text')))
WHERE json_extract(data, '$.kind') IN ('user', 'text')
  AND json_extract(data, '$.parent') IS NULL;
CREATE INDEX IF NOT EXISTS transcript_message_text_search ON transcript (
  session, seq, lower(json_extract(data, '$.input.text')))
WHERE json_extract(data, '$.kind') = 'tool'
  AND json_extract(data, '$.server') = 'hypercube'
  AND json_extract(data, '$.tool') = 'send_message';
PRAGMA user_version = 3;`); err != nil {
				return err
			}
		}
		if version < 4 {
			// Discovered catalogs are not user-configured model entries.
			if err := tx.exec(`DROP TABLE IF EXISTS catalogs; PRAGMA user_version = 4;`); err != nil {
				return err
			}
		}
		// A session without an agent waits on nobody.
		return tx.exec(`UPDATE sessions SET attention = 0 WHERE attention = 1 AND (ended = 1 OR launch = '')`)
	})
}

// addModelTitle preserves configured models and gives Claude aliases readable titles.
func (t *Tx) addModelTitle() error {
	has, err := t.exists(`SELECT 1 FROM pragma_table_info('model_entries') WHERE name = 'title'`)
	if err != nil || has {
		return err
	}
	return t.exec(`
ALTER TABLE model_entries ADD COLUMN title TEXT NOT NULL DEFAULT '';
UPDATE model_entries SET title = CASE
  WHEN harness = 'claude' AND model IN ('opus', 'sonnet', 'haiku')
    THEN upper(substr(model, 1, 1)) || substr(model, 2)
  ELSE model END;`)
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

// addBaseColumn upgrades databases from before worktree sessions.
func (t *Tx) addBaseColumn() error {
	hasBase, err := t.exists(`SELECT 1 FROM pragma_table_info('sessions') WHERE name = 'base'`)
	if err != nil || hasBase {
		return err
	}
	return t.exec(`ALTER TABLE sessions ADD COLUMN base TEXT NOT NULL DEFAULT '';`)
}

func (t *Tx) addChildColumns() error {
	for _, column := range []struct{ table, name, definition string }{
		{"sessions", "fork", "TEXT NOT NULL DEFAULT ''"},
		{"sessions", "input_pending", "INTEGER NOT NULL DEFAULT 0"},
		{"messages", "report", "INTEGER NOT NULL DEFAULT 0"},
	} {
		has, err := t.exists(`SELECT 1 FROM pragma_table_info('` + column.table + `') WHERE name = '` + column.name + `'`)
		if err != nil {
			return err
		}
		if !has {
			if err := t.exec(`ALTER TABLE ` + column.table + ` ADD COLUMN ` + column.name + ` ` + column.definition); err != nil {
				return err
			}
		}
	}
	return nil
}

// dropRunColumns upgrades databases from before sessions started and stopped
// on their own.
func (t *Tx) dropRunColumns() error {
	for _, name := range []string{"running", "awaiting_user"} {
		has, err := t.exists(`SELECT 1 FROM pragma_table_info('sessions') WHERE name = '` + name + `'`)
		if err != nil {
			return err
		}
		if has {
			if err := t.exec(`ALTER TABLE sessions DROP COLUMN ` + name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *Tx) exists(query string) (bool, error) {
	var one int
	err := t.tx.QueryRow(query).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// checkLegacy requires one lead per session and workers for the other agents.
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

// convertLegacy maps leads to their sessions and workers to child sessions,
// keeping their conversations, queued messages, and transcripts. The previous
// session tables have been renamed to old_* before the current schema is made.
func (t *Tx) convertLegacy() error {
	const columns = `id, workspace, parent, name, title, ended, unseen,
 harness, model, effort, dir, branch, conversation, activity, attention,
 context, window, cost, created, last_active`
	err := t.exec(`
CREATE TEMP VIEW legacy_agents AS
 SELECT id AS agent, CASE role WHEN 'lead' THEN session ELSE id END AS session,
  session || '/' || name AS address FROM agents;

INSERT INTO sessions (` + columns + `)
 SELECT s.id, s.workspace, NULL, '', s.title, 0, s.unseen,
  a.harness, a.model, a.effort, a.dir, a.branch, a.conversation, a.activity, a.attention,
  a.context, a.window, a.cost, s.created, s.last_active
 FROM old_sessions s JOIN agents a ON a.session = s.id AND a.role = 'lead';

INSERT INTO sessions (` + columns + `)
 SELECT a.id, s.workspace, s.id, a.name, a.name, 0, 0,
  a.harness, a.model, a.effort, a.dir, a.branch, a.conversation, a.activity, a.attention,
  a.context, a.window, a.cost, a.created, a.created
 FROM agents a JOIN old_sessions s ON s.id = a.session WHERE a.role = 'worker';

-- An item's sender was addressed by session/name in the old transcripts.
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
