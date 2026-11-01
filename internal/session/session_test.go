package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

func open(t *testing.T) *store.DB {
	t.Helper()
	tmp, err := os.MkdirTemp("/tmp", "hc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	t.Setenv("TMUX_TMPDIR", tmp)
	t.Setenv("TMUX", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
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

// Delete refuses, changing nothing, while a worker's branch is unmerged or its
// worktree is dirty, and otherwise removes worktrees, branches, and the session.
func TestDelete(t *testing.T) {
	db := open(t)
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
	if err := Delete(db, "s"); err == nil {
		t.Fatal("delete deleted an unmerged branch")
	}
	git(root, "merge", "-q", branch)
	os.WriteFile(filepath.Join(dir, "scratch"), nil, 0o644)
	if err := Delete(db, "s"); err == nil {
		t.Fatal("delete deleted a dirty worktree")
	}
	if n := count(t, db); n != 3 {
		t.Errorf("refused delete left %d agents", n)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("refused delete removed the worktree: %v", err)
	}

	os.Remove(filepath.Join(dir, "scratch"))
	if err := Delete(db, "s"); err != nil {
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
	db := open(t)
	root := filepath.Join(t.TempDir(), "gone")
	seed(t, db, root, store.Agent{ID: "b", Name: "w", Dir: filepath.Join(root, ".hypercube/s/w"), Branch: "hypercube/s/w"})
	if err := DeleteWorkspace(db, "w"); err != nil {
		t.Fatal(err)
	}
	db.Read(func(tx *store.Tx) error {
		if list, _ := tx.Workspaces(); len(list) != 0 {
			t.Errorf("workspaces = %+v", list)
		}
		return nil
	})
}
