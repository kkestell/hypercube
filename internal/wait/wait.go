// Package wait hands an idle agent's queued messages to its harness plugin,
// which starts a turn through the harness's own wake-up mechanism. Nothing is
// typed into the pane, so the user's draft prompt stays.
package wait

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// Run returns true after writing messages. The CLI maps that to status 2.
func Run(out io.Writer) (bool, error) {
	id := os.Getenv("HYPERCUBE_AGENT_ID")
	if id == "" {
		return false, nil
	}
	db, err := store.Open()
	if err != nil {
		return false, err
	}
	defer db.Close()
	return Wait(db, id, out)
}

// Wait replaces earlier waiters for this agent. It reads without the write
// lock while nothing is ready, then rechecks and delivers in one transaction.
// A failed write leaves the messages queued and the agent idle.
func Wait(db *store.DB, id string, out io.Writer) (bool, error) {
	parent := os.Getppid()
	var number int64
	err := db.Tx(func(tx *store.Tx) error {
		a, err := tx.Agent(id)
		if err != nil {
			return err
		}
		a.Waiter++
		number = a.Waiter
		return tx.SaveAgent(a)
	})
	if err != nil {
		return false, err
	}
	// ready reports whether the agent is idle with messages queued, its
	// session is not awaiting the user, and this is still its newest waiter.
	ready := func(tx *store.Tx) (current, ready bool, err error) {
		a, err := tx.Agent(id)
		if err != nil || a.Waiter != number {
			return false, false, nil
		}
		s, err := tx.Session(a.Session)
		if err != nil || a.Activity != store.Idle || s.AwaitingUser {
			return true, false, err
		}
		queued, err := tx.Queued(id)
		return true, len(queued) > 0, err
	}
	for os.Getppid() == parent {
		var current, deliver bool
		err := db.Read(func(tx *store.Tx) error {
			var err error
			current, deliver, err = ready(tx)
			return err
		})
		if err != nil || !current {
			return false, err
		}
		if deliver {
			delivered := false
			err := db.Tx(func(tx *store.Tx) error {
				if current, deliver, err = ready(tx); err != nil || !deliver {
					return err
				}
				messages, err := tx.Take(id)
				if err != nil {
					return err
				}
				if _, err := fmt.Fprint(out, report.Format(tx, messages)+"\n"); err != nil {
					return fmt.Errorf("write messages: %w", err)
				}
				a, err := tx.Agent(id)
				if err != nil {
					return err
				}
				a.Activity = store.Busy
				delivered = true
				return tx.SaveAgent(a)
			})
			if err != nil || delivered || !current {
				return delivered, err
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false, nil
}
