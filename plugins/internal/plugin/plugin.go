// Package plugin holds the code the harness plugins share. Each plugin keeps
// its own harness behavior.
package plugin

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// JSONString marshals v as compact JSON.
func JSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// WriteFile writes text to name in dir and returns the file's path. Running
// harness processes may be reading the file, so it is replaced only when it
// changes, through a rename.
func WriteFile(dir, name string, text []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if before, err := os.ReadFile(path); err == nil && bytes.Equal(before, text) {
		return path, nil
	}
	file, err := os.CreateTemp(dir, "index-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(text); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return path, os.Rename(file.Name(), path)
}
