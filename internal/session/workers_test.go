package session

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestWorkerLifecycle(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	worker := f.addWorker(lead, "w1", "the brief")
	if _, err := os.Stat(filepath.Join(worker.Dir, ".git")); err != nil {
		t.Fatal("no worktree:", err)
	}
	// The brief is the worker's first input.
	f.until("the brief", func() bool { return len(f.queued(worker.ID)) == 0 && len(f.inputs(worker.ID)) == 1 })
	if got := f.inputs(worker.ID)[0]; !strings.Contains(got, "the brief") || !strings.Contains(got, lead.Address()) {
		t.Fatalf("brief = %q", got)
	}
	// The transcript shows the brief as a message from the lead.
	if items := f.transcript(worker.ID); len(items) == 0 || items[0].From != lead.Address() || items[0].Text != "the brief" {
		t.Fatalf("brief item = %+v", items)
	}
	if start := f.log(worker.ID, "start"); start[0].Role != "worker" || start[0].Dir != worker.Dir {
		t.Fatalf("worker start = %v", start)
	}
	if _, err := f.rt.AddWorker(lead.ID, "w1", harness.Profile{Spec: harness.Spec{Harness: "test"}}, "", ""); err == nil {
		t.Fatal("a second worker took the name")
	}
	// A dirty worktree keeps its worker running.
	dirty := filepath.Join(worker.Dir, "unfinished")
	if err := os.WriteFile(dirty, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.RemoveWorker(lead.ID, "w1"); err == nil || len(f.agents(lead.Session)) != 2 || len(f.log(worker.ID, "shutdown")) != 0 {
		t.Fatalf("removing a dirty worker: %v", err)
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	pid := f.log(worker.ID, "start")[0].Pid
	if _, err := f.rt.RemoveWorker(lead.ID, "w1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worker.Dir); !os.IsNotExist(err) || len(f.agents(lead.Session)) != 1 || syscall.Kill(pid, 0) != syscall.ESRCH {
		t.Fatal("the worker, its worktree, or its process remains")
	}
	// A worker that cannot start leaves nothing behind.
	if _, err := f.rt.AddWorker(lead.ID, "w2", harness.Profile{Spec: harness.Spec{Harness: "missing"}}, "", ""); err == nil || !strings.Contains(err.Error(), "hypercube-harness-missing") {
		t.Fatalf("worker with no plugin: %v", err)
	}
	if len(f.agents(lead.Session)) != 1 {
		t.Fatal("the failed worker remains")
	}
	if _, err := os.Stat(filepath.Join(f.root, ".hypercube", lead.Session, "w2")); !os.IsNotExist(err) {
		t.Fatal("the failed worker's worktree remains")
	}
	if err := f.rt.Stop(lead.Session); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.AddWorker(lead.ID, "w3", harness.Profile{Spec: harness.Spec{Harness: "test"}}, ".", ""); err == nil {
		t.Fatal("a stopped session took a worker")
	}
}

// A worker can be created while another agent's request to its plugin is
// waiting, as when a native tool call reaches the MCP tools.
func TestWorkerCreatedWhileARequestWaits(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	held := make(chan error, 1)
	go func() { held <- f.rt.Input(lead.ID, "!hold") }()
	f.until("the held input", func() bool { return len(f.inputs(lead.ID)) == 2 })
	f.addWorker(lead, "w1", "brief")
	f.queue("user", lead.ID, "queued behind the held input")
	select {
	case err := <-held:
		t.Fatalf("the held input returned early: %v", err)
	default:
	}
	if err := f.rt.Interrupt(lead.ID); err != nil {
		t.Fatal(err)
	}
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	f.until("the queued message", func() bool { return len(f.queued(lead.ID)) == 0 })
}

func TestWorkerFailureReachesItsLead(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	other, otherLead := f.session()
	worker := f.addWorker(lead, "w1", "")
	f.input(worker.ID, "!fail")
	f.until("the failure message", func() bool {
		for _, in := range f.inputs(lead.ID) {
			if strings.Contains(in, "My turn failed: test provider failure") && strings.Contains(in, s.ID+"/w1") {
				return true
			}
		}
		return false
	})
	if got := strings.Join(f.inputs(otherLead.ID), "|"); got != "start" || f.agent(worker.ID).Activity != store.Idle {
		t.Fatalf("the failure reached %s's lead, or the worker is %s", other.ID, f.agent(worker.ID).Activity)
	}
}

func TestSetStatus(t *testing.T) {
	f := newFixture(t)
	f.seed(store.Agent{ID: "worker", Name: "worker"})
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	if err := f.rt.SetStatus("worker", "  Reading\nthe code. "); err != nil {
		t.Fatal(err)
	}
	if change := nextChange(t, changes, func(Change) bool { return true }); change.Agent != "" {
		t.Fatalf("status change = %+v", change)
	}
	if a := f.agent("worker"); a.Status != "Reading the code." || a.StatusAt == 0 {
		t.Errorf("status = %+v", a)
	}
	for _, c := range []struct{ agent, text string }{{"a", "Planning"}, {"worker", " \n"}, {"missing", "Working"}} {
		if err := f.rt.SetStatus(c.agent, c.text); err == nil {
			t.Fatalf("accepted status %+v", c)
		}
	}
}
