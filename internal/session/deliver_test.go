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
	f.queue("gone", s.ID, "first")
	f.queue("gone", s.ID, "second")
	f.queue("gone", s.ID, "third")
	f.until("three deliveries", func() bool { return len(f.inputs(s.ID)) == 4 && len(f.queued(s.ID)) == 0 })
	inputs := f.inputs(s.ID)
	for i, want := range []string{"start", "Message from gone:\nfirst", "Message from gone:\nsecond", "Message from gone:\nthird"} {
		if !strings.HasPrefix(inputs[i], want) {
			t.Fatalf("input %d = %q, want prefix %q", i, inputs[i], want)
		}
	}
	f.until("the session idle", func() bool { return f.storedSession(s.ID).Activity == store.Idle && len(f.transcript(s.ID)) == 8 })
	// The plugin's replies and the next delivery race, so a reply can land
	// after the next message's item. The messages keep their order.
	var users []string
	replies := 0
	for _, it := range f.transcript(s.ID) {
		if it.Kind == harness.KindUser {
			users = append(users, it.Text)
		} else {
			replies++
		}
	}
	if strings.Join(users, ",") != "start,first,second,third" || replies != 4 {
		t.Fatalf("transcript = %q", f.summary(s.ID))
	}
	// Waking an awake session changes nothing.
	if err := f.wake(s.ID); err != nil || len(f.log(s.ID, "start")) != 1 {
		t.Fatalf("wake of an awake session: %v", err)
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
	f.queue("gone", parent.ID, "!reject this", "after")
	f.until("a refusal", func() bool { return refusals() == 1 })
	// The next attempt sends the refused message again, not the one after it.
	f.rt.Deliver(parent.ID)
	f.until("a second refusal", func() bool { return refusals() == 2 })
	inputs := f.inputs(parent.ID)
	if len(inputs) != 3 || inputs[0] != "start" || !strings.HasPrefix(inputs[1], "Message from gone:\n!reject this") || inputs[1] != inputs[2] || len(f.queued(parent.ID)) != 2 {
		t.Fatalf("after two refusals: inputs %q, queue %+v", inputs, f.queued(parent.ID))
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
	f.queue("gone", parent.ID, "change course")
	f.until("steering", func() bool { return len(f.inputs(parent.ID)) == 3 && len(f.queued(parent.ID)) == 0 })
	f.input(parent.ID, "!finish")
	f.until("an idle session", func() bool { return f.storedSession(parent.ID).Activity == store.Idle })
	// A parent's finished turn is a reply for the user to see.
	if !f.storedSession(parent.ID).Unseen {
		t.Fatal("the session's reply is not marked unseen")
	}
}
