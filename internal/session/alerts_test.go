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
	f := newFixture(t)
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
			if err := f.rt.Answer(s.ID, it.ID, "yes", ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.input(s.ID, "!ping")
	if b := f.nextAlert(alerts); !strings.HasPrefix(b.Key, "reply:") || b.Text != "ack" {
		t.Fatalf("alert after the request = %+v", b)
	}
}

func TestReplyAlerts(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	child := f.child(s, "worker", "")
	f.until("the child idle", func() bool { return f.idle(child.ID) })
	alerts := f.watch()
	f.input(child.ID, "!work")
	f.until("the child working", func() bool { return !f.idle(child.ID) })
	f.input(child.ID, "!finish")
	a := f.nextAlert(alerts)
	if !strings.HasPrefix(a.Key, "reply:"+child.ID+":") || a.Session != child.ID || a.Text != "finished" ||
		a.Subtitle != filepath.Base(f.root)+" › "+f.storedSession(s.ID).Title {
		t.Fatalf("reply alert = %+v", a)
	}
}

// A page that marks the shown session seen as the turn ends does not hide the
// reply from the watcher.
func TestSeenRepliesStillAlert(t *testing.T) {
	f := newFixture(t)
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
	f := newFixture(t)
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
