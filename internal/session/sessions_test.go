package session

import (
	"errors"
	"os"
	"os/exec"
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
	s, err := f.rt.Create(f.workspace.ID, "lead", false, "\n  Fix the  parser \nand the lexer")
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
	s, err = f.rt.Create(f.workspace.ID, "worker", false, "", png)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.storedSession(s.ID).Title; got != "Image" {
		t.Errorf("image-only title = %q", got)
	}
}

func TestCreateRefusesAnUnknownProfile(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"", "missing"} {
		if _, err := f.rt.Create(f.workspace.ID, name, false, "start"); err == nil || !strings.Contains(err.Error(), "no profile named") {
			t.Errorf("Create(%q) error = %v", name, err)
		}
	}
	f.read(func(tx *store.Tx) error {
		sessions, err := tx.Sessions(f.workspace.ID)
		if len(sessions) != 0 {
			t.Errorf("sessions = %v", sessions)
		}
		return err
	})
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
		if _, err := f.rt.Create(f.workspace.ID, "lead", false, c.text, c.images...); err == nil {
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

func TestParkedSessionWakesForInputAndMessages(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.park(s.ID)
	if a := f.storedSession(s.ID); a.Activity != store.Exited || a.Launch != "" || syscall.Kill(f.log(s.ID, "start")[0].Pid, 0) != syscall.ESRCH {
		t.Fatalf("after parking: %+v", a)
	}
	if got := f.summary(s.ID); got != "user:start|text:ack" {
		t.Fatalf("parking left a note: %q", got)
	}
	// A prompt wakes the session, resuming its conversation, and goes first.
	f.input(s.ID, "continue")
	start := f.log(s.ID, "start")
	if len(start) != 2 || start[1].Conversation != s.ID+"-conversation" {
		t.Fatalf("wake: %v", start)
	}
	f.until("the prompt", func() bool { return f.summary(s.ID) == "user:start|text:ack|user:continue|text:ack" && f.idle(s.ID) })
	// A queued message wakes it again.
	f.park(s.ID)
	f.queue("user", s.ID, "queued while parked")
	f.until("the queued message", func() bool { return len(f.inputs(s.ID)) == 3 && len(f.queued(s.ID)) == 0 })
	if got := f.inputs(s.ID); !strings.Contains(got[2], "queued while parked") || len(f.log(s.ID, "start")) != 3 {
		t.Fatalf("inputs = %q", got)
	}
}

func TestQuitLeavesSessionsParkedAndInputResumes(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.input(s.ID, "!work")
	f.until("a working session", func() bool { return f.storedSession(s.ID).Activity == store.Busy })
	pid := f.log(s.ID, "start")[0].Pid
	f.rt.Quit()
	// Quit saves the unfinished item and ends the process.
	if a := f.storedSession(s.ID); a.Activity != store.Exited || a.Launch != "" {
		t.Fatalf("after quit: %+v", a)
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
	if err := f.rt.Input(s.ID, "too late"); !errors.Is(err, errClosing) {
		t.Fatalf("a finished runtime woke a session: %v", err)
	}
	rt := f.newRuntime()
	f.rt = rt
	if err := rt.Restore(nil); err != nil {
		t.Fatal(err)
	}
	if len(f.log(s.ID, "start")) != 1 || f.awake(s.ID) {
		t.Fatal("restore woke a parked session")
	}
	f.input(s.ID, "continue")
	if start := f.log(s.ID, "start"); len(start) != 2 || start[1].Conversation != s.ID+"-conversation" {
		t.Fatalf("resume: %v", start)
	}
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
	open := func() string {
		if _, err := f.rt.Shell(s.ID, 80, 24); err != nil {
			t.Fatal(err)
		}
		return s.ID
	}
	sh := open()
	// A shell outlives its session's agent.
	f.park(s.ID)
	if f.shellEnded(sh) {
		t.Fatal("parking ended the shell")
	}
	sh = open()
	if err := f.rt.Delete(s.ID); err != nil || !f.shellEnded(sh) {
		t.Fatalf("Delete left the shell running: %v", err)
	}
	s = f.session()
	sh = open()
	f.rt.Quit()
	if !f.shellEnded(sh) {
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

// A session cannot wake while Delete waits for its plugins to end, so no
// plugin of it outlives the delete.
func TestDeletingSessionCannotStart(t *testing.T) {
	t.Setenv("HYPERCUBE_TEST_END_DELAY", "500ms")
	f := newFixture(t)
	s := f.session()

	deleted := make(chan error, 1)
	go func() { deleted <- f.rt.Delete(s.ID) }()
	f.until("delete to stop the session", func() bool {
		f.rt.mu.Lock()
		defer f.rt.mu.Unlock()
		l := f.rt.live[s.ID]
		return f.rt.deleting[s.ID] && (l == nil || l.stopping)
	})
	if err := f.wake(s.ID); !errors.Is(err, errDeleting) {
		t.Fatalf("wake during delete: %v", err)
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

// worktreeSession creates a worktree session from "start" and waits for its
// reply.
func (f *fixture) worktreeSession() store.Session {
	f.t.Helper()
	s, err := f.rt.Create(f.workspace.ID, "lead", true, "start")
	if err != nil {
		f.t.Fatal(err)
	}
	f.until("the worktree session idle", func() bool { return f.idle(s.ID) })
	return s
}

// merge waits for the session's measured merge status to satisfy check.
func (f *fixture) merge(id, what string, check func(workspace.Merge) bool) {
	f.t.Helper()
	f.until(what, func() bool {
		m, ok := f.rt.Merges()[id]
		return ok && check(m)
	})
}

func TestWorktreeSession(t *testing.T) {
	f := newFixture(t)
	head, err := workspace.Head(f.root)
	if err != nil {
		t.Fatal(err)
	}
	s := f.worktreeSession()
	dir, branch := workspace.SessionDir(f.root, s.ID), workspace.SessionBranch(s.ID)
	if got := f.storedSession(s.ID); got.Dir != dir || got.Branch != branch || got.Base != head {
		t.Fatalf("session = %+v", got)
	}
	if b, err := workspace.Branch(dir); err != nil || b != branch {
		t.Fatalf("worktree branch = %q, %v", b, err)
	}
	f.merge(s.ID, "no commits yet", func(m workspace.Merge) bool {
		return m == workspace.Merge{Branch: branch, Target: "main"}
	})

	f.git(dir, "commit", "-q", "--allow-empty", "-m", "work")
	f.input(s.ID, "continue")
	f.merge(s.ID, "one unmerged commit", func(m workspace.Merge) bool { return m.Unmerged == 1 && m.Committed })
	if err := f.rt.Delete(s.ID); err == nil || !strings.Contains(err.Error(), "not merged") {
		t.Fatalf("delete of an unmerged worktree session: %v", err)
	}
	if _, err := os.Stat(dir); err != nil || len(f.family(s.ID)) != 1 {
		t.Fatalf("refused delete changed the session: %v", err)
	}

	f.park(s.ID)
	if err := f.wake(s.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.storedSession(s.ID); got.Dir != dir || got.Branch != branch {
		t.Fatalf("after waking: %+v", got)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("parking removed the worktree: %v", err)
	}

	f.git(f.root, "merge", "-q", branch)
	f.until("the restarted session idle", func() bool { return f.idle(s.ID) })
	f.input(s.ID, "continue")
	f.merge(s.ID, "merged", func(m workspace.Merge) bool { return m.Unmerged == 0 && m.Committed })
	if err := f.rt.Delete(s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("worktree remains: %v", err)
	}
	if b, _ := workspace.Branch(f.root); b != "main" {
		t.Errorf("checkout branch = %q", b)
	}
	if len(f.family(s.ID)) != 0 {
		t.Error("session remains")
	}
	if exec.Command("git", "-C", f.root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil {
		t.Error("branch remains")
	}
}

func TestCreateRefusesAWorktreeOutsideGit(t *testing.T) {
	f := newFixture(t)
	outside, err := f.rt.AddWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.Create(outside.ID, "lead", true, "start"); err == nil {
		t.Fatal("created a worktree session outside Git")
	}
	f.read(func(tx *store.Tx) error {
		sessions, err := tx.Sessions(outside.ID)
		if err == nil && len(sessions) != 0 {
			t.Errorf("sessions = %+v", sessions)
		}
		return err
	})
}

func TestMergeStatusFailureStaysWithItsSession(t *testing.T) {
	f := newFixture(t)
	s := f.worktreeSession()
	f.park(s.ID)
	if err := os.RemoveAll(s.Dir); err != nil {
		t.Fatal(err)
	}
	other := f.session()
	f.input(other.ID, "continue")
	f.merge(s.ID, "the failed status", func(m workspace.Merge) bool { return m.Branch == s.Branch && m.Error != "" })
	f.until("the other session's turn", func() bool {
		return f.summary(other.ID) == "user:start|text:ack|user:continue|text:ack" && f.idle(other.ID)
	})
}

// A worktree session's child branches from the session's worktree, and both
// worktrees go once their work is merged.
func TestWorktreeSessionChild(t *testing.T) {
	f := newFixture(t)
	s := f.worktreeSession()
	f.git(s.Dir, "commit", "-q", "--allow-empty", "-m", "parent work")
	head, err := workspace.Head(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	child := f.child(s, "w1", "")
	if want := workspace.WorktreeDir(s.Dir, s.ID, "w1"); child.Dir != want {
		t.Fatalf("child dir = %s, want %s", child.Dir, want)
	}
	if got, err := workspace.Head(child.Dir); err != nil || got != head {
		t.Fatalf("child HEAD = %s, %v, want %s", got, err, head)
	}
	f.git(child.Dir, "commit", "-q", "--allow-empty", "-m", "child work")
	f.git(s.Dir, "merge", "-q", child.Branch)
	f.git(f.root, "merge", "-q", s.Branch)
	if err := f.rt.Delete(s.ID); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{child.Dir, s.Dir} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s remains: %v", dir, err)
		}
	}
	for _, branch := range []string{child.Branch, s.Branch} {
		if exec.Command("git", "-C", f.root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil {
			t.Errorf("branch %s remains", branch)
		}
	}
}

// A worktree session whose worktree was removed still deletes with its child,
// whose branch the checkout holds.
func TestDeleteWorktreeSessionWithoutItsWorktree(t *testing.T) {
	f := newFixture(t)
	s := f.worktreeSession()
	child := f.child(s, "w1", "")
	f.park(child.ID)
	f.park(s.ID)
	if err := os.RemoveAll(s.Dir); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Delete(s.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.family(s.ID)) != 0 {
		t.Error("the family remains")
	}
	for _, branch := range []string{child.Branch, s.Branch} {
		if exec.Command("git", "-C", f.root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil {
			t.Errorf("branch %s remains", branch)
		}
	}
}

func TestArchiveFamily(t *testing.T) {
	f := newFixture(t)
	s := f.worktreeSession()
	child := f.child(s, "worker", "brief")
	f.until("child reply", func() bool { return f.summary(child.ID) == "user:brief|text:ack" && f.idle(child.ID) })
	f.rt.Terminals.Command = []string{"/bin/sleep", "300"}
	if _, err := f.rt.Shell(child.ID, 80, 24); err != nil {
		t.Fatal(err)
	}
	before := map[string]string{s.ID: f.summary(s.ID), child.ID: f.summary(child.ID)}
	if err := f.rt.Archive(s.ID); err != nil {
		t.Fatal(err)
	}
	for _, member := range []store.Session{s, child} {
		if saved := f.storedSession(member.ID); !saved.Ended || f.summary(member.ID) != before[member.ID] {
			t.Errorf("archived session = %+v, transcript = %s", saved, f.summary(member.ID))
		}
		if syscall.Kill(f.log(member.ID, "start")[0].Pid, 0) != syscall.ESRCH {
			t.Errorf("agent %s remains", member.ID)
		}
		if _, err := os.Stat(member.Dir); !os.IsNotExist(err) {
			t.Errorf("worktree %s remains: %v", member.Dir, err)
		}
		if exec.Command("git", "-C", f.root, "rev-parse", "--verify", "--quiet", "refs/heads/"+member.Branch).Run() == nil {
			t.Errorf("branch %s remains", member.Branch)
		}
		if err := f.rt.Input(member.ID, "again"); err == nil {
			t.Error("archived session accepted input")
		}
	}
	if !f.shellEnded(child.ID) {
		t.Error("child shell remains")
	}
	if err := f.wake(s.ID); !errors.Is(err, errEnded) {
		t.Errorf("archived session woke: %v", err)
	}
}

func TestArchiveRefusesDirtyWorktree(t *testing.T) {
	f := newFixture(t)
	s := f.worktreeSession()
	child := f.child(s, "worker", "")
	if err := os.WriteFile(filepath.Join(child.Dir, "unfinished"), []byte("work"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.Archive(s.ID); err == nil {
		t.Fatal("archived a dirty worktree")
	}
	for _, member := range []store.Session{s, child} {
		if f.storedSession(member.ID).Ended || syscall.Kill(f.log(member.ID, "start")[0].Pid, 0) == syscall.ESRCH {
			t.Errorf("refusal changed %s", member.ID)
		}
		if _, err := os.Stat(member.Dir); err != nil {
			t.Errorf("refusal removed %s: %v", member.Dir, err)
		}
	}
}
