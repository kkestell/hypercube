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

// models asks a harness for the models it offers in a session's directory,
// or in a workspace's for a new session.
func (s *server) models(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var dir string
	err := s.db.Read(func(tx *store.Tx) error {
		switch {
		case q.Get("session") != "":
			session, err := tx.Session(q.Get("session"))
			dir = session.Dir
			return err
		case q.Get("workspace") != "":
			workspace, err := tx.Workspace(q.Get("workspace"))
			dir = workspace.Path
			return err
		}
		return errors.New("a session or workspace is required")
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	catalog, err := s.rt.Models(r.Context(), q.Get("harness"), dir, q.Get("model"))
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
