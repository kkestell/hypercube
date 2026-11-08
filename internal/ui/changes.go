package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// comparisonCheckout validates the comparison and resolves its server-owned target.
func (s *server) comparisonCheckout(w http.ResponseWriter, r *http.Request) (string, bool) {
	comparison := r.URL.Query().Get("comparison")
	if comparison == "" || comparison == "uncommitted" {
		return "", true
	}
	if comparison != "branch" {
		http.Error(w, "unknown change comparison", http.StatusBadRequest)
		return "", false
	}
	target := requestTarget(r)
	checkout := ""
	unsupported := target.workspace != ""
	if !unsupported {
		err := s.db.Read(func(tx *store.Tx) error {
			session, err := tx.Session(target.session)
			if err != nil {
				return err
			}
			if session.Base == "" {
				unsupported = true
				return nil
			}
			workspace, err := tx.Workspace(session.Workspace)
			checkout = workspace.Path
			return err
		})
		if fileError(w, err) {
			return "", false
		}
	}
	if unsupported {
		http.Error(w, "branch comparison requires a managed worktree session", http.StatusConflict)
		return "", false
	}
	return checkout, true
}

// gitDir returns the target's folder and whether it is inside a Git
// repository.
func (s *server) gitDir(target fileTarget) (dir, top string, ok bool, err error) {
	dir, err = s.targetDir(target)
	if err != nil {
		return "", "", false, err
	}
	if _, err := os.Stat(dir); err != nil {
		return "", "", false, err
	}
	top, err = workspace.Root(dir)
	if errors.Is(err, workspace.ErrNotGit) {
		return dir, "", false, nil
	}
	return dir, top, err == nil, err
}

func (s *server) listChanges(w http.ResponseWriter, r *http.Request) {
	checkout, valid := s.comparisonCheckout(w, r)
	if !valid {
		return
	}
	dir, top, ok, err := s.gitDir(requestTarget(r))
	if fileError(w, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		if checkout != "" {
			http.Error(w, "folder is outside Git", http.StatusConflict)
			return
		}
		json.NewEncoder(w).Encode(nil)
		return
	}
	var changes workspace.ChangesResult
	if checkout != "" {
		changes, err = workspace.BranchChanges(dir, top, checkout)
	} else {
		changes, err = workspace.Changes(dir, top)
	}
	if fileError(w, err) {
		return
	}
	root, err := os.OpenRoot(dir)
	if fileError(w, err) {
		return
	}
	defer root.Close()
	files := changes.Files[:0]
	for _, file := range changes.Files {
		if file.Status == "untracked" {
			added, removed, err := untrackedLines(root, file.Path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if fileError(w, err) {
				return
			}
			file.Added, file.Removed = added, removed
		}
		files = append(files, file)
	}
	changes.Files = files
	json.NewEncoder(w).Encode(changes)
}

func untrackedLines(root *os.Root, path string) (*int, *int, error) {
	info, err := root.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return nil, nil, nil
	}
	added, removed := 1, 0
	if info.Mode()&os.ModeSymlink == 0 {
		file, err := root.Open(path)
		if err != nil {
			return nil, nil, err
		}
		defer file.Close()
		content, err := readText(file, info.Size())
		if errors.Is(err, errTooLarge) || errors.Is(err, errBinary) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
		added = bytes.Count(content, []byte{'\n'})
		if len(content) > 0 && content[len(content)-1] != '\n' {
			added++
		}
	}
	return &added, &removed, nil
}

func (s *server) readDiff(w http.ResponseWriter, r *http.Request) {
	checkout, valid := s.comparisonCheckout(w, r)
	if !valid {
		return
	}
	dir, _, ok, err := s.gitDir(requestTarget(r))
	if fileError(w, err) {
		return
	}
	if !ok {
		http.Error(w, "folder is outside Git", http.StatusConflict)
		return
	}
	path := r.URL.Query().Get("path")
	if !filepath.IsLocal(path) || filepath.Clean(path) == "." {
		http.Error(w, "path must name a file in the folder", http.StatusBadRequest)
		return
	}
	var patch string
	if checkout != "" {
		patch, err = workspace.BranchDiff(dir, checkout, path, maxFile)
	} else {
		patch, err = workspace.Diff(dir, path, maxFile)
	}
	if fileError(w, err) {
		return
	}
	if strings.Contains(patch, "\nBinary files ") {
		fileError(w, errBinary)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(patch))
}
