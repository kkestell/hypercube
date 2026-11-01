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

func TestWaiterReplacementAndDelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	update := func(f func(*store.State)) {
		t.Helper()
		if err := store.Update(path, func(s *store.State) error { f(s); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	update(func(s *store.State) {
		s.Agents = []store.Agent{{ID: "o", Name: "lead", Team: "session-1"}, {ID: "w", Name: "worker", Team: "session-1", Activity: store.Busy}}
		s.Messages = []store.Message{{From: "o", To: "w", Text: "first"}, {From: "w", To: "o", Text: "other"}, {From: "o", To: "w", Text: "second"}}
	})
	done := []chan bool{make(chan bool, 1), make(chan bool, 1)}
	output := []*bytes.Buffer{{}, {}}
	for i := range done {
		go func() {
			ok, err := Wait("w", path, output[i])
			if err != nil {
				t.Error(err)
			}
			done[i] <- ok
		}()
		deadline := time.Now().Add(2 * time.Second)
		for {
			s, _ := store.Load(path)
			if s.Agents[1].Waiter == uint64(i+1) {
				break
			}
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
	update(func(s *store.State) { s.Agents[1].Activity = store.Idle })
	select {
	case ok := <-done[1]:
		if !ok {
			t.Fatal("idle agent was not woken")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("delivery timed out")
	}
	if got := output[1].String(); !strings.Contains(got, "Message from session-1/lead:\nfirst\n") || !strings.Contains(got, "Message from session-1/lead:\nsecond\n") {
		t.Fatal(output[1].String())
	}
	s, _ := store.Load(path)
	if len(s.Messages) != 1 || s.Messages[0].To != "o" || s.Agents[1].Activity != store.Busy {
		t.Fatalf("state after wake: %+v", s)
	}
}

func TestAwaitingUserKeepsMessagesQueued(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{
			{ID: "l", Role: store.Lead, Team: "t", AwaitingUser: true},
			{ID: "w", Role: store.Worker, Team: "t", Activity: store.Idle},
		}
		s.Messages = []store.Message{{From: "user", To: "w", Text: "queued"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	done := make(chan bool, 1)
	go func() {
		ok, err := Wait("w", path, &out)
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
	s, err := store.Load(path)
	if err != nil || len(s.Messages) != 1 || s.Agents[1].Waiter != 1 || s.Agents[1].Activity != store.Idle {
		t.Fatalf("waiting state: %+v, %v", s, err)
	}
	if err := store.Update(path, func(s *store.State) error {
		s.TeamLead("t").AwaitingUser = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case ok := <-done:
		if !ok || !strings.Contains(out.String(), "queued") {
			t.Fatalf("released waiter: delivered=%v, output=%q", ok, out.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("released session did not deliver messages")
	}
	s, err = store.Load(path)
	if err != nil || len(s.Messages) != 0 || s.Agents[1].Activity != store.Busy {
		t.Fatalf("released state: %+v, %v", s, err)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestFailedWritesKeepMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "w", Activity: store.Idle}}
		s.Messages = []store.Message{{From: "user", To: "w", Text: "preserved"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if ok, err := Wait("w", path, brokenWriter{}); ok || err == nil {
		t.Fatal(ok, err)
	}
	s, _ := store.Load(path)
	if len(s.Messages) != 1 || s.Agents[0].Activity != store.Idle || s.Agents[0].Waiter != 1 {
		t.Fatal(s)
	}
}
