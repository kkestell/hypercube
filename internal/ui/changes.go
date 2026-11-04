package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/workspace"
)

// gitDir returns the session's folder and whether it is inside a Git
// repository.
func (s *server) gitDir(id string) (dir string, ok bool, err error) {
	dir, err = s.sessionDir(id)
	if err != nil {
		return "", false, err
	}
	if _, err := os.Stat(dir); err != nil {
		return "", false, err
	}
	_, err = workspace.Root(dir)
	return dir, err == nil, nil
}

func (s *server) listChanges(w http.ResponseWriter, r *http.Request) {
	dir, ok, err := s.gitDir(r.URL.Query().Get("session"))
	if fileError(w, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		json.NewEncoder(w).Encode(nil)
		return
	}
	changes, err := workspace.Changes(dir)
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
	dir, ok, err := s.gitDir(r.URL.Query().Get("session"))
	if fileError(w, err) {
		return
	}
	if !ok {
		http.Error(w, "folder is outside Git", http.StatusConflict)
		return
	}
	path := r.URL.Query().Get("path")
	if !filepath.IsLocal(path) || filepath.Clean(path) == "." {
		http.Error(w, "path must name a file in the session folder", http.StatusBadRequest)
		return
	}
	patch, err := workspace.Diff(dir, path, maxFile)
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
