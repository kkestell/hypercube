package mcp

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kkestell/hypercube/internal/session"
)

type openFileArgs struct {
	Path  string              `json:"path"`
	Lines []session.LineRange `json:"lines,omitempty"`
}

func openFile(rt *session.Runtime, dir, self string, args openFileArgs) (string, error) {
	path := args.Path
	if filepath.IsAbs(path) {
		var err error
		path, err = filepath.Rel(dir, path)
		if err != nil {
			return "", err
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	content, err := root.ReadFile(path)
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
	path = filepath.ToSlash(filepath.Clean(path))
	rt.OpenFile(self, session.FileOpen{Path: path, Lines: args.Lines})
	return fmt.Sprintf("%s opens in a tab in your session's view.", path), nil
}
