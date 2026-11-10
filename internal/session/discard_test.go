package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/workspace"
)

func TestDiscardReturnsTheFamilyToItsStartingCommit(t *testing.T) {
	f := newFixture(t)
	f.idleAfter(time.Hour)
	s := f.worktreeSession()
	child := f.child(s, "worker", "")
	f.until("the child idle", func() bool { return f.idle(child.ID) })
	base := f.storedSession(s.ID).Base
	for _, dir := range []string{s.Dir, child.Dir} {
		f.git(dir, "commit", "-q", "--allow-empty", "-m", "work")
		if err := os.WriteFile(filepath.Join(dir, "scratch"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.rt.Archive(s.ID); err == nil {
		t.Fatal("archived a family with unmerged work")
	}

	if err := f.rt.Discard(s.ID); err != nil {
		t.Fatal(err)
	}
	for _, tree := range []struct{ dir, branch string }{{s.Dir, s.Branch}, {child.Dir, child.Branch}} {
		if head, err := workspace.Head(tree.dir); err != nil || head != base {
			t.Errorf("%s HEAD = %s, %v; want %s", tree.dir, head, err, base)
		}
		if tip, err := workspace.BranchTip(f.root, tree.branch); err != nil || tip != base {
			t.Errorf("%s tip = %s, %v; want %s", tree.branch, tip, err, base)
		}
		if _, err := os.Stat(filepath.Join(tree.dir, "scratch")); err == nil {
			t.Errorf("%s kept an untracked file", tree.dir)
		}
	}
	if f.awake(s.ID) || f.awake(child.ID) {
		t.Error("an agent of the discarded family is running")
	}
	if f.storedSession(s.ID).Ended || f.storedSession(child.ID).Ended {
		t.Error("discarding ended a session")
	}
	if err := f.rt.Archive(s.ID); err != nil {
		t.Fatalf("archive after discard = %v", err)
	}
}

func TestDiscardRefusals(t *testing.T) {
	f := newFixture(t)
	f.idleAfter(time.Hour)
	checkout := f.session()
	worktree := f.worktreeSession()
	child := f.child(worktree, "worker", "")
	archived := f.worktreeSession()
	if err := f.rt.Archive(archived.ID); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ id, want string }{
		{checkout.ID, "only a worktree session"},
		{child.ID, "only a top-level session"},
		{archived.ID, "archived"},
	} {
		if err := f.rt.Discard(c.id); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Discard(%s) = %v, want %q", c.id, err, c.want)
		}
	}
}
