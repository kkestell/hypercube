package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestRelaunchAfterTheProcessEnds(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "hello")
	f.until("the first turn", func() bool { return len(f.transcript(lead.ID)) == 4 })
	f.input(lead.ID, "!crash")
	f.until("a relaunch", func() bool { return len(f.log(lead.ID, "start")) == 2 && f.agent(lead.ID).Activity == store.Idle })
	if start := f.log(lead.ID, "start"); start[1].Conversation != lead.ID+"-conversation" {
		t.Fatalf("relaunch did not resume: %v", start[1])
	}
	// The ended launch left a note, and the new launch's items follow the old
	// ones even though it numbers its own from the start.
	f.input(lead.ID, "again")
	f.until("a turn after the relaunch", func() bool { return len(f.transcript(lead.ID)) == 8 })
	got := f.summary(lead.ID)
	if !strings.HasPrefix(got, "user:start|text:ack|user:hello|text:ack|user:!crash|error:The agent ended: ") ||
		!strings.HasSuffix(got, "|user:again|text:ack") {
		t.Fatalf("transcript = %q", got)
	}
}

func TestAgentThatKeepsEndingStaysEnded(t *testing.T) {
	f := newFixture(t)
	t.Setenv("HYPERCUBE_TEST_CRASH", "1")
	// The plugin can exit before the first prompt reaches it, which Create
	// reports; the session and its lead remain.
	s, lead, _ := f.rt.Create(f.workspace.ID, "lead", "start")
	ended := func() (n int) {
		for _, it := range f.transcript(lead.ID) {
			if strings.HasPrefix(it.Text, "The agent ended: ") {
				n++
			}
		}
		return n
	}
	f.until("the agent to end three times", func() bool { return ended() == maxQuickExits })
	// AddWorker refuses only when the lead has no launch, even a starting one.
	if _, err := f.rt.AddWorker(lead.ID, "w1", harness.Profile{Spec: harness.Spec{Harness: "test"}}, ".", ""); err == nil || !strings.Contains(err.Error(), "the session is not running") {
		t.Fatalf("a lead that stayed ended took a worker: %v", err)
	}
	if n := len(f.log(lead.ID, "start")); n != maxQuickExits {
		t.Fatalf("%d launches", n)
	}
	if f.agent(lead.ID).Activity != store.Exited || !f.storedSession(s.ID).Running {
		t.Fatalf("agent %+v, session %+v", f.agent(lead.ID), f.storedSession(s.ID))
	}
}

func TestFailedResumeIsAnError(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	if err := f.rt.Stop(s.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tx(func(tx *store.Tx) error {
		a, err := tx.Agent(lead.ID)
		if err != nil {
			return err
		}
		a.Conversation = "unresumable"
		return tx.SaveAgent(a)
	}); err != nil {
		t.Fatal(err)
	}
	err := f.rt.Start(s.ID)
	if err == nil || !strings.Contains(err.Error(), "cannot resume conversation unresumable") {
		t.Fatalf("start: %v", err)
	}
	// The agent stays ended with the error in its transcript, and no fresh
	// conversation starts in its place.
	if got := f.summary(lead.ID); !strings.HasSuffix(got, "|error:Could not start the agent: cannot resume conversation unresumable") {
		t.Fatalf("transcript = %q", got)
	}
	starts := f.log(lead.ID, "start")
	if a := f.agent(lead.ID); a.Activity != store.Exited || a.Conversation != "unresumable" || len(starts) != 2 || starts[1].Conversation != "unresumable" {
		t.Fatalf("after a failed resume: agent %+v, starts %v", a, starts)
	}
	// Nothing of the failed launch remains, so starting again tries again.
	if err := f.rt.Start(s.ID); err == nil || len(f.log(lead.ID, "start")) != 3 {
		t.Fatalf("second start: %v", err)
	}
}

func TestUnsupportedHarnessFailsTheStart(t *testing.T) {
	f := newFixture(t)
	script := "#!/bin/sh\necho 'unsupported harness: not ported' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(f.plugins, "hypercube-harness-old"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tx(func(tx *store.Tx) error {
		if err := tx.AddSession(store.Session{ID: "s", Workspace: f.workspace.ID}); err != nil {
			return err
		}
		return tx.AddAgent(store.Agent{ID: "a", Session: "s", Name: "lead", Role: store.Lead, Harness: "old", Dir: f.root, Activity: store.Exited})
	}); err != nil {
		t.Fatal(err)
	}
	err := f.rt.Start("s")
	if err == nil || !strings.Contains(err.Error(), "unsupported harness: not ported") {
		t.Fatalf("start: %v", err)
	}
	items := f.transcript("a")
	if len(items) != 1 || items[0].Kind != harness.KindError || !strings.Contains(items[0].Text, "unsupported harness") || f.agent("a").Activity != store.Exited {
		t.Fatalf("transcript = %+v", items)
	}
}
