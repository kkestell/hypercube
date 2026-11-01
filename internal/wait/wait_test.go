package wait

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/store"
)

// open returns a database with session s holding the given agents.
func open(t *testing.T, s store.Session, agents ...store.Agent) *store.DB {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s.Workspace = "w"
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: "/repo"}); err != nil {
			return err
		}
		if err := tx.AddSession(s); err != nil {
			return err
		}
		for _, a := range agents {
			a.Session = s.ID
			if err := tx.AddAgent(a); err != nil {
				return err
			}
		}
		return nil
	})
	return db
}

func change(t *testing.T, db *store.DB, fn func(*store.Tx) error) {
	t.Helper()
	if err := db.Tx(fn); err != nil {
		t.Fatal(err)
	}
}

func agent(t *testing.T, db *store.DB, id string) store.Agent {
	t.Helper()
	var a store.Agent
	change(t, db, func(tx *store.Tx) error { var err error; a, err = tx.Agent(id); return err })
	return a
}

func queued(t *testing.T, db *store.DB, id string) []store.Message {
	t.Helper()
	var m []store.Message
	change(t, db, func(tx *store.Tx) error { var err error; m, err = tx.Queued(id); return err })
	return m
}

func setActivity(t *testing.T, db *store.DB, id, activity string) {
	t.Helper()
	change(t, db, func(tx *store.Tx) error {
		a, err := tx.Agent(id)
		if err != nil {
			return err
		}
		a.Activity = activity
		return tx.SaveAgent(a)
	})
}

func TestWaiterReplacementAndDelivery(t *testing.T) {
	db := open(t, store.Session{ID: "session-1"},
		store.Agent{ID: "o", Name: "lead", Role: store.Lead, Activity: store.Idle},
		store.Agent{ID: "w", Name: "worker", Role: store.Worker, Activity: store.Busy})
	change(t, db, func(tx *store.Tx) error {
		for _, m := range []store.Message{{From: "o", To: "w", Text: "first"}, {From: "w", To: "o", Text: "other"}, {From: "o", To: "w", Text: "second"}} {
			if err := tx.Queue(m); err != nil {
				return err
			}
		}
		return nil
	})
	done := []chan bool{make(chan bool, 1), make(chan bool, 1)}
	output := []*bytes.Buffer{{}, {}}
	for i := range done {
		go func() {
			ok, err := Wait(db, "w", output[i])
			if err != nil {
				t.Error(err)
			}
			done[i] <- ok
		}()
		deadline := time.Now().Add(2 * time.Second)
		for agent(t, db, "w").Waiter != int64(i+1) {
			if time.Now().After(deadline) {
				t.Fatal("waiter did not register")
			}
			time.Sleep(time.Millisecond)
		}
	}
	if ok := <-done[0]; ok || output[0].Len() != 0 {
		t.Fatal("superseded waiter took messages")
	}
	select {
	case <-done[1]:
		t.Fatal("busy agent was woken")
	default:
	}
	setActivity(t, db, "w", store.Idle)
	select {
	case ok := <-done[1]:
		if !ok {
			t.Fatal("idle agent was not woken")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("delivery timed out")
	}
	if got := output[1].String(); !strings.Contains(got, "Message from session-1/lead:\nfirst\n") || !strings.Contains(got, "Message from session-1/lead:\nsecond\n") {
		t.Fatal(got)
	}
	if len(queued(t, db, "w")) != 0 || len(queued(t, db, "o")) != 1 || agent(t, db, "w").Activity != store.Busy {
		t.Fatal("state after wake")
	}
}

func TestAwaitingUserKeepsMessagesQueued(t *testing.T) {
	db := open(t, store.Session{ID: "t", AwaitingUser: true},
		store.Agent{ID: "l", Name: "lead", Role: store.Lead, Activity: store.Idle},
		store.Agent{ID: "w", Name: "worker", Role: store.Worker, Activity: store.Idle})
	change(t, db, func(tx *store.Tx) error { return tx.Queue(store.Message{From: "user", To: "w", Text: "queued"}) })
	var out bytes.Buffer
	done := make(chan bool, 1)
	go func() {
		ok, err := Wait(db, "w", &out)
		if err != nil {
			t.Error(err)
		}
		done <- ok
	}()
	select {
	case <-done:
		t.Fatal("session awaiting the user delivered messages")
	case <-time.After(500 * time.Millisecond):
	}
	if a := agent(t, db, "w"); len(queued(t, db, "w")) != 1 || a.Waiter != 1 || a.Activity != store.Idle {
		t.Fatalf("waiting state: %+v", a)
	}
	change(t, db, func(tx *store.Tx) error {
		s, err := tx.Session("t")
		if err != nil {
			return err
		}
		s.AwaitingUser = false
		return tx.SaveSession(s)
	})
	select {
	case ok := <-done:
		if !ok || !strings.Contains(out.String(), "queued") {
			t.Fatalf("released waiter: delivered=%v, output=%q", ok, out.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("released session did not deliver messages")
	}
	if len(queued(t, db, "w")) != 0 || agent(t, db, "w").Activity != store.Busy {
		t.Fatal("released state")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestFailedWritesKeepMessages(t *testing.T) {
	db := open(t, store.Session{ID: "t"}, store.Agent{ID: "w", Name: "worker", Role: store.Worker, Activity: store.Idle})
	change(t, db, func(tx *store.Tx) error { return tx.Queue(store.Message{From: "user", To: "w", Text: "preserved"}) })
	if ok, err := Wait(db, "w", brokenWriter{}); ok || err == nil {
		t.Fatal(ok, err)
	}
	if a := agent(t, db, "w"); len(queued(t, db, "w")) != 1 || a.Activity != store.Idle || a.Waiter != 1 {
		t.Fatal(a)
	}
}
