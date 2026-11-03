package store

// AddGrant always allows the harness's tool: in one session, or in every
// session when session is empty. An existing grant is kept.
func (t *Tx) AddGrant(session, harness, tool string) error {
	return t.exec(`INSERT OR IGNORE INTO grants (session, harness, tool) VALUES (NULLIF(?, ''), ?, ?)`, session, harness, tool)
}

// Granted reports whether the session may use the tool without asking.
func (t *Tx) Granted(session, harness, tool string) (bool, error) {
	var granted bool
	err := t.tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM grants
 WHERE harness = ? AND tool = ? AND (session IS NULL OR session = ?))`, harness, tool, session).Scan(&granted)
	return granted, err
}
