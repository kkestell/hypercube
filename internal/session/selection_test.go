package session

import (
	"context"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestSelect(t *testing.T) {
	t.Parallel()
	for _, working := range []bool{false, true} {
		t.Run(map[bool]string{false: "idle", true: "working"}[working], func(t *testing.T) {
			t.Parallel()
			f := newPlainFixture(t)
			s := f.session()
			if working {
				f.input(s.ID, "!work")
				f.until("working", func() bool { return f.storedSession(s.ID).Activity == store.Busy })
			}
			if err := f.rt.Select(s.ID, harness.Selection{Effort: "high"}); err == nil {
				t.Fatal("selected no model")
			}
			if err := f.rt.Select(s.ID, harness.Selection{Model: "next", Effort: "high"}); err != nil {
				t.Fatal(err)
			}
			if got := f.storedSession(s.ID); got.Model != "next" || got.Effort != "high" || got.Harness != "test" {
				t.Fatal(got)
			}
			if len(f.log(s.ID, "configure")) != 1 {
				t.Fatal("an invalid selection reached the plugin")
			}
			f.restart()
			f.until("reattached", func() bool { return f.idle(s.ID) || f.storedSession(s.ID).Activity == store.Busy })
			f.park(s.ID)
			if err := f.rt.Select(s.ID, harness.Selection{Model: "m", Effort: "low"}); err == nil {
				t.Fatal("configured parked session")
			}
			if err := f.wake(s.ID); err != nil {
				t.Fatal(err)
			}
			starts := f.log(s.ID, "start")
			if len(starts) != 2 || starts[1].Model != "next" || starts[1].Effort != "high" || starts[1].Conversation != s.ID+"-conversation" {
				t.Fatal(starts)
			}
		})
	}
}

func TestSelectPartialAndRejected(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	for _, tc := range []struct{ name, model, effort, failure string }{
		{"partial", "partial", "low", "effort rejected"},
		{"reject", "partial", "low", "model rejected"},
	} {
		if err := f.rt.Select(s.ID, harness.Selection{Model: tc.name, Effort: "high"}); err == nil || !strings.Contains(err.Error(), tc.failure) {
			t.Fatal(err)
		}
		if got := f.storedSession(s.ID); got.Model != tc.model || got.Effort != tc.effort {
			t.Fatal(got)
		}
		items := f.transcript(s.ID)
		if last := items[len(items)-1]; last.Kind != harness.KindError || !strings.Contains(last.Text, tc.failure) {
			t.Fatal(items)
		}
	}
}

func TestSelectUnansweredAcrossReattachment(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	s := f.session()
	result := make(chan error, 1)
	go func() { result <- f.rt.Select(s.ID, harness.Selection{Model: "hold", Effort: "high"}) }()
	f.until("configure received", func() bool { return len(f.log(s.ID, "configure")) == 1 })
	f.rt.Detach()
	<-result
	f.rt = f.newRuntime()
	f.restore()
	f.until("attached", func() bool {
		f.rt.mu.Lock()
		defer f.rt.mu.Unlock()
		return f.rt.live[s.ID] != nil && f.rt.live[s.ID].client != nil
	})
	if err := f.rt.Interrupt(s.ID); err != nil {
		t.Fatal(err)
	}
	f.until("configure applied", func() bool { return f.storedSession(s.ID).Model == "hold" })
	if got := f.storedSession(s.ID); got.Effort != "high" {
		t.Fatal(got)
	}
	if len(f.log(s.ID, "configure")) != 1 {
		t.Fatal("configure replayed twice")
	}
}

// A child inherits its parent's accepted settings, and the harness reports
// the effort it chose when none was given.
func TestChildInheritsTheParentsCurrentSettings(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	parent := f.session()
	if err := f.rt.Select(parent.ID, harness.Selection{Model: "next", Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	inherited := f.child(parent, "inherited", "")
	if starts := f.log(inherited.ID, "start"); len(starts) != 1 || starts[0].Model != "next" || starts[0].Effort != "high" {
		t.Fatalf("inherited %+v", starts)
	}
	other, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "other", Title: "Title", Dir: ".", Settings: harness.Settings{Model: "m"}})
	if err != nil {
		t.Fatal(err)
	}
	if starts := f.log(other.ID, "start"); len(starts) != 1 || starts[0].Model != "m" || starts[0].Effort != "" {
		t.Fatalf("starts %+v", starts)
	}
	// CreateChild returns once the start, with the harness's effort, is saved.
	if other.Effort != "low" || f.storedSession(other.ID).Effort != "low" {
		t.Fatalf("started %+v", other)
	}
	cfg, err := f.config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Models = []config.ModelEntry{{Harness: "test", Model: "m", Title: "Model", Efforts: []string{"low", "high"}, DefaultEffort: "high"}}
	if err := f.config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	configured, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "configured", Title: "Title", Dir: ".", Settings: harness.Settings{Model: "m"}})
	if err != nil {
		t.Fatal(err)
	}
	if starts := f.log(configured.ID, "start"); len(starts) != 1 || starts[0].Model != "m" || starts[0].Effort != "high" {
		t.Fatalf("configured default %+v", starts)
	}
	if configured.Effort != "high" || f.storedSession(configured.ID).Effort != "high" {
		t.Fatalf("configured child %+v", configured)
	}
	// The harness rejects a model in its start, and the child is removed.
	if _, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "invalid", Title: "Title", Dir: ".", Settings: harness.Settings{Model: "invalid"}}); err == nil || !strings.Contains(err.Error(), "not offered") {
		t.Fatal(err)
	}
	if len(f.family(parent.ID)) != 4 {
		t.Fatalf("family %+v", f.family(parent.ID))
	}
}

// A proposal starts with the settings it showed, though the parent's change.
func TestProposalKeepsItsSettings(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	parent := f.session()
	id := f.proposal(parent.ID, ChildOptions{Name: "proposed", Title: "Proposed", Dir: ".", Message: "brief"})
	if err := f.rt.Select(parent.ID, harness.Selection{Model: "next", Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	child, err := f.rt.StartProposal(parent.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if starts := f.log(child.ID, "start"); len(starts) != 1 || starts[0].Model != "m" || starts[0].Effort != "low" {
		t.Fatalf("started %+v", starts)
	}
}

func TestInstructionsIncludeGuidance(t *testing.T) {
	t.Parallel()
	f := newPlainFixture(t)
	if err := f.config.Save(config.Config{ReviewPrompt: config.DefaultReviewPrompt, Theme: "system", TerminalFontSize: 13, ChildGuidance: "Use next for reviews."}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.ScanHarnesses(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := f.session()
	starts := f.log(s.ID, "start")
	if len(starts) != 1 || !strings.Contains(starts[0].Instructions, "Use next for reviews.") || !strings.Contains(starts[0].Instructions, "Installed harnesses: test.") {
		t.Fatal(starts)
	}
}
