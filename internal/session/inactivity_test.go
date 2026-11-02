package session

import (
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func (f *fixture) settled(id string) {
	f.t.Helper()
	f.until("delivery to settle", func() bool {
		f.rt.mu.Lock()
		defer f.rt.mu.Unlock()
		l := f.rt.live[id]
		return l != nil && !l.starting && !l.delivering
	})
}

func TestIdleSessionStopsAndResumes(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	f.input(lead.ID, "hello")
	f.until("the reply", func() bool { return len(f.transcript(lead.ID)) == 2 && f.agent(lead.ID).Activity == store.Idle })
	f.settled(lead.ID)
	if err := f.db.Tx(func(tx *store.Tx) error {
		return tx.Queue(store.Message{From: "user", To: lead.ID, Text: "saved for later"})
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.UnixMilli(f.session1(s.ID).LastActive).Add(idleTimeout)
	f.rt.stopIdleSessions(deadline.Add(-time.Millisecond))
	if !f.session1(s.ID).Running {
		t.Fatal("session stopped before ten minutes")
	}
	f.rt.stopIdleSessions(deadline)
	if f.session1(s.ID).Running || f.agent(lead.ID).Activity != store.Exited || len(f.log(lead.ID, "shutdown")) != 1 {
		t.Fatal("idle session did not stop its agent")
	}
	if len(f.transcript(lead.ID)) != 2 || len(f.queued(lead.ID)) != 1 {
		t.Fatal("idle stop lost the transcript or queued message")
	}
	if err := f.rt.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	f.until("the resumed lead", func() bool { return f.agent(lead.ID).Activity == store.Idle })
	if !f.session1(s.ID).Running || !f.session1(s.ID).AwaitingUser || len(f.queued(lead.ID)) != 1 {
		t.Fatal("idle session did not resume with its messages held")
	}
	start := f.log(lead.ID, "start")
	if len(start) != 2 || start[1]["conversation"] != lead.ID+"-conversation" {
		t.Fatal("idle session did not resume its conversation")
	}
}

func TestBusyWorkerKeepsSessionRunning(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	worker, err := f.rt.AddWorker(lead.ID, "worker", harness.Profile{Harness: "test", Model: "m"}, ".", "")
	if err != nil {
		t.Fatal(err)
	}
	f.input(worker.ID, "!work")
	f.until("a busy worker", func() bool { return f.agent(worker.ID).Activity == store.Busy })
	f.rt.stopIdleSessions(time.Now().Add(2 * idleTimeout))
	if !f.session1(s.ID).Running {
		t.Fatal("idle lead caused its busy worker's session to stop")
	}
	f.input(worker.ID, "!finish")
	f.until("an idle worker", func() bool { return f.agent(worker.ID).Activity == store.Idle })
	f.settled(lead.ID)
	f.settled(worker.ID)
	deadline := time.UnixMilli(f.session1(s.ID).LastActive).Add(idleTimeout)
	f.rt.stopIdleSessions(deadline.Add(-time.Millisecond))
	if !f.session1(s.ID).Running {
		t.Fatal("session did not receive ten minutes after the worker finished")
	}
	f.rt.stopIdleSessions(deadline)
	if f.session1(s.ID).Running || f.agent(worker.ID).Activity != store.Exited {
		t.Fatal("session did not stop after the worker became idle")
	}
}

func TestViewedSessionKeepsRunningUntilLastPageLeaves(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	f.settled(lead.ID)
	first := f.rt.ViewSession(s.ID)
	second := f.rt.ViewSession(s.ID)
	expired := time.Now().Add(2 * idleTimeout)
	f.rt.stopIdleSessions(expired)
	if !f.session1(s.ID).Running {
		t.Fatal("selected session stopped after ten minutes")
	}
	first()
	f.rt.stopIdleSessions(expired)
	if !f.session1(s.ID).Running {
		t.Fatal("session stopped while another page still selected it")
	}
	second()
	f.rt.stopIdleSessions(expired)
	if f.session1(s.ID).Running {
		t.Fatal("expired session kept running after its last page left")
	}
}

func TestPromptRefreshesIdleDeadlineButSeenDoesNot(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	old := time.Now().Add(-idleTimeout).UnixMilli()
	if err := f.db.Tx(func(tx *store.Tx) error { return tx.TouchSession(s.ID, old) }); err != nil {
		t.Fatal(err)
	}
	f.input(lead.ID, "hello")
	f.until("the reply", func() bool { return f.agent(lead.ID).Activity == store.Idle && len(f.transcript(lead.ID)) == 2 })
	f.settled(lead.ID)
	active := f.session1(s.ID).LastActive
	if active <= old {
		t.Fatal("prompt and reply did not refresh activity")
	}
	if err := Seen(f.db, s.ID); err != nil {
		t.Fatal(err)
	}
	if err := Rename(f.db, s.ID, "Another title"); err != nil {
		t.Fatal(err)
	}
	if f.session1(s.ID).LastActive != active {
		t.Fatal("viewing or renaming refreshed activity")
	}
	f.rt.stopIdleSessions(time.UnixMilli(old).Add(idleTimeout))
	if !f.session1(s.ID).Running {
		t.Fatal("session stopped against its old deadline")
	}
}
