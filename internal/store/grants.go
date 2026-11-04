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

// Grant is a saved always-allow choice and its session title, if scoped.
type Grant struct {
	Session string `json:"session"`
	Title   string `json:"title"`
	Harness string `json:"harness"`
	Tool    string `json:"tool"`
}

// Grants lists saved choices with their current session titles.
func (t *Tx) Grants() ([]Grant, error) {
	rows, err := t.tx.Query(`SELECT COALESCE(g.session, ''), COALESCE(s.title, ''), g.harness, g.tool
 FROM grants g LEFT JOIN sessions s ON s.id = g.session ORDER BY g.harness, g.tool, g.session`)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(row interface{ Scan(...any) error }) (Grant, error) {
		var g Grant
		err := row.Scan(&g.Session, &g.Title, &g.Harness, &g.Tool)
		return g, err
	})
}

// DeleteGrant removes one saved choice.
func (t *Tx) DeleteGrant(session, harness, tool string) error {
	return t.exec(`DELETE FROM grants WHERE session IS NULLIF(?, '') AND harness = ? AND tool = ?`, session, harness, tool)
}
