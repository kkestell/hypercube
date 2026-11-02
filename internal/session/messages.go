package session

import (
	"errors"
	"strings"

	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// SendMessage queues a message between sessions and attempts delivery.
func (r *Runtime) SendMessage(from, to, text string) (recipient store.Session, running bool, err error) {
	if strings.TrimSpace(text) == "" {
		return recipient, false, errors.New("text is required")
	}
	err = r.save(func(tx *write) error {
		caller, err := tx.Session(from)
		if err != nil {
			return err
		}
		a, err := tx.FindSession(caller, to)
		if err != nil {
			return err
		}
		if a.ID == from {
			return errors.New("cannot send a message to yourself")
		}
		if a.Ended {
			return errors.New("cannot send a message to an ended child session")
		}
		recipient = a
		running, err = tx.Running(a)
		if err != nil {
			return err
		}
		return tx.Queue(store.Message{From: from, To: a.ID, Text: text})
	})
	if err != nil {
		return recipient, running, err
	}
	r.Deliver(recipient.ID)
	return recipient, running, nil
}

func queueFailure(w *write, child store.Session, detail string) error {
	return w.Queue(store.Message{From: child.ID, To: child.Parent, Text: report.FailureMessage(detail)})
}
