package session

import (
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestApplyEvent(t *testing.T) {
	worker := store.Agent{Role: store.Worker, Activity: store.Busy}
	lead := store.Agent{Role: store.Lead, Activity: store.Idle}
	var s store.Session
	applyEvent(&lead, &s, harness.Event{State: harness.Working, Conversation: "c1"})
	if lead.Activity != store.Busy || lead.Conversation != "c1" || s.Unseen {
		t.Fatalf("working: %+v %+v", lead, s)
	}
	applyEvent(&lead, &s, harness.Event{Items: []harness.Item{{ID: "x", Kind: harness.KindText}}})
	if lead.Activity != store.Busy || lead.Conversation != "c1" {
		t.Fatalf("an event without state or conversation changed the agent: %+v", lead)
	}
	applyEvent(&lead, &s, harness.Event{State: harness.Idle, Usage: &harness.Usage{Context: 45200, Window: 200000, Cost: 0.31}})
	if lead.Activity != store.Idle || !s.Unseen || lead.Usage != (harness.Usage{Context: 45200, Window: 200000, Cost: 0.31}) {
		t.Fatalf("lead idle: %+v %+v", lead, s)
	}
	s.Unseen = false
	applyEvent(&lead, &s, harness.Event{State: harness.Idle})
	if s.Unseen {
		t.Fatal("a lead that was already idle marked its session unseen")
	}
	applyEvent(&worker, &s, harness.Event{State: harness.Idle})
	if worker.Activity != store.Idle || s.Unseen {
		t.Fatalf("worker idle: %+v %+v", worker, s)
	}
}
