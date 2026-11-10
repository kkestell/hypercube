package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscard(t *testing.T) {
	// Git refuses local submodule URLs unless file transport is allowed.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")
	sub, run := gitRepo(t)
	write(t, filepath.Join(sub, "lib"), "lib")
	run(sub, "add", "lib")
	run(sub, "commit", "-q", "-m", "lib")

	root, _ := gitRepo(t)
	run(root, "submodule", "-q", "add", sub, "vendor/sub")
	write(t, filepath.Join(root, ".gitignore"), "ignored\n")
	write(t, filepath.Join(root, "tracked"), "tracked")
	run(root, "add", ".gitignore", "tracked")
	run(root, "commit", "-q", "-m", "setup")
	base, err := Head(root)
	if err != nil {
		t.Fatal(err)
	}
	dir, branch := SessionDir(root, "s1"), SessionBranch("s1")
	if err := AddWorktree(root, dir, branch, ""); err != nil {
		t.Fatal(err)
	}

	write(t, filepath.Join(dir, "committed"), "committed")
	run(dir, "add", "committed")
	run(dir, "commit", "-q", "-m", "work")
	write(t, filepath.Join(dir, "tracked"), "modified")
	write(t, filepath.Join(dir, "untracked/file"), "untracked")
	write(t, filepath.Join(dir, "ignored"), "ignored")
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	run(nested, "init", "-q")
	write(t, filepath.Join(nested, "file"), "nested")
	submodule := filepath.Join(dir, "vendor/sub")
	run(submodule, "commit", "-q", "--allow-empty", "-m", "moved")
	write(t, filepath.Join(submodule, "scratch"), "untracked in the submodule")
	if n, err := uncommitted(dir); err != nil || n == 0 {
		t.Fatalf("the worktree is not dirty: %d, %v", n, err)
	}

	if err := Discard(dir, branch, base); err != nil {
		t.Fatal(err)
	}
	if head, err := Head(dir); err != nil || head != base {
		t.Errorf("HEAD = %s, %v; want %s", head, err, base)
	}
	if tip, err := BranchTip(root, branch); err != nil || tip != base {
		t.Errorf("branch tip = %s, %v; want %s", tip, err, base)
	}
	if status, err := git(dir, "status", "--porcelain"); err != nil || status != "" {
		t.Errorf("status = %q, %v", status, err)
	}
	for _, name := range []string{"committed", "untracked", "nested", "vendor/sub/scratch"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s remains", name)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "ignored")); err != nil || string(data) != "ignored" {
		t.Errorf("the ignored file = %q, %v", data, err)
	}
	if err := Removable(root, dir, branch, base); err != nil {
		t.Errorf("Removable after Discard = %v", err)
	}
}

// The session's branch is reset even when the agent left it for another one.
func TestDiscardReturnsToTheSessionBranch(t *testing.T) {
	t.Parallel()
	root, run := gitRepo(t)
	base, err := Head(root)
	if err != nil {
		t.Fatal(err)
	}
	dir, branch := SessionDir(root, "s1"), SessionBranch("s1")
	if err := AddWorktree(root, dir, branch, ""); err != nil {
		t.Fatal(err)
	}
	run(dir, "commit", "-q", "--allow-empty", "-m", "on the session branch")
	run(dir, "switch", "-q", "-c", "other")
	run(dir, "commit", "-q", "--allow-empty", "-m", "on the other branch")
	other, err := Head(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := Discard(dir, branch, base); err != nil {
		t.Fatal(err)
	}
	if current, err := Branch(dir); err != nil || current != branch {
		t.Errorf("checked out branch = %q, %v; want %s", current, err, branch)
	}
	if head, err := Head(dir); err != nil || head != base {
		t.Errorf("HEAD = %s, %v; want %s", head, err, base)
	}
	if tip, err := BranchTip(root, "other"); err != nil || tip != other {
		t.Errorf("the other branch = %s, %v; want %s", tip, err, other)
	}
	if err := Removable(root, dir, branch, base); err != nil {
		t.Errorf("Removable after Discard = %v", err)
	}
}

// A folder under .hypercube/ that is not a worktree belongs to the checkout
// that encloses it, which Discard must leave alone.
func TestDiscardRefusesAFolderThatIsNotAWorktree(t *testing.T) {
	t.Parallel()
	root, run := gitRepo(t)
	base, err := Head(root)
	if err != nil {
		t.Fatal(err)
	}
	run(root, "commit", "-q", "--allow-empty", "-m", "later")
	tip, err := Head(root)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "draft"), "uncommitted")
	dir := SessionDir(root, "plain")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Discard(dir, "main", base); err == nil {
		t.Fatal("Discard accepted a folder that is not a worktree")
	}
	if head, err := Head(root); err != nil || head != tip {
		t.Errorf("the checkout's HEAD = %s, %v; want %s", head, err, tip)
	}
	if data, err := os.ReadFile(filepath.Join(root, "draft")); err != nil || string(data) != "uncommitted" {
		t.Errorf("the checkout's uncommitted file = %q, %v", data, err)
	}
}

// An agent stopped in the middle of a conflicted merge or rebase leaves Git's
// operation state in the worktree.
func TestDiscardAbandonsAnOperationInProgress(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"merge", "rebase"} {
		t.Run(operation, func(t *testing.T) {
			root, run := gitRepo(t)
			base, err := Head(root)
			if err != nil {
				t.Fatal(err)
			}
			dir, branch := SessionDir(root, "s1"), SessionBranch("s1")
			if err := AddWorktree(root, dir, branch, ""); err != nil {
				t.Fatal(err)
			}
			run(root, "branch", "side")
			side := SessionDir(root, "side")
			run(root, "worktree", "add", "-q", side, "side")
			write(t, filepath.Join(side, "file"), "side")
			run(side, "add", "file")
			run(side, "commit", "-q", "-m", "side")
			write(t, filepath.Join(dir, "file"), "session")
			run(dir, "add", "file")
			run(dir, "commit", "-q", "-m", "session")

			cmd := exec.Command("git", "-c", "user.name=t", "-c", "user.email=t@t", operation, "side")
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("git %s did not conflict: %s", operation, out)
			}
			if err := Discard(dir, branch, base); err != nil {
				t.Fatal(err)
			}
			if head, err := Head(dir); err != nil || head != base {
				t.Errorf("HEAD = %s, %v; want %s", head, err, base)
			}
			if status, err := git(dir, "status", "--porcelain"); err != nil || status != "" {
				t.Errorf("status = %q, %v", status, err)
			}
			if state, err := git(dir, "status"); err != nil || strings.Contains(state, "in progress") || strings.Contains(state, "rebase") {
				t.Errorf("an operation remains: %q, %v", state, err)
			}
			if err := Removable(root, dir, branch, base); err != nil {
				t.Errorf("Removable after Discard = %v", err)
			}
		})
	}
}
