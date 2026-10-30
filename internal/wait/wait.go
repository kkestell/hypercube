// Package wait delivers queued messages through the harness's own wake-up
// mechanism. It never types into the pane or changes the user's draft prompt.
package wait

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kkestell/hypercube/internal/hook"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/tmux"
)

// Run returns true after writing messages. The CLI maps that to status 2.
func Run(stdin io.Reader, out io.Writer) (bool, error) {
	id, path := os.Getenv("HYPERCUBE_AGENT_ID"), os.Getenv("HYPERCUBE_STATE")
	if id == "" || path == "" {
		return false, nil
	}
	var in struct {
		AgentID *string `json:"agent_id"`
	}
	_ = json.NewDecoder(stdin).Decode(&in)
	if in.AgentID != nil {
		return false, nil
	}
	return Wait(id, path, out)
}

// Wait replaces earlier waiters for this agent. Read without a lock while
// nothing is ready, then recheck and deliver under the state lock. A failed
// write leaves the messages queued and the agent idle.
func Wait(id, path string, out io.Writer) (bool, error) {
	parent := os.Getppid()
	var number uint64
	err := store.Update(path, func(s *store.State) error {
		a, err := s.Agent(id)
		if err != nil {
			return err
		}
		a.Waiter++
		number = a.Waiter
		return nil
	})
	if err != nil {
		return false, err
	}
	for os.Getppid() == parent {
		s, err := store.Load(path)
		if err != nil {
			return false, err
		}
		a, err := s.Agent(id)
		if err != nil || a.Waiter != number {
			return false, nil
		}
		queued := false
		for _, m := range s.Messages {
			if m.To == id {
				queued = true
				break
			}
		}
		if a.Activity == store.Idle && queued {
			ready, replaced := false, false
			err := store.Update(path, func(s *store.State) error {
				a, err := s.Agent(id)
				if err != nil || a.Waiter != number {
					replaced = true
					return nil
				}
				if a.Activity != store.Idle {
					return nil
				}
				messages := s.Take(id)
				if len(messages) == 0 {
					return nil
				}
				text := hook.Format(s, messages) + "\n"
				if _, err := fmt.Fprint(out, text); err != nil {
					return fmt.Errorf("write messages: %w", err)
				}
				a.Activity = store.Busy
				ready = true
				return nil
			})
			if err != nil || ready {
				if err == nil {
					tmux.RefreshActivity(path, id)
				}
				return ready, err
			}
			if replaced {
				return false, nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false, nil
}
