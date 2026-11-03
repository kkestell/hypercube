package session

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// Change is one change the runtime made. A transcript change names the
// session, item, and the item's position. Other writes leave Session empty.
type Change struct {
	Session string
	Seq     int64
	Item    harness.Item
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
	items   []Change
	runtime *Runtime
}

// save runs fn in one transaction and, once it commits, publishes the write and
// then each item fn saved. Callers that save items hold mu, as event does when
// it publishes streaming items, so each session's changes are published in the
// order they were made.
func (r *Runtime) save(fn func(*write) error) error {
	var w write
	err := r.db.Tx(func(tx *store.Tx) error {
		w.Tx = tx
		w.runtime = r
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

// putItem saves an item. One that replaces a saved item keeps its start time,
// and its end time if the saved item was complete.
func (w *write) putItem(id string, seq int64, item harness.Item) error {
	saved, replaces, err := w.Entry(id, item.ID)
	if err != nil {
		return err
	}
	if replaces {
		var old harness.Item
		if err := json.Unmarshal(saved.Data, &old); err != nil {
			return err
		}
		item.At = old.At
		if old.Done {
			item.End = old.End
		}
	}
	if item.At == 0 {
		item.At = time.Now().UnixMilli()
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	seq, err = w.PutEntry(id, store.Entry{Seq: seq, Item: item.ID, Data: data})
	if err != nil {
		return err
	}
	w.items = append(w.items, Change{Session: id, Seq: seq, Item: item})
	return w.runtime.mirrorLocked(w, id, item)
}

// mirrorLocked writes only opening and saved attention items. The caller holds mu.
func (r *Runtime) mirrorLocked(tx *write, id string, item harness.Item) error {
	if item.Kind != harness.KindAttention {
		return nil
	}
	s, err := tx.Session(id)
	if err != nil || s.Parent == "" {
		return err
	}
	if err := r.loadSeq(tx.Tx, s.Parent); err != nil {
		return err
	}
	item.From, item.Parent = id, ""
	return tx.putItem(s.Parent, r.nextSeq(s.Parent), item)
}
