package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
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
	if _, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "w9", Title: " ", Dir: "", Message: ""}); err == nil {
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
	if _, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "w1", Title: "Title", Dir: "", Message: ""}); err == nil {
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
	if _, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "w2", Title: "Title", Settings: harness.Settings{Harness: "missing", Model: "m"}, Dir: "", Message: ""}); err == nil || !strings.Contains(err.Error(), `harness "missing" has no plugin`) {
		t.Fatalf("child with no plugin: %v", err)
	}
	if len(f.family(parent.ID)) != 2 {
		t.Fatal("the failed child remains")
	}
	if _, err := os.Stat(filepath.Join(f.root, ".hypercube", parent.ID, "w2")); !os.IsNotExist(err) {
		t.Fatal("the failed child's worktree remains")
	}
	f.park(parent.ID)
	if _, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "w3", Title: "Title", Dir: ".", Message: ""}); err == nil {
		t.Fatal("a parked session took a child")
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
	direct, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "direct", Title: "Title", Settings: lead, Dir: ".", Message: ""})
	if err != nil || direct.Dir != f.root || direct.Branch != "" {
		t.Fatalf("direct child=%+v,%v", direct, err)
	}
	if _, err := f.rt.CreateChild(direct.ID, ChildOptions{Name: "nested", Title: "Title", Settings: lead, Dir: ".", Message: ""}); err == nil {
		t.Fatal("child created a child")
	}
	outside, err := f.rt.AddWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	top, err := f.rt.Create(outside.ID, lead, false, "start")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.CreateChild(top.ID, ChildOptions{Name: "missing-dir", Title: "Title", Settings: lead, Dir: "", Message: ""}); err == nil || !strings.Contains(err.Error(), "dir is required") {
		t.Fatalf("non-Git child: %v", err)
	}
	child, err := f.rt.CreateChild(top.ID, ChildOptions{Name: "direct", Title: "Title", Settings: lead, Dir: ".", Message: ""})
	if err != nil || child.Dir != outside.Path || child.Branch != "" {
		t.Fatalf("non-Git child=%+v,%v", child, err)
	}
}

// A fork works where its parent works: in the original checkout or a folder
// outside Git, it shares the parent's folder; from a worktree session, it gets
// a new worktree from the parent's HEAD.
func TestForkPlacement(t *testing.T) {
	f := newFixture(t)
	fork := func(parent store.Session, name, dir string) (store.Session, error) {
		return f.rt.CreateChild(parent.ID, ChildOptions{Name: name, Title: "Fork", Settings: lead, Dir: dir, Fork: true})
	}
	checkout := f.session()
	if err := os.WriteFile(filepath.Join(f.root, "uncommitted"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	shared, err := fork(checkout, "shared", "")
	if err != nil || shared.Dir != f.root || shared.Branch != "" {
		t.Fatalf("fork of the original checkout = %+v, %v", shared, err)
	}
	for _, dir := range []string{".", t.TempDir()} {
		if _, err := fork(checkout, "elsewhere", dir); err == nil {
			t.Fatalf("a fork took the directory %s", dir)
		}
	}
	if family := f.family(checkout.ID); len(family) != 2 {
		t.Fatalf("rejected forks created children: %+v", family)
	}
	if _, err := f.rt.CreateChild(checkout.ID, ChildOptions{Name: "other", Title: "Fork", Settings: harness.Settings{Harness: "other", Model: "m"}, Fork: true}); err == nil {
		t.Fatal("a fork took another harness")
	}

	parent := f.worktreeSession()
	f.git(parent.Dir, "commit", "-q", "--allow-empty", "-m", "parent work")
	head, err := workspace.Head(parent.Dir)
	if err != nil {
		t.Fatal(err)
	}
	separate, err := fork(parent, "separate", "")
	if err != nil || separate.Dir == parent.Dir || separate.Branch == "" {
		t.Fatalf("fork of a worktree session = %+v, %v", separate, err)
	}
	if got, err := workspace.Head(separate.Dir); err != nil || got != head {
		t.Fatalf("fork worktree HEAD = %s, %v; want %s", got, err, head)
	}
	if _, err := fork(parent, "saved", "."); err == nil {
		t.Fatal("a fork of a worktree session took a folder")
	}

	outside, err := f.rt.AddWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	top, err := f.rt.Create(outside.ID, lead, false, "start")
	if err != nil {
		t.Fatal(err)
	}
	f.until("the session outside Git idle", func() bool { return f.idle(top.ID) && f.storedSession(top.ID).Conversation != "" })
	if plain, err := fork(top, "plain", ""); err != nil || plain.Dir != outside.Path {
		t.Fatalf("fork outside Git = %+v, %v", plain, err)
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
	if _, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: child.Name, Title: "Title", Dir: ".", Message: ""}); err == nil || !strings.Contains(err.Error(), "choose another") {
		t.Fatalf("reused child name: %v", err)
	}
	if err := f.wake(child.ID); !errors.Is(err, errEnded) || len(f.log(child.ID, "start")) != 1 {
		t.Fatalf("ended child woke: %v", err)
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

func TestDeleteRefusesAnActiveChild(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "child", "")
	if err := f.rt.Delete(child.ID); err == nil || !strings.Contains(err.Error(), "must be ended") {
		t.Fatalf("Delete(active child) = %v", err)
	}
	if _, err := os.Stat(child.Dir); err != nil || f.storedSession(child.ID).Activity == store.Exited {
		t.Fatalf("a refusal changed the child: %v", err)
	}
}

// A child cannot work in a worktree of the checkout that Hypercube did not
// create, because Hypercube would neither track nor remove it.
func TestChildRefusesForeignWorktree(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	foreign := filepath.Join(t.TempDir(), "foreign")
	f.git(f.root, "worktree", "add", "-q", "-b", "foreign", foreign, "HEAD")
	if _, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "w1", Title: "Title", Dir: foreign}); err == nil || !strings.Contains(err.Error(), "Hypercube does not manage") {
		t.Fatalf("child in a foreign worktree: %v", err)
	}
	f.read(func(tx *store.Tx) error {
		children, err := tx.Children(parent.ID)
		if err == nil && len(children) != 0 {
			t.Errorf("children = %+v", children)
		}
		return err
	})
}
