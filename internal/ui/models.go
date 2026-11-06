package ui

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kkestell/hypercube/internal/store"
)

// harnesses scans the installed harnesses again, so one installed after the
// app started appears.
func (s *server) harnesses(w http.ResponseWriter, r *http.Request) {
	list, err := s.rt.ScanHarnesses(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

// models returns a workspace's saved catalog, refreshing it when requested.
func (s *server) models(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var dir string
	err := s.db.Read(func(tx *store.Tx) error {
		if q.Get("workspace") == "" {
			return errors.New("a workspace is required")
		}
		workspace, err := tx.Workspace(q.Get("workspace"))
		dir = workspace.Path
		return err
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	models := s.rt.CachedModels
	if q.Get("refresh") == "1" {
		models = s.rt.Models
	}
	catalog, err := models(r.Context(), q.Get("harness"), dir, q.Get("model"))
	if refused(w, err) {
		return
	}
	writeJSON(w, catalog)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
