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

// TranscriptImage returns an image of the session's item, streaming or saved.
func (r *Runtime) TranscriptImage(id, item string, index int) (harness.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var found harness.Item
	if l := r.live[id]; l != nil && l.pending[item] != nil {
		found = l.pending[item].item
	} else if err := r.db.Read(func(tx *store.Tx) error {
		e, ok, err := tx.Entry(id, item)
		if err != nil || !ok {
			return cmp.Or(err, store.ErrNotFound)
		}
		return json.Unmarshal(e.Data, &found)
	}); err != nil {
		return harness.Image{}, err
	}
	if index < 0 || index >= len(found.Images) {
		return harness.Image{}, store.ErrNotFound
	}
	return found.Images[index], nil
}

// CompactTranscript visits the compact view newest first, before the exclusive
// cursor (or from the end when nonpositive). Returning false stops the read.
func (r *Runtime) CompactTranscript(id string, cursor int64, visit func(store.CompactEntry) bool) error {
	return r.db.Read(func(tx *store.Tx) error {
		pending, replaced, err := r.compactPending(tx, id, cursor)
		if err != nil {
			return err
		}
		slices.SortFunc(pending, func(a, b store.CompactEntry) int { return cmp.Compare(b.Seq, a.Seq) })
		const batchSize = 64
		for {
			entries, err := tx.CompactEntries(id, cursor, batchSize)
			if err != nil {
				return err
			}
			for _, e := range entries {
				for len(pending) > 0 && pending[0].Seq >= e.Seq {
					if !visit(pending[0]) {
						return nil
					}
					pending = pending[1:]
				}
				if !replaced[e.ID] && !visit(e) {
					return nil
				}
			}
			if len(entries) < batchSize {
				break
			}
			cursor = entries[len(entries)-1].Seq
		}
		for _, e := range pending {
			if !visit(e) {
				break
			}
		}
		return nil
	})
}

// compactPending pairs a copy of pending fields with the transaction's saved
// snapshot. Completing an item cannot move it between these two views.
func (r *Runtime) compactPending(tx *store.Tx, id string, cursor int64) ([]store.CompactEntry, map[string]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var pending []store.CompactEntry
	replaced := map[string]bool{}
	if l := r.live[id]; l != nil {
		for itemID, p := range l.pending {
			replaced[itemID] = true
			if cursor > 0 && p.seq >= cursor {
				continue
			}
			it := p.item
			pending = append(pending, store.CompactEntry{
				Seq: p.seq, ID: it.ID, Parent: it.Parent, Kind: it.Kind,
				Text: it.Text, From: it.From, Tool: it.Tool, Answer: it.Answer,
				State: it.State, Automatic: it.Automatic, Failed: it.Failed,
				HasFinal: len(it.Final) > 0,
			})
		}
	}
	// BeginTx alone does not establish a SQLite read snapshot; the first
	// query must run before releasing the lock that guards pending items.
	_, err := tx.LastSeq(id)
	return pending, replaced, err
}
