package session

import (
	"github.com/kkestell/hypercube/internal/store"
	"testing"
)

func TestSendMessageScope(t *testing.T) {
	f := newFixture(t)
	f.seed(store.Session{ID: "child", Name: "child"}, store.Session{ID: "sibling", Name: "sibling"})
	if err := f.db.Tx(func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "far", Path: "/other"}); err != nil {
			return err
		}
		for _, s := range []store.Session{{ID: "peer", Workspace: f.workspace.ID, Running: true}, {ID: "peer-child", Parent: "peer", Name: "child", Workspace: f.workspace.ID}, {ID: "far", Workspace: "far"}} {
			if err := tx.AddSession(s); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		from, to string
		allowed  bool
	}{
		{"s", "child", true}, {"child", "s", true}, {"s", "peer", true}, {"peer", "s", true},
		{"s", "s", false}, {"child", "sibling", false}, {"child", "peer", false}, {"child", "far", false},
		{"s", "peer-child", false}, {"s", "far", false}, {"s", "missing", false},
	} {
		recipient, running, err := f.rt.SendMessage(c.from, c.to, "message")
		if (err == nil) != c.allowed {
			t.Fatalf("%s to %s: %+v %v", c.from, c.to, recipient, err)
		}
		if err == nil && running != (recipient.ID == "peer") {
			t.Errorf("running=%v for %+v", running, recipient)
		}
	}
	if _, _, err := f.rt.SendMessage("s", "child", " \n"); err == nil {
		t.Fatal("blank message accepted")
	}
}
