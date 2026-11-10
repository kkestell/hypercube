// Package workspace finds the checkout, manages the branches and worktrees of
// worktree sessions and child sessions, measures their merge state, and lists
// uncommitted files and their diffs.
package workspace

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
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
	out, err := run(dir, env, stdin, 0, args...)
	return strings.TrimSpace(out), err
}

// run preserves output, including on a nonzero exit, and bounds it when limit > 0.
func run(dir string, env []string, stdin string, limit int, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	var reader io.Reader = pipe
	if limit > 0 {
		reader = io.LimitReader(pipe, int64(limit)+1)
	}
	out, readErr := io.ReadAll(reader)
	if limit > 0 && len(out) > limit {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", ErrTooLarge
	}
	err = cmd.Wait()
	if readErr != nil {
		return "", readErr
	}
	if err != nil {
		return string(out), &gitError{args, strings.TrimSpace(stderr.String()), err}
	}
	return string(out), nil
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

// ErrNotGit means an existing directory has no Git metadata in its ancestors.
var ErrNotGit = errors.New("folder is outside Git")

// HasGitMetadata reports whether dir or an ancestor has a .git entry, including
// a worktree marker file. Its presence does not establish that Git works.
func HasGitMetadata(dir string) (bool, error) {
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

// At returns the workspace root for dir: the top level of its checkout, or
// dir itself outside Git. A relative dir is relative to /.
func At(dir string) (string, error) {
	root, err := Resolve("/", dir)
	if err != nil {
		return "", err
	}
	top, err := Root(root)
	if errors.Is(err, ErrNotGit) {
		return root, nil
	}
	return top, err
}

// Root returns the top level of the checkout containing dir, or ErrNotGit for
// an existing directory without Git metadata. Other Git failures are preserved.
func Root(dir string) (string, error) {
	top, gitErr := git(dir, "rev-parse", "--show-toplevel")
	if gitErr == nil {
		return top, nil
	}
	dir, err := Resolve("/", dir)
	if err != nil {
		return "", err
	}
	// Git discovers repositories through physical parents, not symlink aliases.
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	inGit, err := HasGitMetadata(dir)
	if err != nil {
		return "", err
	}
	if inGit {
		return "", gitErr
	}
	return "", ErrNotGit
}

// CheckChildDir refuses a dir in a worktree of the workspace's repository
// other than the workspace's checkout and the worktrees in the workspace's
// .hypercube directory. Hypercube neither tracks nor removes such a worktree.
// Directories outside Git and in other repositories pass.
func CheckChildDir(workspace, dir string) error {
	top, err := Root(dir)
	if errors.Is(err, ErrNotGit) {
		return nil
	}
	if err != nil {
		return err
	}
	common, err := git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	own, err := git(workspace, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	checkout, err := Root(workspace)
	if err != nil {
		return err
	}
	// Git reports physical paths.
	physical, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return err
	}
	managed := filepath.Join(physical, ".hypercube") + string(filepath.Separator)
	if common != own || top == checkout || strings.HasPrefix(top, managed) {
		return nil
	}
	return fmt.Errorf("%s is a Git worktree that Hypercube does not manage; omit dir to give the child its own worktree", top)
}

// Branch returns the branch checked out in dir.
func Branch(dir string) (string, error) {
	return git(dir, "branch", "--show-current")
}

// Head returns the commit checked out in dir.
func Head(dir string) (string, error) {
	return git(dir, "rev-parse", "HEAD")
}

// SessionBranch names a top-level worktree session's branch. It differs from
// the hypercube/<id>/ prefix that the session's children's branches use,
// because Git cannot keep a branch and branches beneath its name.
func SessionBranch(id string) string {
	return "hypercube/session-" + id
}

// SessionDir is where a top-level worktree session's worktree lives.
func SessionDir(root, id string) string {
	return filepath.Join(root, ".hypercube", id)
}

// ChildBranch names a child session's branch.
func ChildBranch(parent, name string) string {
	return "hypercube/" + parent + "/" + name
}

// WorktreeDir is where a child session's worktree lives: beside its parent's
// worktree in the workspace's .hypercube directory, never inside it. root is
// the workspace's path, also for a parent in its own worktree. Worktrees sit
// inside the checkout so that harnesses extend the checkout's folder trust to
// them.
func WorktreeDir(root, parent, name string) string {
	return filepath.Join(root, ".hypercube", parent+"-"+name)
}

// AddWorktree creates the new branch at start and checks it out in dir. It
// refuses a branch that exists. start is any commit or ref that resolves in
// root; empty means HEAD. It adds .hypercube/ to the checkout's local excludes.
func AddWorktree(root, dir, branch, start string) error {
	if err := CheckNewBranch(root, branch); err != nil {
		return err
	}
	start = cmp.Or(start, "HEAD")
	commit, err := git(root, "rev-parse", "--verify", "--end-of-options", start+"^{commit}")
	if err != nil {
		return fmt.Errorf("starting point %s is not a commit: %w", start, err)
	}
	if err := excludeWorktrees(root); err != nil {
		return err
	}
	if _, err := git(root, "worktree", "add", "-b", branch, dir, commit); err != nil {
		if exists(root, branch) {
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
// .worktreeinclude, in .gitignore syntax, and that Git ignores. Where the file
// system supports it, the copies are copy-on-write clones, so even large
// directories copy quickly.
func copyIncluded(root, dir string) error {
	if _, err := os.Stat(filepath.Join(root, ".worktreeinclude")); err != nil {
		return nil
	}
	matched, err := run(root, nil, "", 0, "ls-files", "--others", "--ignored", "--exclude-from=.worktreeinclude", "--directory", "-z")
	if err != nil || matched == "" {
		return err
	}
	ignored, err := run(root, nil, matched, 0, "check-ignore", "--stdin", "-z")
	// check-ignore exits 1 when it matches nothing.
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil
	} else if err != nil {
		return err
	}
	// With --directory, Git lists a directory and also the files in it.
	var copied []string
	for p := range strings.SplitSeq(strings.TrimSuffix(ignored, "\x00"), "\x00") {
		// Managed worktrees belong to other sessions, even with a broad include.
		if p == ".hypercube" || strings.HasPrefix(p, ".hypercube/") {
			continue
		}
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
		if out, err := exec.Command("cp", "-R", cloneFlag(), filepath.Join(root, p), target).CombinedOutput(); err != nil {
			return fmt.Errorf("copying %s: %s", p, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// cloneFlag is the cp option that clones files where the file system can: APFS
// on macOS, Btrfs and XFS on Linux. Elsewhere cp copies them.
func cloneFlag() string {
	if runtime.GOOS == "darwin" {
		return "-c"
	}
	return "--reflink=auto"
}

// Removable returns an error if the worktree has uncommitted changes, its
// branch or checked-out commit has commits that no kept ref reaches, or one of
// its submodules has commits that no remote branch of that submodule has. The
// kept refs are root's HEAD, base when it is not empty, and every
// remote-tracking branch. A worktree or branch that no longer exists passes.
func Removable(root, dir, branch, base string) error {
	if exists(root, branch) {
		if n, err := unkept(root, "refs/heads/"+branch, base); err != nil {
			return err
		} else if n > 0 {
			return fmt.Errorf("branch %s has commits that are not on %s", branch, keptRefs(base))
		}
	}
	if !isWorktree(dir) {
		return nil
	}
	head, err := git(dir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if n, err := unkept(root, head, base); err != nil {
		return err
	} else if n > 0 {
		return fmt.Errorf("worktree %s has commits that are not on %s", dir, keptRefs(base))
	}
	n, err := uncommitted(dir)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("worktree %s has uncommitted changes", dir)
	}
	path, err := unpushedSubmodule(dir)
	if err != nil {
		return err
	}
	if path != "" {
		return fmt.Errorf("submodule %s has commits that are not on a remote branch", path)
	}
	return nil
}

// Discard checks out branch in the worktree dir, moved to commit, and deletes
// the worktree's uncommitted changes and untracked files, including those of
// its submodules. Ignored files stay. It refuses a dir that is not the top
// level of a worktree, because Git would act on the checkout that encloses it.
// A merge, cherry-pick, revert, or rebase in progress is abandoned.
func Discard(dir, branch, commit string) error {
	if !isWorktree(dir) {
		return fmt.Errorf("%s is not a worktree", dir)
	}
	// Git refuses to switch branches mid-operation. The reset clears merge,
	// cherry-pick, and revert state; a rebase's state is left for rebase --quit.
	if _, err := git(dir, "reset", "--quiet", "--hard"); err != nil {
		return err
	}
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		path, err := git(dir, "rev-parse", "--git-path", name)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		if _, err := os.Stat(path); err == nil {
			if _, err := git(dir, "rebase", "--quit"); err != nil {
				return err
			}
			break
		}
	}
	if _, err := git(dir, "switch", "--quiet", "--discard-changes", "--recurse-submodules", "--force-create", branch, commit); err != nil {
		return err
	}
	// The second -f removes untracked nested repositories.
	if _, err := git(dir, "clean", "-ffdq"); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitmodules")); err == nil {
		_, err = git(dir, "submodule", "foreach", "--quiet", "--recursive", "git clean -ffdq")
		return err
	}
	return nil
}

// keptRefs names the refs whose commits Removable keeps for base.
func keptRefs(base string) string {
	if base == "" {
		return "the checkout's branch or a remote branch"
	}
	return "the checkout's branch, the session's starting commit, or a remote branch"
}

// unkept counts the commits reachable from tip that no kept ref reaches.
func unkept(root, tip, base string) (int, error) {
	args := []string{"rev-list", "--count", tip, "--not", "HEAD"}
	if base != "" {
		args = append(args, base)
	}
	count, err := git(root, append(args, "--remotes")...)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(count)
}

// unpushedSubmodule returns the path, relative to dir, of the first initialized
// submodule at any depth whose HEAD or local branches have commits that none of
// its remote-tracking branches has. A submodule's repository lives in the
// worktree's Git directory, so removing the worktree deletes those commits.
func unpushedSubmodule(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, ".gitmodules")); err != nil {
		return "", nil
	}
	paths, err := run(dir, nil, "", 0, "submodule", "foreach", "--quiet", "--recursive", `printf "%s\0" "$displaypath"`)
	if err != nil || paths == "" {
		return "", err
	}
	for path := range strings.SplitSeq(strings.TrimSuffix(paths, "\x00"), "\x00") {
		count, err := git(filepath.Join(dir, path), "rev-list", "--count", "HEAD", "--branches", "--not", "--remotes")
		if err != nil {
			return "", err
		}
		if count != "0" {
			return path, nil
		}
	}
	return "", nil
}

// uncommitted counts the changed and untracked files in the checkout in dir,
// listing each file in an untracked folder.
func uncommitted(dir string) (int, error) {
	// Status runs while agents write; it must not lock their index to refresh it.
	status, err := gitWith(dir, []string{"GIT_OPTIONAL_LOCKS=0"}, "", "status", "--porcelain", "--untracked-files=all")
	if err != nil || status == "" {
		return 0, err
	}
	return strings.Count(status, "\n") + 1, nil
}

// Merge is a worktree branch measured against the branch checked out in its
// checkout.
type Merge struct {
	Branch      string `json:"branch"`
	Target      string `json:"target"`      // the checkout's branch
	Unmerged    int    `json:"unmerged"`    // commits made after base that are not in the target
	Committed   bool   `json:"committed"`   // the branch has moved past its base
	Uncommitted int    `json:"uncommitted"` // changed and untracked files in the worktree
	// Error is why the status could not be read; the other fields but Branch
	// are then empty. MergeStatus leaves it empty.
	Error string `json:"error,omitempty"`
}

// MergeStatus measures the branch checked out in the worktree dir, which
// started at the commit base, against the checkout at root.
func MergeStatus(root, dir, branch, base string) (Merge, error) {
	m := Merge{Branch: branch}
	var err error
	if m.Target, err = Branch(root); err != nil {
		return Merge{}, err
	}
	// Commits on the way to base are not the session's work, so a session
	// started at a pull request's head counts only the commits it made.
	args := []string{"rev-list", "--count", branch, "--not", "HEAD"}
	if base != "" {
		args = append(args, base)
	}
	count, err := git(root, args...)
	if err != nil {
		return Merge{}, err
	}
	if m.Unmerged, err = strconv.Atoi(count); err != nil {
		return Merge{}, err
	}
	tip, err := git(root, "rev-parse", branch)
	if err != nil {
		return Merge{}, err
	}
	m.Committed = tip != base
	if m.Uncommitted, err = uncommitted(dir); err != nil {
		return Merge{}, err
	}
	return m, nil
}

// RemoveWorktree removes the worktree and deletes its branch if Removable
// allows it with base. Removable has checked that a kept ref reaches every
// commit on the branch, so the deletion ignores the branch's upstream, and has
// checked the worktree and its submodules, so --force only lets Git remove a
// worktree with submodules. A worktree or branch that is already gone is skipped.
func RemoveWorktree(root, dir, branch, base string) error {
	if err := Removable(root, dir, branch, base); err != nil {
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
	if err := removeEmptyParents(dir); err != nil {
		return err
	}
	if !exists(root, branch) {
		return nil
	}
	_, err := git(root, "branch", "-D", branch)
	return err
}

// removeEmptyParents removes the directories between the worktree dir and its
// .hypercube directory, and that directory too, while they are empty.
func removeEmptyParents(dir string) error {
	sep := string(filepath.Separator)
	i := strings.LastIndex(dir, sep+".hypercube"+sep)
	if i < 0 {
		return nil
	}
	base := dir[:i+len(sep+".hypercube")]
	for parent := filepath.Dir(dir); len(parent) >= len(base); parent = filepath.Dir(parent) {
		if err := os.Remove(parent); errors.Is(err, syscall.ENOTEMPTY) {
			return nil
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// exists reports whether the branch exists.
func exists(root, branch string) bool {
	_, err := git(root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// BranchTip returns the commit at the tip of the branch, or "" when the branch
// does not exist.
func BranchTip(root, branch string) (string, error) {
	if !exists(root, branch) {
		return "", nil
	}
	return git(root, "rev-parse", "refs/heads/"+branch)
}

// HasCommit reports whether the commit's object is in root's repository.
func HasCommit(root, commit string) bool {
	_, err := git(root, "cat-file", "-e", "--end-of-options", commit+"^{commit}")
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
