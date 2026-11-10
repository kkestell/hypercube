package session

import (
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
	"github.com/kkestell/hypercube/internal/report"
)

// useUtilityModel makes test/m/low the utility model.
func useUtilityModel(t *testing.T) {
	t.Helper()
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	c.UtilitySelection = &harness.Settings{Harness: "test", Model: "m", Effort: "low"}
	if err := config.Save(c); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) generated() []harnesstest.Entry {
	return harnesstest.Log(f.t, f.logs, "generate", "generate")
}

func TestUtilityModelNamesANewSession(t *testing.T) {
	f := newFixture(t)
	useUtilityModel(t)
	s, err := f.rt.Create(f.workspace.ID, lead, false, "Fix the flaky parser test")
	if err != nil {
		t.Fatal(err)
	}
	f.rt.utilityCalls.Wait()
	if got := f.storedSession(s.ID).Title; got != harnesstest.UtilityTitle {
		t.Errorf("title = %q", got)
	}
	got := f.generated()
	if len(got) != 1 || got[0].Model != "m" || got[0].Effort != "low" || !strings.Contains(got[0].Text, "Fix the flaky parser test") {
		t.Errorf("request = %+v", got)
	}
}

func TestWithoutAUtilityModelTheTitleIsTheFirstMessage(t *testing.T) {
	f := newFixture(t)
	s, err := f.rt.Create(f.workspace.ID, lead, false, "Fix the flaky parser test")
	if err != nil {
		t.Fatal(err)
	}
	f.rt.utilityCalls.Wait()
	if got := f.storedSession(s.ID).Title; got != report.Title("Fix the flaky parser test") {
		t.Errorf("title = %q", got)
	}
	if got := f.generated(); len(got) != 0 {
		t.Errorf("generate calls = %+v", got)
	}
}

func TestAFailedUtilityCallKeepsTheTitle(t *testing.T) {
	f := newFixture(t)
	useUtilityModel(t)
	s, err := f.rt.Create(f.workspace.ID, lead, false, "!utility-fail")
	if err != nil {
		t.Fatal(err)
	}
	f.rt.utilityCalls.Wait()
	if got := f.storedSession(s.ID).Title; got != "!utility-fail" {
		t.Errorf("title = %q", got)
	}
	if got := f.generated(); len(got) != 1 {
		t.Errorf("generate calls = %+v", got)
	}
}

func TestApplyTitle(t *testing.T) {
	f := newFixture(t)
	renamed, err := f.rt.Create(f.workspace.ID, lead, false, "first")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := f.rt.Create(f.workspace.ID, lead, false, "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Rename(renamed.ID, "Mine"); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.applyTitle(renamed.ID, "Generated"); err != nil {
		t.Fatal(err)
	}
	if got := f.storedSession(renamed.ID); got.Title != "Mine" || !got.Renamed {
		t.Errorf("renamed session = %q, renamed %v", got.Title, got.Renamed)
	}
	if err := f.rt.applyTitle(plain.ID, "\"Generated.\""); err != nil {
		t.Fatal(err)
	}
	if got := f.storedSession(plain.ID); got.Title != "Generated" || got.Renamed {
		t.Errorf("session = %q, renamed %v", got.Title, got.Renamed)
	}
	if err := f.rt.Delete(plain.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.applyTitle(plain.ID, "Late"); err != nil {
		t.Errorf("deleted session: %v", err)
	}
}

func TestUtilityModelNamesAChildFromItsBrief(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	useUtilityModel(t)
	child, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "worker", Title: "Initial", Message: "Review the lexer"})
	if err != nil {
		t.Fatal(err)
	}
	f.rt.utilityCalls.Wait()
	if got := f.storedSession(child.ID).Title; got != harnesstest.UtilityTitle {
		t.Errorf("title = %q", got)
	}
	if got := f.generated(); len(got) != 1 || !strings.Contains(got[0].Text, "Review the lexer") {
		t.Errorf("request = %+v", got)
	}
}

func TestQuitCancelsAHeldUtilityCall(t *testing.T) {
	f := newFixture(t)
	useUtilityModel(t)
	s, err := f.rt.Create(f.workspace.ID, lead, false, "!utility-hold")
	if err != nil {
		t.Fatal(err)
	}
	f.until("the utility call started", func() bool { return len(f.generated()) == 1 })
	done := make(chan struct{})
	go func() { f.rt.Quit(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("Quit did not return")
	}
	if got := f.storedSession(s.ID).Title; got != "!utility-hold" {
		t.Errorf("title = %q", got)
	}
}
