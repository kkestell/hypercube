package store

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func open(t *testing.T) *DB {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state with spaces"))
	db, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func tx(t *testing.T, db *DB, fn func(*Tx) error) {
	t.Helper()
	if err := db.Tx(fn); err != nil {
		t.Fatal(err)
	}
}

// seed adds workspace w at path with session s holding agents named in order,
// the first a lead.
func seed(t *testing.T, db *DB, w, path, s string, agents ...string) {
	t.Helper()
	tx(t, db, func(tx *Tx) error {
		if _, err := tx.Workspace(w); err != nil {
			if err := tx.AddWorkspace(Workspace{ID: w, Path: path}); err != nil {
				return err
			}
		}
		if err := tx.AddSession(Session{ID: s, Workspace: w, Running: true}); err != nil {
			return err
		}
		for i, name := range agents {
			role := Worker
			if i == 0 {
				role = Lead
			}
			if err := tx.AddAgent(Agent{ID: s + "-" + name, Session: s, Name: name, Role: role, Activity: Idle}); err != nil {
				return err
			}
		}
		return nil
	})
}

func TestOpenCreatesSchema(t *testing.T) {
	db := open(t)
	if _, err := os.Stat(Path()); err != nil {
		t.Fatal(err)
	}
	seed(t, db, "w", "/repo", "s", "lead")
	db.Close()
	again, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if err := again.Read(func(tx *Tx) error { _, err := tx.Lead("s"); return err }); err != nil {
		t.Fatal("reopened database lost its rows:", err)
	}
}

func TestDuplicatesFail(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "lead")
	if err := db.Tx(func(tx *Tx) error { return tx.AddWorkspace(Workspace{ID: "w2", Path: "/repo"}) }); err == nil {
		t.Error("added a second workspace at the same path")
	}
	if err := db.Tx(func(tx *Tx) error {
		return tx.AddAgent(Agent{ID: "x", Session: "s", Name: "lead", Role: Worker, Activity: Idle})
	}); err == nil {
		t.Error("added a second agent with the same name in a session")
	}
}

func TestDeleteSessionCascades(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "lead", "w1")
	seed(t, db, "w", "/repo", "t", "lead")
	tx(t, db, func(tx *Tx) error {
		tx.Queue(Message{From: "t-lead", To: "s-w1", Text: "hi"})
		tx.Queue(Message{From: "s-w1", To: "t-lead", Text: "back"})
		return tx.DeleteSession("s")
	})
	tx(t, db, func(tx *Tx) error {
		if a, _ := tx.Agents("s"); len(a) != 0 {
			t.Errorf("agents remain: %+v", a)
		}
		if m, _ := tx.Queued("s-w1"); len(m) != 0 {
			t.Errorf("messages remain: %+v", m)
		}
		if m, _ := tx.Queued("t-lead"); len(m) != 1 {
			t.Errorf("another session's messages: %+v", m)
		}
		return nil
	})
}

func TestTakeAndFind(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "x", "lead", "one")
	seed(t, db, "w", "/repo", "y", "lead")
	tx(t, db, func(tx *Tx) error {
		for i, to := range []string{"x-one", "x-lead", "x-one"} {
			tx.Queue(Message{From: "user", To: to, Text: fmt.Sprint(i)})
		}
		got, err := tx.Take("x-one")
		if err != nil || len(got) != 2 || got[0].Text != "0" || got[1].Text != "2" {
			t.Errorf("Take = %+v, %v", got, err)
		}
		if left, _ := tx.Queued("x-lead"); len(left) != 1 {
			t.Errorf("Take took another agent's messages: %+v", left)
		}
		for _, session := range []string{"x", "y"} {
			if a, err := tx.Find(session, "x/one"); err != nil || a.ID != "x-one" {
				t.Errorf("Find(%s, x/one) = %+v, %v", session, a, err)
			}
		}
		if a, err := tx.Find("y", "one"); err == nil {
			t.Errorf("Find(y, one) = %+v", a)
		}
		return nil
	})
}

func TestSnapshotOrder(t *testing.T) {
	db := open(t)
	seed(t, db, "b", "/src/zeta", "s1", "lead")
	seed(t, db, "a", "/src/alpha", "s2", "lead")
	seed(t, db, "b", "/src/zeta", "s3", "lead", "w")
	list, err := Snapshot(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "zeta" {
		t.Fatalf("workspaces = %+v", list)
	}
	z := list[1].Sessions
	if len(z) != 2 || z[0].ID != "s1" || z[1].ID != "s3" || z[0].Title != Untitled || len(z[1].Agents) != 2 || z[1].Agents[0].Role != Lead {
		t.Fatalf("sessions = %+v", z)
	}
}

// Concurrent transactions from goroutines and from another process all commit.
func TestConcurrentTransactions(t *testing.T) {
	if os.Getenv("HYPERCUBE_STORE_CHILD") == "1" {
		db, err := Open()
		if err != nil {
			os.Exit(1)
		}
		for i := 0; i < 50; i++ {
			if err := db.Tx(func(tx *Tx) error { return tx.Queue(Message{From: "child", To: "s-lead", Text: "c"}) }); err != nil {
				os.Exit(2)
			}
		}
		os.Exit(0)
	}
	db := open(t)
	seed(t, db, "w", "/repo", "s", "lead")
	exe, _ := os.Executable()
	child := exec.Command(exe, "-test.run", "^TestConcurrentTransactions$")
	child.Env = append(os.Environ(), "HYPERCUBE_STORE_CHILD=1")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Go(func() {
			if err := db.Tx(func(tx *Tx) error { return tx.Queue(Message{From: "parent", To: "s-lead", Text: "p"}) }); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := child.Wait(); err != nil {
		t.Fatal("child:", err)
	}
	tx(t, db, func(tx *Tx) error {
		if m, _ := tx.Queued("s-lead"); len(m) != 100 {
			t.Errorf("%d messages, want 100", len(m))
		}
		return nil
	})
}
