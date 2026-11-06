package store

import "testing"

func TestOpenUpgradesChildCoordinationState(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "child")
	tx(t, db, func(tx *Tx) error {
		return tx.Queue(Message{From: "s-child", To: "s", Text: "saved message"})
	})
	if _, err := db.sql.Exec(`ALTER TABLE sessions DROP COLUMN fork; ALTER TABLE sessions DROP COLUMN input_pending; ALTER TABLE messages DROP COLUMN report;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		db, err := Open()
		if err != nil {
			t.Fatal(err)
		}
		err = db.Read(func(tx *Tx) error {
			s, err := tx.Session("s")
			if err != nil {
				return err
			}
			if s.Fork != "" || s.InputPending {
				t.Fatalf("unexpected fork %q", s.Fork)
			}
			messages, err := tx.Queued("s")
			if err != nil {
				return err
			}
			if len(messages) != 1 || messages[0].Report || messages[0].Text != "saved message" {
				t.Fatalf("messages %+v", messages)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpenUpgradesDatabaseBeforeDurableLaunches(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "child")
	tx(t, db, func(tx *Tx) error {
		s, err := tx.Session("s")
		if err != nil {
			return err
		}
		s.Title, s.Conversation = "Saved session", "conversation"
		if err := tx.SaveSession(s); err != nil {
			return err
		}
		if _, err := tx.PutEntry("s", Entry{Seq: 1, Item: "reply", Data: []byte(`{"kind":"text","text":"Saved reply"}`)}); err != nil {
			return err
		}
		return tx.Queue(Message{From: "s-child", To: "s", Text: "Saved message"})
	})
	// Recreate the schema of the previous version, keeping its saved data.
	if _, err := db.sql.Exec(`
DROP TABLE commands;
ALTER TABLE sessions DROP COLUMN launch;
ALTER TABLE sessions DROP COLUMN position;
ALTER TABLE sessions DROP COLUMN ready;
ALTER TABLE sessions DROP COLUMN quick;
ALTER TABLE sessions DROP COLUMN base;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		again, err := Open()
		if err != nil {
			t.Fatal(err)
		}
		err = again.Read(func(tx *Tx) error {
			sessions, err := tx.Sessions("w")
			if err != nil {
				return err
			}
			if len(sessions) != 1 {
				t.Fatalf("sessions after upgrade = %+v", sessions)
			}
			s := sessions[0]
			if s.ID != "s" || s.Title != "Saved session" || s.Conversation != "conversation" || s.Launch != "" || s.Position != 0 || s.Ready != 0 || s.Quick != 0 || s.Base != "" {
				t.Errorf("session after upgrade = %+v", s)
			}
			children, err := tx.Children("s")
			if err != nil {
				return err
			}
			if len(children) != 1 || children[0].ID != "s-child" {
				t.Errorf("children after upgrade = %+v", children)
			}
			entries, err := tx.Transcript("s")
			if err != nil {
				return err
			}
			if len(entries) != 1 || entries[0].Item != "reply" {
				t.Errorf("transcript after upgrade = %+v", entries)
			}
			messages, err := tx.Queued("s")
			if err != nil {
				return err
			}
			if len(messages) != 1 || messages[0].Text != "Saved message" {
				t.Errorf("messages after upgrade = %+v", messages)
			}
			_, err = tx.Commands("s")
			return err
		})
		if closeErr := again.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpenUpgradesDatabaseBeforeWorktreeSessions(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s")
	if _, err := db.sql.Exec(`ALTER TABLE sessions DROP COLUMN base`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		again, err := Open()
		if err != nil {
			t.Fatal(err)
		}
		err = again.Read(func(tx *Tx) error {
			s, err := tx.Session("s")
			if err == nil && s.Base != "" {
				t.Errorf("session after upgrade = %+v", s)
			}
			return err
		})
		if closeErr := again.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
