package session

import (
	"errors"
	"sync"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
)

func (f *fixture) proposal(parentID string, options ChildOptions) string {
	f.t.Helper()
	if _, err := f.rt.ProposeChild(parentID, options); err != nil {
		f.t.Fatal(err)
	}
	for _, item := range f.transcript(parentID) {
		if item.Kind == harness.KindProposal && !item.Done {
			return item.ID
		}
	}
	f.t.Fatal("proposal was not recorded")
	return ""
}

func TestProposalPersistsAndStartsOnlyOnce(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	id := f.proposal(parent.ID, ChildOptions{Name: "proposed", Title: "Proposed task", Settings: lead, Dir: ".", Message: "brief"})
	if len(f.family(parent.ID)) != 1 || len(f.requests()) != 0 {
		t.Fatal("proposal launched or blocked the parent")
	}
	f.park(parent.ID)
	if err := f.wake(parent.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := f.rt.StartProposal(parent.ID, id); errors <- err })
	}
	wg.Wait()
	close(errors)
	successes := 0
	for err := range errors {
		if err == nil {
			successes++
		}
	}
	if successes != 1 || len(f.family(parent.ID)) != 2 {
		t.Fatalf("successes %d, family %+v", successes, f.family(parent.ID))
	}
	if err := f.rt.DismissProposal(parent.ID, id); err == nil {
		t.Fatal("dismissed a started proposal")
	}
}

func TestProposalCanRetryAFailedLaunchOrBeDismissed(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	options := ChildOptions{Name: "retry", Title: "Retry", Settings: harness.Settings{Harness: "missing", Model: "m", Effort: "low"}, Dir: "."}
	id := f.proposal(parent.ID, options)
	if _, err := f.rt.StartProposal(parent.ID, id); err == nil {
		t.Fatal("missing plugin started")
	}
	if len(f.family(parent.ID)) != 1 {
		t.Fatal("failed child remains")
	}
	harnesstest.Install(t, f.plugins, "missing")
	child, err := f.rt.StartProposal(parent.ID, id)
	if err != nil || child.Name != options.Name {
		t.Fatalf("retry %+v: %v", child, err)
	}
	options.Name = "dismiss"
	id = f.proposal(parent.ID, options)
	if err := f.rt.DismissProposal(parent.ID, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.StartProposal(parent.ID, id); err == nil {
		t.Fatal("dismissed proposal started")
	}
	if len(f.family(parent.ID)) != 2 {
		t.Fatal("dismissal created a child")
	}
}

func TestForkRejectsADifferentHarness(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	_, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "fork", Title: "Fork", Settings: harness.Settings{Harness: "other", Model: "m"}, Fork: true})
	if err == nil || len(f.family(parent.ID)) != 1 {
		t.Fatalf("incompatible fork: %v", err)
	}
}

func TestArchivedProposalCannotBeDismissed(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	id := f.proposal(parent.ID, ChildOptions{Name: "proposed", Title: "Task", Settings: lead, Dir: "."})
	if err := f.rt.Archive(parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.DismissProposal(parent.ID, id); !errors.Is(err, errEnded) {
		t.Fatalf("dismissing an archived proposal = %v", err)
	}
	for _, item := range f.transcript(parent.ID) {
		if item.ID == id && (item.Done || item.Answer != "") {
			t.Fatalf("archived proposal changed: %+v", item)
		}
	}
	if err := f.rt.Unarchive(parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.DismissProposal(parent.ID, id); err != nil {
		t.Fatal(err)
	}
}
