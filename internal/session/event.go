package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// apply commits every durable effect with the position of its source record.
func (r *Runtime) apply(l *live, rec harness.Record, next int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.current(l) || r.detached {
		return nil
	}
	if rec.Event != nil {
		return r.eventLocked(l, *rec.Event, next)
	}
	reply := *rec.Reply
	if _, ok := l.commands[reply.ID]; !ok {
		return fmt.Errorf("reply to unknown request %d", reply.ID)
	}
	if err := r.save(func(tx *write) error {
		if err := r.reply(tx, l, reply); err != nil {
			return err
		}
		return r.checkpoint(tx, l, next)
	}); err != nil {
		return err
	}
	l.position = next
	delete(l.commands, reply.ID)
	if ch := l.waiters[reply.ID]; ch != nil {
		var err error
		if reply.Error != "" {
			err = errors.New(reply.Error)
		}
		ch <- err
	}
	_ = l.client.Discard(next)
	if reply.Error == "" {
		r.kickFamilyLocked(l.root)
	}
	return nil
}

// checkpoint saves streaming items as unfinished, without mirroring already
// opened attention requests a second time.
func (r *Runtime) checkpoint(tx *write, l *live, next int64) error {
	for _, p := range l.pending {
		data, err := json.Marshal(p.item)
		if err != nil {
			return err
		}
		if _, err = tx.PutEntry(l.id, store.Entry{Seq: p.seq, Item: p.item.ID, Data: data}); err != nil {
			return err
		}
	}
	s, err := tx.Session(l.id)
	if err != nil {
		return err
	}
	s.Position = next
	return tx.SaveSession(s)
}

func (r *Runtime) eventLocked(l *live, ev harness.Event, next int64) error {
	l.position = next
	var completed []*pendingItem
	var failures []string
	var opened, allowed []harness.Item
	at := time.Now().UnixMilli()
	// A turn's work marker precedes the items the same event starts, and its
	// idle marker follows the items the event finishes.
	if ev.State == harness.Working {
		completed = append(completed, r.stateMarker(l, ev, next, at))
	}
	for _, it := range ev.Items {
		it.ID = l.prefix + it.ID
		if it.Parent != "" {
			it.Parent = l.prefix + it.Parent
		}
		p := l.pending[it.ID]
		if p == nil && it.Kind == harness.KindAttention && it.Grant != "" {
			// A granted request is answered without being shown, and its
			// completion is not shown either.
			if it.Done {
				continue
			}
			granted := false
			if !l.stopping {
				var err error
				if granted, err = r.granted(l, it.Grant); err != nil {
					return err
				}
			}
			if granted {
				allowed = append(allowed, it)
				continue
			}
		}
		it.At = at
		if p != nil {
			it.At = p.item.At
		}
		if p == nil {
			if it.Kind == harness.KindAttention {
				opened = append(opened, it)
			}
			p = &pendingItem{seq: r.nextSeq(l.id)}
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
			r.feed.publish(Change{Session: l.id, Seq: p.seq, Item: it})
		}
	}
	if ev.State == harness.Idle {
		completed = append(completed, r.stateMarker(l, ev, next, at))
	}
	// A stopping launch waits on nobody, whatever it still has pending.
	attention := false
	for _, p := range l.pending {
		attention = attention || p.item.Kind == harness.KindAttention && !l.stopping
	}
	changed := ev.Conversation != "" || ev.State != "" || ev.Usage != nil || attention != l.attention
	if len(completed) == 0 && len(opened) == 0 && len(allowed) == 0 && len(failures) == 0 && len(ev.Limits) == 0 && !changed {
		return nil
	}
	err := r.save(func(tx *write) error {
		for _, item := range opened {
			if err := r.mirrorLocked(tx, l.id, item); err != nil {
				return err
			}
		}
		for _, item := range allowed {
			if _, err := r.recordAllow(tx, l, item, false); err != nil {
				return err
			}
		}
		for _, e := range completed {
			if err := tx.putItem(l.id, e.seq, e.item); err != nil {
				return err
			}
		}
		s, err := tx.Session(l.id)
		if err != nil {
			return err
		}
		for _, limit := range ev.Limits {
			if err := tx.PutLimit(s.Harness, limit); err != nil {
				return err
			}
		}
		if changed {
			applyEvent(&s, ev)
			s.Attention = attention
			if err := tx.SaveSession(s); err != nil {
				return err
			}
		}
		if s.Parent != "" {
			for _, text := range failures {
				if err := queueFailure(tx, s, text); err != nil {
					return err
				}
			}
		}
		return r.checkpoint(tx, l, next)
	})
	if err != nil {
		return err
	}
	l.attention = attention
	if len(allowed) > 0 {
		go r.send(l)
	}
	if len(failures) > 0 {
		r.kickFamilyLocked(l.root)
	} else if ev.State == harness.Idle {
		r.kickLocked(l.id)
	}
	_ = l.client.Discard(next)
	return nil
}

// stateMarker is the saved record of an event's work state. Its ID names the
// source record's position, so a record read again replaces its marker.
func (r *Runtime) stateMarker(l *live, ev harness.Event, next, at int64) *pendingItem {
	item := harness.Item{ID: fmt.Sprintf("%sstate-%d", l.prefix, next), Kind: harness.KindState, State: ev.State, Done: true, At: at}
	for _, id := range ev.Final {
		item.Final = append(item.Final, l.prefix+id)
	}
	return &pendingItem{seq: r.nextSeq(l.id), item: item}
}

// applyEvent records an event's conversation, work state, and usage. A session that finishes a
// turn has a reply for the user to see.
func applyEvent(s *store.Session, ev harness.Event) {
	if ev.Conversation != "" {
		s.Conversation = ev.Conversation
	}
	switch ev.State {
	case harness.Working:
		s.Activity = store.Busy
	case harness.Idle:
		if s.Activity == store.Busy {
			s.Unseen = true
		}
		s.Activity = store.Idle
	}
	if ev.Usage != nil {
		s.Usage = *ev.Usage
	}
}
