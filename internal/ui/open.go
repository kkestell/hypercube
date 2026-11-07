package ui

import (
	"net/http"
	"os/exec"
	"path/filepath"

	"github.com/kkestell/hypercube/internal/store"
)

func (s *server) openInApp(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var args []string
	switch q.Get("app") {
	case "vscode":
		args = []string{"-a", "Visual Studio Code"}
	case "finder":
	case "terminal":
		args = []string{"-a", "Terminal"}
	default:
		http.Error(w, "unknown app", http.StatusBadRequest)
		return
	}
	var dir string
	var err error
	switch q.Get("kind") {
	case "session":
		dir, err = s.sessionDir(q.Get("id"))
	case "workspace":
		err = s.db.Read(func(tx *store.Tx) error {
			workspace, err := tx.Workspace(q.Get("id"))
			dir = workspace.Path
			return err
		})
	case "path":
		if q.Get("app") != "finder" || q.Get("path") == "" {
			http.Error(w, "a path and Finder are required", http.StatusBadRequest)
			return
		}
		dir, err = s.sessionDir(q.Get("id"))
		name := q.Get("path")
		if !filepath.IsAbs(name) {
			name = filepath.Join(dir, name)
		}
		dir = name
		args = []string{"-R"}
	default:
		http.Error(w, "unknown kind", http.StatusBadRequest)
		return
	}
	if fileError(w, err) {
		return
	}
	if err := exec.Command("open", append(args, dir)...).Run(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
