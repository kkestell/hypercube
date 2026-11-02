package session

import (
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestSendMessageScope(t *testing.T) {
	f := newFixture(t)
	f.seed(store.Agent{ID: "worker", Name: "worker"})
	if err := f.db.Tx(func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "far", Path: "/other"}); err != nil {
			return err
		}
		for _, s := range []store.Session{{ID: "other", Workspace: f.workspace.ID, Running: true}, {ID: "far", Workspace: "far"}} {
			if err := tx.AddSession(s); err != nil {
				return err
			}
			for _, role := range []string{store.Lead, store.Worker} {
				if err := tx.AddAgent(store.Agent{ID: s.ID + "-" + role, Session: s.ID, Name: role, Role: role}); err != nil {
					return err
				}
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
		{"a", "worker", true},
		{"worker", "lead", true},
		{"a", "other/lead", true},
		{"a", "lead", false},
		{"worker", "other/lead", false},
		{"a", "other/worker", false},
		{"worker", "other/worker", false},
		{"a", "far/lead", false},
		{"a", "missing", false},
	} {
		t.Run(c.from+" to "+c.to, func(t *testing.T) {
			recipient, running, err := f.rt.SendMessage(c.from, c.to, "message")
			if (err == nil) != c.allowed {
				t.Fatalf("SendMessage = %+v, %v, %v", recipient, running, err)
			}
			if err == nil {
				if running != (recipient.Session == "other") {
					t.Fatalf("running = %v for %+v", running, recipient)
				}
				if err := f.db.Read(func(tx *store.Tx) error {
					messages, err := tx.Queued(recipient.ID)
					if err == nil && (len(messages) != 1 || messages[0].From != c.from || messages[0].Text != "message") {
						t.Errorf("queued = %+v", messages)
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if _, _, err := f.rt.SendMessage("a", "worker", " \n"); err == nil {
		t.Fatal("blank message accepted")
	}
}
