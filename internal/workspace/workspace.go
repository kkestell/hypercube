// Package workspace finds the checkout, manages child session branches and
// worktrees, and resolves child session directories.
package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// gitError is a failed git command. It wraps the command's exec error so that
// callers can read the exit code.
type gitError struct {
	args   []string
	output string
	err    error
}

func (e *gitError) Error() string {
	return fmt.Sprintf("git %s: %s", strings.Join(e.args, " "), e.output)
}

func (e *gitError) Unwrap() error { return e.err }

func git(dir string, args ...string) (string, error) {
	return gitWith(dir, nil, "", args...)
}

// gitWith runs git in dir with extra environment variables and standard input,
// and returns its trimmed output.
func gitWith(dir string, env []string, stdin string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", &gitError{args, strings.TrimSpace(stderr.String()), err}
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

// At returns the workspace root for dir: the top level of its checkout, or
// dir itself outside Git. A relative dir is relative to /.
func At(dir string) (string, error) {
	root, err := Resolve("/", dir)
	if err != nil {
		return "", err
	}
	if top, err := Root(root); err == nil {
		root = top
	}
	return root, nil
}

// Root returns the top level of the checkout containing dir.
func Root(dir string) (string, error) {
	return git(dir, "rev-parse", "--show-toplevel")
}

// Branch returns the branch checked out in dir.
func Branch(dir string) (string, error) {
	return git(dir, "branch", "--show-current")
}

// ChildBranch names a child session's branch.
func ChildBranch(parent, name string) string {
	return "hypercube/" + parent + "/" + name
}

// WorktreeDir is where a child session's worktree lives. Worktrees sit inside the
// checkout so that harnesses extend the checkout's folder trust to them.
func WorktreeDir(root, parent, name string) string {
	return filepath.Join(root, ".hypercube", parent, name)
}

// AddWorktree creates the child session's branch from HEAD and checks it out in dir.
// It adds .hypercube/ to the checkout's local excludes.
func AddWorktree(root, dir, branch string) error {
	if err := excludeWorktrees(root); err != nil {
		return err
	}
	existed := exists(root, branch)
	if _, err := git(root, "worktree", "add", "-b", branch, dir, "HEAD"); err != nil {
		if !existed {
			_, deleteErr := git(root, "branch", "-D", branch)
			err = errors.Join(err, deleteErr)
		}
		return err
	}
	if err := setUp(root, dir); err != nil {
		_, removeErr := git(root, "worktree", "remove", "--force", dir)
		_, deleteErr := git(root, "branch", "-D", branch)
		return errors.Join(err, removeErr, deleteErr)
	}
	return nil
}

// excludeWorktrees adds .hypercube/ to the checkout's local excludes.
func excludeWorktrees(root string) error {
	path, err := git(root, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	text := string(data)
	if slices.Contains(strings.Split(text, "\n"), "/.hypercube/") {
		return nil
	}
	rule := "/.hypercube/\n"
	if text != "" && !strings.HasSuffix(text, "\n") {
		rule = "\n" + rule
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString(rule)
	return errors.Join(err, f.Close())
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
	_, err := gitWith(dir, []string{"GIT_TERMINAL_PROMPT=0"}, "", "submodule", "update", "--init", "--recursive")
	return err
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
	ignored, err := gitWith(root, nil, matched, "check-ignore", "--stdin")
	// check-ignore exits 1 when it matches nothing.
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil
	} else if err != nil {
		return err
	}
	// With --directory, Git lists a directory and also the files in it.
	var copied []string
	for _, p := range strings.Split(ignored, "\n") {
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
	dirty, err := isDirty(dir)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("worktree %s has uncommitted changes", dir)
	}
	return nil
}

// isDirty reports whether the checkout in dir has uncommitted changes or
// untracked files.
func isDirty(dir string) (bool, error) {
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
	base := filepath.Join(root, ".hypercube")
	for parent := filepath.Dir(dir); parent == base || strings.HasPrefix(parent, base+string(filepath.Separator)); parent = filepath.Dir(parent) {
		if err := os.Remove(parent); errors.Is(err, syscall.ENOTEMPTY) {
			break
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
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
