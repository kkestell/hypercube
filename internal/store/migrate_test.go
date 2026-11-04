package store

import "testing"

func TestOpenDeletesLegacyStatusItems(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "child")
	kept := []struct {
		session string
		entry   Entry
	}{
		{"s", Entry{Seq: 2, Item: "text", Data: []byte(`{"id":"text","kind":"text","text":"Saved reply"}`)}},
		{"s-child", Entry{Seq: 2, Item: "unknown", Data: []byte(`{"id":"unknown","kind":"unknown","text":"Saved item"}`)}},
	}
	tx(t, db, func(tx *Tx) error {
		for _, k := range kept {
			if _, err := tx.PutEntry(k.session, Entry{Seq: 1, Item: "status", Data: []byte(`{"id":"status","kind":"status","text":"Working"}`)}); err != nil {
				return err
			}
			if _, err := tx.PutEntry(k.session, k.entry); err != nil {
				return err
			}
		}
		return nil
	})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reopen(t).Read(func(tx *Tx) error {
		for _, k := range kept {
			entries, err := tx.Transcript(k.session)
			if err != nil {
				return err
			}
			if len(entries) != 1 || entries[0].Seq != k.entry.Seq || entries[0].Item != k.entry.Item || string(entries[0].Data) != string(k.entry.Data) {
				t.Errorf("%s transcript after reopening = %+v, want %+v", k.session, entries, k.entry)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenClearsAttentionOfSessionsThatAreNotRunning(t *testing.T) {
	db := open(t)
	waiting := []Session{
		{ID: "run", Running: true},
		{ID: "run-live", Parent: "run", Name: "live"},
		{ID: "run-ended", Parent: "run", Name: "ended", Ended: true},
		{ID: "stopped"},
		{ID: "stopped-child", Parent: "stopped", Name: "child"},
	}
	tx(t, db, func(tx *Tx) error {
		if err := tx.AddWorkspace(Workspace{ID: "w", Path: "/w"}); err != nil {
			return err
		}
		for _, s := range waiting {
			s.Workspace, s.Attention = "w", true
			if err := tx.AddSession(s); err != nil {
				return err
			}
		}
		return nil
	})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	err := reopen(t).Read(func(tx *Tx) error {
		for _, s := range waiting {
			got, err := tx.Session(s.ID)
			if err != nil {
				return err
			}
			if want := s.ID == "run" || s.ID == "run-live"; got.Attention != want {
				t.Errorf("%s attention = %v, want %v", s.ID, got.Attention, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
