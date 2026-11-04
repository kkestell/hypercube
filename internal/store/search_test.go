package store

import (
	"encoding/json"
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
		if err := tx.SaveSession(s); err != nil {
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
		if err := db.Read(func(tx *Tx) error { var err error; got, err = tx.SearchSessions(c.query); return err }); err != nil {
			t.Fatal(err)
		}
		if len(got) != c.want {
			t.Errorf("search %q returned %v, want %d", c.query, got, c.want)
		}
	}
	var matches []SearchResult
	if err := db.Read(func(tx *Tx) error { var err error; matches, err = tx.SearchSessions("needle"); return err }); err != nil {
		t.Fatal(err)
	}
	for _, result := range matches {
		if result.ID == "a" && result.Text != "" || result.ID == "b" && result.Text != "Latest needle" {
			t.Errorf("unexpected match: %+v", result)
		}
	}
}
func mustJSON(s string) []byte { b, _ := json.Marshal(s); return b }
