package session

import (
	"fmt"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// Deliver sends the session's queued messages, if it is running and its session
// is not waiting for the user. Callers queue a message, then call Deliver.
func (r *Runtime) Deliver(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.kickLocked(id)
}

// kickFamilyLocked delivers to every member of the family. The caller holds
// mu.
func (r *Runtime) kickFamilyLocked(session string) {
	for id, l := range r.live {
		if l.root == session {
			r.kickLocked(id)
		}
	}
}

// kickLocked records one delivery. Its unanswered row holds back the next.
func (r *Runtime) kickLocked(id string) {
	l := r.live[id]
	if l == nil || l.ready == 0 || l.stopping || r.closing || l.delivering() {
		return
	}
	out, ok := r.next(l)
	if !ok {
		return
	}
	var command int64
	err := r.save(func(tx *write) error {
		var err error
		command, err = r.record(tx, l, harness.Command{Command: harness.CmdInput, Text: out.text}, out.message, out.seq, &out.item)
		return err
	})
	if err != nil {
		logf("recording delivery: %v", err)
		return
	}
	go r.send(l)
	go r.watchDelivery(l, command)
}

func (r *Runtime) watchDelivery(l *live, command int64) {
	timer := time.NewTimer(requestTimeout)
	defer timer.Stop()
	select {
	case <-l.done:
		return
	case <-timer.C:
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.current(l) || r.detached {
		return
	}
	if _, ok := l.commands[command]; !ok {
		return
	}
	err := r.save(func(tx *write) error {
		return tx.putItem(l.id, r.nextSeq(l.id), harness.Item{ID: l.prefix + fmt.Sprintf("delivery-timeout-%d", command), Kind: harness.KindError, Text: "Delivering a message got no reply within 30 seconds; later messages wait for it", Done: true})
	})
	if err != nil {
		logf("noting delivery: %v", err)
	}
}

func (l *live) delivering() bool {
	for _, c := range l.commands {
		if c.Message != 0 {
			return true
		}
	}
	return false
}

// outgoing is a queued message ready to send.
type outgoing struct {
	message int64
	// text is what the plugin receives, and item what the transcript shows.
	text string
	item harness.Item
	// seq is the item's position. It is taken before sending, so the items of
	// the plugin's reply follow it.
	seq int64
}

// next returns l's oldest queued message, unless l is over, its session waits
// for the user, or nothing is queued. The caller holds mu.
func (r *Runtime) next(l *live) (out outgoing, ok bool) {
	if !r.current(l) {
		return out, false
	}
	err := r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Root(l.id)
		if err != nil || s.AwaitingUser || !s.Running || r.deleting[l.root] {
			return err
		}
		queued, err := tx.Queued(l.id)
		if err != nil || len(queued) == 0 {
			return err
		}
		m := queued[0]
		text := report.Format(tx, m)
		out = outgoing{message: m.ID, text: text, item: harness.Item{Kind: harness.KindUser, Text: text, Done: true}}
		// The transcript shows a session's message as written, with its sender.
		if m.From != "" {
			out.item.Text, out.item.From = m.Text, m.From
		}
		ok = true
		return nil
	})
	if err != nil {
		logf("reading messages for %s: %v", l.id, err)
		return out, false
	}
	if ok {
		out.item.ID = l.prefix + "host-" + store.NewID()
		out.seq = r.nextSeq(l.id)
	}
	return out, ok
}
