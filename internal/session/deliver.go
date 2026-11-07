package session

import (
	"errors"
	"fmt"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// Deliver sends the session's queued messages, waking the session when it has
// no running agent. Callers queue a message, then call Deliver.
func (r *Runtime) Deliver(id string) {
	r.mu.Lock()
	l := r.live[id]
	needsWake := l == nil || l.stopping
	r.kickLocked(id)
	r.mu.Unlock()
	if !needsWake {
		return
	}
	wanted := false
	err := r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		queued, err := tx.Queued(id)
		wanted = !s.Ended && len(queued) > 0
		return err
	})
	if err == nil && wanted {
		_, err = r.wake(id)
	}
	if err != nil && !errors.Is(err, errClosing) && !errors.Is(err, errDeleting) {
		logf("waking %s for its messages: %v", id, err)
	}
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
		if len(out.reports) > 0 {
			if err := tx.BatchReports(out.reports, out.batch); err != nil {
				return err
			}
		}
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
	timer := time.NewTimer(r.requestTimeout)
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
		return tx.putItem(l.id, r.nextSeq(l.id), harness.Item{ID: l.prefix + fmt.Sprintf("delivery-timeout-%d", command), Kind: harness.KindError, Text: "Delivering a message got no reply in time; later messages wait for it", Done: true})
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
	reports []int64
	// batch is the reports' text, which replaces the first report's.
	batch string
	// text is what the plugin receives, and item what the transcript shows.
	text string
	item harness.Item
	// seq is the item's position. It is taken before sending, so the items of
	// the plugin's reply follow it.
	seq int64
}

// next returns l's oldest queued message, unless l is over or nothing is
// queued. The caller holds mu.
func (r *Runtime) next(l *live) (out outgoing, ok bool) {
	if !r.current(l) {
		return out, false
	}
	err := r.db.Read(func(tx *store.Tx) error {
		queued, err := tx.Queued(l.id)
		if err != nil || len(queued) == 0 {
			return err
		}
		for _, m := range queued {
			if m.Report {
				continue
			}
			out = outgoing{message: m.ID, text: m.Text, item: harness.Item{Kind: harness.KindUser, Text: m.Text, Automatic: true, Done: true}}
			// The transcript shows a session's message as written, with its sender.
			if m.From != "" {
				out.text = report.Format(tx, m)
				out.item.From, out.item.Automatic = m.From, false
			}
			ok = true
			return nil
		}
		parent, err := tx.Session(l.id)
		if err != nil {
			return err
		}
		ready, waiting, err := r.reportsReady(tx, parent)
		if err != nil || !ready {
			return err
		}
		out = reportInput(queued, waiting)
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
