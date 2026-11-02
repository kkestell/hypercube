package session

import (
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestEventsWithoutStateKeepTheAgentWorking(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	f.input(parent.ID, "!work")
	f.until("the parent working", func() bool { return f.storedSession(parent.ID).Activity == store.Busy })
	conversation := f.storedSession(parent.ID).Conversation
	f.input(parent.ID, "!usage")
	f.until("the usage event", func() bool {
		return f.storedSession(parent.ID).Usage == (harness.Usage{Context: 45200, Window: 200000, Cost: 0.31})
	})
	if a := f.storedSession(parent.ID); a.Activity != store.Busy || a.Conversation != conversation {
		t.Fatalf("an event without state or conversation changed the agent: %+v", a)
	}
}

func TestIdleParentAndFinishedChildDoNotMarkTheSessionUnseen(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	if err := f.rt.Seen(s.ID); err != nil {
		t.Fatal(err)
	}
	// The image arrives with an idle event, while the parent is already idle.
	f.input(s.ID, "!image")
	f.until("the image", func() bool { return len(f.transcript(s.ID)) == 4 })
	if f.storedSession(s.ID).Unseen {
		t.Fatal("a session that was already idle marked its session unseen")
	}
	child := f.child(s, "worker", "")
	f.input(child.ID, "!work")
	f.until("the child working", func() bool { return f.storedSession(child.ID).Activity == store.Busy })
	f.input(child.ID, "!finish")
	f.until("the child idle", func() bool { return f.storedSession(child.ID).Activity == store.Idle })
	if f.storedSession(s.ID).Unseen {
		t.Fatal("a child's finished turn marked its session unseen")
	}
}
