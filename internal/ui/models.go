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

func (s *server) listModelEntries(w http.ResponseWriter, r *http.Request) {
	var list []store.ModelEntry
	err := s.db.Tx(func(tx *store.Tx) (err error) { list, err = tx.ModelEntries(); return })
	if refused(w, err) {
		return
	}
	writeJSON(w, list)
}

func (s *server) saveModelEntry(w http.ResponseWriter, r *http.Request) {
	var entry store.ModelEntry
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entry); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := entry.Check(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var list []store.ModelEntry
	err := s.db.Tx(func(tx *store.Tx) (err error) {
		if err = tx.SaveModelEntry(entry); err != nil {
			return
		}
		list, err = tx.ModelEntries()
		return
	})
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if errors.Is(err, store.ErrDuplicateModel) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if refused(w, err) {
		return
	}
	writeJSON(w, list)
}

func (s *server) deleteModelEntry(w http.ResponseWriter, r *http.Request) {
	var list []store.ModelEntry
	err := s.db.Tx(func(tx *store.Tx) (err error) {
		if err = tx.DeleteModelEntry(r.PathValue("id")); err != nil {
			return
		}
		list, err = tx.ModelEntries()
		return
	})
	if refused(w, err) {
		return
	}
	writeJSON(w, list)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
