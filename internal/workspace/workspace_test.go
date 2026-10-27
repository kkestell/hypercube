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
