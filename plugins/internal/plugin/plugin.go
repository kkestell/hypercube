// Package plugin holds the code the harness plugins share. Each plugin keeps
// its own harness behavior.
package plugin

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

// MCPCommand returns the executable and arguments a harness runs for the
// Hypercube MCP server: the bridge, or for a start saved by an earlier
// version, the backend's mcp command.
func MCPCommand(a harness.StartArgs) (string, []string, error) {
	switch {
	case a.MCP != "":
		return a.MCP, []string{}, nil
	case a.Hypercube != "":
		return a.Hypercube, []string{"mcp"}, nil
	}
	return "", nil, errors.New("the start names no MCP server")
}

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
