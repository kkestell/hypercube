package session

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

func TestArchivedWorktreesAreNotMeasured(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.db.Tx(func(tx *store.Tx) error {
		return tx.AddSession(store.Session{ID: "archived", Workspace: f.workspace.ID, Ended: true,
			Base: "base", Branch: "hypercube/archived", Dir: filepath.Join(f.root, "removed")})
	}); err != nil {
		t.Fatal(err)
	}
	s := f.worktreeSession()
	// The configure reply follows the idle event's merge refresh.
	if err := f.rt.Select(s.ID, harness.Selection{Model: "m", Effort: "low"}); err != nil {
		t.Fatal(err)
	}
	merges := f.rt.Merges()
	if _, ok := merges["archived"]; ok {
		t.Fatal("measured an archived session's removed worktree")
	}
	if _, ok := merges[s.ID]; !ok {
		t.Fatal("the active worktree was not measured")
	}
}

func TestArchivingForgetsMergeStatus(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := f.worktreeSession()
	f.merge(s.ID, "the worktree's merge status", func(workspace.Merge) bool { return true })
	if err := f.rt.Archive(s.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.rt.Merges()[s.ID]; ok {
		t.Fatal("the archived session kept the merge status of its removed branch")
	}
}

func TestMergeUsesTheLiveTarget(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := f.worktreeSession()
	f.merge(s.ID, "the target main", func(m workspace.Merge) bool { return m.Target == "main" })
	before := len(f.inputs(s.ID))

	f.git(f.root, "switch", "-q", "-c", "release")
	if err := f.rt.Merge(s.ID, "main"); err == nil || !strings.Contains(err.Error(), "now on release") {
		t.Fatalf("Merge into a branch the checkout left = %v", err)
	}
	if got := f.inputs(s.ID); len(got) != before {
		t.Fatalf("a refused merge sent input: %q", got)
	}
	f.merge(s.ID, "the target release", func(m workspace.Merge) bool { return m.Target == "release" })

	if err := f.rt.Merge(s.ID, "release"); err != nil {
		t.Fatal(err)
	}
	want := "Commit any remaining work, then merge your branch into release in the checkout at " + f.root + ", resolving any conflicts, and report the result."
	f.until("the merge prompt", func() bool { return slices.Contains(f.inputs(s.ID), want) })
}

func TestMergeRefusesSessionsWithoutTheirOwnBranch(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	f.seed(store.Session{ID: "child", Name: "worker"})
	if err := f.db.Tx(func(tx *store.Tx) error {
		return tx.AddSession(store.Session{ID: "archived", Workspace: f.workspace.ID, Ended: true,
			Base: "base", Branch: "hypercube/archived", Dir: filepath.Join(f.root, "removed")})
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, want string }{
		{"archived", "the session has ended"},
		{"s", "only a top-level worktree session can be merged"},
		{"child", "only a top-level worktree session can be merged"},
	} {
		if err := f.rt.Merge(tc.id, "main"); err == nil || err.Error() != tc.want {
			t.Errorf("Merge(%s) = %v, want %q", tc.id, err, tc.want)
		}
	}
}
