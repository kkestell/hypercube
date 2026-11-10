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
// It reports whether the record could have queued a message for a child's
// parent.
func (r *Runtime) apply(l *live, rec harness.Record, next int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.current(l) || r.detached {
		return false, nil
	}
	if rec.Event != nil {
		return r.eventLocked(l, *rec.Event, next)
	}
	reply := *rec.Reply
	command, ok := l.commands[reply.ID]
	if !ok {
		return false, fmt.Errorf("reply to unknown request %d", reply.ID)
	}
	if err := r.save(func(tx *write) error {
		if err := r.reply(tx, l, reply); err != nil {
			return err
		}
		return r.checkpoint(tx, l, next)
	}); err != nil {
		return false, err
	}
	l.position = next
	// The launch has started once its start and accepted settings are saved.
	if command.Command == harness.CmdStart && reply.Error == "" {
		close(l.started)
	}
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
	} else {
		// Other family members may now be eligible, but do not immediately
		// retry a rejected queued message to this session.
		for id, member := range r.live {
			if member.root == l.root && id != l.id {
				r.kickLocked(id)
			}
		}
		if command.Message == 0 {
			r.kickLocked(l.id)
		}
	}
	r.settleLocked(l.id)
	return false, nil
}

// checkpoint saves streaming items as unfinished.
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

// eventLocked applies an event to the live launch, saves its effects with the
// event's position in one transaction, and then schedules what they allow. It
// reports whether a child's event could have queued a report or failure for
// its parent.
func (r *Runtime) eventLocked(l *live, ev harness.Event, next int64) (bool, error) {
	l.position = next
	applied, err := r.applyItems(l, ev, next)
	if err != nil {
		return false, err
	}
	// A stopping launch waits on nobody, whatever it still has pending.
	attention := false
	for _, p := range l.pending {
		attention = attention || p.item.Kind == harness.KindAttention && !l.stopping
	}
	changed := ev.Conversation != "" || ev.State != "" || ev.Usage != nil || attention != l.attention
	if len(applied.completed) == 0 && len(applied.allowed) == 0 && len(applied.failures) == 0 && !applied.questions && len(ev.Limits) == 0 && !changed {
		// Streaming output is published and saved at the next checkpoint.
		return false, nil
	}
	if err := r.save(func(tx *write) error { return r.saveEvent(tx, l, ev, applied, changed, attention, next) }); err != nil {
		return false, err
	}
	l.attention = attention
	if len(applied.allowed) > 0 {
		go r.send(l)
	}
	// Failures and finished turns queue messages, and a question that opens or
	// closes changes whether the parent can take its children's reports.
	if len(applied.failures) > 0 || ev.State == harness.Idle || applied.questions {
		r.kickFamilyLocked(l.root)
	}
	_ = l.client.Discard(next)
	r.settleLocked(l.id)
	return l.root != l.id && (len(applied.failures) > 0 || ev.State == harness.Idle), nil
}

// appliedItems is what an event's items did to the live launch.
type appliedItems struct {
	completed []*pendingItem // finished items and state markers, in transcript order
	allowed   []harness.Item // approvals answered by saved grants
	failures  []string       // the text of new error items
	questions bool           // a question opened or closed
}

// applyItems namespaces the event's items, answers granted approvals, merges
// streaming items, and publishes unfinished ones. A turn's work marker
// precedes the items the same event starts, and its idle marker follows the
// items the event finishes. The caller holds mu.
func (r *Runtime) applyItems(l *live, ev harness.Event, next int64) (appliedItems, error) {
	var out appliedItems
	at := time.Now().UnixMilli()
	if ev.State == harness.Working {
		out.completed = append(out.completed, r.stateMarker(l, ev, next, at))
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
					return out, err
				}
			}
			if granted {
				out.allowed = append(out.allowed, it)
				continue
			}
		}
		out.questions = out.questions || it.Kind == harness.KindAttention && (p == nil || it.Done)
		it.At = at
		if p != nil {
			it.At = p.item.At
		}
		if p == nil {
			p = &pendingItem{seq: r.nextSeq(l.id)}
			l.pending[it.ID] = p
			if it.Kind == harness.KindError {
				out.failures = append(out.failures, it.Text)
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
			out.completed = append(out.completed, p)
		} else {
			r.feed.publish(Change{Session: l.id, Seq: p.seq, Item: it})
		}
	}
	if ev.State == harness.Idle {
		out.completed = append(out.completed, r.stateMarker(l, ev, next, at))
	}
	return out, nil
}

// saveEvent saves the event's durable effects: granted answers, finished
// items, limits, the session's state, a child's failure and report messages,
// and the checkpoint.
func (r *Runtime) saveEvent(tx *write, l *live, ev harness.Event, applied appliedItems, changed, attention bool, next int64) error {
	for _, item := range applied.allowed {
		if _, err := r.recordAllow(tx, l, item, false); err != nil {
			return err
		}
	}
	for _, e := range applied.completed {
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
		for _, text := range applied.failures {
			if err := queueFailure(tx, s, text); err != nil {
				return err
			}
		}
		if err := r.queueReport(tx, s, ev.Final, l.prefix); err != nil {
			return err
		}
	}
	return r.checkpoint(tx, l, next)
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

// applyEvent records an event's conversation, work state, and usage. A top-level session that
// finishes a turn has a reply for the user to see; a child session reports to its parent instead.
func applyEvent(s *store.Session, ev harness.Event) {
	if ev.Conversation != "" {
		s.Conversation = ev.Conversation
		s.Fork = ""
	}
	switch ev.State {
	case harness.Working:
		s.Activity = store.Busy
		s.InputPending = false
	case harness.Idle:
		if s.Activity == store.Busy && s.Parent == "" {
			s.Unseen = true
		}
		if s.Activity == store.Busy || len(ev.Items) > 0 || len(ev.Final) > 0 || ev.InputPending {
			s.InputPending = ev.InputPending
		}
		s.Activity = store.Idle
	}
	if ev.Usage != nil {
		s.Usage = *ev.Usage
	}
}
