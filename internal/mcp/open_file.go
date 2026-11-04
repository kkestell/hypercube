package mcp

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/session"
)

type openFileArgs struct {
	Path  string              `json:"path"`
	Lines []session.LineRange `json:"lines,omitempty"`
}

// viewPath resolves path, which is relative to dir, absolute, or starts with
// "~", through symlinks. It returns the file's real path and the path the
// browser view uses: relative to dir inside it, otherwise the real path.
func viewPath(dir, path string) (real, view string, err error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		path = filepath.Join(home, path[1:])
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	real, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", "", err
	}
	base, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(base, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return real, real, nil
	}
	return real, filepath.ToSlash(rel), nil
}

func openFile(rt *session.Runtime, dir, self string, args openFileArgs) (string, error) {
	real, path, err := viewPath(dir, args.Path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(real)
	if err != nil {
		return "", err
	}
	count := bytes.Count(content, []byte("\n")) + 1
	if args.Lines == nil {
		args.Lines = []session.LineRange{}
	}
	for i := range args.Lines {
		line := &args.Lines[i]
		if line.End == 0 {
			line.End = line.Start
		}
		if line.Start < 1 || line.End < line.Start || line.End > count {
			return "", fmt.Errorf("line range %d-%d is outside the file's %d lines", line.Start, line.End, count)
		}
	}
	rt.OpenFile(self, session.FileOpen{Path: path, Lines: args.Lines})
	return fmt.Sprintf("%s opens in a tab in your session's view.", path), nil
}
