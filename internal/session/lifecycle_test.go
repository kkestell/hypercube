package session

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// idleAfter sets how long a session waits before it parks.
func (f *fixture) idleAfter(d time.Duration) {
	f.rt.mu.Lock()
	defer f.rt.mu.Unlock()
	f.rt.idleTimeout = d
}

// parkable reports whether nothing keeps the session awake.
func (f *fixture) parkable(id string) bool {
	f.rt.mu.Lock()
	defer f.rt.mu.Unlock()
	return f.rt.parkableLocked(id)
}

func (f *fixture) untilParked(id string) {
	f.t.Helper()
	f.until(id+" parked", func() bool { return !f.awake(id) && f.storedSession(id).Launch == "" })
}

func (f *fixture) view(id string) func() {
	f.t.Helper()
	release, err := f.rt.View(id)
	if err != nil {
		f.t.Fatal(err)
	}
	return release
}

func TestReleasingTheLastViewParksAnIdleSession(t *testing.T) {
	f := newFixture(t)
	f.idleAfter(time.Hour)
	s := f.session()
	first, second := f.view(s.ID), f.view(s.ID)
	if f.parkable(s.ID) {
		t.Fatal("a viewed session can park")
	}
	f.idleAfter(50 * time.Millisecond)
	first()
	if f.parkable(s.ID) {
		t.Fatal("a session another page views can park")
	}
	second()
	f.untilParked(s.ID)
	if syscall.Kill(f.log(s.ID, "start")[0].Pid, 0) != syscall.ESRCH {
		t.Fatal("the agent's process remains")
	}
	if got := f.summary(s.ID); got != "user:start|text:ack" || f.storedSession(s.ID).Attention {
		t.Fatalf("parking changed the transcript: %q", got)
	}
}

func TestSessionsWakeAndParkOnTheirOwn(t *testing.T) {
	f := newFixture(t)
	f.idleAfter(time.Hour)
	parent := f.session()
	child := f.child(parent, "child", "brief")
	f.until("the child's reply", func() bool { return f.idle(child.ID) && f.idle(parent.ID) })
	// A viewed parent leaves its idle child free to park.
	release := f.view(parent.ID)
	if !f.parkable(child.ID) || f.parkable(parent.ID) {
		t.Fatal("viewing the parent changed whether its child can park")
	}
	release()
	// A busy child leaves its idle parent free to park.
	f.input(child.ID, "!work")
	f.until("the busy child", func() bool { return f.storedSession(child.ID).Activity == store.Busy })
	if f.parkable(child.ID) || !f.parkable(parent.ID) {
		t.Fatal("the busy child changed whether its parent can park")
	}
	f.input(child.ID, "!finish")
	f.until("the reports delivered", func() bool { return f.idle(child.ID) && f.idle(parent.ID) && len(f.queued(parent.ID)) == 0 })
	f.park(parent.ID)
	f.park(child.ID)
	// Viewing one session wakes only it, resuming its conversation.
	defer f.view(child.ID)()
	f.until("the child awake", func() bool { return f.idle(child.ID) })
	starts := f.log(child.ID, "start")
	if len(starts) != 2 || starts[1].Conversation != f.storedSession(child.ID).Conversation || f.awake(parent.ID) {
		t.Fatalf("viewing the child: starts %v, parent awake %v", starts, f.awake(parent.ID))
	}
}

func TestBusySessionsDoNotPark(t *testing.T) {
	for _, c := range []struct {
		name, text string
		ready      func(*fixture, string) bool
	}{
		{"working", "!work", func(f *fixture, id string) bool { return f.storedSession(id).Activity == store.Busy }},
		{"question", "!ask", func(f *fixture, id string) bool { return len(f.requests()) == 1 }},
		{"input pending", "!accept", func(f *fixture, id string) bool {
			f.rt.mu.Lock()
			defer f.rt.mu.Unlock()
			return f.storedSession(id).InputPending && len(f.rt.live[id].commands) == 0
		}},
		{"delivery in flight", "", func(f *fixture, id string) bool { return len(f.inputs(id)) == 2 }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.idleAfter(time.Hour)
			s := f.session()
			if c.text != "" {
				f.input(s.ID, c.text)
			} else {
				f.queue("user", s.ID, "!hold")
			}
			f.until(c.name, func() bool { return c.ready(f, s.ID) })
			if f.parkable(s.ID) {
				t.Fatalf("a session with %s can park", c.name)
			}
		})
	}
}

// A blocker that ends with no idle event still starts a fresh idle timeout.
func TestAnsweredQuestionStartsTheIdleTimeout(t *testing.T) {
	f := newFixture(t)
	f.idleAfter(time.Hour)
	s := f.session()
	f.input(s.ID, "!ask")
	f.until("the question", func() bool { return len(f.requests()) == 1 })
	f.input(s.ID, "!finish")
	f.until("the finished turn", func() bool { return f.idle(s.ID) })
	if f.parkable(s.ID) {
		t.Fatal("a session with an open question can park")
	}
	f.idleAfter(50 * time.Millisecond)
	if err := f.rt.Answer(s.ID, f.requests()[0].Item.ID, []string{"yes"}, ""); err != nil {
		t.Fatal(err)
	}
	f.untilParked(s.ID)
}

func TestReportWakesAParkedParent(t *testing.T) {
	f := newFixture(t)
	f.idleAfter(time.Hour)
	parent := f.session()
	child := f.child(parent, "child", "")
	f.park(parent.ID)
	f.input(child.ID, "!answer")
	f.until("the report delivered", func() bool {
		inputs := f.inputs(parent.ID)
		return len(inputs) == 2 && strings.Contains(inputs[1], "the answer") && len(f.queued(parent.ID)) == 0
	})
	if starts := f.log(parent.ID, "start"); len(starts) != 2 || starts[1].Conversation != parent.ID+"-conversation" {
		t.Fatalf("starts = %v", starts)
	}
}

// A view or message that arrives while the agent stops wakes it once it has
// ended.
func TestWakeWhileParking(t *testing.T) {
	t.Setenv("HYPERCUBE_TEST_END_DELAY", "300ms")
	for _, c := range []struct {
		name string
		wake func(*fixture, string)
	}{
		{"view", func(f *fixture, id string) { f.view(id) }},
		{"message", func(f *fixture, id string) { f.queue("user", id, "while parking") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.idleAfter(time.Hour)
			s := f.session()
			parked := make(chan struct{})
			go func() { f.park(s.ID); close(parked) }()
			f.until("the agent stopping", func() bool {
				f.rt.mu.Lock()
				defer f.rt.mu.Unlock()
				l := f.rt.live[s.ID]
				return l != nil && l.stopping
			})
			c.wake(f, s.ID)
			<-parked
			f.until("the agent awake again", func() bool { return len(f.log(s.ID, "start")) == 2 && f.idle(s.ID) && len(f.queued(s.ID)) == 0 })
		})
	}
}

// Restart replaces a running agent and starts a parked one, each resuming its
// conversation without an error in the transcript.
func TestRestart(t *testing.T) {
	f := newFixture(t)
	f.idleAfter(time.Hour)
	s := f.session()
	first := f.log(s.ID, "start")[0]
	if err := f.rt.Restart(s.ID); err != nil {
		t.Fatal(err)
	}
	f.until("the agent running again", func() bool { return len(f.log(s.ID, "start")) == 2 && f.idle(s.ID) })
	if syscall.Kill(first.Pid, 0) != syscall.ESRCH {
		t.Fatal("the first agent's process remains")
	}
	conversation := f.storedSession(s.ID).Conversation
	if second := f.log(s.ID, "start")[1]; second.Conversation != conversation || conversation == "" {
		t.Fatalf("the restart resumed %q, want %q", second.Conversation, conversation)
	}
	f.park(s.ID)
	if err := f.rt.Restart(s.ID); err != nil {
		t.Fatal(err)
	}
	f.until("the parked agent started", func() bool { return len(f.log(s.ID, "start")) == 3 && f.idle(s.ID) })
	if got := f.summary(s.ID); got != "user:start|text:ack" {
		t.Fatalf("restarting changed the transcript: %q", got)
	}
}

func TestUnarchive(t *testing.T) {
	f := newFixture(t)
	f.idleAfter(time.Hour)
	s := f.worktreeSession()
	child := f.child(s, "worker", "")
	conversation := f.storedSession(s.ID).Conversation
	if err := f.rt.Unarchive(s.ID); err == nil || !strings.Contains(err.Error(), "not archived") {
		t.Fatalf("unarchived an active session: %v", err)
	}
	if err := f.rt.Archive(s.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Unarchive(child.ID); err == nil || !strings.Contains(err.Error(), "top-level") {
		t.Fatalf("unarchived a child: %v", err)
	}
	f.git(f.root, "commit", "-q", "--allow-empty", "-m", "later")
	head, err := workspace.Head(f.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Unarchive(s.ID); err != nil {
		t.Fatal(err)
	}
	got := f.storedSession(s.ID)
	if got.Ended || got.Base != head || f.awake(s.ID) || !f.storedSession(child.ID).Ended {
		t.Fatalf("unarchived session = %+v", got)
	}
	if b, err := workspace.Branch(s.Dir); err != nil || b != s.Branch {
		t.Fatalf("worktree branch = %q, %v", b, err)
	}
	if _, err := os.Stat(child.Dir); !os.IsNotExist(err) {
		t.Fatalf("the child's worktree returned: %v", err)
	}
	f.input(s.ID, "continue")
	if starts := f.log(s.ID, "start"); len(starts) != 2 || starts[1].Conversation != conversation || starts[1].Dir != s.Dir {
		t.Fatalf("starts = %v", starts)
	}
}

func TestUnarchiveCheckoutSession(t *testing.T) {
	f := newFixture(t)
	f.idleAfter(time.Hour)
	s := f.session()
	if err := f.rt.Archive(s.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Input(s.ID, "archived"); !errors.Is(err, errEnded) {
		t.Fatalf("input to an archived session: %v", err)
	}
	if err := f.rt.Unarchive(s.ID); err != nil {
		t.Fatal(err)
	}
	f.input(s.ID, "continue")
	f.until("the resumed turn", func() bool { return f.summary(s.ID) == "user:start|text:ack|user:continue|text:ack" })
	if starts := f.log(s.ID, "start"); len(starts) != 2 || starts[1].Conversation != s.ID+"-conversation" {
		t.Fatalf("starts = %v", starts)
	}
}
