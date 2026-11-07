package session

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func compactRuntime(t *testing.T) (*store.DB, *Runtime) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Tx(func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: "/repo"}); err != nil {
			return err
		}
		return tx.AddSession(store.Session{ID: "s", Workspace: "w"})
	}); err != nil {
		t.Fatal(err)
	}
	return db, New(db, "")
}

func putCompactItem(tx *store.Tx, seq int64, it harness.Item) error {
	data, err := json.Marshal(it)
	if err != nil {
		return err
	}
	_, err = tx.PutEntry("s", store.Entry{Seq: seq, Item: it.ID, Data: data})
	return err
}

func TestCompactTranscriptMergesPendingAcrossSnapshot(t *testing.T) {
	db, rt := compactRuntime(t)
	if err := db.Tx(func(tx *store.Tx) error {
		for seq := int64(1); seq <= 140; seq++ {
			if seq == 35 || seq == 70 {
				continue
			}
			if err := putCompactItem(tx, seq, harness.Item{ID: fmt.Sprint(seq), Kind: harness.KindText, Text: "saved"}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	l := &live{pending: map[string]*pendingItem{
		"160": {seq: 160, item: harness.Item{ID: "160", Kind: harness.KindText, Text: "pending latest"}},
		"10":  {seq: 10, item: harness.Item{ID: "10", Kind: harness.KindText, Text: "pending replacement"}},
		"35":  {seq: 35, item: harness.Item{ID: "35", Kind: harness.KindText, Text: "pending insertion"}},
		"75":  {seq: 75, item: harness.Item{ID: "75", Kind: harness.KindReasoning, Text: "filtered replacement"}},
		// A pending ID suppresses its saved version even outside the cursor.
		"2": {seq: 200, item: harness.Item{ID: "2", Kind: harness.KindText, Text: "outside cursor"}},
	}}
	rt.live["s"] = l
	complete := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-complete
		rt.mu.Lock()
		defer rt.mu.Unlock()
		done <- db.Tx(func(tx *store.Tx) error {
			for id, p := range l.pending {
				p.item.Text = "completed"
				p.item.Kind = harness.KindText
				if err := putCompactItem(tx, p.seq, p.item); err != nil {
					return err
				}
				delete(l.pending, id)
			}
			// This position is in a later batch and must stay outside the snapshot.
			return putCompactItem(tx, 70, harness.Item{ID: "new", Kind: harness.KindText})
		})
	}()
	var got []store.CompactEntry
	err := rt.CompactTranscript("s", 180, func(e store.CompactEntry) bool {
		if len(got) == 0 {
			close(complete)
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(5 * time.Second):
				t.Error("runtime activity blocked during compact read")
				return false
			}
		}
		got = append(got, e)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 139 {
		t.Fatalf("read %d entries, want 139", len(got))
	}
	seen := map[string]bool{}
	for i, e := range got {
		if seen[e.ID] || e.ID == "2" || e.ID == "new" || (i > 0 && got[i-1].Seq <= e.Seq) {
			t.Fatalf("duplicate, new, excluded, or unordered entry: %+v", e)
		}
		seen[e.ID] = true
		switch e.ID {
		case "160":
			if e.Text != "pending latest" {
				t.Fatalf("latest = %+v", e)
			}
		case "10":
			if e.Text != "pending replacement" {
				t.Fatalf("replacement = %+v", e)
			}
		case "35":
			if e.Text != "pending insertion" {
				t.Fatalf("insertion = %+v", e)
			}
		case "75":
			if e.Kind != harness.KindReasoning || e.Text != "filtered replacement" {
				t.Fatalf("filtered replacement = %+v", e)
			}
		}
	}
}

func TestCompactTranscriptWithOnlyPendingEntries(t *testing.T) {
	_, rt := compactRuntime(t)
	rt.live["s"] = &live{pending: map[string]*pendingItem{
		"1": {seq: 1, item: harness.Item{ID: "1", Kind: harness.KindUser, Text: "first"}},
		"2": {seq: 2, item: harness.Item{ID: "2", Kind: harness.KindText, Text: "streaming"}},
	}}
	for _, cursor := range []int64{0, 2} {
		var got []store.CompactEntry
		if err := rt.CompactTranscript("s", cursor, func(e store.CompactEntry) bool {
			got = append(got, e)
			return true
		}); err != nil {
			t.Fatal(err)
		}
		if cursor == 0 && (len(got) != 2 || got[0].ID != "2" || got[1].ID != "1") {
			t.Fatalf("latest pending = %+v", got)
		}
		if cursor == 2 && (len(got) != 1 || got[0].ID != "1") {
			t.Fatalf("pending before 2 = %+v", got)
		}
	}
}

func TestCompactTranscriptStopsBeforeOlderBatches(t *testing.T) {
	db, rt := compactRuntime(t)
	if err := db.Tx(func(tx *store.Tx) error {
		if _, err := tx.PutEntry("s", store.Entry{Seq: 1, Item: "invalid", Data: []byte(`{"kind":"state","automatic":"invalid boolean"}`)}); err != nil {
			return err
		}
		for seq := int64(2); seq <= 130; seq++ {
			if err := putCompactItem(tx, seq, harness.Item{ID: fmt.Sprint(seq), Kind: harness.KindText}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var visited []int64
	if err := rt.CompactTranscript("s", 0, func(e store.CompactEntry) bool {
		visited = append(visited, e.Seq)
		return false
	}); err != nil {
		t.Fatal(err)
	}
	if len(visited) != 1 || visited[0] != 130 {
		t.Fatalf("visited = %v", visited)
	}
}
