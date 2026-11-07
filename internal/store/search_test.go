package store

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestSearchSessions(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/one", "a", "child")
	seed(t, db, "v", "/two", "b")
	tx(t, db, func(tx *Tx) error {
		s, err := tx.Session("a")
		if err != nil {
			return err
		}
		s.Title = "Title needle"
		s.Ended = true
		if err := tx.SaveSession(s); err != nil {
			return err
		}
		child, err := tx.Session("a-child")
		if err != nil {
			return err
		}
		child.Ended = true
		if err := tx.SaveSession(child); err != nil {
			return err
		}
		items := []struct{ session, kind, text, extra string }{
			{"a-child", "user", "Message needle", ""},
			{"b", "text", "Response needle", ""},
			{"b", "text", "Latest needle", ""},
			{"b", "reasoning", "hiddenreason", ""},
			{"b", "tool", "hiddentool", `,"output":"hiddenoutput"`},
			{"b", "tool", "", `,"server":"hypercube","tool":"send_message","input":{"text":"outgoingword","to":"targetword"}`},
			{"b", "text", "nestedword", `,"parent":"tool"`},
		}
		for i, it := range items {
			data := []byte(`{"id":"x","kind":` + string(mustJSON(it.kind)) + `,"text":` + string(mustJSON(it.text)) + it.extra + `}`)
			if _, err := tx.PutEntry(it.session, Entry{Seq: int64(i + 1), Item: it.kind + it.text, Data: data}); err != nil {
				return err
			}
		}
		return nil
	})
	for _, c := range []struct {
		query string
		want  int
	}{{"NEEDLE", 3}, {"outgoingword", 1}, {"hiddenreason", 0}, {"hiddentool", 0}, {"hiddenoutput", 0}, {"targetword", 0}, {"nestedword", 0}} {
		var got []SearchResult
		if err := db.Read(func(tx *Tx) error {
			var err error
			got, err = tx.SearchSessions(context.Background(), c.query)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if len(got) != c.want {
			t.Errorf("search %q returned %v, want %d", c.query, got, c.want)
		}
	}
	var matches []SearchResult
	if err := db.Read(func(tx *Tx) error {
		var err error
		matches, err = tx.SearchSessions(context.Background(), "needle")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, result := range matches {
		if result.ID == "a" && result.Text != "" || result.ID == "b" && result.Text != "Latest needle" {
			t.Errorf("unexpected match: %+v", result)
		}
	}
	// Replacing an item keeps its sequence while updating both search indexes.
	tx(t, db, func(tx *Tx) error {
		_, err := tx.PutEntry("b", Entry{Seq: 99, Item: "textLatest needle", Data: []byte(`{"kind":"text","text":"replacementword"}`)})
		return err
	})
	var updated []SearchResult
	if err := db.Read(func(tx *Tx) error {
		var err error
		updated, err = tx.SearchSessions(context.Background(), "needle")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(updated) != 3 {
		t.Fatalf("after replacement: %+v", updated)
	}
	for _, result := range updated {
		if result.ID == "b" && result.Text != "Response needle" {
			t.Errorf("latest remaining match: %+v", result)
		}
	}
}

func TestSearchIndexesMigrateSavedEntries(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/one", "s")
	tx(t, db, func(tx *Tx) error {
		_, err := tx.PutEntry("s", Entry{Seq: 1, Item: "one", Data: []byte(`{"kind":"user","text":"savedword"}`)})
		return err
	})
	if _, err := db.sql.Exec(`DROP INDEX transcript_root_text_search;
DROP INDEX transcript_message_text_search;
PRAGMA user_version = 2;`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	again := reopen(t)
	var got []SearchResult
	if err := again.Read(func(tx *Tx) error {
		var err error
		got, err = tx.SearchSessions(context.Background(), "savedword")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []SearchResult{{ID: "s", Workspace: "/one", Text: "savedword"}}) {
		t.Fatalf("migrated search: %+v", got)
	}
}
func mustJSON(s string) []byte { b, _ := json.Marshal(s); return b }
