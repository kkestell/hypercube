package session

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// Change is one change the runtime made. A transcript change names the
// session, agent, item, and the item's position. Other writes leave Agent empty.
type Change struct {
	Session, Agent string
	Seq            int64
	Item           harness.Item
}

type feed struct {
	mu          sync.Mutex
	subscribers map[chan Change]struct{}
}

// Subscribe returns runtime changes until canceled or the reader falls behind.
// A dropped reader reconnects and reads the current state and transcripts.
func (r *Runtime) Subscribe() (<-chan Change, func()) {
	f := &r.feed
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subscribers == nil {
		f.subscribers = map[chan Change]struct{}{}
	}
	ch := make(chan Change, 1024)
	f.subscribers[ch] = struct{}{}
	return ch, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if _, ok := f.subscribers[ch]; ok {
			delete(f.subscribers, ch)
			close(ch)
		}
	}
}

func (f *feed) publish(change Change) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for ch := range f.subscribers {
		select {
		case ch <- change:
		default:
			delete(f.subscribers, ch)
			close(ch)
		}
	}
}

type write struct {
	*store.Tx
	items []Change
}

func (r *Runtime) save(fn func(*write) error) error {
	var w write
	err := r.db.Tx(func(tx *store.Tx) error {
		w.Tx = tx
		return fn(&w)
	})
	if err != nil {
		return err
	}
	r.feed.publish(Change{})
	for _, change := range w.items {
		r.feed.publish(change)
	}
	return nil
}

func (w *write) putItem(l *live, seq int64, item harness.Item) error {
	if item.At == 0 {
		item.At = time.Now().UnixMilli()
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if err := w.PutEntry(l.id, store.Entry{Seq: seq, Item: item.ID, Data: data}); err != nil {
		return err
	}
	w.items = append(w.items, Change{Session: l.session, Agent: l.id, Seq: seq, Item: item})
	return nil
}
