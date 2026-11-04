package session

import (
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestTranscriptKeepsOrderAndReplacesItems(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	f.input(parent.ID, "!work")
	f.until("an unfinished item", func() bool {
		items := f.transcript(parent.ID)
		return len(items) == 4 && items[3].Text == "working" && !items[3].Done
	})
	// The item's later version replaces it in place instead of appearing again.
	f.input(parent.ID, "!finish")
	f.until("the replaced item", func() bool {
		return f.summary(parent.ID) == "user:start|text:ack|user:!work|text:finished|user:!finish"
	})
	f.input(parent.ID, "!stream")
	f.until("the streamed turn", func() bool {
		return f.storedSession(parent.ID).Activity == store.Idle && len(f.transcript(parent.ID)) == 8
	})
	items := f.transcript(parent.ID)
	text, tool := items[6], items[7]
	if items[5].Text != "!stream" || text.Text != "Hello!" || !text.Done || tool.Kind != harness.KindTool || tool.Output != "ok" || tool.Input != nil {
		t.Fatalf("streamed items = %+v", items[5:])
	}
	if tool.Parent != text.ID || !strings.HasSuffix(text.ID, ":s") {
		t.Fatalf("the tool's parent is %q, the text's ID %q", tool.Parent, text.ID)
	}
}

func TestRequestsNewestFirst(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "w1", "")
	f.input(parent.ID, "!ask")
	f.until("the parent's request", func() bool { return len(f.rt.Requests()) == 1 })
	f.input(child.ID, "!ask")
	f.until("the child's request", func() bool { return len(f.rt.Requests()) == 2 })
	if requests := f.rt.Requests(); requests[0].Session != child.ID || requests[1].Session != parent.ID {
		t.Fatalf("requests = %+v", requests)
	}
}

func TestTranscriptTimestampsSurviveUpdatesAndReload(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	f.input(parent.ID, "!work")
	f.until("the unfinished item", func() bool { return len(f.transcript(parent.ID)) == 4 })
	first := f.transcript(parent.ID)[3]
	if first.At == 0 {
		t.Fatalf("streamed item has no arrival timestamp: %+v", first)
	}
	f.input(parent.ID, "!finish")
	f.until("the finished item", func() bool { return f.storedSession(parent.ID).Activity == store.Idle })
	f.rt.Quit()
	reloaded := f.newRuntime()
	items, err := reloaded.Transcript(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range items {
		if it.At == 0 {
			t.Fatalf("saved item %q has no arrival timestamp", it.ID)
		}
		if it.ID == first.ID {
			found = true
			if it.At != first.At || !it.Done || it.Text != "finished" {
				t.Fatalf("arrival timestamp changed or item was not completed: %+v", it)
			}
		}
	}
	if !found {
		t.Fatal("reloaded transcript lost the completed item")
	}
}
