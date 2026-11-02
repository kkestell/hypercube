// Package plugin holds the code the harness plugins share. Each plugin keeps
// its own harness behavior.
package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

// MCPTool splits a tool name of the form mcp__<server>__<tool>, which Claude
// Code and Pi give the tools of MCP servers. Any other name is the tool's own.
func MCPTool(name string) (server, tool string) {
	if rest, ok := strings.CutPrefix(name, "mcp__"); ok {
		if server, tool, ok := strings.Cut(rest, "__"); ok && server != "" && tool != "" {
			return server, tool
		}
	}
	return "", name
}

// JSONString marshals v as compact JSON.
func JSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// Errors numbers a harness's error items. The owner's lock guards it.
type Errors struct{ n int }

// Item returns the next finished error item, with IDs error-1, error-2, ...
func (e *Errors) Item(text string) harness.Item {
	e.n++
	return harness.Item{ID: fmt.Sprintf("error-%d", e.n), Kind: harness.KindError, Text: text, Done: true}
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
	file, err := os.CreateTemp(dir, "."+name+"-*.tmp")
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
