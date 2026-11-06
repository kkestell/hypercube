package session

import (
	"path/filepath"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestArchivedWorktreesAreNotMeasured(t *testing.T) {
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
