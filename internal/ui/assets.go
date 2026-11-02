package ui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"sync"
)

//go:embed static
var embedded embed.FS

// assets returns the browser files, rooted at the page.
func assets() fs.FS {
	files, _ := fs.Sub(embedded, "static")
	return files
}

// Icon returns the Hypercube SVG shared by the browser and menu bar.
func Icon() []byte {
	icon, _ := fs.ReadFile(assets(), "favicon.svg")
	return icon
}

// buildID identifies the embedded browser files, so an open page can tell
// that the server now serves a different build.
var buildID = sync.OnceValue(func() string {
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
})
