package store

// migrate creates the current schema and updates saved state.
func (d *DB) migrate() error {
	return d.Tx(func(tx *Tx) error {
		if err := tx.exec(schema); err != nil {
			return err
		}
		if err := tx.exec(`DELETE FROM transcript WHERE json_extract(data, '$.kind') = 'status'`); err != nil {
			return err
		}
		// A child's requests belong to its own transcript alone.
		if err := tx.exec(`DELETE FROM transcript WHERE json_extract(data, '$.kind') = 'attention' AND json_extract(data, '$.from') IS NOT NULL`); err != nil {
			return err
		}
		// A session that is not running waits on nobody.
		return tx.exec(`
UPDATE sessions SET attention = 0
WHERE attention = 1 AND (ended = 1
  OR NOT (SELECT running FROM sessions AS root WHERE root.id = COALESCE(sessions.parent, sessions.id)))`)
	})
}
