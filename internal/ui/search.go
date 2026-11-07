package ui

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/kkestell/hypercube/internal/store"
)

func (s *server) searchSessions(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	results := []store.SearchResult{}
	if query != "" {
		err := s.db.ReadContext(r.Context(), func(tx *store.Tx) error {
			var err error
			results, err = tx.SearchSessions(r.Context(), query)
			return err
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	for i := range results {
		results[i].Workspace = filepath.Base(results[i].Workspace)
		results[i].Text = searchSnippet(results[i].Text, query)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func searchSnippet(text, query string) string {
	// Match SQLite lower(), which folds ASCII only and preserves byte offsets.
	lower := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= 'A' && r <= 'Z' {
				return r + ('a' - 'A')
			}
			return r
		}, s)
	}
	index := strings.Index(lower(text), lower(query))
	if index < 0 {
		return ""
	}
	start := index
	for n := 0; start > 0 && n < 60; n++ {
		_, size := utf8.DecodeLastRuneInString(text[:start])
		start -= size
	}
	end := index + len(query)
	if end > len(text) {
		end = len(text)
	}
	for n := 0; end < len(text) && n < 100; n++ {
		_, size := utf8.DecodeRuneInString(text[end:])
		end += size
	}
	snippet := strings.Join(strings.Fields(text[start:end]), " ")
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(text) {
		snippet += "…"
	}
	return snippet
}
