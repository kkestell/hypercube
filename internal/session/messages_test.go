package session

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestSendMessageScope(t *testing.T) {
	f := newFixture(t)
	f.seed(store.Session{ID: "own-child", Name: "child"}, store.Session{ID: "sibling", Name: "sibling"}, store.Session{ID: "ended", Name: "ended", Ended: true})
	if err := f.db.Tx(func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "far", Path: "/other"}); err != nil {
			return err
		}
		for _, s := range []store.Session{
			{ID: "peer", Workspace: f.workspace.ID},
			{ID: "peer-child", Parent: "peer", Name: "child", Workspace: f.workspace.ID},
			{ID: "far", Workspace: "far"},
			{ID: "far-child", Parent: "far", Name: "child", Workspace: "far"},
			{ID: "stopped", Workspace: "far"},
			{ID: "stopped-child", Parent: "stopped", Name: "child", Workspace: "far"},
		} {
			if err := tx.AddSession(s); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ids := []string{"s", "own-child", "sibling", "ended", "peer", "peer-child", "far", "far-child", "stopped", "stopped-child"}
	for _, c := range []struct {
		from, to, text, want, failure string
	}{
		{"s", "child", "hello", "own-child", ""},
		{"own-child", "s", "hello", "s", ""},
		{"s", "peer", "hello", "peer", ""},
		{"peer", "s", "hello", "s", ""},
		{"s", "far", "hello", "far", ""},
		{"far", "s", "hello", "s", ""},
		{"s", "s", "no", "", "yourself"},
		{"own-child", "sibling", "no", "", "not found"},
		{"own-child", "peer", "no", "", "not found"},
		{"own-child", "far", "no", "", "not found"},
		{"own-child", "peer-child", "no", "", "not found"},
		{"own-child", "far-child", "no", "", "not found"},
		{"s", "own-child", "no", "", "not found"},
		{"s", "peer-child", "no", "", "not found"},
		{"s", "far-child", "no", "", "not found"},
		{"s", "stopped", "hello", "stopped", ""},
		{"stopped", "child", "hello", "stopped-child", ""},
		{"stopped-child", "stopped", "hello", "stopped", ""},
		{"s", "ended", "no", "", "ended session"},
		{"s", "missing", "no", "", "not found"},
		{"s", "child", " \n", "", "text is required"},
	} {
		t.Run(c.from+" to "+c.to+" "+c.failure, func(t *testing.T) {
			before := map[string]int{}
			for _, id := range ids {
				before[id] = len(f.queued(id))
			}
			recipient, err := f.rt.SendMessage(c.from, c.to, c.text)
			if c.failure != "" {
				if err == nil || !strings.Contains(err.Error(), c.failure) {
					t.Fatalf("SendMessage: %+v %v, want %q", recipient, err, c.failure)
				}
			} else if err != nil || recipient.ID != c.want {
				t.Fatalf("SendMessage: %+v %v, want %s", recipient, err, c.want)
			}
			for _, id := range ids {
				want := before[id]
				if id == c.want {
					want++
				}
				if got := len(f.queued(id)); got != want {
					t.Errorf("%s queued %d messages, want %d", id, got, want)
				}
			}
		})
	}
}

func TestSendMessageAcrossWorkspaces(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	outside, err := f.rt.AddWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	peer, err := f.rt.Create(outside.ID, lead, false, "start")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.SendMessage(parent.ID, peer.ID, "across workspaces"); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("Message from %s (%q, workspace %s):\nacross workspaces", parent.ID, parent.Title, f.workspace.Path)
	f.until("cross-workspace delivery with the sender's identity", func() bool {
		return strings.Contains(strings.Join(f.inputs(peer.ID), "\n"), want) && len(f.queued(peer.ID)) == 0
	})
	// A message to a parked session wakes it, resuming its conversation.
	f.park(peer.ID)
	if _, err := f.rt.SendMessage(parent.ID, peer.ID, "after parking"); err != nil {
		t.Fatal(err)
	}
	f.until("the parked session to wake and take the message", func() bool {
		return strings.Contains(strings.Join(f.inputs(peer.ID), "\n"), "after parking") && len(f.queued(peer.ID)) == 0
	})
	if starts := f.log(peer.ID, "start"); len(starts) != 2 || starts[1].Conversation != peer.ID+"-conversation" {
		t.Fatalf("starts = %v", starts)
	}
}
