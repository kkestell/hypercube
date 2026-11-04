package store

import "testing"

func TestGrantsApplyToTheirScopeAndHarness(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "a", "b")
	granted := func(session, harness, tool string) (ok bool) {
		t.Helper()
		if err := db.Read(func(tx *Tx) (err error) { ok, err = tx.Granted(session, harness, tool); return }); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	tx(t, db, func(tx *Tx) error {
		if err := tx.AddGrant("a", "claude", "Bash"); err != nil {
			return err
		}
		if err := tx.AddGrant("a", "claude", "Bash"); err != nil {
			return err
		}
		return tx.AddGrant("", "codex", "fileChange")
	})
	for _, c := range []struct {
		session, harness, tool string
		want                   bool
	}{
		{"a", "claude", "Bash", true},
		{"b", "claude", "Bash", false},
		{"a", "codex", "Bash", false},
		{"a", "claude", "Edit", false},
		{"a", "codex", "fileChange", true},
		{"b", "codex", "fileChange", true},
		{"b", "claude", "fileChange", false},
	} {
		if got := granted(c.session, c.harness, c.tool); got != c.want {
			t.Errorf("Granted(%q, %q, %q) = %v", c.session, c.harness, c.tool, got)
		}
	}
	tx(t, db, func(tx *Tx) error { return tx.DeleteSession("a") })
	if granted("a", "claude", "Bash") || !granted("a", "codex", "fileChange") {
		t.Error("deleting a session should delete only its own grants")
	}
}

func TestListAndRemoveGrant(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "a", "b")
	tx(t, db, func(tx *Tx) error {
		if err := tx.AddGrant("a", "claude", "Bash"); err != nil {
			return err
		}
		return tx.AddGrant("", "claude", "Bash")
	})
	var grants []Grant
	if err := db.Read(func(tx *Tx) (err error) { grants, err = tx.Grants(); return }); err != nil {
		t.Fatal(err)
	}
	if len(grants) != 2 {
		t.Fatalf("got grants: %+v", grants)
	}
	tx(t, db, func(tx *Tx) error { return tx.DeleteGrant("", "claude", "Bash") })
	if err := db.Read(func(tx *Tx) error {
		a, err := tx.Granted("a", "claude", "Bash")
		if err != nil {
			return err
		}
		b, err := tx.Granted("b", "claude", "Bash")
		if err != nil {
			return err
		}
		if !a || b {
			t.Errorf("after removal: session a = %v, b = %v", a, b)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
