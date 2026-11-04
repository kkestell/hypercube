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
	rows, err := t.tx.Query(`SELECT s.id, s.title, w.path,
 COALESCE((SELECT CASE WHEN json_extract(data, '$.kind') = 'tool'
   THEN json_extract(data, '$.input.text') ELSE json_extract(data, '$.text') END
 FROM transcript WHERE session = s.id AND (
   (json_extract(data, '$.kind') IN ('user', 'text') AND json_extract(data, '$.parent') IS NULL) OR
   (json_extract(data, '$.kind') = 'tool' AND json_extract(data, '$.server') = 'hypercube' AND json_extract(data, '$.tool') = 'send_message'))
 AND instr(lower(CASE WHEN json_extract(data, '$.kind') = 'tool' THEN json_extract(data, '$.input.text') ELSE json_extract(data, '$.text') END), lower(?)) > 0
 ORDER BY seq DESC LIMIT 1), '') AS text
 FROM sessions s JOIN workspaces w ON w.id = s.workspace
 WHERE instr(lower(s.title), lower(?)) > 0 OR text <> '' ORDER BY s.created DESC`, query, query)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(r interface{ Scan(...any) error }) (SearchResult, error) {
		var result SearchResult
		err := r.Scan(&result.ID, &result.Title, &result.Workspace, &result.Text)
		return result, err
	})
}
