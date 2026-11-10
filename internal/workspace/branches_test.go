package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCheckNewBranch(t *testing.T) {
	root, run := gitRepo(t)
	run(root, "branch", "taken")
	run(root, "branch", "previous")
	run(root, "checkout", "-q", "previous")
	run(root, "checkout", "-q", "main")
	for _, c := range []struct{ name, want string }{
		{"feature/login", ""},
		{"HEAD", "not a valid branch name"},
		{"-x", "not a valid branch name"},
		{"a b", "not a valid branch name"},
		{"a..b", "not a valid branch name"},
		{"x.lock", "not a valid branch name"},
		{"@{-1}", "not a valid branch name"},
		{"taken", "already exists"},
	} {
		err := CheckNewBranch(root, c.name)
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("CheckNewBranch(%q) = %v, want %q", c.name, err, c.want)
		}
	}
}

func TestAddWorktreeRefusesExistingBranch(t *testing.T) {
	root, run := gitRepo(t)
	run(root, "branch", "taken")
	before, err := BranchTip(root, "taken")
	if err != nil {
		t.Fatal(err)
	}
	run(root, "commit", "-q", "--allow-empty", "-m", "later")
	if err := AddWorktree(root, SessionDir(root, "s1"), "taken", ""); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("AddWorktree on an existing branch = %v", err)
	}
	if after, err := BranchTip(root, "taken"); err != nil || after != before {
		t.Errorf("existing branch moved to %s, %v; want %s", after, err, before)
	}
}

// A non-empty target makes Git create the branch and then fail.
func TestAddWorktreeFailureLeavesNoBranch(t *testing.T) {
	root, _ := gitRepo(t)
	dir := SessionDir(root, "s1")
	write(t, filepath.Join(dir, "file"), "occupied")
	if err := AddWorktree(root, dir, "feature/x", ""); err == nil {
		t.Fatal("AddWorktree into a non-empty directory succeeded")
	}
	if exists(root, "feature/x") {
		t.Error("a failed add left its branch")
	}
	if _, err := os.Stat(filepath.Join(dir, "file")); err != nil {
		t.Errorf("the occupied directory changed: %v", err)
	}
}

func TestBranches(t *testing.T) {
	root, run := gitRepo(t)
	remote := t.TempDir()
	run(remote, "init", "-q", "--bare", "-b", "main")
	run(root, "remote", "add", "origin", remote)
	run(root, "push", "-q", "origin", "main")
	run(root, "fetch", "-q", "origin")
	run(root, "remote", "set-head", "origin", "main")
	run(root, "branch", "feature")
	local, remotes, err := Branches(root)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(local)
	if !slices.Equal(local, []string{"feature", "main"}) || !slices.Equal(remotes, []string{"origin/main"}) {
		t.Errorf("Branches = %v, %v", local, remotes)
	}
}
