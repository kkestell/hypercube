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
	os.Mkdir(sub, 0o755)
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
		os.Mkdir(d, 0o755)
	}
	os.WriteFile(filepath.Join(base, "file"), nil, 0o644)

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
	root := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(root, "init", "-q", "-b", "main")
	run(root, "commit", "-q", "--allow-empty", "-m", "init")
	run(root, "branch", "other")
	exclude := filepath.Join(root, ".git/info/exclude")
	os.WriteFile(exclude, []byte("local-only"), 0o644)
	dir, branch := WorktreeDir(root, "w1"), WorkerBranch("w1")
	if err := AddWorktree(root, dir, branch); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(exclude); string(data) != "local-only\n/.hypercube/\n" {
		t.Errorf("exclude = %q", data)
	}
	if dirty, err := Dirty(root); err != nil || dirty {
		t.Errorf("worktree dir makes the checkout dirty: %v, %v", dirty, err)
	}

	run(dir, "commit", "-q", "--allow-empty", "-m", "work")
	if err := RemoveWorktree(root, dir, branch); err == nil {
		t.Error("removed an unmerged branch")
	}
	run(root, "merge", "-q", branch)
	os.WriteFile(filepath.Join(dir, "scratch"), nil, 0o644)
	if err := RemoveWorktree(root, dir, branch); err == nil {
		t.Error("removed a dirty worktree")
	}
	os.Remove(filepath.Join(dir, "scratch"))
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

func TestWorktreeSetUp(t *testing.T) {
	// Git refuses local submodule URLs unless file transport is allowed.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")
	sub, run := gitRepo(t)
	os.WriteFile(filepath.Join(sub, "lib"), []byte("lib"), 0o644)
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
		os.MkdirAll(filepath.Join(root, filepath.Dir(name)), 0o755)
		os.WriteFile(filepath.Join(root, name), []byte(data), 0o644)
	}
	run(root, "add", ".gitignore", ".worktreeinclude")
	run(root, "commit", "-q", "-m", "setup")

	dir, branch := WorktreeDir(root, "w1"), WorkerBranch("w1")
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
	if dirty, err := Dirty(dir); err != nil || dirty {
		t.Errorf("new worktree is dirty: %v, %v", dirty, err)
	}

	os.WriteFile(filepath.Join(dir, "vendor/sub/lib"), []byte("changed"), 0o644)
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
	dir, branch := WorktreeDir(root, "w1"), WorkerBranch("w1")
	if err := AddWorktree(root, dir, branch); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(dir)
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
