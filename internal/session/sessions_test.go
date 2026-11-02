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

func TestCreateSendsTheFirstPrompt(t *testing.T) {
	f := newFixture(t)
	s, err := f.rt.Create(f.workspace.ID, "lead", "\n  Fix the  parser \nand the lexer")
	parent := s
	if err != nil {
		t.Fatal(err)
	}
	if got := f.storedSession(s.ID).Title; got != "Fix the parser" {
		t.Errorf("title = %q", got)
	}
	if got := f.inputs(parent.ID); len(got) != 1 || got[0] != "\n  Fix the  parser \nand the lexer" {
		t.Errorf("plugin inputs = %q", got)
	}
	f.until("the prompt in the transcript", func() bool {
		items := f.transcript(parent.ID)
		return len(items) > 0 && items[0].Kind == harness.KindUser && items[0].Text == "\n  Fix the  parser \nand the lexer"
	})
	png := harness.Image{MIMEType: "image/png", Data: "iVBORw0KGgo="}
	s, err = f.rt.Create(f.workspace.ID, "lead", "", png)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.storedSession(s.ID).Title; got != "Image" {
		t.Errorf("image-only title = %q", got)
	}
}

func TestCreateRefusesABadPromptBeforeMakingAnything(t *testing.T) {
	f := newFixture(t)
	for _, c := range []struct {
		text   string
		images []harness.Image
	}{
		{"", nil},
		{" \n", nil},
		{"", []harness.Image{{MIMEType: "image/png", Data: "bm90IGEgcG5n"}}},
	} {
		if _, err := f.rt.Create(f.workspace.ID, "lead", c.text, c.images...); err == nil {
			t.Errorf("Create accepted %q and %d images", c.text, len(c.images))
		}
	}
	f.read(func(tx *store.Tx) error {
		sessions, err := tx.Sessions(f.workspace.ID)
		if err == nil && len(sessions) != 0 {
			t.Errorf("sessions = %v", sessions)
		}
		return err
	})
	if entries, _ := os.ReadDir(f.logs); len(entries) != 0 {
		t.Errorf("a plugin started: %v", entries)
	}
}

func TestRestoredSessionHoldsMessagesUntilThePrompt(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	if err := f.rt.Stop(s.ID); err != nil {
		t.Fatal(err)
	}
	if a := f.storedSession(s.ID); a.Activity != store.Exited || f.storedSession(s.ID).Running || syscall.Kill(f.log(s.ID, "start")[0].Pid, 0) != syscall.ESRCH {
		t.Fatalf("after stop: %+v", a)
	}
	if err := f.rt.Input(s.ID, "to a stopped agent"); err == nil {
		t.Fatal("input reached a stopped agent")
	}
	f.queue("user", s.ID, "queued while stopped")
	if err := f.rt.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	f.until("the session idle", func() bool { return f.storedSession(s.ID).Activity == store.Idle })
	start := f.log(s.ID, "start")
	if len(start) != 2 || start[1].Conversation != s.ID+"-conversation" || !f.storedSession(s.ID).AwaitingUser {
		t.Fatalf("restart: %v awaiting=%v", start, f.storedSession(s.ID).AwaitingUser)
	}
	// A delivery attempt sends nothing until the user's prompt, which goes
	// first; then the queue.
	f.rt.Deliver(s.ID)
	f.input(s.ID, "continue")
	f.until("the held message", func() bool { return len(f.inputs(s.ID)) == 3 && len(f.queued(s.ID)) == 0 })
	if got := f.inputs(s.ID); got[1] != "continue" || !strings.Contains(got[2], "queued while stopped") {
		t.Fatalf("inputs = %q", got)
	}
	if got := f.storedSession(s.ID); got.AwaitingUser || got.Title != "start" {
		t.Fatalf("session = %+v", got)
	}
}

func TestShutdownKeepsTheRunningMarkAndRestoreResumes(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.input(s.ID, "!work")
	f.until("a working session", func() bool { return f.storedSession(s.ID).Activity == store.Busy })
	pid := f.log(s.ID, "start")[0].Pid
	f.rt.Quit()
	// Shutdown saves the unfinished item, ends the process, and leaves the
	// session marked running.
	if a := f.storedSession(s.ID); a.Activity != store.Exited || !f.storedSession(s.ID).Running {
		t.Fatalf("after shutdown: %+v", a)
	}
	if got := f.summary(s.ID); got != "user:start|text:ack|user:!work|text:working" {
		t.Fatalf("transcript = %q", got)
	}
	saved, err := f.rt.Transcript(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("the plugin process remains: %v", err)
	}
	if err := f.rt.Start(s.ID); err == nil {
		t.Fatal("a finished runtime started a session")
	}
	rt := f.newRuntime()
	f.rt = rt
	rt.Restore()
	f.until("the restored session", func() bool { return f.storedSession(s.ID).Activity == store.Idle })
	if start := f.log(s.ID, "start"); len(start) != 2 || start[1].Conversation != s.ID+"-conversation" || !f.storedSession(s.ID).AwaitingUser {
		t.Fatalf("restore: %v", start)
	}
	f.input(s.ID, "continue")
	f.until("a turn after restore", func() bool {
		return f.summary(s.ID) == "user:start|text:ack|user:!work|text:working|user:continue|text:ack"
	})
	items, err := rt.Transcript(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range items {
		if i < len(saved) && (item.ID != saved[i].ID || item.Seq != saved[i].Seq) {
			t.Fatalf("saved item moved: %+v became %+v", saved[i], item)
		}
		if i > 0 && item.Seq <= items[i-1].Seq {
			t.Fatalf("transcript positions are not increasing: %+v", items)
		}
	}
}

func TestDeletingASessionEndsItsAgents(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	child := f.child(s, "w1", "")
	f.input(child.ID, "!child")
	var childPID int
	f.until("the child's subprocess", func() bool {
		for _, e := range f.log(child.ID, "child") {
			childPID = e.Pid
		}
		return childPID != 0
	})
	if err := f.rt.Delete(s.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.family(s.ID)) != 0 || syscall.Kill(f.log(s.ID, "start")[0].Pid, 0) != syscall.ESRCH || syscall.Kill(f.log(child.ID, "start")[0].Pid, 0) != syscall.ESRCH {
		t.Fatal("the session or its children remain")
	}
	f.until("the child to end", func() bool { return syscall.Kill(childPID, 0) == syscall.ESRCH })
	if _, err := os.Stat(child.Dir); !os.IsNotExist(err) {
		t.Fatal("the worktree remains")
	}
}

func TestShellsEndWithTheirSession(t *testing.T) {
	f := newFixture(t)
	f.rt.Terminals.Command = []string{"/bin/sleep", "300"}
	s := f.session()
	open := func() interface{ Done() <-chan struct{} } {
		sh, err := f.rt.Shell(s.ID, 80, 24)
		if err != nil {
			t.Fatal(err)
		}
		return sh
	}
	ended := func(sh interface{ Done() <-chan struct{} }) bool {
		select {
		case <-sh.Done():
			return true
		default:
			return false
		}
	}
	sh := open()
	if err := f.rt.Stop(s.ID); err != nil || !ended(sh) {
		t.Fatalf("Stop left the shell running: %v", err)
	}
	if _, err := f.rt.Shell(s.ID, 80, 24); err == nil {
		t.Fatal("a stopped session opened a shell")
	}
	if err := f.rt.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	sh = open()
	if err := f.rt.Delete(s.ID); err != nil || !ended(sh) {
		t.Fatalf("Delete left the shell running: %v", err)
	}
	s = f.session()
	sh = open()
	f.rt.Quit()
	if !ended(sh) {
		t.Fatal("Shutdown left the shell running")
	}
}

func TestWorkspaceAndSessionEdits(t *testing.T) {
	f := newFixture(t)
	if w, err := f.rt.AddWorkspace(f.root); err != nil || w.ID != f.workspace.ID {
		t.Fatalf("existing workspace = %+v, %v", w, err)
	}
	if _, err := f.rt.AddWorkspace(filepath.Join(f.root, "missing")); err == nil {
		t.Fatal("added a missing directory")
	}
	s := f.session()
	if err := f.rt.Rename(s.ID, "  Changed  "); err != nil || f.storedSession(s.ID).Title != "Changed" {
		t.Fatalf("rename: %v", err)
	}
	if err := f.rt.Rename(s.ID, " \n"); err == nil {
		t.Fatal("blank title accepted")
	}
	if err := f.rt.Seen(s.ID); err != nil || f.storedSession(s.ID).Unseen {
		t.Fatalf("seen: %v", err)
	}
	if err := f.rt.Rename("missing", "Title"); err == nil {
		t.Fatal("renamed a missing session")
	}
	if err := f.rt.Seen("missing"); err == nil {
		t.Fatal("marked a missing session seen")
	}
}

// Delete refuses, changing nothing, while a child's branch is unmerged or its
// worktree is dirty, and otherwise removes worktrees, branches, and the session.
func TestDelete(t *testing.T) {
	f := newFixture(t)
	dir, branch := workspace.WorktreeDir(f.root, "s", "w1"), workspace.ChildBranch("s", "w1")
	if err := workspace.AddWorktree(f.root, dir, branch); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	f.seed(store.Session{ID: "b", Name: "w1", Dir: dir, Branch: branch}, store.Session{ID: "c", Name: "w2", Dir: other})

	f.git(dir, "commit", "-q", "--allow-empty", "-m", "work")
	if err := f.rt.Delete("s"); err == nil {
		t.Fatal("delete deleted an unmerged branch")
	}
	f.git(f.root, "merge", "-q", branch)
	scratch := filepath.Join(dir, "scratch")
	if err := os.WriteFile(scratch, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Delete("s"); err == nil {
		t.Fatal("delete deleted a dirty worktree")
	}
	if n := len(f.family("s")); n != 3 {
		t.Errorf("refused delete left %d sessions", n)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("refused delete removed the worktree: %v", err)
	}

	if err := os.Remove(scratch); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Delete("s"); err != nil {
		t.Fatal(err)
	}
	if n := len(f.family("s")); n != 0 {
		t.Errorf("delete left %d sessions", n)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Errorf("%s remains", dir)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("delete touched a child's own directory: %v", err)
	}
}

func TestDeleteMissingCheckout(t *testing.T) {
	f := newFixture(t)
	f.seed(store.Session{ID: "b", Name: "w", Dir: filepath.Join(f.root, ".hypercube/s/w"), Branch: "hypercube/s/w"})
	if err := os.RemoveAll(f.root); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.DeleteWorkspace(f.workspace.ID); err != nil {
		t.Fatal(err)
	}
	f.read(func(tx *store.Tx) error {
		list, err := tx.Workspaces()
		if err == nil && len(list) != 0 {
			t.Errorf("workspaces = %+v", list)
		}
		return err
	})
}

// A session cannot start while Delete waits for its plugins to end, so no
// plugin of it outlives the delete.
func TestDeletingSessionCannotStart(t *testing.T) {
	t.Setenv("HYPERCUBE_TEST_END_DELAY", "500ms")
	f := newFixture(t)
	s := f.session()

	deleted := make(chan error, 1)
	go func() { deleted <- f.rt.Delete(s.ID) }()
	f.until("delete to claim the session", func() bool { f.rt.mu.Lock(); defer f.rt.mu.Unlock(); return f.rt.deleting[s.ID] })
	if err := f.rt.Start(s.ID); !errors.Is(err, errDeleting) {
		t.Fatalf("start during delete: %v", err)
	}
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	if n := len(f.log(s.ID, "start")); n != 1 {
		t.Fatalf("the session started %d times", n)
	}
	f.rt.mu.Lock()
	defer f.rt.mu.Unlock()
	if len(f.rt.live) != 0 {
		t.Fatal("an agent of the deleted session is running")
	}
}
