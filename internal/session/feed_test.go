package session

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

func nextChange(t *testing.T, changes <-chan Change, match func(Change) bool) Change {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case change, ok := <-changes:
			if !ok {
				t.Fatal("subscription closed")
			}
			if match(change) {
				return change
			}
		case <-timer.C:
			t.Fatal("timed out waiting for a change")
		}
	}
}

func TestWritePublishesAfterCommit(t *testing.T) {
	f := newFixture(t)
	s, _ := f.session()
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	if err := f.rt.Rename(s.ID, "Changed"); err != nil {
		t.Fatal(err)
	}
	change := nextChange(t, changes, func(Change) bool { return true })
	if change.Agent != "" || f.session1(s.ID).Title != "Changed" {
		t.Fatalf("rename change = %+v", change)
	}
}

func TestFailedWritePublishesNothing(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	f.rt.mu.Lock()
	l := f.rt.live[lead.ID]
	want := errors.New("rollback")
	err := f.rt.save(func(w *write) error {
		if err := w.putItem(l, f.rt.nextSeq(lead.ID), harness.Item{ID: "rolled-back", Kind: harness.KindText, Text: "lost", Done: true}); err != nil {
			return err
		}
		s.Title = "Lost"
		if err := w.SaveSession(s); err != nil {
			return err
		}
		return want
	})
	f.rt.mu.Unlock()
	if !errors.Is(err, want) || f.session1(s.ID).Title == "Lost" || len(f.transcript(lead.ID)) != 0 {
		t.Fatalf("failed write: %v", err)
	}
	select {
	case change := <-changes:
		t.Fatalf("failed transaction published %+v", change)
	default:
	}
}

func TestStreamedChangesKeepTheirPositionAndText(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	f.rt.mu.Lock()
	l := f.rt.live[lead.ID]
	f.rt.mu.Unlock()
	itemChange := func(c Change) bool { return c.Agent == lead.ID }
	f.rt.event(l, harness.Event{Items: []harness.Item{{ID: "stream", Kind: harness.KindText, Text: "Hel"}}})
	first := nextChange(t, changes, itemChange)
	f.rt.event(l, harness.Event{Items: []harness.Item{{ID: "stream", Kind: harness.KindText, Append: true, Text: "lo"}}})
	second := nextChange(t, changes, itemChange)
	f.rt.event(l, harness.Event{Items: []harness.Item{{ID: "stream", Kind: harness.KindText, Append: true, Text: "!", Done: true}}})
	last := nextChange(t, changes, itemChange)
	if first.Session != s.ID || first.Seq <= 2 || first.Seq != second.Seq || second.Seq != last.Seq ||
		first.Item.Text != "Hel" || second.Item.Text != "Hello" || last.Item.Text != "Hello!" ||
		first.Item.Done || second.Item.Done || !last.Item.Done || first.Item.At == 0 || first.Item.At != last.Item.At {
		t.Fatalf("stream changes: %+v, %+v, %+v", first, second, last)
	}
	items, err := f.rt.Transcript(lead.ID)
	if err != nil || items[2].Seq != first.Seq || items[2].Text != last.Item.Text {
		t.Fatalf("transcript = %+v, %v", items, err)
	}
}

func TestDeliveredMessagePublishesItsItem(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	worker := f.addWorker(lead, "worker", "")
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	if _, _, err := f.rt.SendMessage(worker.ID, "lead", "message"); err != nil {
		t.Fatal(err)
	}
	change := nextChange(t, changes, func(c Change) bool { return c.Agent == lead.ID && c.Item.Kind == harness.KindUser })
	if change.Session != s.ID || change.Seq <= 2 || change.Item.Text != "message" || change.Item.From != worker.Address() || !change.Item.Done {
		t.Fatalf("delivered message = %+v", change)
	}
}

func TestEndedLaunchPublishesDrainedItemsAndNote(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "!work")
	f.until("a pending item", func() bool { return len(f.transcript(lead.ID)) == 2 })
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	f.input(lead.ID, "!crash")
	drained := nextChange(t, changes, func(c Change) bool { return c.Agent == lead.ID && c.Item.Kind == harness.KindText && c.Item.Done })
	note := nextChange(t, changes, func(c Change) bool { return c.Agent == lead.ID && c.Item.Kind == harness.KindError })
	if drained.Item.Text != "working" || note.Seq <= drained.Seq || !strings.HasPrefix(note.Item.Text, "The agent ended: ") {
		t.Fatalf("end changes = %+v, %+v", drained, note)
	}
}

func TestStoppedSessionPublishesDrainedItems(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	f.input(lead.ID, "!work")
	f.until("a pending item", func() bool { return len(f.transcript(lead.ID)) == 2 })
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	if err := f.rt.Stop(s.ID); err != nil {
		t.Fatal(err)
	}
	change := nextChange(t, changes, func(c Change) bool { return c.Agent == lead.ID })
	if change.Session != s.ID || change.Item.Text != "working" || !change.Item.Done {
		t.Fatalf("stopped item = %+v", change)
	}
}

func TestSlowSubscriberIsDropped(t *testing.T) {
	r := &Runtime{}
	changes, cancel := r.Subscribe()
	defer cancel()
	for range 1025 {
		r.feed.publish(Change{})
	}
	var count int
	for range changes {
		count++
	}
	if count != 1024 {
		t.Fatalf("buffered changes = %d", count)
	}
	current, stop := r.Subscribe()
	defer stop()
	r.feed.publish(Change{Agent: "current"})
	if change := <-current; change.Agent != "current" {
		t.Fatalf("new subscription = %+v", change)
	}
}

func TestCancelStopsDelivery(t *testing.T) {
	r := &Runtime{}
	changes, cancel := r.Subscribe()
	cancel()
	cancel()
	r.feed.publish(Change{})
	if change, ok := <-changes; ok {
		t.Fatalf("canceled subscription received %+v", change)
	}
}
