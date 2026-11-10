package ui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"os"
	"sync"
)

//go:embed dist
var embedded embed.FS

// devUI names the directory a dev session serves the browser files from, so
// it can replace them without restarting the server. Otherwise the server
// serves the embedded files.
var devUI = os.Getenv("HYPERCUBE_DEV_UI")

// assets returns the browser files, rooted at the page.
func assets() fs.FS {
	if devUI != "" {
		return os.DirFS(devUI)
	}
	files, _ := fs.Sub(embedded, "dist")
	return files
}

// build identifies the browser files being served, so an open page can tell
// that the server now serves a different build.
var build struct {
	mu sync.Mutex
	id string
	// changed closes when the dev session replaces the browser files.
	changed chan struct{}
}

func buildID() string {
	build.mu.Lock()
	defer build.mu.Unlock()
	if build.id == "" {
		build.id = hashAssets()
	}
	return build.id
}

// buildChanged returns a channel that closes when the browser files change.
func buildChanged() <-chan struct{} {
	build.mu.Lock()
	defer build.mu.Unlock()
	if build.changed == nil {
		build.changed = make(chan struct{})
	}
	return build.changed
}

// reloadUI rehashes the browser files after the dev session replaces them, and
// wakes open pages, which reload when the build differs.
func reloadUI(w http.ResponseWriter, r *http.Request) {
	build.mu.Lock()
	defer build.mu.Unlock()
	build.id = hashAssets()
	if build.changed != nil {
		close(build.changed)
		build.changed = nil
	}
}

func hashAssets() string {
	hash := sha256.New()
	_ = fs.WalkDir(assets(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(assets(), path)
		if err != nil {
			return err
		}
		hash.Write([]byte(path))
		hash.Write([]byte{0})
		hash.Write(data)
		return nil
	})
	return hex.EncodeToString(hash.Sum(nil))
}
