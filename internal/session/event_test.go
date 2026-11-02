package session

import (
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestEventsWithoutStateKeepTheAgentWorking(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "!work")
	f.until("the lead working", func() bool { return f.agent(lead.ID).Activity == store.Busy })
	conversation := f.agent(lead.ID).Conversation
	f.input(lead.ID, "!usage")
	f.until("the usage event", func() bool {
		return f.agent(lead.ID).Usage == (harness.Usage{Context: 45200, Window: 200000, Cost: 0.31})
	})
	if a := f.agent(lead.ID); a.Activity != store.Busy || a.Conversation != conversation {
		t.Fatalf("an event without state or conversation changed the agent: %+v", a)
	}
}

func TestIdleLeadAndFinishedWorkerDoNotMarkTheSessionUnseen(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	if err := f.rt.Seen(s.ID); err != nil {
		t.Fatal(err)
	}
	// The image arrives with an idle event, while the lead is already idle.
	f.input(lead.ID, "!image")
	f.until("the image", func() bool { return len(f.transcript(lead.ID)) == 4 })
	if f.storedSession(s.ID).Unseen {
		t.Fatal("a lead that was already idle marked its session unseen")
	}
	worker := f.addWorker(lead, "worker", "")
	f.input(worker.ID, "!work")
	f.until("the worker working", func() bool { return f.agent(worker.ID).Activity == store.Busy })
	f.input(worker.ID, "!finish")
	f.until("the worker idle", func() bool { return f.agent(worker.ID).Activity == store.Idle })
	if f.storedSession(s.ID).Unseen {
		t.Fatal("a worker's finished turn marked its session unseen")
	}
}
