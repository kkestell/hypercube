package session

import (
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
	s, lead, err := f.rt.Create(f.workspace.ID, "lead", "\n  Fix the  parser \nand the lexer")
	if err != nil {
		t.Fatal(err)
	}
	if got := f.storedSession(s.ID).Title; got != "Fix the parser" {
		t.Errorf("title = %q", got)
	}
	if got := f.inputs(lead.ID); len(got) != 1 || got[0] != "\n  Fix the  parser \nand the lexer" {
		t.Errorf("plugin inputs = %q", got)
	}
	f.until("the prompt in the transcript", func() bool {
		items := f.transcript(lead.ID)
		return len(items) > 0 && items[0].Kind == harness.KindUser && items[0].Text == "\n  Fix the  parser \nand the lexer"
	})
	png := harness.Image{MIMEType: "image/png", Data: "iVBORw0KGgo="}
	s, lead, err = f.rt.Create(f.workspace.ID, "lead", "", png)
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
		if _, _, err := f.rt.Create(f.workspace.ID, "lead", c.text, c.images...); err == nil {
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
	s, lead := f.session()
	if err := f.rt.Stop(s.ID); err != nil {
		t.Fatal(err)
	}
	if a := f.agent(lead.ID); a.Activity != store.Exited || f.storedSession(s.ID).Running || len(f.log(lead.ID, "shutdown")) != 1 {
		t.Fatalf("after stop: %+v", a)
	}
	if err := f.rt.Input(lead.ID, "to a stopped agent"); err == nil {
		t.Fatal("input reached a stopped agent")
	}
	f.queue("user", lead.ID, "queued while stopped")
	if err := f.rt.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	f.until("the lead idle", func() bool { return f.agent(lead.ID).Activity == store.Idle })
	start := f.log(lead.ID, "start")
	if len(start) != 2 || start[1].Conversation != lead.ID+"-conversation" || !f.storedSession(s.ID).AwaitingUser {
		t.Fatalf("restart: %v awaiting=%v", start, f.storedSession(s.ID).AwaitingUser)
	}
	// A delivery attempt sends nothing until the user's prompt, which goes
	// first; then the queue.
	f.rt.Deliver(lead.ID)
	f.input(lead.ID, "continue")
	f.until("the held message", func() bool { return len(f.inputs(lead.ID)) == 3 && len(f.queued(lead.ID)) == 0 })
	if got := f.inputs(lead.ID); got[1] != "continue" || !strings.Contains(got[2], "queued while stopped") {
		t.Fatalf("inputs = %q", got)
	}
	if got := f.storedSession(s.ID); got.AwaitingUser || got.Title != "start" {
		t.Fatalf("session = %+v", got)
	}
}

func TestShutdownKeepsTheRunningMarkAndRestoreResumes(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	f.input(lead.ID, "!work")
	f.until("a working lead", func() bool { return f.agent(lead.ID).Activity == store.Busy })
	pid := f.log(lead.ID, "start")[0].Pid
	f.rt.Shutdown()
	// Shutdown saves the unfinished item, ends the process, and leaves the
	// session marked running.
	if a := f.agent(lead.ID); a.Activity != store.Exited || !f.storedSession(s.ID).Running {
		t.Fatalf("after shutdown: %+v", a)
	}
	if got := f.summary(lead.ID); got != "user:start|text:ack|user:!work|text:working" {
		t.Fatalf("transcript = %q", got)
	}
	saved, err := f.rt.Transcript(lead.ID)
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
	f.until("the restored lead", func() bool { return f.agent(lead.ID).Activity == store.Idle })
	if start := f.log(lead.ID, "start"); len(start) != 2 || start[1].Conversation != lead.ID+"-conversation" || !f.storedSession(s.ID).AwaitingUser {
		t.Fatalf("restore: %v", start)
	}
	f.input(lead.ID, "continue")
	f.until("a turn after restore", func() bool {
		return f.summary(lead.ID) == "user:start|text:ack|user:!work|text:working|user:continue|text:ack"
	})
	items, err := rt.Transcript(lead.ID)
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
	s, lead := f.session()
	worker := f.addWorker(lead, "w1", "")
	f.input(worker.ID, "!child")
	var child int
	f.until("the worker's child", func() bool {
		for _, e := range f.log(worker.ID, "child") {
			child = e.Pid
		}
		return child != 0
	})
	if err := f.rt.Delete(s.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.agents(s.ID)) != 0 || len(f.log(lead.ID, "shutdown")) != 1 || len(f.log(worker.ID, "shutdown")) != 1 {
		t.Fatal("the session or its agents remain")
	}
	f.until("the child to end", func() bool { return syscall.Kill(child, 0) == syscall.ESRCH })
	if _, err := os.Stat(worker.Dir); !os.IsNotExist(err) {
		t.Fatal("the worktree remains")
	}
}

func TestShellsEndWithTheirSession(t *testing.T) {
	f := newFixture(t)
	f.rt.Terminals.Command = []string{"/bin/sleep", "300"}
	s, _ := f.session()
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
	s, _ = f.session()
	sh = open()
	f.rt.Shutdown()
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
	s, _ := f.session()
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

// Delete refuses, changing nothing, while a worker's branch is unmerged or its
// worktree is dirty, and otherwise removes worktrees, branches, and the session.
func TestDelete(t *testing.T) {
	f := newFixture(t)
	dir, branch := workspace.WorktreeDir(f.root, "s/w1"), workspace.WorkerBranch("s/w1")
	if err := workspace.AddWorktree(f.root, dir, branch); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	f.seed(store.Agent{ID: "b", Name: "w1", Dir: dir, Branch: branch}, store.Agent{ID: "c", Name: "w2", Dir: other})

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
	if n := len(f.agents("s")); n != 3 {
		t.Errorf("refused delete left %d agents", n)
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
	if n := len(f.agents("s")); n != 0 {
		t.Errorf("delete left %d agents", n)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Errorf("%s remains", dir)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("delete touched a worker's own directory: %v", err)
	}
}

func TestDeleteMissingCheckout(t *testing.T) {
	f := newFixture(t)
	f.seed(store.Agent{ID: "b", Name: "w", Dir: filepath.Join(f.root, ".hypercube/s/w"), Branch: "hypercube/s/w"})
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
