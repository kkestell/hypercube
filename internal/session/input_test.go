package session

import (
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestAttentionRequests(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "!ask")
	f.until("attention", func() bool { return f.agent(lead.ID).Attention })
	if requests := f.rt.Requests(); len(requests) != 1 || requests[0].Agent != lead.ID || requests[0].Item.Kind != harness.KindAttention {
		t.Fatalf("requests = %+v", requests)
	}
	var request string
	for _, it := range f.transcript(lead.ID) {
		if it.Kind == harness.KindAttention {
			request = it.ID
			if it.Text != "Proceed?" || len(it.Choices) != 2 || it.Done {
				t.Fatalf("request = %+v", it)
			}
		}
	}
	if err := f.rt.Answer(lead.ID, "nothing", "yes", ""); err == nil || !strings.Contains(err.Error(), "finished") {
		t.Fatalf("answer to an unknown request: %v", err)
	}
	if err := f.rt.Answer(lead.ID, request, "yes", ""); err != nil {
		t.Fatal(err)
	}
	f.until("the answered request", func() bool { return !f.agent(lead.ID).Attention })
	if requests := f.rt.Requests(); len(requests) != 0 {
		t.Fatalf("answered requests = %+v", requests)
	}
	if got := f.log(lead.ID, "answer"); len(got) != 1 || got[0].Request != "q" || got[0].Choice != "yes" {
		t.Fatalf("the plugin was answered with %v", got)
	}
	var answered harness.Item
	for _, it := range f.transcript(lead.ID) {
		if it.ID == request {
			answered = it
		}
	}
	if !answered.Done || answered.Answer != "yes" {
		t.Fatalf("answered item = %+v", answered)
	}
	if err := f.rt.Answer(lead.ID, request, "no", ""); err == nil || !strings.Contains(err.Error(), "finished") {
		t.Fatalf("a second answer: %v", err)
	}
}

func TestInterruptAndUsage(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "!work")
	f.until("a working lead", func() bool { return f.agent(lead.ID).Activity == store.Busy })
	if err := f.rt.Interrupt(lead.ID); err != nil {
		t.Fatal(err)
	}
	f.until("an idle lead", func() bool { return f.agent(lead.ID).Activity == store.Idle })
	// The interrupted turn stays, and nothing sends it again.
	if got := f.summary(lead.ID); got != "user:start|text:ack|user:!work|text:working" || len(f.inputs(lead.ID)) != 2 {
		t.Fatalf("transcript = %q, inputs %q", got, f.inputs(lead.ID))
	}
	f.input(lead.ID, "!usage")
	f.until("usage", func() bool {
		return f.agent(lead.ID).Usage == harness.Usage{Context: 45200, Window: 200000, Cost: 0.31}
	})
}

func TestImagePrompts(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	png := harness.Image{MIMEType: "image/png", Data: "iVBORw0KGgo="}
	if err := f.rt.Input(lead.ID, "", harness.Image{MIMEType: "image/png", Data: "bm90IGEgcG5n"}); err == nil {
		t.Fatal("input accepted bytes that are not a PNG")
	}
	if err := f.rt.Input(lead.ID, "  "); err == nil {
		t.Fatal("input accepted neither text nor an image")
	}
	// An image alone is a prompt: the plugin gets it, and the user's item holds
	// it. The session keeps its title.
	if err := f.rt.Input(lead.ID, "", png); err != nil {
		t.Fatal(err)
	}
	if got := f.storedSession(s.ID).Title; got != "start" {
		t.Errorf("title = %q", got)
	}
	if got := f.log(lead.ID, "input"); len(got) != 2 || got[1].Images != 1 {
		t.Errorf("plugin inputs = %v", got)
	}
	f.until("the image prompt in the transcript", func() bool {
		items := f.transcript(lead.ID)
		return len(items) > 2 && items[2].Kind == harness.KindUser && len(items[2].Images) == 1 && items[2].Images[0] == png
	})
	// An image item from the plugin reaches the transcript whole.
	f.input(lead.ID, "!image")
	f.until("the assistant's image", func() bool {
		for _, it := range f.transcript(lead.ID) {
			if it.Kind == harness.KindText && it.Text == "" && len(it.Images) == 1 && it.Images[0] == png {
				return true
			}
		}
		return false
	})
}
