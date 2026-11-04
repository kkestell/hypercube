package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
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

func (s *server) sessionDir(id string) (string, error) {
	var dir string
	err := s.db.Read(func(tx *store.Tx) error {
		session, err := tx.Session(id)
		if err != nil {
			return err
		}
		dir = session.Dir
		return nil
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

func (s *server) sessionRoot(id string) (*os.Root, error) {
	dir, err := s.sessionDir(id)
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(dir)
}

var errTooLarge = errors.New("file is too large")
var errBinary = errors.New("file is binary or uses an unsupported text encoding")

func readText(file *os.File, size int64) ([]byte, error) {
	if size > maxFile {
		return nil, errTooLarge
	}
	content, err := io.ReadAll(io.LimitReader(file, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxFile {
		return nil, errTooLarge
	}
	if bytes.IndexByte(content[:min(len(content), 8000)], 0) >= 0 {
		return nil, errBinary
	}
	return content, nil
}

// openFile opens a file of the session: a relative path within its directory,
// or an absolute path anywhere, which an agent can ask the view to show.
func (s *server) openFile(id, name string) (*os.File, error) {
	root, err := s.sessionRoot(id)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if filepath.IsAbs(name) {
		return os.Open(name)
	}
	return root.Open(name)
}

func fileError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, errTooLarge) || errors.Is(err, workspace.ErrTooLarge) {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return true
	}
	if errors.Is(err, errBinary) {
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return true
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, store.ErrNotFound) {
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
	name := r.URL.Query().Get("path")
	if name == "" {
		name = "."
	}
	file, err := s.openFile(r.URL.Query().Get("session"), name)
	if fileError(w, err) {
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if fileError(w, err) {
		return
	}
	if info.IsDir() {
		http.Error(w, "cannot read a folder", http.StatusBadRequest)
		return
	}
	if info.Size() > maxFile {
		fileError(w, errTooLarge)
		return
	}
	etag := fmt.Sprintf(`"%d-%d"`, info.Size(), info.ModTime().UnixNano())
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	content, err := readText(file, info.Size())
	if fileError(w, err) {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}
