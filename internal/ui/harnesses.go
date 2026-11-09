package ui

import (
	"encoding/json"
	"net/http"
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
