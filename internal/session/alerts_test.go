package session

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// watch runs WatchAlerts until the test ends and returns the alerts it sends.
func (f *fixture) watch() <-chan Alert {
	f.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	alerts := make(chan Alert, 16)
	done := make(chan error, 1)
	go func() { done <- f.rt.WatchAlerts(ctx, func(a Alert) { alerts <- a }) }()
	f.t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			f.t.Error(err)
		}
	})
	return alerts
}

func (f *fixture) nextAlert(alerts <-chan Alert) Alert {
	f.t.Helper()
	select {
	case a := <-alerts:
		return a
	case <-time.After(10 * time.Second):
		f.t.Fatal("no alert")
		return Alert{}
	}
}

func TestRequestAlerts(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	alerts := f.watch()
	f.input(s.ID, "!ask")
	a := f.nextAlert(alerts)
	title := f.storedSession(s.ID).Title
	if !strings.HasPrefix(a.Key, "request:"+s.ID+":") || a.Session != s.ID || a.Title != title ||
		a.Subtitle != filepath.Base(f.root) || a.Text != "Proceed?" {
		t.Fatalf("request alert = %+v", a)
	}
	// Later changes do not repeat the request: the next alert is the reply.
	for _, it := range f.transcript(s.ID) {
		if it.Kind == "attention" {
			if err := f.rt.Answer(s.ID, it.ID, []string{"yes"}, ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.input(s.ID, "!ping")
	if b := f.nextAlert(alerts); !strings.HasPrefix(b.Key, "reply:") || b.Text != "ack" {
		t.Fatalf("alert after the request = %+v", b)
	}
}

// Only top-level sessions alert on replies; a child reports to its parent.
func TestReplyAlerts(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	child := f.child(s, "worker", "")
	f.until("the child idle", func() bool { return f.idle(child.ID) })
	alerts := f.watch()
	f.input(child.ID, "!work")
	f.until("the child working", func() bool { return !f.idle(child.ID) })
	f.input(child.ID, "!finish")
	f.until("the child idle again", func() bool { return f.idle(child.ID) })
	f.input(s.ID, "!work")
	f.until("the parent working", func() bool { return !f.idle(s.ID) })
	f.input(s.ID, "!finish")
	a := f.nextAlert(alerts)
	if !strings.HasPrefix(a.Key, "reply:"+s.ID+":") || a.Session != s.ID || a.Text != "finished" ||
		a.Subtitle != filepath.Base(f.root) {
		t.Fatalf("reply alert = %+v", a)
	}
}

// A page that marks the shown session seen as the turn ends does not hide the
// reply from the watcher.
func TestSeenRepliesStillAlert(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	w := newAlertWatch(f.rt)
	if err := w.read(nil); err != nil {
		t.Fatal(err)
	}
	var alerts []Alert
	read := func() {
		t.Helper()
		if err := w.read(func(a Alert) { alerts = append(alerts, a) }); err != nil {
			t.Fatal(err)
		}
	}
	f.input(s.ID, "!work")
	f.until("working", func() bool { return !f.idle(s.ID) })
	read()
	f.input(s.ID, "!finish")
	f.until("idle", func() bool { return f.idle(s.ID) })
	if err := f.rt.Seen(s.ID); err != nil {
		t.Fatal(err)
	}
	read()
	if len(alerts) != 1 || alerts[0].Text != "finished" {
		t.Fatalf("alerts = %+v", alerts)
	}
	read()
	if len(alerts) != 1 {
		t.Fatalf("a later read repeated the reply: %+v", alerts)
	}
}

func TestExistingRequestsAndRepliesAreNotAlerted(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	f.input(s.ID, "!ask")
	f.until("attention", func() bool { return f.storedSession(s.ID).Attention })
	other := f.session()
	if !f.storedSession(other.ID).Unseen {
		t.Fatal("the other session has no unseen reply to ignore")
	}
	w := newAlertWatch(f.rt)
	if err := w.read(nil); err != nil {
		t.Fatal(err)
	}
	if err := w.read(func(a Alert) { t.Errorf("alert for existing state: %+v", a) }); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherStartedDuringQuestionIgnoresEarlierReply(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	f.input(s.ID, "!ask")
	f.until("the question", func() bool { return f.storedSession(s.ID).Attention })
	w := newAlertWatch(f.rt)
	if err := w.read(nil); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Answer(s.ID, f.requests()[0].Item.ID, []string{"yes"}, ""); err != nil {
		t.Fatal(err)
	}
	f.until("the answer", func() bool { return !f.storedSession(s.ID).Attention && f.idle(s.ID) })
	var alerts []Alert
	notify := func(a Alert) { alerts = append(alerts, a) }
	if err := w.read(notify); err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 0 {
		t.Fatalf("alerts for the earlier reply = %+v", alerts)
	}
	f.input(s.ID, "next")
	f.until("the next reply", func() bool { return strings.HasSuffix(f.summary(s.ID), "user:next|text:ack") && f.idle(s.ID) })
	if err := w.read(notify); err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 || alerts[0].Text != "ack" {
		t.Fatalf("alerts after the new reply = %+v", alerts)
	}
}

func TestOfferRequestAlerts(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	w := newAlertWatch(f.rt)
	if err := w.read(nil); err != nil {
		t.Fatal(err)
	}
	id, err := f.rt.OfferChild(s.ID, "New task")
	if err != nil {
		t.Fatal(err)
	}
	f.park(s.ID)
	var alerts []Alert
	for range 2 {
		if err := w.read(func(a Alert) { alerts = append(alerts, a) }); err != nil {
			t.Fatal(err)
		}
	}
	if len(alerts) != 1 || alerts[0].Key != "request:"+s.ID+":"+id || alerts[0].Text != "New task" {
		t.Fatalf("offer alerts = %+v", alerts)
	}
}

func TestDeletedCapturedRequestDoesNotStopAlerts(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	w := newAlertWatch(f.rt)
	if err := w.read(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.OfferChild(s.ID, "Deleted task"); err != nil {
		t.Fatal(err)
	}
	requests := f.requests()
	if len(requests) != 1 {
		t.Fatalf("captured requests = %+v", requests)
	}
	// Deletion falls between the watcher's request capture and metadata read.
	if err := f.rt.Delete(s.ID); err != nil {
		t.Fatal(err)
	}
	var alerts []Alert
	notify := func(a Alert) { alerts = append(alerts, a) }
	if err := w.readStored(requests, notify); err != nil {
		t.Fatalf("reading a deleted request: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("alerts for the deleted session = %+v", alerts)
	}
	other := f.session()
	if err := w.read(nil); err != nil {
		t.Fatal(err)
	}
	id, err := f.rt.OfferChild(other.ID, "Another task")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.read(notify); err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 || alerts[0].Key != "request:"+other.ID+":"+id {
		t.Fatalf("alerts after deleting another session = %+v", alerts)
	}
}
