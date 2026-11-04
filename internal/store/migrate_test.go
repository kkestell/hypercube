package store

import "testing"

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
