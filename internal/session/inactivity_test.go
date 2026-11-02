package session

import (
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// stopsWhenIdleAt runs the idle check at now until the session stops. A
// delivery still running when the agent turns idle keeps the session for the
// moment, so the check repeats.
func (f *fixture) stopsWhenIdleAt(session string, now time.Time) {
	f.t.Helper()
	f.until("the idle session to stop", func() bool {
		f.rt.stopIdleSessions(now)
		return !f.storedSession(session).Running
	})
}

func TestIdleSessionStopsAndResumes(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.input(s.ID, "hello")
	f.until("the reply", func() bool { return len(f.transcript(s.ID)) == 4 && f.storedSession(s.ID).Activity == store.Idle })
	if err := f.db.Tx(func(tx *store.Tx) error {
		return tx.Queue(store.Message{From: "user", To: s.ID, Text: "saved for later"})
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.UnixMilli(f.storedSession(s.ID).LastActive).Add(idleTimeout)
	f.rt.stopIdleSessions(deadline.Add(-time.Millisecond))
	if !f.storedSession(s.ID).Running {
		t.Fatal("session stopped before ten minutes")
	}
	f.stopsWhenIdleAt(s.ID, deadline)
	if f.storedSession(s.ID).Activity != store.Exited || len(f.log(s.ID, "shutdown")) != 1 {
		t.Fatal("idle session did not stop its agent")
	}
	if len(f.transcript(s.ID)) != 4 || len(f.queued(s.ID)) != 1 {
		t.Fatal("idle stop lost the transcript or queued message")
	}
	if err := f.rt.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	f.until("the resumed session", func() bool { return f.storedSession(s.ID).Activity == store.Idle })
	if !f.storedSession(s.ID).Running || !f.storedSession(s.ID).AwaitingUser || len(f.queued(s.ID)) != 1 {
		t.Fatal("idle session did not resume with its messages held")
	}
	start := f.log(s.ID, "start")
	if len(start) != 2 || start[1].Conversation != s.ID+"-conversation" {
		t.Fatal("idle session did not resume its conversation")
	}
}

func TestBusyChildKeepsSessionRunning(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	child, err := f.rt.CreateChild(s.ID, "worker", harness.Profile{Harness: "test", Model: "m"}, ".", "")
	if err != nil {
		t.Fatal(err)
	}
	f.input(child.ID, "!work")
	f.until("a busy child", func() bool { return f.storedSession(child.ID).Activity == store.Busy })
	f.rt.stopIdleSessions(time.Now().Add(2 * idleTimeout))
	if !f.storedSession(s.ID).Running {
		t.Fatal("an idle parent stopped while its child was busy")
	}
	f.input(child.ID, "!finish")
	f.until("an idle child", func() bool { return f.storedSession(child.ID).Activity == store.Idle })
	deadline := time.UnixMilli(f.storedSession(s.ID).LastActive).Add(idleTimeout)
	f.rt.stopIdleSessions(deadline.Add(-time.Millisecond))
	if !f.storedSession(s.ID).Running {
		t.Fatal("session did not receive ten minutes after the child finished")
	}
	f.stopsWhenIdleAt(s.ID, deadline)
	if f.storedSession(child.ID).Activity != store.Exited {
		t.Fatal("session did not stop after the child became idle")
	}
}

func TestViewedSessionKeepsRunningUntilLastPageLeaves(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	first := f.rt.ViewSession(s.ID)
	second := f.rt.ViewSession(s.ID)
	expired := time.Now().Add(2 * idleTimeout)
	f.rt.stopIdleSessions(expired)
	if !f.storedSession(s.ID).Running {
		t.Fatal("selected session stopped after ten minutes")
	}
	first()
	f.rt.stopIdleSessions(expired)
	if !f.storedSession(s.ID).Running {
		t.Fatal("session stopped while another page still selected it")
	}
	second()
	f.stopsWhenIdleAt(s.ID, expired)
}

func TestPromptRefreshesIdleDeadlineButSeenDoesNot(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	old := time.Now().Add(-idleTimeout).UnixMilli()
	if err := f.db.Tx(func(tx *store.Tx) error { return tx.TouchSession(s.ID, old) }); err != nil {
		t.Fatal(err)
	}
	f.input(s.ID, "hello")
	f.until("the reply", func() bool { return f.storedSession(s.ID).Activity == store.Idle && len(f.transcript(s.ID)) == 4 })
	active := f.storedSession(s.ID).LastActive
	if active <= old {
		t.Fatal("prompt and reply did not refresh activity")
	}
	if err := f.rt.Seen(s.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Rename(s.ID, "Another title"); err != nil {
		t.Fatal(err)
	}
	if f.storedSession(s.ID).LastActive != active {
		t.Fatal("viewing or renaming refreshed activity")
	}
	f.rt.stopIdleSessions(time.UnixMilli(old).Add(idleTimeout))
	if !f.storedSession(s.ID).Running {
		t.Fatal("session stopped against its old deadline")
	}
}

func TestViewedChildKeepsItsFamilyRunning(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "child", "")
	release := f.rt.ViewSession(child.ID)
	expired := time.Now().Add(2 * idleTimeout)
	f.rt.stopIdleSessions(expired)
	if !f.storedSession(parent.ID).Running {
		t.Fatal("viewed child did not hold family")
	}
	release()
	f.stopsWhenIdleAt(parent.ID, expired)
}
