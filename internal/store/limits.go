package store

import "github.com/kkestell/hypercube/internal/harness"

// Limit is the newest reading of one usage window of a harness, such as
// claude.
type Limit struct {
	Harness string `json:"harness"`
	harness.Limit
}

// PutLimit saves a reading of the harness's window unless the saved reading
// of that window is at least as new. Agents' readings arrive in no shared
// order, such as when several agents' saved output is read after a restart.
func (t *Tx) PutLimit(harness string, l harness.Limit) error {
	return t.exec(`INSERT INTO limits (harness, minutes, used, resets, at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (harness, minutes) DO UPDATE SET used = excluded.used, resets = excluded.resets, at = excluded.at
WHERE excluded.at > limits.at`, harness, l.Minutes, l.Used, l.Resets, l.At)
}

// Limits returns every saved reading, by harness, then by window length.
func (t *Tx) Limits() ([]Limit, error) {
	rows, err := t.tx.Query(`SELECT harness, minutes, used, resets, at FROM limits ORDER BY harness, minutes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	limits := []Limit{}
	for rows.Next() {
		var l Limit
		if err := rows.Scan(&l.Harness, &l.Minutes, &l.Used, &l.Resets, &l.At); err != nil {
			return nil, err
		}
		limits = append(limits, l)
	}
	return limits, rows.Err()
}
