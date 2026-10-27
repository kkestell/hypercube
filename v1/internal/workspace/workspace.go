// Package workspace finds the checkout, manages worker branches and
// worktrees, and resolves worker directories.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Resolve returns dir as an absolute path. A leading ~/ is the home
// directory, and a relative path is relative to base. dir must be an
// existing directory.
func Resolve(base, dir string) (string, error) {
	if rest, ok := strings.CutPrefix(dir, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, rest)
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(base, dir)
	}
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", dir)
	}
	return dir, nil
}

// Root returns the top level of the checkout containing dir.
func Root(dir string) (string, error) {
	return git(dir, "rev-parse", "--show-toplevel")
}

// MainRoot returns the top level of the main checkout, also from a worktree.
func MainRoot(dir string) (string, error) {
	common, err := git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	return filepath.Dir(common), err
}

// Branch returns the branch checked out in dir.
func Branch(dir string) (string, error) {
	return git(dir, "branch", "--show-current")
}

// Short returns path with the home directory written as ~.
func Short(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// Name names a workspace from its checkout path. It is used for the state
// file and the tmux session.
func Name(root string) string {
	sum := sha256.Sum256([]byte(root))
	return unsafe.ReplaceAllString(filepath.Base(root), "-") + "-" + hex.EncodeToString(sum[:3])
}

// WorkerBranch names a worker's branch.
func WorkerBranch(name string) string {
	return "hypercube/" + name
}

// WorktreeDir is where a worker's worktree lives. Worktrees sit inside the
// checkout so that harnesses extend the checkout's folder trust to them.
func WorktreeDir(root, name string) string {
	return filepath.Join(root, ".hypercube", name)
}

// AddWorktree creates the worker's branch from HEAD and checks it out in dir.
// It adds .hypercube/ to the checkout's local excludes.
func AddWorktree(root, dir, branch string) error {
	exclude, err := git(root, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(exclude) {
		exclude = filepath.Join(root, exclude)
	}
	data, _ := os.ReadFile(exclude)
	if !slices.Contains(strings.Split(string(data), "\n"), "/.hypercube/") {
		rule := "/.hypercube/\n"
		if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
			rule = "\n" + rule
		}
		if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, err = f.WriteString(rule)
		f.Close()
		if err != nil {
			return err
		}
	}
	if _, err := git(root, "worktree", "add", "-b", branch, dir, "HEAD"); err != nil {
		return err
	}
	if err := setUp(root, dir); err != nil {
		git(root, "worktree", "remove", "--force", dir)
		git(root, "branch", "-D", branch)
		return err
	}
	return nil
}

// setUp gives a new worktree what the checkout has but Git does not check
// out: the gitignored paths that .worktreeinclude selects, and submodules.
func setUp(root, dir string) error {
	if err := copyIncluded(root, dir); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitmodules")); err != nil {
		return nil
	}
	cmd := exec.Command("git", "submodule", "update", "--init", "--recursive")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git submodule update: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// copyIncluded copies the paths in root that match a pattern in root's
// .worktreeinclude, in .gitignore syntax, and that Git ignores. The copies
// are APFS clones, so even large directories copy quickly.
func copyIncluded(root, dir string) error {
	if _, err := os.Stat(filepath.Join(root, ".worktreeinclude")); err != nil {
		return nil
	}
	matched, err := git(root, "ls-files", "--others", "--ignored", "--exclude-from=.worktreeinclude", "--directory")
	if err != nil || matched == "" {
		return err
	}
	cmd := exec.Command("git", "check-ignore", "--stdin")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(matched)
	out, err := cmd.Output()
	// check-ignore exits 1 when it matches nothing.
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return nil
	} else if err != nil {
		return fmt.Errorf("git check-ignore: %w", err)
	}
	// With --directory, Git lists a directory and also the files in it.
	var copied []string
	for _, p := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if slices.ContainsFunc(copied, func(d string) bool { return strings.HasPrefix(p, d) }) {
			continue
		}
		if strings.HasSuffix(p, "/") {
			copied = append(copied, p)
		}
		target := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if out, err := exec.Command("cp", "-c", "-R", filepath.Join(root, p), target).CombinedOutput(); err != nil {
			return fmt.Errorf("copying %s: %s", p, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// Removable returns an error if the worktree has uncommitted changes, or its
// branch or checked-out commit is not merged into HEAD. A worktree or branch
// that no longer exists passes.
func Removable(root, dir, branch string) error {
	if exists(root, branch) {
		if _, err := git(root, "merge-base", "--is-ancestor", branch, "HEAD"); err != nil {
			return fmt.Errorf("branch %s is not merged into HEAD", branch)
		}
	}
	if !isWorktree(dir) {
		return nil
	}
	head, err := git(dir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if _, err := git(root, "merge-base", "--is-ancestor", head, "HEAD"); err != nil {
		return fmt.Errorf("worktree %s has commits not merged into HEAD", dir)
	}
	dirty, err := Dirty(dir)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("worktree %s has uncommitted changes", dir)
	}
	return nil
}

// Dirty reports whether the checkout in dir has uncommitted changes or
// untracked files.
func Dirty(dir string) (bool, error) {
	status, err := git(dir, "status", "--porcelain")
	return status != "", err
}

// RemoveWorktree removes the worktree and deletes its branch if Removable
// allows it. Removable has checked the branch against HEAD, so the deletion
// ignores the branch's upstream, and has checked the worktree, so --force
// only lets Git remove a worktree with submodules. A worktree or branch that
// is already gone is skipped.
func RemoveWorktree(root, dir, branch string) error {
	if err := Removable(root, dir, branch); err != nil {
		return err
	}
	if isWorktree(dir) {
		if _, err := git(root, "worktree", "remove", "--force", dir); err != nil {
			return err
		}
	}
	// Forget a worktree whose directory was deleted, so its branch can go.
	if _, err := git(root, "worktree", "prune"); err != nil {
		return err
	}
	if !exists(root, branch) {
		return nil
	}
	_, err := git(root, "branch", "-D", branch)
	return err
}

// exists reports whether the branch exists.
func exists(root, branch string) bool {
	_, err := git(root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// isWorktree reports whether dir is the top level of a checkout. A deleted
// worktree is not, and neither is a plain directory left in its place.
func isWorktree(dir string) bool {
	top, err := Root(dir)
	if err != nil {
		return false
	}
	top, _ = filepath.EvalSymlinks(top)
	dir, _ = filepath.EvalSymlinks(dir)
	return top == dir
}
