package store

import (
	"errors"
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

func seed(t *testing.T, db *DB, w, path, s string, children ...string) {
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
		for _, name := range children {
			if err := tx.AddSession(Session{ID: s + "-" + name, Parent: s, Workspace: w, Name: name}); err != nil {
				return err
			}
		}
		return nil
	})
}

func TestOpenCreatesSchema(t *testing.T) {
	db := open(t)
	if _, err := os.Stat(dbPath()); err != nil {
		t.Fatal(err)
	}
	seed(t, db, "w", "/repo", "s")
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
		_, err = tx.Session("s")
		return err
	}); err != nil {
		t.Fatal("reopened database lost its rows:", err)
	}
}

func TestLookupsReturnErrNotFound(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s")
	tx(t, db, func(tx *Tx) error {
		lookups := map[string]error{
			"Workspace": func() error { _, err := tx.Workspace("x"); return err }(),
			"Session":   func() error { _, err := tx.Session("x"); return err }(),
		}
		for name, err := range lookups {
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("%s = %v, want ErrNotFound", name, err)
			}
		}
		return nil
	})
}

func TestSaveSessionWritesMutableFields(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s")
	tx(t, db, func(tx *Tx) error {
		if err := tx.TouchSession("s", 1234); err != nil {
			return err
		}
		s, err := tx.Session("s")
		if err != nil {
			return err
		}
		s.Title, s.LastActive = "renamed", 1
		if err := tx.SaveSession(s); err != nil {
			return err
		}
		if s, err = tx.Session("s"); err != nil {
			return err
		}
		if s.Title != "renamed" || s.LastActive != 1 {
			t.Errorf("after SaveSession: title %q, last active %d", s.Title, s.LastActive)
		}
		return nil
	})
}

func TestDuplicatesFail(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "w1")
	if err := db.Tx(func(tx *Tx) error { return tx.AddWorkspace(Workspace{ID: "w2", Path: "/repo"}) }); err == nil {
		t.Error("added a second workspace at the same path")
	}
	if err := db.Tx(func(tx *Tx) error {
		return tx.AddSession(Session{ID: "x", Workspace: "w", Parent: "s", Name: "w1"})
	}); err == nil {
		t.Error("added a second agent with the same name in a session")
	}
}

func TestDeleteSessionCascades(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "w1")
	seed(t, db, "w", "/repo", "t")
	tx(t, db, func(tx *Tx) error {
		for _, m := range []Message{{From: "t", To: "s-w1", Text: "hi"}, {From: "s-w1", To: "t", Text: "back"}} {
			if err := tx.Queue(m); err != nil {
				return err
			}
		}
		return tx.DeleteSession("s")
	})
	tx(t, db, func(tx *Tx) error {
		if a, _ := tx.Children("s"); len(a) != 0 {
			t.Errorf("children remain: %+v", a)
		}
		if m, _ := tx.Queued("s-w1"); len(m) != 0 {
			t.Errorf("messages remain: %+v", m)
		}
		if m, _ := tx.Queued("t"); len(m) != 1 {
			t.Errorf("another session's messages: %+v", m)
		}
		return nil
	})
}

func TestDequeueAndFind(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "x", "one")
	seed(t, db, "w", "/repo", "y")
	tx(t, db, func(tx *Tx) error {
		for i, to := range []string{"x-one", "x", "x-one"} {
			if err := tx.Queue(Message{From: "user", To: to, Text: fmt.Sprint(i)}); err != nil {
				return err
			}
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
		if left, _ := tx.Queued("x"); len(left) != 1 {
			t.Errorf("Dequeue took another agent's messages: %+v", left)
		}
		return nil
	})
}

func TestTranscript(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "w1")
	tx(t, db, func(tx *Tx) error {
		if n, err := tx.LastSeq("s"); err != nil || n != 0 {
			t.Errorf("LastSeq of an empty transcript = %d, %v", n, err)
		}
		// Items come back in position order, whatever order they were saved in.
		for _, e := range []Entry{{3, "c", []byte(`{"n":3}`)}, {1, "a", []byte(`{"n":1}`)}, {2, "b", []byte(`{"n":2}`)}} {
			if _, err := tx.PutEntry("s", e); err != nil {
				return err
			}
		}
		// An item saved again replaces its data and keeps its position.
		if seq, err := tx.PutEntry("s", Entry{9, "a", []byte(`{"n":"again"}`)}); err != nil || seq != 1 {
			t.Errorf("replaced entry's position = %d, %v", seq, err)
		}
		_, err := tx.PutEntry("s-w1", Entry{1, "a", []byte(`{}`)})
		return err
	})
	tx(t, db, func(tx *Tx) error {
		got, err := tx.Transcript("s")
		if err != nil || len(got) != 3 || got[0].Item != "a" || got[0].Seq != 1 || string(got[0].Data) != `{"n":"again"}` || got[1].Item != "b" || got[2].Item != "c" {
			t.Errorf("Transcript = %+v, %v", got, err)
		}
		if n, _ := tx.LastSeq("s"); n != 3 {
			t.Errorf("LastSeq = %d", n)
		}
		return tx.DeleteSession("s-w1")
	})
	tx(t, db, func(tx *Tx) error {
		if got, _ := tx.Transcript("s-w1"); len(got) != 0 {
			t.Errorf("deleted agent's transcript remains: %+v", got)
		}
		if got, _ := tx.Transcript("s"); len(got) != 3 {
			t.Errorf("another agent's transcript: %+v", got)
		}
		return nil
	})
}

func TestLastText(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "child")
	tx(t, db, func(tx *Tx) error {
		if text, seq, err := tx.LastText("s"); err != nil || text != "" || seq != 0 {
			t.Errorf("empty LastText = %q, %d, %v", text, seq, err)
		}
		for _, entry := range []Entry{
			{Seq: 1, Item: "first", Data: []byte(`{"kind":"text","text":"first","done":true}`)},
			{Seq: 2, Item: "last", Data: []byte(`{"kind":"text","text":"last","done":true}`)},
			{Seq: 3, Item: "child", Data: []byte(`{"kind":"text","text":"nested","done":true,"parent":"last"}`)},
			{Seq: 4, Item: "tool", Data: []byte(`{"kind":"tool","text":"tool"}`)},
			{Seq: 5, Item: "stream", Data: []byte(`{"kind":"text","text":"unfinished"}`)},
		} {
			if _, err := tx.PutEntry("s", entry); err != nil {
				return err
			}
		}
		if text, seq, err := tx.LastText("s"); err != nil || text != "last" || seq != 2 {
			t.Errorf("LastText = %q, %d, %v", text, seq, err)
		}
		if text, seq, err := tx.LastText("s-child"); err != nil || text != "" || seq != 0 {
			t.Errorf("another session's LastText = %q, %d, %v", text, seq, err)
		}
		return nil
	})
}

// Concurrent transactions from goroutines and from another process all commit.
func TestConcurrentTransactions(t *testing.T) {
	if os.Getenv("HYPERCUBE_STORE_CHILD") == "1" {
		db, err := Open()
		if err != nil {
			os.Exit(1)
		}
		for i := 0; i < 50; i++ {
			if err := db.Tx(func(tx *Tx) error { return tx.Queue(Message{From: "child", To: "s", Text: "c"}) }); err != nil {
				os.Exit(2)
			}
		}
		os.Exit(0)
	}
	db := open(t)
	seed(t, db, "w", "/repo", "s")
	exe, _ := os.Executable()
	child := exec.Command(exe, "-test.run", "^TestConcurrentTransactions$")
	child.Env = append(os.Environ(), "HYPERCUBE_STORE_CHILD=1")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Go(func() {
			if err := db.Tx(func(tx *Tx) error { return tx.Queue(Message{From: "parent", To: "s", Text: "p"}) }); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := child.Wait(); err != nil {
		t.Fatal("child:", err)
	}
	tx(t, db, func(tx *Tx) error {
		if m, _ := tx.Queued("s"); len(m) != 100 {
			t.Errorf("%d messages, want 100", len(m))
		}
		return nil
	})
}

func TestSessionScopeAndFamilyState(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "p", "one", "two")
	seed(t, db, "w", "/repo", "q", "one")
	seed(t, db, "v", "/other", "r")
	tx(t, db, func(tx *Tx) error {
		p, _ := tx.Session("p")
		one, _ := tx.Session("p-one")
		tops, _ := tx.Sessions("")
		kids, _ := tx.Children("p")
		if len(tops) != 3 || len(kids) != 2 || kids[0].Name != "one" || kids[1].Name != "two" {
			t.Fatalf("top-level=%+v children=%+v", tops, kids)
		}
		for _, c := range []struct {
			caller   Session
			to, want string
		}{
			{p, "one", "p-one"}, {p, "q", "q"}, {p, "r", ""}, {p, "q-one", ""},
			{one, "p", "p"}, {one, "two", ""}, {one, "q", ""}, {one, "r", ""}, {one, "one", ""},
		} {
			got, err := tx.FindSession(c.caller, c.to)
			if c.want == "" {
				if !errors.Is(err, ErrNotFound) {
					t.Errorf("FindSession(%s,%s)=%+v,%v", c.caller.ID, c.to, got, err)
				}
			} else if err != nil || got.ID != c.want {
				t.Errorf("FindSession=%+v,%v", got, err)
			}
		}
		root, err := tx.Root(one.ID)
		if err != nil || root.ID != p.ID {
			t.Fatalf("Root=%+v,%v", root, err)
		}
		running, err := tx.Running(one)
		if err != nil || !running {
			t.Fatalf("Running=%v,%v", running, err)
		}
		one.Ended = true
		if err := tx.SaveSession(one); err != nil {
			return err
		}
		running, err = tx.Running(one)
		if err != nil || running {
			t.Fatalf("ended Running=%v,%v", running, err)
		}
		if err := tx.TouchSession(one.ID, 123); err != nil {
			return err
		}
		p, _ = tx.Session(p.ID)
		if p.LastActive != 123 {
			t.Error("child did not touch root")
		}
		if _, err := tx.PutEntry(one.ID, Entry{Seq: 1, Item: "i", Data: []byte(`{}`)}); err != nil {
			return err
		}
		if err := tx.Queue(Message{From: p.ID, To: one.ID, Text: "queued"}); err != nil {
			return err
		}
		if err := tx.DeleteSession(p.ID); err != nil {
			return err
		}
		if _, err := tx.Session(one.ID); !errors.Is(err, ErrNotFound) {
			t.Error("child survives parent")
		}
		entries, _ := tx.Transcript(one.ID)
		messages, _ := tx.Queued(one.ID)
		if len(entries) != 0 || len(messages) != 0 {
			t.Error("child data survives parent")
		}
		return nil
	})
}

func TestCommandsReservePositionsAndIncreaseIDs(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "child")
	tx(t, db, func(tx *Tx) error {
		s, err := tx.Session("s")
		if err != nil {
			return err
		}
		s.Launch, s.Position, s.Ready, s.Quick = "launch", 123, 456, 2
		if err = tx.SaveSession(s); err != nil {
			return err
		}
		s, err = tx.Session("s")
		if err != nil {
			return err
		}
		if s.Launch != "launch" || s.Position != 123 || s.Ready != 456 || s.Quick != 2 {
			t.Fatal(s)
		}
		id, err := tx.AddCommand(Command{Session: "s", Command: "input", Seq: 4, Item: []byte(`{"id":"user","kind":"user","done":true}`)})
		if err != nil {
			return err
		}
		line := `{"id":1,"command":"input","text":"hello"}` + "\n"
		if err = tx.SetCommandLine(id, line); err != nil {
			return err
		}
		commands, err := tx.Commands("s")
		if err != nil {
			return err
		}
		if len(commands) != 1 || commands[0].ID != id || commands[0].Line != line {
			t.Fatal(commands)
		}
		if seq, err := tx.LastSeq("s"); err != nil || seq != 4 {
			t.Fatalf("reserved seq %d: %v", seq, err)
		}
		if err = tx.DeleteCommand(id); err != nil {
			return err
		}
		next, err := tx.AddCommand(Command{Session: "s", Command: "interrupt"})
		if err != nil {
			return err
		}
		if next <= id {
			t.Fatal("ID reused")
		}
		return nil
	})
}
