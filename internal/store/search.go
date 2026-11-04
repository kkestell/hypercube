package store

// SearchResult identifies a matching session and its workspace and text.
type SearchResult struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Workspace string `json:"workspace"`
	Text      string `json:"text,omitempty"`
}

// SearchSessions searches saved conversation text and titles in every workspace.
func (t *Tx) SearchSessions(query string) ([]SearchResult, error) {
	rows, err := t.tx.Query(`WITH conversation AS (
 SELECT session, seq, json_extract(data, '$.text') AS text FROM transcript
 WHERE json_extract(data, '$.kind') IN ('user', 'text')
   AND json_extract(data, '$.parent') IS NULL
 UNION ALL
 SELECT session, seq, json_extract(data, '$.input.text') AS text FROM transcript
 WHERE json_extract(data, '$.kind') = 'tool'
   AND json_extract(data, '$.server') = 'hypercube'
   AND json_extract(data, '$.tool') = 'send_message'
), matches AS (
 SELECT session, text, row_number() OVER (PARTITION BY session ORDER BY seq DESC) AS rank
 FROM conversation WHERE instr(lower(text), lower(?)) > 0
)
SELECT s.id, s.title, w.path, COALESCE(m.text, '')
FROM sessions s JOIN workspaces w ON w.id = s.workspace
LEFT JOIN matches m ON m.session = s.id AND m.rank = 1
WHERE instr(lower(s.title), lower(?)) > 0 OR m.session IS NOT NULL
ORDER BY s.created DESC`, query, query)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(r interface{ Scan(...any) error }) (SearchResult, error) {
		var result SearchResult
		err := r.Scan(&result.ID, &result.Title, &result.Workspace, &result.Text)
		return result, err
	})
}
