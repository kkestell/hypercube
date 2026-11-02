package session

import (
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// event applies an event from l's plugin.
func (r *Runtime) event(l *live, ev harness.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.current(l) {
		return
	}
	var completed []*pendingItem
	var failures []string
	at := time.Now().UnixMilli()
	for _, it := range ev.Items {
		native := it.ID
		it.ID = l.prefix + it.ID
		if it.Parent != "" {
			it.Parent = l.prefix + it.Parent
		}
		p := l.pending[it.ID]
		it.At = at
		if p != nil {
			it.At = p.item.At
		}
		if p == nil {
			p = &pendingItem{seq: r.nextSeq(l.id), native: native}
			l.pending[it.ID] = p
			if it.Kind == harness.KindError {
				failures = append(failures, it.Text)
			}
		} else if it.Append {
			merged := p.item
			merged.Text += it.Text
			merged.Done = it.Done
			it = merged
		}
		p.item = it
		if it.Done {
			delete(l.pending, it.ID)
			completed = append(completed, p)
		} else {
			r.feed.publish(Change{Session: l.session, Agent: l.id, Seq: p.seq, Item: it})
		}
	}
	attention := false
	for _, p := range l.pending {
		attention = attention || p.item.Kind == harness.KindAttention
	}
	changed := ev.Conversation != "" || ev.State != "" || ev.Usage != nil || attention != l.attention
	if len(completed) == 0 && len(failures) == 0 && !changed {
		return
	}
	err := r.save(func(tx *write) error {
		if err := tx.TouchSession(l.session, time.Now().UnixMilli()); err != nil {
			return err
		}
		for _, e := range completed {
			if err := tx.putItem(l, e.seq, e.item); err != nil {
				return err
			}
		}
		if !changed && len(failures) == 0 {
			return nil
		}
		a, err := tx.Agent(l.id)
		if err != nil {
			return err
		}
		if changed {
			s, err := tx.Session(a.Session)
			if err != nil {
				return err
			}
			applyEvent(&a, &s, ev)
			a.Attention = attention
			if err := tx.SaveAgent(a); err != nil {
				return err
			}
			if err := tx.SaveSession(s); err != nil {
				return err
			}
		}
		if a.Role == store.Worker {
			for _, text := range failures {
				if err := queueFailure(tx, a, text); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		logf("recording an event from %s: %v", l.id, err)
		return
	}
	l.attention = attention
	if len(failures) > 0 {
		r.kickSessionLocked(l.session)
	} else if ev.State == harness.Idle {
		r.kickLocked(l.id)
	}
}

// applyEvent records an event's conversation, work state, and usage. A lead that
// finishes a turn has a reply for the user to see.
func applyEvent(a *store.Agent, s *store.Session, ev harness.Event) {
	if ev.Conversation != "" {
		a.Conversation = ev.Conversation
	}
	switch ev.State {
	case harness.Working:
		a.Activity = store.Busy
	case harness.Idle:
		if a.Activity == store.Busy && a.Role == store.Lead {
			s.Unseen = true
		}
		a.Activity = store.Idle
	}
	if ev.Usage != nil {
		a.Usage = *ev.Usage
	}
}
