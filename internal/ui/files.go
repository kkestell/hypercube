package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

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
		dir = session.Dir
		return nil
	})
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(dir)
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
		http.Error(w, "file is too large", http.StatusRequestEntityTooLarge)
		return
	}
	etag := fmt.Sprintf(`"%d-%d"`, info.Size(), info.ModTime().UnixNano())
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
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

// mentionFiles lists files for the composer, excluding ignored files in Git worktrees.
func (s *server) mentionFiles(w http.ResponseWriter, r *http.Request) {
	var root *os.Root
	var err error
	if id := r.URL.Query().Get("workspace"); id != "" {
		var dir string
		err = s.db.Read(func(tx *store.Tx) error {
			workspace, err := tx.Workspace(id)
			if err == nil {
				dir = workspace.Path
			}
			return err
		})
		if err == nil {
			root, err = os.OpenRoot(dir)
		}
	} else {
		root, err = s.sessionRoot(r.URL.Query().Get("session"))
	}
	if fileError(w, err) {
		return
	}
	defer root.Close()
	dir := root.Name()
	files := []string{}
	cmd := exec.CommandContext(r.Context(), "git", "-C", dir, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	output, err := cmd.Output()
	if err == nil {
		for _, name := range strings.Split(string(output), "\x00") {
			if name == "" {
				continue
			}
			skip := false
			for _, part := range strings.Split(name, "/") {
				if hidden(part) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			info, err := root.Stat(name)
			if err == nil && !info.IsDir() {
				files = append(files, name)
			}
		}
	} else {
		err = filepath.WalkDir(dir, func(name string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if name == dir {
				return nil
			}
			if hidden(entry.Name()) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.IsDir() {
				rel, err := filepath.Rel(dir, name)
				if err != nil {
					return err
				}
				files = append(files, filepath.ToSlash(rel))
			}
			return nil
		})
		if fileError(w, err) {
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}
