package session

import (
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestEventsWithoutStateKeepTheAgentWorking(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	parent := f.session()
	f.input(parent.ID, "!work")
	f.until("the parent working", func() bool { return f.storedSession(parent.ID).Activity == store.Busy })
	conversation := f.storedSession(parent.ID).Conversation
	f.input(parent.ID, "!usage")
	f.until("the usage event", func() bool {
		return f.storedSession(parent.ID).Usage == (harness.Usage{Context: 45200, Window: 200000, Cost: 0.31})
	})
	if a := f.storedSession(parent.ID); a.Activity != store.Busy || a.Conversation != conversation {
		t.Fatalf("an event without state or conversation changed the agent: %+v", a)
	}
}

func TestLimitsAreSavedUnderTheSessionsHarness(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	f.input(s.ID, "!usage")
	var limits []store.Limit
	f.until("the limits", func() bool {
		f.read(func(tx *store.Tx) (err error) { limits, err = tx.Limits(); return })
		return len(limits) > 0
	})
	want := store.Limit{Harness: f.storedSession(s.ID).Harness, Limit: harness.Limit{Minutes: 300, Used: 42, Resets: 1793767800, At: 1793750712115}}
	if len(limits) != 1 || limits[0] != want {
		t.Fatalf("limits = %+v, want %+v", limits, want)
	}
}

func TestFinishedTurnsMarkOnlyTopLevelSessionsUnseen(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	if err := f.rt.Seen(s.ID); err != nil {
		t.Fatal(err)
	}
	// The image arrives with an idle event, while the parent is already idle.
	f.input(s.ID, "!image")
	f.until("the image", func() bool { return len(f.transcript(s.ID)) == 4 })
	if f.storedSession(s.ID).Unseen {
		t.Fatal("a session that was already idle marked its session unseen")
	}
	child := f.child(s, "worker", "")
	f.input(child.ID, "!work")
	f.until("the child working", func() bool { return f.storedSession(child.ID).Activity == store.Busy })
	f.input(child.ID, "!finish")
	f.until("the child idle", func() bool { return f.storedSession(child.ID).Activity == store.Idle })
	if f.storedSession(child.ID).Unseen || f.storedSession(s.ID).Unseen {
		t.Fatal("a child's finished turn marked a session unseen")
	}
	f.input(s.ID, "!work")
	f.until("the parent working", func() bool { return f.storedSession(s.ID).Activity == store.Busy })
	f.input(s.ID, "!finish")
	f.until("the parent idle", func() bool { return f.storedSession(s.ID).Activity == store.Idle })
	if !f.storedSession(s.ID).Unseen {
		t.Fatal("a top-level session's finished turn did not mark it unseen")
	}
}

// states returns the agent's saved items in order, with state markers shown
// as "working" or "idle".
func (f *fixture) states(agent string) (kinds []string, items []TranscriptItem) {
	f.t.Helper()
	items, err := f.rt.Transcript(agent)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, it := range items {
		kind := it.Kind
		if kind == harness.KindState {
			kind = it.State
		}
		kinds = append(kinds, kind)
	}
	return kinds, items
}

func TestStateMarkersBracketTheTurnsItems(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	f.input(s.ID, "!answer")
	f.until("the answer", func() bool {
		kinds, _ := f.states(s.ID)
		return strings.HasSuffix(strings.Join(kinds, " "), "user working text text idle")
	})
	_, items := f.states(s.ID)
	idle, answer := items[len(items)-1], items[len(items)-2]
	prefix, _, _ := strings.Cut(answer.ID, ":")
	if idle.Kind != harness.KindState || !idle.Done || idle.At == 0 || len(idle.Final) != 1 || idle.Final[0] != answer.ID || !strings.HasPrefix(idle.ID, prefix+":") {
		t.Fatalf("idle marker %+v, answer %+v", idle.Item, answer.Item)
	}
	if work := items[len(items)-4]; work.State != harness.Working || work.At < items[len(items)-5].At {
		t.Fatalf("working marker %+v", work.Item)
	}

	// The markers are saved with the transcript.
	f.rt.Quit()
	reloaded := f.newRuntime()
	saved, err := reloaded.Transcript(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := saved[len(saved)-1]; got.ID != idle.ID || len(got.Final) != 1 || got.Final[0] != answer.ID || got.Seq != idle.Seq {
		t.Fatalf("reloaded %+v, saved %+v", got, idle)
	}
}
