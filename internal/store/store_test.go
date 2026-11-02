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
	tx(t, db, func(tx *Tx) error { return tx.TouchSession("s", 1234) })
	db.Close()
	again, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if err := again.Read(func(tx *Tx) error {
		s, err := tx.Session("s")
		if err != nil {
			return err
		}
		if s.LastActive != 1234 {
			t.Errorf("reopened last activity = %d", s.LastActive)
		}
		_, err = tx.Lead("s")
		return err
	}); err != nil {
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

func TestDequeueAndFind(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "x", "lead", "one")
	seed(t, db, "w", "/repo", "y", "lead")
	tx(t, db, func(tx *Tx) error {
		for i, to := range []string{"x-one", "x-lead", "x-one"} {
			tx.Queue(Message{From: "user", To: to, Text: fmt.Sprint(i)})
		}
		got, err := tx.Queued("x-one")
		if err != nil || len(got) != 2 || got[0].Text != "0" || got[1].Text != "2" {
			t.Errorf("Queued = %+v, %v", got, err)
		}
		if err := tx.Dequeue(got[0].ID); err != nil {
			t.Error(err)
		}
		if left, _ := tx.Queued("x-one"); len(left) != 1 || left[0].Text != "2" {
			t.Errorf("Dequeue left %+v", left)
		}
		if left, _ := tx.Queued("x-lead"); len(left) != 1 {
			t.Errorf("Dequeue took another agent's messages: %+v", left)
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

func TestTranscript(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "lead", "w1")
	tx(t, db, func(tx *Tx) error {
		if n, err := tx.LastSeq("s-lead"); err != nil || n != 0 {
			t.Errorf("LastSeq of an empty transcript = %d, %v", n, err)
		}
		// Items come back in position order, whatever order they were saved in.
		for _, e := range []Entry{{3, "c", []byte(`{"n":3}`)}, {1, "a", []byte(`{"n":1}`)}, {2, "b", []byte(`{"n":2}`)}} {
			if err := tx.PutEntry("s-lead", e); err != nil {
				return err
			}
		}
		// An item saved again replaces its data and keeps its position.
		if err := tx.PutEntry("s-lead", Entry{9, "a", []byte(`{"n":"again"}`)}); err != nil {
			return err
		}
		return tx.PutEntry("s-w1", Entry{1, "a", []byte(`{}`)})
	})
	tx(t, db, func(tx *Tx) error {
		got, err := tx.Transcript("s-lead")
		if err != nil || len(got) != 3 || got[0].Item != "a" || got[0].Seq != 1 || string(got[0].Data) != `{"n":"again"}` || got[1].Item != "b" || got[2].Item != "c" {
			t.Errorf("Transcript = %+v, %v", got, err)
		}
		if n, _ := tx.LastSeq("s-lead"); n != 3 {
			t.Errorf("LastSeq = %d", n)
		}
		return tx.DeleteAgent("s-lead")
	})
	tx(t, db, func(tx *Tx) error {
		if got, _ := tx.Transcript("s-lead"); len(got) != 0 {
			t.Errorf("deleted agent's transcript remains: %+v", got)
		}
		if got, _ := tx.Transcript("s-w1"); len(got) != 1 {
			t.Errorf("another agent's transcript: %+v", got)
		}
		return nil
	})
}

func TestOpenDeletesObsoleteFiles(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	locks := filepath.Join(Dir(), "locks")
	if err := os.MkdirAll(locks, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(locks, "s.lock"), filepath.Join(Dir(), "tools.log")} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	db, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, path := range []string{locks, filepath.Join(Dir(), "tools.log")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s remains", path)
		}
	}
}

func TestSnapshotOrder(t *testing.T) {
	db := open(t)
	seed(t, db, "b", "/src/zeta", "s1", "lead")
	seed(t, db, "a", "/src/alpha", "s2", "lead")
	seed(t, db, "b", "/src/zeta", "s3", "lead", "w")
	seed(t, db, "b", "/src/zeta", "s4", "lead")
	seed(t, db, "b", "/src/zeta", "s5", "lead")
	tx(t, db, func(tx *Tx) error {
		for i, id := range []string{"s1", "s3", "s4", "s5"} {
			if err := tx.TouchSession(id, int64(i+1)); err != nil {
				return err
			}
			if i >= 2 {
				s, err := tx.Session(id)
				if err != nil {
					return err
				}
				s.Running = false
				if err := tx.SaveSession(s); err != nil {
					return err
				}
			}
		}
		return nil
	})
	list, err := Snapshot(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "zeta" {
		t.Fatalf("workspaces = %+v", list)
	}
	z := list[1].Sessions
	if len(z) != 4 || z[0].ID != "s3" || z[1].ID != "s1" || z[2].ID != "s5" || z[3].ID != "s4" || z[0].Title != Untitled || len(z[0].Agents) != 2 || z[0].Agents[0].Role != Lead {
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
