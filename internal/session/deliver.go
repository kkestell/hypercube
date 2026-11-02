package session

import (
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// Deliver sends the agent's queued messages, if it is running and its session
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

// kickLocked starts delivering to the agent if it is ready and no delivery is
// running. The caller holds mu.
func (r *Runtime) kickLocked(id string) {
	l := r.live[id]
	if l == nil || l.starting || l.delivering {
		return
	}
	l.delivering = true
	go r.deliver(l, l.client)
}

// deliver sends l's queued messages one at a time, removing each after the
// plugin accepts it. Messages go out whether the agent is idle or working; the
// plugin steers or queues them natively. A message the plugin refuses stays
// queued until the next delivery attempt.
//
// deliver holds mu except while it waits on the plugin, so finding the queue
// empty and clearing delivering happen together.
func (r *Runtime) deliver(l *live, client *harness.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer func() { l.delivering = false }()
	for {
		out, ok := r.next(l)
		if !ok {
			return
		}
		r.mu.Unlock()
		sent := request(client, harness.Command{Command: harness.CmdInput, Text: out.text})
		r.mu.Lock()
		if !r.delivered(l, out, sent) {
			return
		}
	}
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
		if err != nil || s.AwaitingUser {
			return err
		}
		queued, err := tx.Queued(l.id)
		if err != nil || len(queued) == 0 {
			return err
		}
		m := queued[0]
		text := report.Format(tx, m)
		out = outgoing{message: m.ID, text: text, item: harness.Item{Kind: harness.KindUser, Text: text, Done: true}}
		// The transcript shows an agent's message as written, with its sender.
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

// delivered records the result of sending out: on success it removes the
// message from the queue and adds its item to the transcript, and on failure
// it notes the error. It reports whether to send the next message. The caller
// holds mu.
func (r *Runtime) delivered(l *live, out outgoing, sent error) bool {
	err := r.save(func(tx *write) error {
		if sent != nil {
			return r.note(tx, l, harness.KindError, "Delivering a message failed: "+sent.Error())
		}
		if err := tx.Dequeue(out.message); err != nil {
			return err
		}
		if err := tx.TouchSession(l.root, time.Now().UnixMilli()); err != nil {
			return err
		}
		return tx.putItem(l.id, out.seq, out.item)
	})
	if err != nil {
		logf("delivering to %s: %v", l.id, err)
	}
	return sent == nil && err == nil
}
