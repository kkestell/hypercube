package main

import (
	"os"
	"path/filepath"

	"github.com/kkestell/hypercube/internal/session"
)

// programs returns the plugins and helpers beside the real hypercube
// executable. bin/hypercube and installed CLI links are symlinks into the
// bundle, so the path is resolved first.
func programs() (session.Programs, error) {
	exe, err := os.Executable()
	if err != nil {
		return session.Programs{}, err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return session.Programs{}, err
	}
	dir := filepath.Dir(exe)
	return session.Programs{Plugins: dir, MCP: filepath.Join(dir, "hypercube-mcp")}, nil
}
