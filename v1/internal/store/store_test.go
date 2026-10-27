package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Concurrent updates all land: each holds the lock while it reads and writes.
func TestUpdateIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := Update(path, func(s *State) error {
				s.Messages = append(s.Messages, Message{Text: fmt.Sprint(i), To: "a"})
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if s, _ := Load(path); len(s.Messages) != 30 {
		t.Errorf("%d messages, want 30", len(s.Messages))
	}
}

func TestUpdateErrorWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	Update(path, func(s *State) error { s.Agents = []Agent{{ID: "a"}}; return nil })
	Update(path, func(s *State) error { s.Agents = nil; return fmt.Errorf("no") })
	if s, _ := Load(path); len(s.Agents) != 1 {
		t.Errorf("agents = %+v", s.Agents)
	}
}

func TestTakeAndRemove(t *testing.T) {
	s := State{
		Agents:   []Agent{{ID: "a", Name: "one"}, {ID: "b", Name: "two"}},
		Messages: []Message{{Text: "1", To: "a"}, {Text: "2", To: "b"}, {Text: "3", To: "a"}},
	}
	if got := s.Take("a"); len(got) != 2 || len(s.Messages) != 1 || s.Messages[0].To != "b" {
		t.Errorf("Take(a) = %+v, left %+v", got, s.Messages)
	}
	s.Messages = append(s.Messages, Message{Text: "4", To: "b"})
	s.Remove("b")
	if len(s.Agents) != 1 || s.Agents[0].ID != "a" || len(s.Messages) != 0 {
		t.Errorf("after Remove(b): %+v %+v", s.Agents, s.Messages)
	}
	if a, err := s.Agent("one"); err != nil || a.ID != "a" {
		t.Errorf("Agent(one) = %+v, %v", a, err)
	}
}

func TestDeleteWaitsForUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	Update(path, func(s *State) error { s.Agents = []Agent{{ID: "a"}}; return nil })
	locked, release, deleted := make(chan struct{}), make(chan struct{}), make(chan error)
	go Update(path, func(s *State) error {
		close(locked)
		<-release
		return nil
	})
	<-locked
	go func() { deleted <- Delete(path) }()
	select {
	case <-deleted:
		t.Fatal("Delete returned while an update held the lock")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state file remains: %v", err)
	}
}
