package store

// migrate upgrades databases created before durable launches. Some existing
// databases already have these columns, so inspect the schema before adding
// the four launch fields together in one transaction.
func (d *DB) migrate() error {
	return d.Tx(func(tx *Tx) error {
		var hasLaunch bool
		if err := tx.tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM pragma_table_info('sessions') WHERE name = 'launch')`).Scan(&hasLaunch); err != nil {
			return err
		}
		if hasLaunch {
			return nil
		}
		return tx.exec(`
ALTER TABLE sessions ADD COLUMN launch TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN ready INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN quick INTEGER NOT NULL DEFAULT 0;`)
	})
}
