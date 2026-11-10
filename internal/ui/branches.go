package ui

import (
	"net/http"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// branches lists a workspace's branches for choosing where a new worktree
// session starts. current is empty when the checkout's HEAD is detached.
func (s *server) branches(w http.ResponseWriter, r *http.Request) {
	var ws store.Workspace
	err := s.db.Read(func(tx *store.Tx) error {
		var err error
		ws, err = tx.Workspace(r.URL.Query().Get("workspace"))
		return err
	})
	if fileError(w, err) {
		return
	}
	if ws.NoGit {
		http.Error(w, "the workspace is not a Git checkout", http.StatusConflict)
		return
	}
	current, err := workspace.Branch(ws.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	local, remote, err := workspace.Branches(ws.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, struct {
		Current string   `json:"current"`
		Local   []string `json:"local"`
		Remote  []string `json:"remote"`
	}{current, local, remote})
}
