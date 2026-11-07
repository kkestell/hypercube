package ui

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxMentionFiles = 50000

// gitWorktree detects a repository without treating a failed Git command as a
// non-Git workspace. Worktrees have a .git file in place of a directory.
func gitWorktree(dir string) (bool, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	for {
		_, err := os.Lstat(filepath.Join(dir, ".git"))
		if err == nil {
			return true, nil
		}
		if !os.IsNotExist(err) {
			return false, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false, nil
		}
		dir = parent
	}
}

func regularFile(root *os.Root, name string) bool {
	info, err := root.Stat(name)
	return err == nil && info.Mode().IsRegular()
}

func gitMentionFiles(ctx context.Context, root *os.Root) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root.Name(), "ls-files", "--cached", "--others", "--exclude-standard", "--deduplicate", "-z")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	files := []string{}
	for _, name := range strings.Split(string(output), "\x00") {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if name == "" {
			continue
		}
		skip := false
		for _, part := range strings.Split(name, "/") {
			if hidden(part) {
				skip = true
				break
			}
		}
		if !skip && regularFile(root, name) {
			files = append(files, name)
			if len(files) == maxMentionFiles {
				break
			}
		}
	}
	return files, nil
}

func walkMentionFiles(ctx context.Context, root *os.Root) ([]string, error) {
	dir := root.Name()
	files := []string{}
	err := filepath.WalkDir(dir, func(name string, entry fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			return nil
		}
		if name == dir {
			return nil
		}
		if hidden(entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}
		if regularFile(root, rel) {
			files = append(files, filepath.ToSlash(rel))
		}
		if len(files) >= maxMentionFiles {
			return filepath.SkipAll
		}
		return nil
	})
	return files, err
}

func (s *server) mentionFiles(w http.ResponseWriter, r *http.Request) {
	root, err := s.targetRoot(requestTarget(r))
	if fileError(w, err) {
		return
	}
	defer root.Close()
	files, err := mentionFileList(r.Context(), root)
	if fileError(w, err) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func mentionFileList(ctx context.Context, root *os.Root) ([]string, error) {
	inGit, err := gitWorktree(root.Name())
	if err != nil {
		return nil, err
	}
	if inGit {
		return gitMentionFiles(ctx, root)
	}
	return walkMentionFiles(ctx, root)
}
