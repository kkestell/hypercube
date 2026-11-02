package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

func open(t *testing.T) (*store.DB, *Runtime) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, New(db, "")
}

// seed adds a stopped session at root with a lead and the given workers.
func seed(t *testing.T, db *store.DB, root string, workers ...store.Agent) {
	t.Helper()
	err := db.Tx(func(tx *store.Tx) error {
		tx.AddWorkspace(store.Workspace{ID: "w", Path: root})
		tx.AddSession(store.Session{ID: "s", Workspace: "w"})
		tx.AddAgent(store.Agent{ID: "a", Session: "s", Name: "lead", Role: store.Lead, Dir: root, Activity: store.Exited})
		for _, a := range workers {
			a.Session, a.Role, a.Activity = "s", store.Worker, store.Exited
			if err := tx.AddAgent(a); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, db *store.DB) (n int) {
	t.Helper()
	db.Read(func(tx *store.Tx) error { a, err := tx.Agents("s"); n = len(a); return err })
	return n
}

func TestWorkspaceAndSessionEdits(t *testing.T) {
	f := newFixture(t)
	if w, err := f.rt.AddWorkspace(f.root); err != nil || w.ID != f.workspace.ID {
		t.Fatalf("existing workspace = %+v, %v", w, err)
	}
	if _, err := f.rt.AddWorkspace(filepath.Join(f.root, "missing")); err == nil {
		t.Fatal("added a missing directory")
	}
	s, _ := f.session()
	if err := f.rt.Rename(s.ID, "  Changed  "); err != nil || f.session1(s.ID).Title != "Changed" {
		t.Fatalf("rename: %v", err)
	}
	if err := f.rt.Rename(s.ID, " \n"); err == nil {
		t.Fatal("blank title accepted")
	}
	if err := f.rt.Seen(s.ID); err != nil || f.session1(s.ID).Unseen {
		t.Fatalf("seen: %v", err)
	}
	if err := f.rt.Rename("missing", "Title"); err == nil {
		t.Fatal("renamed a missing session")
	}
	if err := f.rt.Seen("missing"); err == nil {
		t.Fatal("marked a missing session seen")
	}
}

func TestSetStatus(t *testing.T) {
	db, rt := open(t)
	seed(t, db, t.TempDir(), store.Agent{ID: "worker", Name: "worker"})
	changes, cancel := rt.Subscribe()
	defer cancel()
	if err := rt.SetStatus("worker", "  Reading\nthe code. "); err != nil {
		t.Fatal(err)
	}
	if change := nextChange(t, changes, func(Change) bool { return true }); change.Agent != "" {
		t.Fatalf("status change = %+v", change)
	}
	if err := db.Read(func(tx *store.Tx) error {
		a, err := tx.Agent("worker")
		if err == nil && (a.Status != "Reading the code." || a.StatusAt == 0) {
			t.Errorf("status = %+v", a)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ agent, text string }{{"a", "Planning"}, {"worker", " \n"}, {"missing", "Working"}} {
		if err := rt.SetStatus(c.agent, c.text); err == nil {
			t.Fatalf("accepted status %+v", c)
		}
	}
}

// Delete refuses, changing nothing, while a worker's branch is unmerged or its
// worktree is dirty, and otherwise removes worktrees, branches, and the session.
func TestDelete(t *testing.T) {
	db, rt := open(t)
	root := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(root, "init", "-q", "-b", "main")
	git(root, "commit", "-q", "--allow-empty", "-m", "init")
	dir, branch := workspace.WorktreeDir(root, "s/w1"), workspace.WorkerBranch("s/w1")
	if err := workspace.AddWorktree(root, dir, branch); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	seed(t, db, root, store.Agent{ID: "b", Name: "w1", Dir: dir, Branch: branch}, store.Agent{ID: "c", Name: "w2", Dir: other})

	git(dir, "commit", "-q", "--allow-empty", "-m", "work")
	if err := rt.Delete("s"); err == nil {
		t.Fatal("delete deleted an unmerged branch")
	}
	git(root, "merge", "-q", branch)
	os.WriteFile(filepath.Join(dir, "scratch"), nil, 0o644)
	if err := rt.Delete("s"); err == nil {
		t.Fatal("delete deleted a dirty worktree")
	}
	if n := count(t, db); n != 3 {
		t.Errorf("refused delete left %d agents", n)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("refused delete removed the worktree: %v", err)
	}

	os.Remove(filepath.Join(dir, "scratch"))
	if err := rt.Delete("s"); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db); n != 0 {
		t.Errorf("delete left %d agents", n)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Errorf("%s remains", dir)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("delete touched a worker's own directory: %v", err)
	}
}

func TestDeleteMissingCheckout(t *testing.T) {
	db, rt := open(t)
	root := filepath.Join(t.TempDir(), "gone")
	seed(t, db, root, store.Agent{ID: "b", Name: "w", Dir: filepath.Join(root, ".hypercube/s/w"), Branch: "hypercube/s/w"})
	if err := rt.DeleteWorkspace("w"); err != nil {
		t.Fatal(err)
	}
	db.Read(func(tx *store.Tx) error {
		if list, _ := tx.Workspaces(); len(list) != 0 {
			t.Errorf("workspaces = %+v", list)
		}
		return nil
	})
}
