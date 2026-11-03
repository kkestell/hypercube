package session

import (
	"errors"
	"slices"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// granted reports whether the launch's session always allows the tool.
func (r *Runtime) granted(l *live, tool string) (bool, error) {
	var granted bool
	err := r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(l.id)
		if err != nil {
			return err
		}
		granted, err = tx.Granted(s.ID, s.Harness, tool)
		return err
	})
	return granted, err
}

// recordAllow records an answer to the request with its first choice, which
// allows it once. Only a caller that waits keeps the command's waiter. The
// caller holds mu.
func (r *Runtime) recordAllow(tx *write, l *live, item harness.Item, wait bool) (int64, error) {
	id, err := r.record(tx, l, harness.Command{Command: harness.CmdAnswer, Request: strings.TrimPrefix(item.ID, l.prefix), Choice: item.Choices[0]}, 0, 0, nil)
	if !wait {
		delete(l.waiters, id)
	}
	return id, err
}

// AlwaysAllow answers an approval request with its first choice and always
// allows the request's tool: in the session, or in every session of its harness
// when all is set. The grant also answers the requests for that tool that are
// open now.
func (r *Runtime) AlwaysAllow(id, requestID string, all bool) error {
	r.mu.Lock()
	l, err := r.running(id)
	var p *pendingItem
	if err == nil {
		p = l.pending[requestID]
	}
	if p == nil || p.item.Kind != harness.KindAttention || p.item.Grant == "" {
		r.mu.Unlock()
		return errors.New("the request cannot be always allowed")
	}
	tool, scope := p.item.Grant, id
	if all {
		scope = ""
	}
	var clicked int64
	var answered []*live
	err = r.save(func(tx *write) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		if err = tx.AddGrant(scope, s.Harness, tool); err != nil {
			return err
		}
		for _, o := range r.live {
			if _, err := r.running(o.id); err != nil || o.id != id && !all {
				continue
			}
			other, err := tx.Session(o.id)
			if err != nil {
				return err
			}
			if other.Harness != s.Harness {
				continue
			}
			for _, q := range o.pending {
				if q.item.Kind != harness.KindAttention || q.item.Grant != tool {
					continue
				}
				cmd, err := r.recordAllow(tx, o, q.item, q == p)
				if err != nil {
					return err
				}
				if q == p {
					clicked = cmd
				}
				if !slices.Contains(answered, o) {
					answered = append(answered, o)
				}
			}
		}
		return nil
	})
	r.mu.Unlock()
	if err != nil {
		return err
	}
	for _, o := range answered {
		go r.send(o)
	}
	return r.wait(l, clicked, r.requestTimeout)
}
