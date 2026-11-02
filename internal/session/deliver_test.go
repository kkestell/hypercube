package session

import (
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestSessionStartsAndDeliversInOrder(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	if got := f.storedSession(s.ID); got.AwaitingUser || !got.Running {
		t.Fatalf("new session: %+v", got)
	}
	// The plugin starts in the parent's directory with its instructions.
	start := f.log(s.ID, "start")
	if len(start) != 1 || start[0].Dir != f.root ||
		!strings.Contains(start[0].Instructions, "Hypercube session "+s.ID) ||
		start[0].Env["HYPERCUBE_SESSION_ID"] != s.ID {
		t.Fatalf("start = %v", start)
	}
	if got := f.storedSession(s.ID).Conversation; got != s.ID+"-conversation" {
		t.Fatalf("conversation = %q", got)
	}
	// Each queued message goes out once the plugin accepts the one before it,
	// and leaves the queue when it does.
	f.queue("user", s.ID, "first")
	f.queue("user", s.ID, "second")
	f.queue("user", s.ID, "third")
	f.until("three deliveries", func() bool { return len(f.inputs(s.ID)) == 4 && len(f.queued(s.ID)) == 0 })
	want := "start|Message from user:\nfirst|Message from user:\nsecond|Message from user:\nthird"
	if got := strings.Join(f.inputs(s.ID), "|"); got != want {
		t.Fatalf("inputs = %q", got)
	}
	f.until("the session idle", func() bool { return f.storedSession(s.ID).Activity == store.Idle && len(f.transcript(s.ID)) == 8 })
	// The plugin's replies and the next delivery race, so a reply can land
	// after the next message's item. The messages keep their order.
	var users []string
	replies := 0
	for _, it := range f.transcript(s.ID) {
		if it.Kind == harness.KindUser {
			users = append(users, strings.TrimPrefix(it.Text, "Message from user:\n"))
		} else {
			replies++
		}
	}
	if strings.Join(users, ",") != "start,first,second,third" || replies != 4 {
		t.Fatalf("transcript = %q", f.summary(s.ID))
	}
	// Starting a running session changes nothing.
	if err := f.rt.Start(s.ID); err != nil || len(f.log(s.ID, "start")) != 1 {
		t.Fatalf("start of a running session: %v", err)
	}
}

func TestRefusedMessageStaysQueued(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	refusals := func() (n int) {
		for _, it := range f.transcript(parent.ID) {
			if it.Kind == harness.KindError && strings.Contains(it.Text, "rejected") {
				n++
			}
		}
		return n
	}
	f.queue("user", parent.ID, "!reject this", "after")
	f.until("a refusal", func() bool { return refusals() == 1 })
	// The next attempt sends the refused message again, not the one after it.
	f.rt.Deliver(parent.ID)
	f.until("a second refusal", func() bool { return refusals() == 2 })
	want := "start|Message from user:\n!reject this|Message from user:\n!reject this"
	if got := strings.Join(f.inputs(parent.ID), "|"); got != want || len(f.queued(parent.ID)) != 2 {
		t.Fatalf("after two refusals: inputs %q, queue %+v", got, f.queued(parent.ID))
	}
	// Dropping the bad message lets the next delivery go through.
	if err := f.db.Tx(func(tx *store.Tx) error { return tx.Dequeue(f.queued(parent.ID)[0].ID) }); err != nil {
		t.Fatal(err)
	}
	f.rt.Deliver(parent.ID)
	f.until("the next message", func() bool { return len(f.queued(parent.ID)) == 0 })
}

func TestMessagesGoOutWhileTheAgentWorks(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	f.input(parent.ID, "!work")
	f.until("a working session", func() bool { return f.storedSession(parent.ID).Activity == store.Busy })
	f.queue("user", parent.ID, "change course")
	f.until("steering", func() bool { return len(f.inputs(parent.ID)) == 3 && len(f.queued(parent.ID)) == 0 })
	f.input(parent.ID, "!finish")
	f.until("an idle session", func() bool { return f.storedSession(parent.ID).Activity == store.Idle })
	// A parent's finished turn is a reply for the user to see.
	if !f.storedSession(parent.ID).Unseen {
		t.Fatal("the session's reply is not marked unseen")
	}
}
