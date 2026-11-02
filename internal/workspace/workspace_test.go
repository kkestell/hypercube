package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRoot(t *testing.T) {
	dir := t.TempDir()
	if _, err := Root(dir); err == nil {
		t.Errorf("Root(%s) succeeded before git init", dir)
	}
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{dir, sub} {
		if _, err := Root(d); err != nil {
			t.Errorf("Root(%s) failed after git init: %v", d, err)
		}
	}
}

func TestResolve(t *testing.T) {
	base, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	for _, d := range []string{filepath.Join(base, "rel"), filepath.Join(home, "src")} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(base, "file"), "")

	for dir, want := range map[string]string{
		"rel":                         filepath.Join(base, "rel"),
		"~/src":                       filepath.Join(home, "src"),
		filepath.Join(home, "src"):    filepath.Join(home, "src"),
		filepath.Join(base, "rel/.."): base,
	} {
		if got, err := Resolve(base, dir); err != nil || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", dir, got, err, want)
		}
	}
	for _, dir := range []string{"missing", "file"} {
		if got, err := Resolve(base, dir); err == nil {
			t.Errorf("Resolve(%q) = %q, want an error", dir, got)
		}
	}
}

func TestWorktreeRemoval(t *testing.T) {
	root, run := gitRepo(t)
	run(root, "branch", "other")
	exclude := filepath.Join(root, ".git/info/exclude")
	write(t, exclude, "local-only")
	dir, branch := WorktreeDir(root, "parent", "w1"), ChildBranch("parent", "w1")
	if err := AddWorktree(root, dir, branch); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(exclude); string(data) != "local-only\n/.hypercube/\n" {
		t.Errorf("exclude = %q", data)
	}
	if dirty, err := isDirty(root); err != nil || dirty {
		t.Errorf("worktree dir makes the checkout dirty: %v, %v", dirty, err)
	}

	run(dir, "commit", "-q", "--allow-empty", "-m", "work")
	if err := RemoveWorktree(root, dir, branch); err == nil {
		t.Error("removed an unmerged branch")
	}
	run(root, "merge", "-q", branch)
	write(t, filepath.Join(dir, "scratch"), "")
	if err := RemoveWorktree(root, dir, branch); err == nil {
		t.Error("removed a dirty worktree")
	}
	if err := os.Remove(filepath.Join(dir, "scratch")); err != nil {
		t.Fatal(err)
	}
	run(dir, "checkout", "-q", "--detach")
	run(dir, "commit", "-q", "--allow-empty", "-m", "detached")
	if err := RemoveWorktree(root, dir, branch); err == nil {
		t.Error("removed a worktree with an unmerged detached commit")
	}
	run(dir, "checkout", "-q", branch)
	// The branch is merged into HEAD but not into its upstream.
	run(dir, "branch", "-q", "--set-upstream-to=other")
	if err := RemoveWorktree(root, dir, branch); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("worktree directory remains")
	}
	if _, err := git(root, "rev-parse", "--verify", branch); err == nil {
		t.Error("branch remains")
	}
}

// write creates a file and the directories above it.
func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitRepo(t *testing.T) (string, func(dir string, args ...string)) {
	t.Helper()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	root := t.TempDir()
	run(root, "init", "-q", "-b", "main")
	run(root, "commit", "-q", "--allow-empty", "-m", "init")
	return root, run
}

func TestGitWithKeepsStderrOutOfOutput(t *testing.T) {
	root, run := gitRepo(t)
	hook := filepath.Join(root, ".git/fsmonitor-test")
	write(t, hook, "#!/bin/sh\necho diagnostic-from-fsmonitor >&2\nprintf 'token\\0'\n")
	if err := os.Chmod(hook, 0o755); err != nil {
		t.Fatal(err)
	}
	run(root, "config", "core.fsmonitor", hook)
	write(t, filepath.Join(root, ".gitignore"), ".env\n")
	write(t, filepath.Join(root, ".env"), "value")
	got, err := gitWith(root, nil, ".env", "check-ignore", "--stdin")
	if err != nil || got != ".env" {
		t.Fatalf("check-ignore output = %q, %v", got, err)
	}
}

func TestWorktreeSetUp(t *testing.T) {
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
	files := map[string]string{
		".gitignore":         ".toolchain/\n.env\ncache/\nbuild/\n",
		".worktreeinclude":   ".toolchain/\n.env\ncache/keep\nsrc/\n",
		".toolchain/bin/zig": "zig",
		".env":               "env",
		"cache/keep":         "keep",
		"cache/drop":         "drop",
		"build/out":          "out",
		"src/untracked.go":   "not ignored",
	}
	for name, data := range files {
		write(t, filepath.Join(root, name), data)
	}
	run(root, "add", ".gitignore", ".worktreeinclude")
	run(root, "commit", "-q", "-m", "setup")

	dir, branch := WorktreeDir(root, "parent", "w1"), ChildBranch("parent", "w1")
	if err := AddWorktree(root, dir, branch); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".toolchain/bin/zig", ".env", "cache/keep", "vendor/sub/lib"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s missing from the worktree: %v", name, err)
		} else if want := files[name]; want != "" && string(data) != want {
			t.Errorf("%s = %q, want %q", name, data, want)
		}
	}
	for _, name := range []string{"cache/drop", "build/out", "src/untracked.go"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s was copied into the worktree", name)
		}
	}
	if dirty, err := isDirty(dir); err != nil || dirty {
		t.Errorf("new worktree is dirty: %v, %v", dirty, err)
	}

	write(t, filepath.Join(dir, "vendor/sub/lib"), "changed")
	if err := RemoveWorktree(root, dir, branch); err == nil {
		t.Error("removed a worktree with a changed submodule")
	}
	run(filepath.Join(dir, "vendor/sub"), "checkout", "-q", "lib")
	if err := RemoveWorktree(root, dir, branch); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("worktree directory remains")
	}
}

func TestRemoveMissingWorktree(t *testing.T) {
	root, run := gitRepo(t)
	dir, branch := WorktreeDir(root, "parent", "w1"), ChildBranch("parent", "w1")
	if err := AddWorktree(root, dir, branch); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorktree(root, dir, branch); err != nil {
		t.Fatalf("worktree directory deleted: %v", err)
	}
	if _, err := git(root, "rev-parse", "--verify", branch); err == nil {
		t.Error("branch remains")
	}

	if err := AddWorktree(root, dir, branch); err != nil {
		t.Fatal(err)
	}
	run(root, "worktree", "remove", dir)
	run(root, "branch", "-D", branch)
	if err := RemoveWorktree(root, dir, branch); err != nil {
		t.Errorf("worktree and branch gone: %v", err)
	}

	run(root, "branch", branch)
	run(root, "checkout", "-q", branch)
	run(root, "commit", "-q", "--allow-empty", "-m", "unmerged")
	run(root, "checkout", "-q", "main")
	if err := RemoveWorktree(root, dir, branch); err == nil {
		t.Error("removed an unmerged branch without a worktree")
	}
}

func TestAddWorktreeReportsUnreadableExclude(t *testing.T) {
	root, _ := gitRepo(t)
	exclude := filepath.Join(root, ".git/info/exclude")
	write(t, exclude, "local-only\n")
	if err := os.Chmod(exclude, 0); err != nil {
		t.Fatal(err)
	}
	if err := AddWorktree(root, WorktreeDir(root, "parent", "w1"), ChildBranch("parent", "w1")); err == nil {
		t.Error("AddWorktree ignored an unreadable exclude file")
	}
}
