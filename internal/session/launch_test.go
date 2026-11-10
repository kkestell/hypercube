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
	t.Parallel()
	f := newPlainFixture(t)
	parent := f.session()
	f.input(parent.ID, "hello")
	f.until("the first turn", func() bool { return len(f.transcript(parent.ID)) == 4 })
	f.input(parent.ID, "!crash")
	f.until("a relaunch", func() bool {
		return len(f.log(parent.ID, "start")) == 2 && f.storedSession(parent.ID).Activity == store.Idle
	})
	if start := f.log(parent.ID, "start"); start[1].Conversation != parent.ID+"-conversation" {
		t.Fatalf("relaunch did not resume: %v", start[1])
	}
	// The ended launch left a note, and the new launch's items follow the old
	// ones even though it numbers its own from the start.
	f.input(parent.ID, "again")
	f.until("a turn after the relaunch", func() bool { return len(f.transcript(parent.ID)) == 8 })
	got := f.summary(parent.ID)
	if !strings.HasPrefix(got, "user:start|text:ack|user:hello|text:ack|user:!crash|error:The agent exited: ") ||
		!strings.HasSuffix(got, "|user:again|text:ack") {
		t.Fatalf("transcript = %q", got)
	}
}

func TestAgentThatKeepsEndingStaysEnded(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	parent := f.session()
	ended := func() (n int) {
		for _, it := range f.transcript(parent.ID) {
			if strings.HasPrefix(it.Text, "The agent exited: ") {
				n++
			}
		}
		return n
	}
	for exits := 1; exits <= maxQuickExits; exits++ {
		f.input(parent.ID, "!crash")
		f.until("the exit to be applied", func() bool { return ended() == exits })
		if exits < maxQuickExits {
			f.until("the replacement agent ready", func() bool { return f.idle(parent.ID) })
		}
	}
	f.until("the agent to stay ended", func() bool { return f.storedSession(parent.ID).Launch == "" })
	// CreateChild refuses only when the parent has no launch, even a starting one.
	if _, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "w1", Title: "Title", Dir: ".", Message: ""}); err == nil || !strings.Contains(err.Error(), "the session is not running") {
		t.Fatalf("a session that stayed exited took a child: %v", err)
	}
	if n := len(f.log(parent.ID, "start")); n != maxQuickExits {
		t.Fatalf("%d launches", n)
	}
	if f.storedSession(parent.ID).Activity != store.Exited {
		t.Fatalf("agent %+v", f.storedSession(parent.ID))
	}
}

func TestFailedResumeIsAnError(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	f.park(s.ID)
	if err := f.db.Tx(func(tx *store.Tx) error {
		a, err := tx.Session(s.ID)
		if err != nil {
			return err
		}
		a.Conversation = "unresumable"
		return tx.SaveSession(a)
	}); err != nil {
		t.Fatal(err)
	}
	err := f.wake(s.ID)
	if err == nil || !strings.Contains(err.Error(), "cannot resume conversation unresumable") {
		t.Fatalf("start: %v", err)
	}
	// The agent stays ended with the error in its transcript, and no fresh
	// conversation starts in its place.
	if got := f.summary(s.ID); !strings.HasSuffix(got, "|error:Could not start the agent: cannot resume conversation unresumable") {
		t.Fatalf("transcript = %q", got)
	}
	starts := f.log(s.ID, "start")
	if a := f.storedSession(s.ID); a.Activity != store.Exited || a.Conversation != "unresumable" || len(starts) != 2 || starts[1].Conversation != "unresumable" {
		t.Fatalf("after a failed resume: agent %+v, starts %v", a, starts)
	}
	// Nothing of the failed launch remains, so a prompt tries again.
	if err := f.rt.Input(s.ID, "hello"); err == nil || len(f.log(s.ID, "start")) != 3 {
		t.Fatalf("second start: %v", err)
	}
}

func TestFailedRelaunchStaysEnded(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	parent := f.session()
	if err := f.db.Tx(func(tx *store.Tx) error {
		a, err := tx.Session(parent.ID)
		if err != nil {
			return err
		}
		a.Conversation = "unresumable"
		return tx.SaveSession(a)
	}); err != nil {
		t.Fatal(err)
	}
	f.input(parent.ID, "!crash")
	f.until("the failed relaunch", func() bool {
		return strings.HasSuffix(f.summary(parent.ID), "|error:Could not start the agent: cannot resume conversation unresumable")
	})
	// The failed relaunch has no successor, including one still starting.
	if _, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "w1", Title: "Title", Dir: ".", Message: ""}); err == nil || !strings.Contains(err.Error(), "the session is not running") {
		t.Fatalf("a failed relaunch took a child: %v", err)
	}
	starts := f.log(parent.ID, "start")
	if a := f.storedSession(parent.ID); a.Activity != store.Exited || a.Conversation != "unresumable" || len(starts) != 2 || starts[1].Conversation != "unresumable" {
		t.Fatalf("after a failed relaunch: agent %+v, starts %v", a, starts)
	}
}

func TestUnsupportedHarnessFailsTheStart(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	script := "#!/bin/sh\necho 'unsupported harness: not ported' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(f.plugins, "hypercube-harness-old"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tx(func(tx *store.Tx) error {
		return tx.AddSession(store.Session{ID: "s", Workspace: f.workspace.ID, Harness: "old", Dir: f.root, Activity: store.Exited})
	}); err != nil {
		t.Fatal(err)
	}
	err := f.wake("s")
	if err == nil || !strings.Contains(err.Error(), "unsupported harness: not ported") {
		t.Fatalf("start: %v", err)
	}
	items := f.transcript("s")
	if len(items) != 1 || items[0].Kind != harness.KindError || !strings.Contains(items[0].Text, "unsupported harness") || f.storedSession("s").Activity != store.Exited {
		t.Fatalf("transcript = %+v", items)
	}
}
