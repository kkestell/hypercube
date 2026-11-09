package workspace

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoot(t *testing.T) {
	dir := t.TempDir()
	if _, err := Root(dir); !errors.Is(err, ErrNotGit) {
		t.Errorf("Root(%s) = %v, want ErrNotGit", dir, err)
	}
	if got, err := At(dir); err != nil || got != dir {
		t.Errorf("At(%s) = %q, %v", dir, got, err)
	}
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{dir, sub} {
		for _, discover := range []func(string) (string, error){Root, At} {
			if got, err := discover(d); err != nil || got != want {
				t.Errorf("discovery(%s) = %q, %v; want %q", d, got, err, want)
			}
		}
	}
}

func TestRootWorktree(t *testing.T) {
	root, run := gitRepo(t)
	dir := filepath.Join(t.TempDir(), "worktree")
	run(root, "worktree", "add", "-qb", "linked", dir)
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Root(dir); err != nil || got != want {
		t.Fatalf("Root(worktree) = %q, %v; want %q", got, err, want)
	}
}

func TestRootFailures(t *testing.T) {
	root, _ := gitRepo(t)
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	invalid := t.TempDir()
	write(t, filepath.Join(invalid, ".git"), "gitdir: missing\n")
	alias := filepath.Join(t.TempDir(), "linked-sub")
	if err := os.Symlink(sub, alias); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{sub, invalid, alias} {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\necho test-git-refusal >&2\nexit 128\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			for _, discover := range []func(string) (string, error){Root, At} {
				if _, err := discover(dir); err == nil || errors.Is(err, ErrNotGit) || !strings.Contains(err.Error(), "test-git-refusal") {
					t.Errorf("discovery(%s) = %v, want Git refusal", dir, err)
				}
			}
		})
	}
	if _, err := Root(invalid); err == nil || errors.Is(err, ErrNotGit) {
		t.Errorf("invalid marker = %v, want Git error", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	for _, discover := range []func(string) (string, error){Root, At} {
		if _, err := discover(missing); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("missing directory = %v, want not-exist error", err)
		}
	}
}

func TestRootWithoutGitExecutable(t *testing.T) {
	root, _ := gitRepo(t)
	plain := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	for _, discover := range []func(string) (string, error){Root, At} {
		var startErr *exec.Error
		if _, err := discover(root); !errors.As(err, &startErr) || errors.Is(err, ErrNotGit) {
			t.Errorf("checkout without executable = %v, want command-start error", err)
		}
	}
	if _, err := Root(plain); !errors.Is(err, ErrNotGit) {
		t.Errorf("plain directory without executable = %v, want ErrNotGit", err)
	}
	if got, err := At(plain); err != nil || got != plain {
		t.Errorf("At(plain) without executable = %q, %v", got, err)
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
	if err := AddWorktree(root, dir, branch, ""); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(exclude); string(data) != "local-only\n/.hypercube/\n" {
		t.Errorf("exclude = %q", data)
	}
	if n, err := uncommitted(root); err != nil || n > 0 {
		t.Errorf("worktree dir makes the checkout dirty: %v, %v", n, err)
	}

	run(dir, "commit", "-q", "--allow-empty", "-m", "work")
	if err := RemoveWorktree(root, dir, branch, ""); err == nil {
		t.Error("removed an unmerged branch")
	}
	run(root, "merge", "-q", branch)
	write(t, filepath.Join(dir, "scratch"), "")
	if err := RemoveWorktree(root, dir, branch, ""); err == nil {
		t.Error("removed a dirty worktree")
	}
	if err := os.Remove(filepath.Join(dir, "scratch")); err != nil {
		t.Fatal(err)
	}
	run(dir, "checkout", "-q", "--detach")
	run(dir, "commit", "-q", "--allow-empty", "-m", "detached")
	if err := RemoveWorktree(root, dir, branch, ""); err == nil {
		t.Error("removed a worktree with an unmerged detached commit")
	}
	run(dir, "checkout", "-q", branch)
	// The branch is merged into HEAD but not into its upstream.
	run(dir, "branch", "-q", "--set-upstream-to=other")
	if err := RemoveWorktree(root, dir, branch, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("worktree directory remains")
	}
	if _, err := git(root, "rev-parse", "--verify", branch); err == nil {
		t.Error("branch remains")
	}
}

// A child may work in the checkout, a Hypercube worktree, or another folder,
// but not in a worktree of the same repository that Hypercube did not create.
func TestCheckChildDir(t *testing.T) {
	root, run := gitRepo(t)
	managed := WorktreeDir(root, "parent", "w1")
	if err := AddWorktree(root, managed, ChildBranch("parent", "w1"), ""); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "foreign")
	run(root, "worktree", "add", "-q", "-b", "foreign", foreign, "HEAD")
	other, _ := gitRepo(t)
	write(t, filepath.Join(root, "sub", "file"), "")
	write(t, filepath.Join(foreign, "sub", "file"), "")
	for _, dir := range []string{root, filepath.Join(root, "sub"), managed, other, t.TempDir()} {
		if err := CheckChildDir(root, dir); err != nil {
			t.Errorf("CheckChildDir(%s) = %v", dir, err)
		}
	}
	for _, dir := range []string{foreign, filepath.Join(foreign, "sub")} {
		if err := CheckChildDir(root, dir); err == nil || !strings.Contains(err.Error(), "Hypercube does not manage") {
			t.Errorf("CheckChildDir(%s) = %v, want a refusal", dir, err)
		}
	}
	// A workspace in a subfolder of its checkout keeps its worktrees there.
	sub := filepath.Join(root, "sub")
	nested := WorktreeDir(sub, "parent", "w2")
	if err := AddWorktree(sub, nested, ChildBranch("parent", "w2"), ""); err != nil {
		t.Fatal(err)
	}
	if err := CheckChildDir(sub, nested); err != nil {
		t.Errorf("CheckChildDir(%s) = %v", nested, err)
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
	if err := AddWorktree(root, dir, branch, ""); err != nil {
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
	if n, err := uncommitted(dir); err != nil || n > 0 {
		t.Errorf("new worktree is dirty: %v, %v", n, err)
	}

	write(t, filepath.Join(dir, "vendor/sub/lib"), "changed")
	if err := RemoveWorktree(root, dir, branch, ""); err == nil {
		t.Error("removed a worktree with a changed submodule")
	}
	run(filepath.Join(dir, "vendor/sub"), "checkout", "-q", "lib")
	if err := RemoveWorktree(root, dir, branch, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("worktree directory remains")
	}
}

func TestAddWorktreeAtStart(t *testing.T) {
	root, run := gitRepo(t)
	first, err := Head(root)
	if err != nil {
		t.Fatal(err)
	}
	run(root, "commit", "-q", "--allow-empty", "-m", "later")
	dir, branch := SessionDir(root, "s1"), SessionBranch("s1")
	if err := AddWorktree(root, dir, branch, "HEAD~1"); err != nil {
		t.Fatal(err)
	}
	if head, err := Head(dir); err != nil || head != first {
		t.Errorf("worktree HEAD = %s, %v; want %s", head, err, first)
	}
	dir, branch = SessionDir(root, "s2"), SessionBranch("s2")
	if err := AddWorktree(root, dir, branch, "missing"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("AddWorktree at an unresolvable start = %v", err)
	}
	if exists(root, branch) {
		t.Error("an unresolvable start created a branch")
	}
}

// A worktree is removable when its session's starting commit or a remote
// branch holds its commits, even when the checkout's HEAD does not.
func TestRemovableKeepsStartAndRemoteCommits(t *testing.T) {
	root, run := gitRepo(t)
	run(root, "checkout", "-q", "-b", "side")
	run(root, "commit", "-q", "--allow-empty", "-m", "side")
	start, err := Head(root)
	if err != nil {
		t.Fatal(err)
	}
	run(root, "checkout", "-q", "main")
	run(root, "branch", "-q", "-D", "side")
	dir, branch := SessionDir(root, "s1"), SessionBranch("s1")
	if err := AddWorktree(root, dir, branch, start); err != nil {
		t.Fatal(err)
	}
	if err := Removable(root, dir, branch, ""); err == nil {
		t.Error("removable without its starting commit, which no ref holds")
	}
	if err := Removable(root, dir, branch, start); err != nil {
		t.Errorf("no commits past the starting commit: %v", err)
	}

	remote := t.TempDir()
	run(remote, "init", "-q", "--bare")
	run(root, "remote", "add", "origin", remote)
	run(dir, "commit", "-q", "--allow-empty", "-m", "work")
	if err := Removable(root, dir, branch, start); err == nil {
		t.Error("removable with a commit only on its branch")
	}
	run(dir, "push", "-q", "origin", branch)
	if err := RemoveWorktree(root, dir, branch, start); err != nil {
		t.Fatalf("commits on a remote branch: %v", err)
	}
	if exists(root, branch) {
		t.Error("branch remains")
	}
}

func TestWorktreeIncludesLiteralPaths(t *testing.T) {
	for _, name := range []string{
		"café.local", `quote"name.local`, "tab\tname.local",
		"line\nname.local", " leading.local", "café/config",
	} {
		t.Run(name, func(t *testing.T) {
			root, run := gitRepo(t)
			patterns := "*.local\ncafé/\n"
			write(t, filepath.Join(root, ".gitignore"), patterns)
			write(t, filepath.Join(root, ".worktreeinclude"), patterns)
			write(t, filepath.Join(root, name), "local contents")
			run(root, "add", ".gitignore", ".worktreeinclude")
			run(root, "commit", "-qm", "include local files")
			dir := SessionDir(root, "literal")
			if err := AddWorktree(root, dir, SessionBranch("literal"), ""); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil || string(data) != "local contents" {
				t.Fatalf("included %q = %q, %v", name, data, err)
			}
		})
	}
}

func TestWorktreeIncludesExcludeManagedWorktrees(t *testing.T) {
	root, run := gitRepo(t)
	write(t, filepath.Join(root, ".gitignore"), ".env\n.hypercube-local\n")
	write(t, filepath.Join(root, ".worktreeinclude"), "*\n")
	write(t, filepath.Join(root, ".env"), "local environment")
	write(t, filepath.Join(root, ".hypercube-local"), "ordinary ignored file")
	run(root, "add", ".gitignore", ".worktreeinclude")
	run(root, "commit", "-qm", "include ignored files")
	existing := SessionDir(root, "existing")
	if err := AddWorktree(root, existing, SessionBranch("existing"), ""); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(existing, "private-draft"), "another session's work")
	dir := SessionDir(root, "new")
	if err := AddWorktree(root, dir, SessionBranch("new"), ""); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{".env": "local environment", ".hypercube-local": "ordinary ignored file"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != want {
			t.Errorf("included %s = %q, %v", name, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".hypercube")); !os.IsNotExist(err) {
		t.Fatalf("managed worktrees copied into the new worktree: %v", err)
	}
}

func TestRemoveMissingWorktree(t *testing.T) {
	root, run := gitRepo(t)
	dir, branch := WorktreeDir(root, "parent", "w1"), ChildBranch("parent", "w1")
	if err := AddWorktree(root, dir, branch, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorktree(root, dir, branch, ""); err != nil {
		t.Fatalf("worktree directory deleted: %v", err)
	}
	if _, err := git(root, "rev-parse", "--verify", branch); err == nil {
		t.Error("branch remains")
	}

	if err := AddWorktree(root, dir, branch, ""); err != nil {
		t.Fatal(err)
	}
	run(root, "worktree", "remove", dir)
	run(root, "branch", "-D", branch)
	if err := RemoveWorktree(root, dir, branch, ""); err != nil {
		t.Errorf("worktree and branch gone: %v", err)
	}

	run(root, "branch", branch)
	run(root, "checkout", "-q", branch)
	run(root, "commit", "-q", "--allow-empty", "-m", "unmerged")
	run(root, "checkout", "-q", "main")
	if err := RemoveWorktree(root, dir, branch, ""); err == nil {
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
	if err := AddWorktree(root, WorktreeDir(root, "parent", "w1"), ChildBranch("parent", "w1"), ""); err == nil {
		t.Error("AddWorktree ignored an unreadable exclude file")
	}
}

func TestMergeStatus(t *testing.T) {
	root, run := gitRepo(t)
	dir, branch := SessionDir(root, "s1"), SessionBranch("s1")
	if err := AddWorktree(root, dir, branch, ""); err != nil {
		t.Fatal(err)
	}
	base, err := Head(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		name   string
		change func()
		want   Merge
	}{
		{"fresh", func() {}, Merge{}},
		{"committed", func() { run(dir, "commit", "-q", "--allow-empty", "-m", "work") }, Merge{Unmerged: 1, Committed: true}},
		{"untracked", func() { write(t, filepath.Join(dir, "scratch"), "") }, Merge{Unmerged: 1, Committed: true, Uncommitted: 1}},
		{"untracked folder", func() {
			for _, name := range []string{"a.go", "b.go", "c.go"} {
				write(t, filepath.Join(dir, "pkg", name), "")
			}
		}, Merge{Unmerged: 1, Committed: true, Uncommitted: 4}},
		{"merged", func() { run(root, "merge", "-q", branch) }, Merge{Committed: true, Uncommitted: 4}},
	} {
		step.change()
		step.want.Branch, step.want.Target = branch, "main"
		got, err := MergeStatus(root, dir, branch, base)
		if err != nil {
			t.Fatal(err)
		}
		if got != step.want {
			t.Errorf("%s: MergeStatus = %+v, want %+v", step.name, got, step.want)
		}
	}
}
