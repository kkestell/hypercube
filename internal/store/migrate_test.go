package store

import "testing"

func TestOpenDropsCatalogs(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "child")
	tx(t, db, func(tx *Tx) error {
		return tx.exec(`CREATE TABLE catalogs (harness TEXT, dir TEXT, model TEXT, catalog TEXT);
INSERT INTO catalogs VALUES ('h', '/repo', 'm', '{}');
UPDATE sessions SET harness = 'h', model = 'native/id', effort = 'high' WHERE id = 's';
PRAGMA user_version = 3;`)
	})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reopen(t).Read(func(tx *Tx) error {
		has, err := tx.exists(`SELECT 1 FROM sqlite_master WHERE name = 'catalogs'`)
		if err != nil {
			return err
		}
		if has {
			t.Fatal("catalogs retained")
		}
		var version int
		if err := tx.tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
			return err
		}
		if version != 4 {
			t.Fatalf("version = %d", version)
		}
		list, err := tx.ModelEntries()
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Fatalf("entries = %+v", list)
		}
		s, err := tx.Session("s")
		if err == nil && (s.Workspace != "w" || s.Harness != "h" || s.Model != "native/id" || s.Effort != "high") {
			t.Fatalf("session changed: %+v", s)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

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
		// This state predates the transcript cleanup.
		return tx.exec(`PRAGMA user_version = 0`)
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

func TestOpenDropsRunColumnsAndClearsAttentionWithoutAgent(t *testing.T) {
	db := open(t)
	waiting := []Session{
		{ID: "awake", Launch: "l1"},
		{ID: "awake-child", Parent: "awake", Name: "child", Launch: "l2"},
		{ID: "awake-ended", Parent: "awake", Name: "ended", Ended: true, Launch: "l3"},
		{ID: "parked"},
		{ID: "parked-child", Parent: "parked", Name: "child"},
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
	// Recreate the previous version's columns.
	if _, err := db.sql.Exec(`ALTER TABLE sessions ADD COLUMN running INTEGER NOT NULL DEFAULT 1;
ALTER TABLE sessions ADD COLUMN awaiting_user INTEGER NOT NULL DEFAULT 0;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	err := reopen(t).Read(func(tx *Tx) error {
		for _, column := range []string{"running", "awaiting_user"} {
			if has, err := tx.exists(`SELECT 1 FROM pragma_table_info('sessions') WHERE name = '` + column + `'`); err != nil || has {
				t.Errorf("column %s present=%v, %v", column, has, err)
			}
		}
		for _, s := range waiting {
			got, err := tx.Session(s.ID)
			if err != nil {
				return err
			}
			if want := s.ID == "awake" || s.ID == "awake-child"; got.Attention != want {
				t.Errorf("%s attention = %v, want %v", s.ID, got.Attention, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenDeletesRequestsMirroredIntoParents(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "child")
	items := map[string]string{
		"mirror":  `{"id":"mirror","kind":"attention","from":"s-child","text":"Proceed?"}`,
		"own":     `{"id":"own","kind":"attention","text":"Allow?"}`,
		"message": `{"id":"message","kind":"user","from":"s-child","text":"Done."}`,
	}
	tx(t, db, func(tx *Tx) error {
		seq := int64(0)
		for _, id := range []string{"mirror", "own", "message"} {
			seq++
			if _, err := tx.PutEntry("s", Entry{Seq: seq, Item: id, Data: []byte(items[id])}); err != nil {
				return err
			}
		}
		// This state predates the transcript cleanup.
		return tx.exec(`PRAGMA user_version = 0`)
	})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reopen(t).Read(func(tx *Tx) error {
		entries, err := tx.Transcript("s")
		if err != nil {
			return err
		}
		var got []string
		for _, e := range entries {
			got = append(got, e.Item)
		}
		if len(got) != 2 || got[0] != "own" || got[1] != "message" {
			t.Errorf("items after reopening = %v, want [own message]", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyTranscriptCleanupRunsOnce(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s")
	tx(t, db, func(tx *Tx) error {
		_, err := tx.PutEntry("s", Entry{Seq: 1, Item: "status", Data: []byte(`{"kind":"status"}`)})
		return err
	})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reopen(t).Read(func(tx *Tx) error {
		entries, err := tx.Transcript("s")
		if err == nil && len(entries) != 1 {
			t.Error("reopening repeated the historical transcript cleanup")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenReplacesSavedProfilesWithSettings(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s")
	profile := `{"name":"worker","description":"Small jobs","harness":"codex","model":"sol","effort":"low"}`
	entries := []struct{ before, after string }{
		{`{"id":"p1","kind":"proposal","text":"Task","input":{"name":"task","title":"Task","profile":` + profile + `,"dir":".","message":"brief"}}`,
			`{"id":"p1","kind":"proposal","text":"Task","input":{"name":"task","title":"Task","dir":".","message":"brief","settings":{"harness":"codex","model":"sol","effort":"low"}}}`},
		{`{"id":"p2","kind":"proposal","done":true,"answer":"child-id","input":{"name":"done","profile":` + profile + `}}`,
			`{"id":"p2","kind":"proposal","done":true,"answer":"child-id","input":{"name":"done","settings":{"harness":"codex","model":"sol","effort":"low"}}}`},
		{`{"id":"o1","kind":"attention","offer":true,"done":true,"answer":"New session","input":{"name":"offered","profile":` + profile + `}}`,
			`{"id":"o1","kind":"attention","offer":true,"done":true,"answer":"New session","input":{"name":"offered","settings":{"harness":"codex","model":"sol","effort":"low"}}}`},
		// A recorded tool call keeps what the agent sent.
		{`{"id":"t1","kind":"tool","input":{"profile":"worker"}}`, `{"id":"t1","kind":"tool","input":{"profile":"worker"}}`},
	}
	tx(t, db, func(tx *Tx) error {
		for i, e := range entries {
			if _, err := tx.PutEntry("s", Entry{Seq: int64(i + 1), Item: string(rune('a' + i)), Data: []byte(e.before)}); err != nil {
				return err
			}
		}
		return tx.exec(`PRAGMA user_version = 1`)
	})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reopen(t).Read(func(tx *Tx) error {
		saved, err := tx.Transcript("s")
		if err != nil {
			return err
		}
		for i, e := range entries {
			if saved[i].Seq != int64(i+1) || string(saved[i].Data) != e.after {
				t.Errorf("entry %d = %d %s, want %s", i, saved[i].Seq, saved[i].Data, e.after)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
