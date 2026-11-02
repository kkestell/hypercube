package session

import (
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
	s := f.session()
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	if err := f.rt.Rename(s.ID, "Changed"); err != nil {
		t.Fatal(err)
	}
	change := nextChange(t, changes, func(Change) bool { return true })
	if change.Session != "" || f.storedSession(s.ID).Title != "Changed" {
		t.Fatalf("rename change = %+v", change)
	}
}

func TestFailedWritePublishesNothing(t *testing.T) {
	f := newFixture(t)
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	if err := f.rt.Rename("missing", "Title"); err == nil {
		t.Fatal("renamed a missing session")
	}
	// A write publishes before it returns, so its change would be waiting.
	select {
	case change := <-changes:
		t.Fatalf("failed write published %+v", change)
	default:
	}
}

func TestStreamedChangesKeepTheirPositionAndText(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	f.input(s.ID, "!stream")
	streamed := func(c Change) bool { return c.Session == s.ID && strings.HasSuffix(c.Item.ID, ":s") }
	first := nextChange(t, changes, streamed)
	second := nextChange(t, changes, streamed)
	last := nextChange(t, changes, streamed)
	if first.Session != s.ID || first.Seq != second.Seq || second.Seq != last.Seq ||
		first.Item.Text != "Hel" || second.Item.Text != "Hello" || last.Item.Text != "Hello!" ||
		first.Item.Done || second.Item.Done || !last.Item.Done || first.Item.At == 0 || first.Item.At != last.Item.At {
		t.Fatalf("stream changes: %+v, %+v, %+v", first, second, last)
	}
	items, err := f.rt.Transcript(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	var saved TranscriptItem
	for _, it := range items {
		if it.ID == last.Item.ID {
			saved = it
		}
	}
	if saved.Seq != last.Seq || saved.Text != "Hello!" {
		t.Fatalf("saved item = %+v", saved)
	}
}

func TestDeliveredMessagePublishesItsItem(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	child := f.child(s, "worker", "")
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	if _, _, err := f.rt.SendMessage(child.ID, s.ID, "message"); err != nil {
		t.Fatal(err)
	}
	change := nextChange(t, changes, func(c Change) bool { return c.Session == s.ID && c.Item.Kind == harness.KindUser })
	if change.Session != s.ID || change.Seq <= 2 || change.Item.Text != "message" || change.Item.From != child.ID || !change.Item.Done {
		t.Fatalf("delivered message = %+v", change)
	}
}

func TestEndedLaunchPublishesDrainedItemsAndNote(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	f.input(parent.ID, "!work")
	f.until("a pending item", func() bool { return len(f.transcript(parent.ID)) == 4 })
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	f.input(parent.ID, "!crash")
	drained := nextChange(t, changes, func(c Change) bool { return c.Session == parent.ID && c.Item.Kind == harness.KindText && c.Item.Done })
	note := nextChange(t, changes, func(c Change) bool { return c.Session == parent.ID && c.Item.Kind == harness.KindError })
	if drained.Item.Text != "working" || note.Seq <= drained.Seq || !strings.HasPrefix(note.Item.Text, "The agent exited: ") {
		t.Fatalf("end changes = %+v, %+v", drained, note)
	}
}

func TestStoppedSessionPublishesDrainedItems(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.input(s.ID, "!work")
	f.until("a pending item", func() bool { return len(f.transcript(s.ID)) == 4 })
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	if err := f.rt.Stop(s.ID); err != nil {
		t.Fatal(err)
	}
	change := nextChange(t, changes, func(c Change) bool { return c.Session == s.ID })
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
	r.feed.publish(Change{Session: "current"})
	if change := <-current; change.Session != "current" {
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

// A completed item reported again keeps its saved position, and its change
// says so.
func TestReplacedItemKeepsItsPublishedPosition(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	f.input(parent.ID, "!work")
	f.until("the working item", func() bool {
		for _, item := range f.transcript(parent.ID) {
			if item.Text == "working" && !item.Done {
				return true
			}
		}
		return false
	})
	f.input(parent.ID, "!finish")
	f.until("the finished item", func() bool {
		return f.summary(parent.ID) == "user:start|text:ack|user:!work|text:finished|user:!finish"
	})
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	f.input(parent.ID, "!finish")
	change := nextChange(t, changes, func(c Change) bool { return c.Session == parent.ID && c.Item.Text == "finished" })
	items, err := f.rt.Transcript(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ID == change.Item.ID && it.Seq != change.Seq {
			t.Fatalf("published position %d, saved position %d", change.Seq, it.Seq)
		}
	}
}
