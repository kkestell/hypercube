// Package plugin holds the code the harness plugins share. Each plugin keeps
// its own harness behavior.
package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Response is the context Hypercube returns for a report.
type Response struct {
	Context string `json:"context"`
}

// Report runs `hypercube report` for event with body and decodes the answer
// into r.
func Report(hypercube, event string, body any, r *Response) error {
	cmd := exec.Command(hypercube, "report", event)
	cmd.Stdin, cmd.Stderr = strings.NewReader(JSONString(body)), os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("hypercube report %s: %w", event, err)
	}
	return json.Unmarshal(bytes.TrimSpace(out), r)
}

// JSONString marshals v as compact JSON.
func JSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// ShellQuote quotes s for the POSIX shell.
func ShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

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
