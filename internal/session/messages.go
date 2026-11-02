package session

import (
	"errors"
	"strings"

	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// SendMessage queues a message between sessions and attempts delivery.
func (r *Runtime) SendMessage(from, to, text string) (recipient store.Session, err error) {
	if strings.TrimSpace(text) == "" {
		return recipient, errors.New("text is required")
	}
	err = r.save(func(tx *write) error {
		caller, err := tx.Session(from)
		if err != nil {
			return err
		}
		s, err := tx.FindSession(caller, to)
		if err != nil {
			return err
		}
		if s.ID == from {
			return errors.New("cannot send a message to yourself")
		}
		if s.Ended {
			return errors.New("cannot send a message to an ended child session")
		}
		recipient = s
		running, err := tx.Running(s)
		if err != nil {
			return err
		}
		if !running {
			return errors.New("cannot send a message to a stopped session")
		}
		return tx.Queue(store.Message{From: from, To: s.ID, Text: text})
	})
	if err != nil {
		return recipient, err
	}
	r.Deliver(recipient.ID)
	return recipient, nil
}

func queueFailure(w *write, child store.Session, detail string) error {
	return w.Queue(store.Message{From: child.ID, To: child.Parent, Text: report.FailureMessage(detail)})
}
