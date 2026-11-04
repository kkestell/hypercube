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

func TestChildLifecycle(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "w1", "the brief")
	if _, err := os.Stat(filepath.Join(child.Dir, ".git")); err != nil {
		t.Fatal("no worktree:", err)
	}
	if child.Name != "w1" || child.Title != "Title" {
		t.Fatalf("child name and title = %q, %q", child.Name, child.Title)
	}
	if _, err := f.rt.CreateChild(parent.ID, "w9", " ", harness.Profile{Harness: "test"}, "", ""); err == nil {
		t.Fatal("a child took a blank title")
	}
	// The brief is the child's first input.
	f.until("the brief", func() bool { return len(f.queued(child.ID)) == 0 && len(f.inputs(child.ID)) == 1 })
	if got := f.inputs(child.ID)[0]; !strings.Contains(got, "the brief") || !strings.Contains(got, parent.ID) {
		t.Fatalf("brief = %q", got)
	}
	// The transcript shows the brief as a message from the parent.
	if items := f.transcript(child.ID); len(items) == 0 || items[0].From != parent.ID || items[0].Text != "the brief" {
		t.Fatalf("brief item = %+v", items)
	}
	if start := f.log(child.ID, "start"); start[0].Dir != child.Dir {
		t.Fatalf("child start = %v", start)
	}
	if _, err := f.rt.CreateChild(parent.ID, "w1", "Title", harness.Profile{Harness: "test"}, "", ""); err == nil {
		t.Fatal("a second child took the name")
	}
	// A dirty worktree keeps its child running.
	dirty := filepath.Join(child.Dir, "unfinished")
	if err := os.WriteFile(dirty, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.EndChild(parent.ID, "w1"); err == nil || len(f.family(parent.ID)) != 2 || syscall.Kill(f.log(child.ID, "start")[0].Pid, 0) == syscall.ESRCH {
		t.Fatalf("ending a dirty child: %v", err)
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	pid := f.log(child.ID, "start")[0].Pid
	if _, err := f.rt.EndChild(parent.ID, "w1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(child.Dir); !os.IsNotExist(err) || len(f.family(parent.ID)) != 2 || syscall.Kill(pid, 0) != syscall.ESRCH {
		t.Fatal("the child, its worktree, or its process remains")
	}
	// A child that cannot start leaves nothing behind.
	if _, err := f.rt.CreateChild(parent.ID, "w2", "Title", harness.Profile{Harness: "missing"}, "", ""); err == nil || !strings.Contains(err.Error(), "hypercube-harness-missing") {
		t.Fatalf("child with no plugin: %v", err)
	}
	if len(f.family(parent.ID)) != 2 {
		t.Fatal("the failed child remains")
	}
	if _, err := os.Stat(filepath.Join(f.root, ".hypercube", parent.ID, "w2")); !os.IsNotExist(err) {
		t.Fatal("the failed child's worktree remains")
	}
	if err := f.rt.Stop(parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.CreateChild(parent.ID, "w3", "Title", harness.Profile{Harness: "test"}, ".", ""); err == nil {
		t.Fatal("a stopped session took a child")
	}
}

// A child can be created while another agent's request to its plugin is
// waiting, as when a native tool call reaches the MCP tools.
func TestChildCreatedWhileARequestWaits(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	held := make(chan error, 1)
	go func() { held <- f.rt.Input(parent.ID, "!hold") }()
	f.until("the held input", func() bool { return len(f.inputs(parent.ID)) == 2 })
	f.child(parent, "w1", "brief")
	f.queue("user", parent.ID, "queued behind the held input")
	select {
	case err := <-held:
		t.Fatalf("the held input returned early: %v", err)
	default:
	}
	if err := f.rt.Interrupt(parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	f.until("the queued message", func() bool { return len(f.queued(parent.ID)) == 0 })
}

func TestChildFailureReachesItsParent(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	other := f.session()
	child := f.child(parent, "w1", "")
	f.input(child.ID, "!fail")
	f.until("the failure message", func() bool {
		for _, in := range f.inputs(parent.ID) {
			if strings.Contains(in, "My turn failed: test provider failure") && strings.Contains(in, "Message from w1:") {
				return true
			}
		}
		return false
	})
	if got := strings.Join(f.inputs(other.ID), "|"); got != "start" || f.storedSession(child.ID).Activity != store.Idle {
		t.Fatalf("the failure reached %s, or the child is %s", other.ID, f.storedSession(child.ID).Activity)
	}
}

func TestChildDirectoriesAndScope(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	profile := harness.Profile{Harness: "test", Model: "m", Effort: "low"}
	direct, err := f.rt.CreateChild(parent.ID, "direct", "Title", profile, ".", "")
	if err != nil || direct.Dir != f.root || direct.Branch != "" {
		t.Fatalf("direct child=%+v,%v", direct, err)
	}
	if _, err := f.rt.CreateChild(direct.ID, "nested", "Title", profile, ".", ""); err == nil {
		t.Fatal("child created a child")
	}
	for _, action := range []func(string) error{f.rt.Start, f.rt.Stop, f.rt.Delete} {
		if err := action(direct.ID); err == nil {
			t.Fatal("independent lifecycle accepted for active child")
		}
	}
	outside, err := f.rt.AddWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	top, err := f.rt.Create(outside.ID, "lead", false, "start")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.CreateChild(top.ID, "missing-dir", "Title", profile, "", ""); err == nil || !strings.Contains(err.Error(), "dir is required") {
		t.Fatalf("non-Git child: %v", err)
	}
	child, err := f.rt.CreateChild(top.ID, "direct", "Title", profile, ".", "")
	if err != nil || child.Dir != outside.Path || child.Branch != "" {
		t.Fatalf("non-Git child=%+v,%v", child, err)
	}
}

func TestEndedChildKeepsChatAndCannotResume(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "child", "brief")
	f.until("the child reply", func() bool {
		return len(f.transcript(child.ID)) == 2 && f.idle(child.ID)
	})
	if err := f.db.Tx(func(tx *store.Tx) error { return tx.Queue(store.Message{From: parent.ID, To: child.ID, Text: "held"}) }); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.update(child.ID, func(s *store.Session) { s.Attention = true }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.EndChild(parent.ID, child.Name); err != nil {
		t.Fatal(err)
	}
	saved := f.storedSession(child.ID)
	if !saved.Ended || saved.Attention || saved.Dir != child.Dir || saved.Branch != child.Branch || saved.Activity != store.Exited || len(f.transcript(child.ID)) != 2 || len(f.queued(child.ID)) != 0 {
		t.Fatalf("ended child=%+v", saved)
	}
	if _, err := f.rt.SendMessage(parent.ID, child.Name, "too late"); err == nil {
		t.Fatal("messaged ended child")
	}
	if _, err := f.rt.CreateChild(parent.ID, child.Name, "Title", harness.Profile{Harness: "test"}, ".", ""); err == nil || !strings.Contains(err.Error(), "choose another") {
		t.Fatalf("reused child name: %v", err)
	}
	if err := f.rt.Stop(parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Start(parent.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.log(child.ID, "start")) != 1 {
		t.Fatal("ended child relaunched")
	}
	if err := f.rt.Delete(child.ID); err != nil {
		t.Fatal(err)
	}
	f.read(func(tx *store.Tx) error {
		if _, err := tx.Session(child.ID); err == nil {
			t.Fatal("deleted child remains")
		}
		_, err := tx.Session(parent.ID)
		return err
	})
}

func TestFamilyRestoresAndChildPromptReleasesQueues(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "child", "")
	f.rt.Terminals.Command = []string{"/bin/sleep", "300"}
	if _, err := f.rt.Shell(child.ID, 80, 24); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Stop(parent.ID); err != nil {
		t.Fatal(err)
	}
	if !f.shellEnded(child.ID) {
		t.Fatal("parent stop left child shell running")
	}
	if f.storedSession(child.ID).Activity != store.Exited {
		t.Fatal("parent stop left child running")
	}
	f.queue("user", parent.ID, "held for parent")
	f.queue(parent.ID, child.ID, "held for child")
	if err := f.rt.Start(parent.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.queued(parent.ID)) != 1 || len(f.queued(child.ID)) != 1 || !f.storedSession(parent.ID).AwaitingUser {
		t.Fatal("family queues not held")
	}
	f.input(child.ID, "continue")
	f.until("both family queues", func() bool { return len(f.queued(parent.ID)) == 0 && len(f.queued(child.ID)) == 0 })
	if f.storedSession(parent.ID).AwaitingUser {
		t.Fatal("child prompt did not release family")
	}
	f.rt.Quit()
	f.rt = f.newRuntime()
	f.rt.Restore()
	f.until("restored family", func() bool {
		return len(f.log(child.ID, "start")) == 3 && f.storedSession(child.ID).Activity == store.Idle
	})
	if !f.storedSession(parent.ID).AwaitingUser {
		t.Fatal("restored family not held")
	}
}

func TestDeleteParentSkipsEndedChildren(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "done", "")
	if _, err := f.rt.EndChild(parent.ID, child.Name); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Delete(parent.ID); err != nil {
		t.Fatal(err)
	}
	f.read(func(tx *store.Tx) error {
		if _, err := tx.Session(child.ID); err == nil {
			t.Fatal("ended child survived parent deletion")
		}
		return nil
	})
}

func TestChildFollowsItsParent(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "child", "")
	for name, run := range map[string]func(string) error{"Start": f.rt.Start, "Stop": f.rt.Stop} {
		if err := run(child.ID); err == nil || !strings.Contains(err.Error(), "starts and stops with its parent") {
			t.Errorf("%s(child) = %v", name, err)
		}
	}
	if err := f.rt.Delete(child.ID); err == nil || !strings.Contains(err.Error(), "must be ended") {
		t.Fatalf("Delete(active child) = %v", err)
	}
	if _, err := os.Stat(child.Dir); err != nil || f.storedSession(child.ID).Activity == store.Exited {
		t.Fatalf("a refusal changed the child: %v", err)
	}
}
