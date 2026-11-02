package session

import (
	"errors"
	"strings"

	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// SendMessage queues a message between agents and attempts delivery.
func (r *Runtime) SendMessage(from, to, text string) (recipient store.Agent, running bool, err error) {
	if strings.TrimSpace(text) == "" {
		return recipient, false, errors.New("text is required")
	}
	err = r.save(func(tx *write) error {
		caller, err := tx.Agent(from)
		if err != nil {
			return err
		}
		a, err := tx.Find(caller.Session, to)
		if err != nil {
			return err
		}
		if a.ID == from {
			return errors.New("cannot send a message to yourself")
		}
		source, err := tx.Session(caller.Session)
		if err != nil {
			return err
		}
		s, err := tx.Session(a.Session)
		if err != nil {
			return err
		}
		if a.Session != caller.Session && (caller.Role != store.Lead || a.Role != store.Lead || s.Workspace != source.Workspace) {
			return errors.New("across sessions, only leads in the same workspace can message each other")
		}
		recipient, running = a, s.Running
		return tx.Queue(store.Message{From: from, To: a.ID, Text: text})
	})
	if err != nil {
		return recipient, running, err
	}
	r.Deliver(recipient.ID)
	return recipient, running, nil
}

func queueFailure(w *write, worker store.Agent, detail string) error {
	lead, err := w.Lead(worker.Session)
	if err != nil {
		return err
	}
	return w.Queue(store.Message{From: worker.ID, To: lead.ID, Text: report.FailureMessage(detail)})
}
