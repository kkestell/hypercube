package store

import "context"

// SearchResult identifies a matching session and its workspace and text.
type SearchResult struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Workspace string `json:"workspace"`
	Text      string `json:"text,omitempty"`
}

// SearchSessions searches saved conversation text and titles in every workspace.
func (t *Tx) SearchSessions(ctx context.Context, query string) ([]SearchResult, error) {
	rows, err := t.tx.QueryContext(ctx, `WITH conversation AS (
 SELECT session, seq, lower(json_extract(data, '$.text')) AS text FROM transcript
 WHERE json_extract(data, '$.kind') IN ('user', 'text')
   AND json_extract(data, '$.parent') IS NULL
 UNION ALL
 SELECT session, seq, lower(json_extract(data, '$.input.text')) AS text FROM transcript
 WHERE json_extract(data, '$.kind') = 'tool'
   AND json_extract(data, '$.server') = 'hypercube'
   AND json_extract(data, '$.tool') = 'send_message'
), matches AS (
 SELECT session, MAX(seq) AS seq FROM conversation
 WHERE instr(text, lower(?)) > 0 GROUP BY session
)
SELECT s.id, s.title, w.path, COALESCE(
 CASE WHEN json_extract(t.data, '$.kind') = 'tool'
 THEN json_extract(t.data, '$.input.text')
 ELSE json_extract(t.data, '$.text') END, '')
FROM sessions s JOIN workspaces w ON w.id = s.workspace
LEFT JOIN matches m ON m.session = s.id
LEFT JOIN transcript t ON t.session = m.session AND t.seq = m.seq
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
