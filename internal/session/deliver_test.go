package session

import (
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestSessionStartsAndDeliversInOrder(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	if got := f.storedSession(s.ID); got.AwaitingUser || !got.Running {
		t.Fatalf("new session: %+v", got)
	}
	// The plugin starts in the lead's directory with the team's instructions.
	start := f.log(lead.ID, "start")
	if len(start) != 1 || start[0].Dir != f.root || start[0].Role != "lead" ||
		!strings.Contains(start[0].Instructions, "You are "+lead.Address()) ||
		start[0].Env["HYPERCUBE_AGENT_ID"] != lead.ID {
		t.Fatalf("start = %v", start)
	}
	if got := f.agent(lead.ID).Conversation; got != lead.ID+"-conversation" {
		t.Fatalf("conversation = %q", got)
	}
	// Each queued message goes out once the plugin accepts the one before it,
	// and leaves the queue when it does.
	f.queue("user", lead.ID, "first")
	f.queue("user", lead.ID, "second")
	f.queue("user", lead.ID, "third")
	f.until("three deliveries", func() bool { return len(f.inputs(lead.ID)) == 4 && len(f.queued(lead.ID)) == 0 })
	want := "start|Message from user:\nfirst|Message from user:\nsecond|Message from user:\nthird"
	if got := strings.Join(f.inputs(lead.ID), "|"); got != want {
		t.Fatalf("inputs = %q", got)
	}
	f.until("the lead idle", func() bool { return f.agent(lead.ID).Activity == store.Idle && len(f.transcript(lead.ID)) == 8 })
	// The plugin's replies and the next delivery race, so a reply can land
	// after the next message's item. The messages keep their order.
	var users []string
	replies := 0
	for _, it := range f.transcript(lead.ID) {
		if it.Kind == harness.KindUser {
			users = append(users, strings.TrimPrefix(it.Text, "Message from user:\n"))
		} else {
			replies++
		}
	}
	if strings.Join(users, ",") != "start,first,second,third" || replies != 4 {
		t.Fatalf("transcript = %q", f.summary(lead.ID))
	}
	// Starting a running session changes nothing.
	if err := f.rt.Start(s.ID); err != nil || len(f.log(lead.ID, "start")) != 1 {
		t.Fatalf("start of a running session: %v", err)
	}
}

func TestRefusedMessageStaysQueued(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	refusals := func() (n int) {
		for _, it := range f.transcript(lead.ID) {
			if it.Kind == harness.KindError && strings.Contains(it.Text, "rejected") {
				n++
			}
		}
		return n
	}
	f.queue("user", lead.ID, "!reject this", "after")
	f.until("a refusal", func() bool { return refusals() == 1 })
	// The next attempt sends the refused message again, not the one after it.
	f.rt.Deliver(lead.ID)
	f.until("a second refusal", func() bool { return refusals() == 2 })
	want := "start|Message from user:\n!reject this|Message from user:\n!reject this"
	if got := strings.Join(f.inputs(lead.ID), "|"); got != want || len(f.queued(lead.ID)) != 2 {
		t.Fatalf("after two refusals: inputs %q, queue %+v", got, f.queued(lead.ID))
	}
	// Dropping the bad message lets the next delivery go through.
	if err := f.db.Tx(func(tx *store.Tx) error { return tx.Dequeue(f.queued(lead.ID)[0].ID) }); err != nil {
		t.Fatal(err)
	}
	f.rt.Deliver(lead.ID)
	f.until("the next message", func() bool { return len(f.queued(lead.ID)) == 0 })
}

func TestMessagesGoOutWhileTheAgentWorks(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "!work")
	f.until("a working lead", func() bool { return f.agent(lead.ID).Activity == store.Busy })
	f.queue("user", lead.ID, "change course")
	f.until("steering", func() bool { return len(f.inputs(lead.ID)) == 3 && len(f.queued(lead.ID)) == 0 })
	f.input(lead.ID, "!finish")
	f.until("an idle lead", func() bool { return f.agent(lead.ID).Activity == store.Idle })
	// A lead's finished turn is a reply for the user to see.
	if !f.storedSession(lead.Session).Unseen {
		t.Fatal("the lead's reply is not marked unseen")
	}
}
