package store

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestCompactEntriesProjectionAndBoundaries(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s")
	it := harness.Item{
		ID: "item", Parent: "parent", Kind: harness.KindTool, Text: "文字",
		From: "sender", Tool: "shell", Answer: "yes", State: harness.Idle,
		Automatic: true, Failed: true, Final: []string{"response"},
		Output: strings.Repeat("output", 100000),
		Images: []harness.Image{{MIMEType: "image/png", Data: strings.Repeat("a", 1000000)}},
		Input:  json.RawMessage(`{"large":"unused"}`),
	}
	data, err := json.Marshal(it)
	if err != nil {
		t.Fatal(err)
	}
	tx(t, db, func(tx *Tx) error {
		for _, e := range []Entry{
			{Seq: 1, Item: "empty", Data: []byte(`{"kind":"state","state":"idle"}`)},
			{Seq: 5, Item: it.ID, Data: data},
			{Seq: 9, Item: "latest", Data: []byte(`{"kind":"text","text":"latest"}`)},
		} {
			if _, err := tx.PutEntry("s", e); err != nil {
				return err
			}
		}
		return nil
	})
	if err := db.Read(func(tx *Tx) error {
		got, err := tx.CompactEntries("s", 9, 1)
		want := []CompactEntry{{Seq: 5, ID: "item", Parent: "parent", Kind: harness.KindTool,
			Text: "文字", From: "sender", Tool: "shell", Answer: "yes", State: harness.Idle,
			Automatic: true, Failed: true, HasFinal: true}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("projection = %+v, %v; want %+v", got, err, want)
		}
		for _, before := range []int64{0, -1} {
			got, err = tx.CompactEntries("s", before, 2)
			if err != nil || len(got) != 2 || got[0].Seq != 9 || got[1].Seq != 5 {
				t.Fatalf("before %d = %+v, %v", before, got, err)
			}
		}
		got, err = tx.CompactEntries("s", 5, 2)
		if err != nil || len(got) != 1 || got[0].HasFinal || got[0].State != harness.Idle {
			t.Fatalf("idle without final = %+v, %v", got, err)
		}
		got, err = tx.CompactEntries("s", 1, 2)
		if err != nil || len(got) != 0 {
			t.Fatalf("before first = %+v, %v", got, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
