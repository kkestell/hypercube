package session

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

// offerItem returns the saved offer in the session's transcript.
func (f *fixture) offerItem(session, id string) harness.Item {
	f.t.Helper()
	for _, it := range f.transcript(session) {
		if it.ID == id {
			return it
		}
	}
	f.t.Fatalf("no offer %s", id)
	return harness.Item{}
}

func TestOfferAnswers(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	options := func(title string) ChildOptions {
		return ChildOptions{Name: "task", Title: title, Settings: lead, Message: "the brief"}
	}

	here, err := f.rt.OfferChild(parent.ID, "Here")
	if err != nil {
		t.Fatal(err)
	}
	if it := f.offerItem(parent.ID, here); it.Done || it.Kind != harness.KindAttention || it.Text != "Here" {
		t.Fatalf("offer = %+v", it)
	}
	if _, err := f.rt.StartOffer(parent.ID, here, options("Here")); err == nil {
		t.Fatal("an unanswered offer started a child")
	}
	if err := f.rt.Answer(parent.ID, here, nil, "typed"); err == nil {
		t.Fatal("an offer took typed text")
	}
	if err := f.rt.Answer(parent.ID, here, []string{offerHere}, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Answer(parent.ID, here, []string{offerNew}, ""); err == nil {
		t.Fatal("an offer was answered twice")
	}
	f.until("the Do it here answer", func() bool { return len(f.inputs(parent.ID)) == 2 && f.idle(parent.ID) })
	if got := f.inputs(parent.ID)[1]; !strings.Contains(got, "Do it here") || strings.Contains(got, here) {
		t.Fatal(got)
	}
	if _, err := f.rt.StartOffer(parent.ID, here, options("Here")); err == nil {
		t.Fatal("Do it here started a child")
	}

	accepted, err := f.rt.OfferChild(parent.ID, "Task")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Answer(parent.ID, accepted, []string{offerNew}, ""); err != nil {
		t.Fatal(err)
	}
	f.until("the New session answer", func() bool { return len(f.inputs(parent.ID)) == 3 && f.idle(parent.ID) })
	if got := f.inputs(parent.ID)[2]; !strings.Contains(got, accepted) {
		t.Fatal(got)
	}
	other := f.session()
	if _, err := f.rt.StartOffer(other.ID, accepted, options("Task")); err == nil {
		t.Fatal("another session used the offer")
	}
	if _, err := f.rt.StartOffer(parent.ID, accepted, options("Another title")); err == nil {
		t.Fatal("an offer started a child with another title")
	}
	forked := options("Task")
	forked.Fork = true
	if _, err := f.rt.StartOffer(parent.ID, accepted, forked); err == nil {
		t.Fatal("an offer started a fork")
	}
	// A start that fails and is cleaned up leaves the offer usable.
	missing := options("Task")
	missing.Settings.Harness = "missing"
	if _, err := f.rt.StartOffer(parent.ID, accepted, missing); err == nil {
		t.Fatal("a child started without its plugin")
	}
	child, err := f.rt.StartOffer(parent.ID, accepted, options("Task"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.StartOffer(parent.ID, accepted, options("Task")); err == nil {
		t.Fatal("an offer started a second child")
	}
	if family := f.family(parent.ID); len(family) != 2 || family[1].ID != child.ID {
		t.Fatalf("family = %+v", family)
	}
	f.until("the brief", func() bool { return len(f.inputs(child.ID)) == 1 })
	if !strings.Contains(f.inputs(child.ID)[0], "the brief") {
		t.Fatal(f.inputs(child.ID))
	}
}

// Dismiss closes an offer without creating a child.
func TestDismissOffer(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	id, err := f.rt.OfferChild(parent.ID, "Task")
	if err != nil {
		t.Fatal(err)
	}
	if choices := f.offerItem(parent.ID, id).Choices; len(choices) != 3 || choices[2] != offerDismiss {
		t.Fatalf("offer choices = %v", choices)
	}
	if err := f.rt.Answer(parent.ID, id, []string{"Not a choice"}, ""); err == nil {
		t.Fatal("offer accepted an invalid choice")
	}
	if err := f.rt.Answer(parent.ID, id, []string{offerDismiss}, ""); err != nil {
		t.Fatal(err)
	}
	item := f.offerItem(parent.ID, id)
	if !item.Done || item.Answer != offerDismiss || len(f.requests()) != 0 {
		t.Fatalf("dismissed offer = %+v; requests = %+v", item, f.requests())
	}
	if _, err := f.rt.StartOffer(parent.ID, id, ChildOptions{Name: "task", Title: "Task", Settings: lead, Message: "brief"}); err == nil {
		t.Fatal("dismissed offer started a child")
	}
	if len(f.family(parent.ID)) != 1 {
		t.Fatal("dismissing an offer created a session")
	}
	f.until("the dismissal message", func() bool { return len(f.inputs(parent.ID)) == 2 && f.idle(parent.ID) })
	got := f.inputs(parent.ID)[1]
	if !strings.Contains(got, "The user dismissed") || !strings.Contains(got, "no work is authorized") || strings.Contains(got, "Do it here") || strings.Contains(got, id) {
		t.Fatal(got)
	}
	if err := f.rt.Answer(parent.ID, id, []string{offerNew}, ""); err == nil {
		t.Fatal("dismissed offer was answered again")
	}
}

// An unanswered offer stays open across interruption, parking, and backend
// reattachment, and its answer wakes the parked parent.
func TestOfferStaysOpen(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	id, err := f.rt.OfferChild(parent.ID, "Task")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Interrupt(parent.ID); err != nil {
		t.Fatal(err)
	}
	f.restart()
	f.until("the reattached parent", func() bool { return f.idle(parent.ID) })
	f.park(parent.ID)
	if it := f.offerItem(parent.ID, id); it.Done {
		t.Fatalf("offer closed: %+v", it)
	}
	if err := f.rt.Answer(parent.ID, id, []string{offerNew}, ""); err != nil {
		t.Fatal(err)
	}
	f.until("the answer after the restart", func() bool {
		inputs := f.inputs(parent.ID)
		return len(inputs) > 0 && strings.Contains(inputs[len(inputs)-1], id) && len(f.queued(parent.ID)) == 0
	})
}

func TestArchivedOfferWaitsForUnarchive(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	id, err := f.rt.OfferChild(parent.ID, "Task")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Archive(parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Answer(parent.ID, id, []string{offerNew}, ""); !errors.Is(err, errEnded) {
		t.Fatalf("answering an archived offer = %v", err)
	}
	if f.offerItem(parent.ID, id).Done || len(f.queued(parent.ID)) != 0 {
		t.Fatal("an archived offer took an answer")
	}
	if err := f.rt.Unarchive(parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Answer(parent.ID, id, []string{offerNew}, ""); err != nil {
		t.Fatal(err)
	}
	f.until("the answer delivered", func() bool { return len(f.inputs(parent.ID)) == 2 && len(f.queued(parent.ID)) == 0 })
}

func TestOfferRequestsSurviveParkingAndRestoration(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	id, err := f.rt.OfferChild(parent.ID, "Task")
	if err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		requests := f.requests()
		if len(requests) != 1 || requests[0].Session != parent.ID || requests[0].Item.ID != id {
			t.Fatalf("offer requests = %+v", requests)
		}
	}
	check()
	f.idleAfter(50 * time.Millisecond)
	f.view(parent.ID)()
	f.untilParked(parent.ID)
	check()
	f.restart()
	check()
	if f.awake(parent.ID) {
		t.Fatal("restoring an offer woke its parked agent")
	}
	if err := f.rt.Answer(parent.ID, id, []string{offerHere}, ""); err != nil {
		t.Fatal(err)
	}
	if requests := f.requests(); len(requests) != 0 {
		t.Fatalf("answered offer remained requested: %+v", requests)
	}
	f.until("the offer answer delivered", func() bool { return len(f.queued(parent.ID)) == 0 && f.idle(parent.ID) })
}

func TestArchivedOffersLeaveRequestsUntilUnarchived(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	id, err := f.rt.OfferChild(parent.ID, "Task")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Archive(parent.ID); err != nil {
		t.Fatal(err)
	}
	if requests := f.requests(); len(requests) != 0 {
		t.Fatalf("archived offer remained requested: %+v", requests)
	}
	if err := f.rt.Unarchive(parent.ID); err != nil {
		t.Fatal(err)
	}
	if requests := f.requests(); len(requests) != 1 || requests[0].Item.ID != id {
		t.Fatalf("unarchived offer requests = %+v", requests)
	}
}
