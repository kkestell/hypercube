package session

import (
	"github.com/kkestell/hypercube/internal/harness"
	"testing"
)

func TestChildRequestMirrorsOpenAndSavedItems(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "child", "")
	f.input(child.ID, "!ask")
	f.until("mirrored request", func() bool { return len(f.transcript(parent.ID)) == 3 })
	opened := f.saved(parent.ID)
	mirror := opened[2]
	if mirror.Kind != harness.KindAttention || mirror.From != child.ID || mirror.Parent != "" || mirror.Done {
		t.Fatalf("mirror=%+v", mirror)
	}
	if err := f.rt.Answer(child.ID, mirror.ID, "yes", ""); err != nil {
		t.Fatal(err)
	}
	f.until("answered mirror", func() bool { items := f.transcript(parent.ID); return items[2].Done && items[2].Answer == "yes" })
	saved := f.saved(parent.ID)
	if len(saved) != 3 || saved[2].Seq != mirror.Seq || len(f.inputs(parent.ID)) != 1 {
		t.Fatalf("mirror moved or reached plugin: %+v", saved)
	}
	own := f.transcript(child.ID)
	if own[1].ID != mirror.ID || !own[1].Done || own[1].Answer != "yes" {
		t.Fatalf("child request=%+v", own)
	}
}

// event applies an event as if the launch had reported it.
func (f *fixture) event(l *live, ev harness.Event) {
	f.t.Helper()
	f.rt.mu.Lock()
	defer f.rt.mu.Unlock()
	if err := f.rt.eventLocked(l, ev, l.position); err != nil {
		f.t.Fatal(err)
	}
}

func TestNestedChildRequestMirrorsWithoutItsToolParent(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "child", "")
	f.rt.mu.Lock()
	launch := f.rt.live[child.ID]
	f.rt.mu.Unlock()
	f.event(launch, harness.Event{Items: []harness.Item{
		{ID: "tool", Kind: harness.KindTool, Tool: "native"},
		{ID: "request", Parent: "tool", Kind: harness.KindAttention, Text: "Question?"},
	}})
	items := f.saved(parent.ID)
	mirror := items[2]
	if mirror.Parent != "" || mirror.From != child.ID || mirror.Done {
		t.Fatalf("nested mirror=%+v", mirror)
	}
	// Streaming edits stay in the child until the item is saved.
	f.event(launch, harness.Event{Items: []harness.Item{{ID: "request", Parent: "tool", Kind: harness.KindAttention, Text: "Changed?"}}})
	if got := f.transcript(parent.ID)[2].Text; got != "Question?" {
		t.Fatalf("stream mirrored: %q", got)
	}
	if err := f.rt.Stop(parent.ID); err != nil {
		t.Fatal(err)
	}
	closed := f.saved(parent.ID)
	if !closed[2].Done || closed[2].Seq != mirror.Seq || closed[2].Text != "Changed?" || len(f.inputs(parent.ID)) != 1 {
		t.Fatalf("drained mirror=%+v", closed[2])
	}
}
