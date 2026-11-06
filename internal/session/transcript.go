package session

import (
	"cmp"
	"encoding/json"
	"slices"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// TranscriptItem is an item with its position in the session's transcript.
type TranscriptItem struct {
	harness.Item
	Seq int64 `json:"seq"`
}

// Request is an open plugin question or a saved task offer.
type Request struct {
	Session string       `json:"session"`
	Item    harness.Item `json:"item"`
}

// Requests returns open plugin requests and saved offers, newest first.
func (r *Runtime) Requests() ([]Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := []Request{}
	for _, l := range r.live {
		for _, p := range l.pending {
			if p.item.Kind == harness.KindAttention {
				requests = append(requests, Request{Session: l.id, Item: p.item})
			}
		}
	}
	if err := r.db.Read(func(tx *store.Tx) error {
		offers, err := tx.OpenOffers()
		if err != nil {
			return err
		}
		for _, offer := range offers {
			var item harness.Item
			if err := json.Unmarshal(offer.Data, &item); err != nil {
				return err
			}
			requests = append(requests, Request{Session: offer.Session, Item: item})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	slices.SortFunc(requests, func(a, b Request) int {
		return cmp.Or(cmp.Compare(b.Item.At, a.Item.At), cmp.Compare(b.Item.ID, a.Item.ID))
	})
	return requests, nil
}

// Transcript returns the session's items in order: those saved, then those still
// streaming.
func (r *Runtime) Transcript(id string) ([]TranscriptItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var entries []store.Entry
	if err := r.db.Read(func(tx *store.Tx) error {
		var err error
		entries, err = tx.Transcript(id)
		return err
	}); err != nil {
		return nil, err
	}
	items := make([]TranscriptItem, 0, len(entries))
	for _, e := range entries {
		var item harness.Item
		if err := json.Unmarshal(e.Data, &item); err != nil {
			return nil, err
		}
		if l := r.live[id]; l != nil && l.pending[item.ID] != nil {
			continue
		}
		items = append(items, TranscriptItem{Item: item, Seq: e.Seq})
	}
	if l := r.live[id]; l != nil {
		for _, p := range l.pending {
			items = append(items, TranscriptItem{Item: p.item, Seq: p.seq})
		}
		slices.SortFunc(items, func(a, b TranscriptItem) int { return cmp.Compare(a.Seq, b.Seq) })
	}
	return items, nil
}
