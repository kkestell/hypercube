package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func profileConfig(t *testing.T) {
	t.Helper()
	config := `{"profiles":[
 {"name":"lead","description":"Test profile","harness":"test","model":"m","effort":"low"},
 {"name":"next","description":"Test profile","harness":"test","model":"next","effort":"high"},
 {"name":"foreign","description":"Test profile","harness":"other","model":"m","effort":"low"},
 {"name":"partial","description":"Test profile","harness":"test","model":"partial","effort":"high"},
 {"name":"reject","description":"Test profile","harness":"test","model":"reject","effort":"high"},
 {"name":"hold","description":"Test profile","harness":"test","model":"hold","effort":"high"}]}`
	if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "hypercube/config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSelectProfile(t *testing.T) {
	for _, working := range []bool{false, true} {
		t.Run(map[bool]string{false: "idle", true: "working"}[working], func(t *testing.T) {
			f := newFixture(t)
			profileConfig(t)
			s := f.session()
			if working {
				f.input(s.ID, "!work")
				f.until("working", func() bool { return f.storedSession(s.ID).Activity == store.Busy })
			}
			for _, name := range []string{"foreign", "unknown"} {
				if err := f.rt.SelectProfile(s.ID, name); err == nil {
					t.Fatalf("selected %s", name)
				}
			}
			if err := f.rt.SelectProfile(s.ID, "next"); err != nil {
				t.Fatal(err)
			}
			if got := f.storedSession(s.ID); got.Model != "next" || got.Effort != "high" || got.Harness != "test" {
				t.Fatal(got)
			}
			if len(f.log(s.ID, "configure")) != 1 {
				t.Fatal("invalid profiles reached plugin")
			}
			f.restart()
			f.until("reattached", func() bool { return f.idle(s.ID) || f.storedSession(s.ID).Activity == store.Busy })
			if err := f.rt.Stop(s.ID); err != nil {
				t.Fatal(err)
			}
			if err := f.rt.SelectProfile(s.ID, "lead"); err == nil {
				t.Fatal("configured stopped session")
			}
			if err := f.rt.Start(s.ID); err != nil {
				t.Fatal(err)
			}
			starts := f.log(s.ID, "start")
			if len(starts) != 2 || starts[1].Model != "next" || starts[1].Effort != "high" || starts[1].Conversation != s.ID+"-conversation" {
				t.Fatal(starts)
			}
		})
	}
}

func TestSelectProfilePartialAndRejected(t *testing.T) {
	f := newFixture(t)
	profileConfig(t)
	s := f.session()
	for _, tc := range []struct{ name, model, effort, failure string }{
		{"partial", "partial", "low", "effort rejected"},
		{"reject", "partial", "low", "model rejected"},
	} {
		if err := f.rt.SelectProfile(s.ID, tc.name); err == nil || !strings.Contains(err.Error(), tc.failure) {
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

func TestSelectProfileUnansweredAcrossReattachment(t *testing.T) {
	f := newFixture(t)
	profileConfig(t)
	s := f.session()
	result := make(chan error, 1)
	go func() { result <- f.rt.SelectProfile(s.ID, "hold") }()
	f.until("configure received", func() bool { return len(f.log(s.ID, "configure")) == 1 })
	f.rt.Detach()
	<-result
	f.rt = f.newRuntime()
	f.rt.Restore()
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
	f.read(func(tx *store.Tx) error {
		cmds, err := tx.Commands(s.ID)
		if len(cmds) != 0 {
			t.Fatal(cmds)
		}
		return err
	})
}
