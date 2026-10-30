package wait

import (
	"bytes"
	"errors"
	"io"
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
		s.Agents = []store.Agent{{ID: "o", Name: "lead"}, {ID: "w", Name: "worker", Activity: store.Busy}}
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
	if output[1].String() != "Message from default/lead:\nfirst\n\nMessage from default/lead:\nsecond\n" {
		t.Fatal(output[1].String())
	}
	s, _ := store.Load(path)
	if len(s.Messages) != 1 || s.Messages[0].To != "o" || s.Agents[1].Activity != store.Busy {
		t.Fatalf("state after wake: %+v", s)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestSubagentsAndFailedWritesKeepMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "w", Activity: store.Idle}}
		s.Messages = []store.Message{{From: "user", To: "w", Text: "preserved"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HYPERCUBE_AGENT_ID", "w")
	t.Setenv("HYPERCUBE_STATE", path)
	if ok, err := Run(strings.NewReader(`{"agent_id":"native"}`), io.Discard); ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, err := Wait("w", path, brokenWriter{}); ok || err == nil {
		t.Fatal(ok, err)
	}
	s, _ := store.Load(path)
	if len(s.Messages) != 1 || s.Agents[0].Activity != store.Idle || s.Agents[0].Waiter != 1 {
		t.Fatal(s)
	}
}
