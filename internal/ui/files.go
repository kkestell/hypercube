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
	"syscall"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// maxFile bounds text reads and syntax-highlighting work.
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
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || errors.Is(err, store.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return true
	}
	return refused(w, err)
}

type fileEntry struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
}

func folderEntries(root *os.Root, dir string) ([]fileEntry, error) {
	if dir == "" {
		dir = "."
	}
	folder, err := root.Open(dir)
	if err != nil {
		return nil, err
	}
	defer folder.Close()
	entries, err := folder.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	list := []fileEntry{}
	for _, e := range entries {
		if hidden(e.Name()) {
			continue
		}
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			info, err := root.Stat(path.Join(dir, e.Name()))
			isDir = err == nil && info.IsDir()
		}
		list = append(list, fileEntry{e.Name(), isDir})
	}
	return list, nil
}

// listFileTree reads the visible folder listings in one request and one
// response, so a large expanded tree is applied to the page once.
func (s *server) listFileTree(w http.ResponseWriter, r *http.Request) {
	root, err := s.sessionRoot(r.URL.Query().Get("session"))
	if fileError(w, err) {
		return
	}
	defer root.Close()
	var dirs []string
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&dirs); err != nil || len(dirs) > 10000 {
		http.Error(w, "invalid folder list", http.StatusBadRequest)
		return
	}
	listings := make(map[string][]fileEntry, len(dirs))
	for _, dir := range dirs {
		if r.Context().Err() != nil {
			return
		}
		if dir != "" && !filepath.IsLocal(dir) {
			http.Error(w, "invalid folder path", http.StatusBadRequest)
			return
		}
		entries, err := folderEntries(root, dir)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			listings[dir] = nil
			continue
		}
		if err != nil {
			if dir == "" {
				fileError(w, err)
				return
			}
			// A folder that cannot be listed keeps its previous rows;
			// other folders still refresh.
			continue
		}
		listings[dir] = entries
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(listings)
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
	etag := fmt.Sprintf(`"%d-%d"`, info.Size(), info.ModTime().UnixNano())
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if typ := previewMIME(name); typ != "" {
		address, err := s.previewURL(r.URL.Query().Get("session"), name, etag)
		if fileError(w, err) {
			return
		}
		var text *string
		if (typ == "text/html" || typ == "image/svg+xml") && info.Size() <= maxFile {
			content, err := readText(file, info.Size())
			switch {
			case err == nil:
				value := string(content)
				text = &value
			case !errors.Is(err, errBinary):
				fileError(w, err)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Text    *string     `json:"text"`
			Preview filePreview `json:"preview"`
		}{text, filePreview{typ, address}})
		return
	}
	content, err := readText(file, info.Size())
	if fileError(w, err) {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}
