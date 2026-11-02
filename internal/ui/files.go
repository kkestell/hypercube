package ui

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/kkestell/hypercube/internal/store"
)

// maxFile keeps a warm syntax highlight near 100 ms.
const maxFile = 256 << 10

func hidden(name string) bool {
	switch name {
	case ".git", ".svn", ".hg", ".DS_Store", "Thumbs.db", ".hypercube":
		return true
	}
	return false
}

func (s *server) sessionRoot(id string) (*os.Root, error) {
	var dir string
	err := s.db.Read(func(tx *store.Tx) error {
		session, err := tx.Session(id)
		if err != nil {
			return err
		}
		workspace, err := tx.Workspace(session.Workspace)
		dir = workspace.Path
		return err
	})
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(dir)
}

func fileError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, sql.ErrNoRows) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return true
	}
	return refused(w, err)
}

func (s *server) listFiles(w http.ResponseWriter, r *http.Request) {
	root, err := s.sessionRoot(r.URL.Query().Get("session"))
	if fileError(w, err) {
		return
	}
	defer root.Close()
	dir := r.URL.Query().Get("path")
	if dir == "" {
		dir = "."
	}
	folder, err := root.Open(dir)
	if fileError(w, err) {
		return
	}
	defer folder.Close()
	entries, err := folder.ReadDir(-1)
	if fileError(w, err) {
		return
	}
	type entry struct {
		Name string `json:"name"`
		Dir  bool   `json:"dir"`
	}
	list := []entry{}
	for _, e := range entries {
		if hidden(e.Name()) {
			continue
		}
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			info, err := root.Stat(path.Join(dir, e.Name()))
			isDir = err == nil && info.IsDir()
		}
		list = append(list, entry{e.Name(), isDir})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (s *server) readFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	root, err := s.sessionRoot(r.URL.Query().Get("session"))
	if fileError(w, err) {
		return
	}
	defer root.Close()
	name := r.URL.Query().Get("path")
	if name == "" {
		name = "."
	}
	info, err := root.Stat(name)
	if fileError(w, err) {
		return
	}
	if info.IsDir() {
		http.Error(w, "cannot read a folder", http.StatusBadRequest)
		return
	}
	if info.Size() > maxFile {
		http.Error(w, "file is too large", http.StatusRequestEntityTooLarge)
		return
	}
	etag := fmt.Sprintf(`"%d-%d"`, info.Size(), info.ModTime().UnixNano())
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	file, err := root.Open(name)
	if fileError(w, err) {
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxFile+1))
	if fileError(w, err) {
		return
	}
	if len(content) > maxFile {
		http.Error(w, "file is too large", http.StatusRequestEntityTooLarge)
		return
	}
	if bytes.IndexByte(content[:min(len(content), 8000)], 0) >= 0 {
		http.Error(w, "file is binary or uses an unsupported text encoding", http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}
