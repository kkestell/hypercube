package store

// migrate creates the current schema and resets attention for stopped sessions.
func (d *DB) migrate() error {
	return d.Tx(func(tx *Tx) error {
		if err := tx.exec(schema); err != nil {
			return err
		}
		// A session that is not running waits on nobody.
		return tx.exec(`
UPDATE sessions SET attention = 0
WHERE attention = 1 AND (ended = 1
  OR NOT (SELECT running FROM sessions AS root WHERE root.id = COALESCE(sessions.parent, sessions.id)))`)
	})
}
