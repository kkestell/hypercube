package session

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// pauseStatus holds the first Git status in dir through its fsmonitor hook.
func (f *fixture) pauseStatus(dir string) (wait, release func()) {
	f.t.Helper()
	return f.pauseGit(dir, "status")
}

// pauseGit holds status or checkout through a repository-local hook. The
// barrier outlives the runtime, and cleanup releases it even after failure.
func (f *fixture) pauseGit(dir, command string) (wait, release func()) {
	t := f.t
	t.Helper()
	barrier, err := os.MkdirTemp(f.dir, "git")
	if err != nil {
		t.Fatal(err)
	}
	pipes := make([]*os.File, 2)
	for i, name := range []string{"reached", "release"} {
		path := filepath.Join(barrier, name)
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		pipes[i], err = os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	script := fmt.Sprintf("#!/bin/sh\nHYPERCUBE_TEST_GIT_DIR=%s\nHYPERCUBE_TEST_GIT_BARRIER=%s\n", quote(dir), quote(barrier))
	path := filepath.Join(barrier, "fsmonitor")
	condition := `[ "$PWD" = "$HYPERCUBE_TEST_GIT_DIR" ]`
	if command == "checkout" {
		path = filepath.Join(barrier, "post-checkout")
		condition = `[ "$1" = 0000000000000000000000000000000000000000 ]`
	}
	script += "if " + condition + ` && mkdir "$HYPERCUBE_TEST_GIT_BARRIER/claimed" 2>/dev/null; then
 printf 'ready\n' > "$HYPERCUBE_TEST_GIT_BARRIER/reached"
 read response < "$HYPERCUBE_TEST_GIT_BARRIER/release"
fi
`
	if command == "status" {
		// A failed fsmonitor hook asks Git to scan every file normally.
		script += "exit 1\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if command == "status" {
		f.git(dir, "config", "core.fsmonitor", path)
	} else {
		f.git(dir, "config", "core.hooksPath", barrier)
	}
	reached := make(chan error, 1)
	go func() {
		var b [6]byte
		_, err := pipes[0].Read(b[:])
		reached <- err
	}()
	var once sync.Once
	release = func() {
		once.Do(func() {
			key := "core.fsmonitor"
			if command == "checkout" {
				key = "core.hooksPath"
			}
			// Only the held command needs the hook. Leaving it installed
			// would spawn a shell for every later index read and status.
			out, err := exec.Command("git", "-C", dir, "config", "--unset", key).CombinedOutput()
			_, _ = pipes[1].Write([]byte("continue\n"))
			if err != nil {
				t.Errorf("remove Git hook: %v: %s", err, out)
			}
		})
	}
	t.Cleanup(func() {
		// Claiming the barrier keeps a later command from waiting on the
		// closed pipes.
		_ = os.Mkdir(filepath.Join(barrier, "claimed"), 0o700)
		release()
		for _, p := range pipes {
			p.Close()
		}
	})
	return func() {
		t.Helper()
		select {
		case err := <-reached:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Git %s did not reach the barrier", command)
		}
	}, release
}

func TestFamilyRemovalClaimsMembershipBeforeCheckingWorktrees(t *testing.T) {
	t.Parallel()
	for _, archive := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete", true: "archive"}[archive], func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			parent := f.worktreeSession()
			// The configure reply follows the first turn's merge refresh, so
			// the barrier can only catch this test's cleanup.
			if err := f.rt.Select(parent.ID, harness.Selection{Model: "m", Effort: "low"}); err != nil {
				t.Fatal(err)
			}
			wait, release := f.pauseStatus(parent.Dir)
			removed := make(chan error, 1)
			go func() {
				if archive {
					removed <- f.rt.Archive(parent.ID)
				} else {
					removed <- f.rt.Delete(parent.ID)
				}
			}()
			wait()
			_, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "late", Title: "Late child", Dir: "", Message: ""})
			if !errors.Is(err, errDeleting) {
				t.Fatalf("child creation during cleanup = %v, want errDeleting", err)
			}
			if _, err := f.rt.Shell(parent.ID, 80, 24); !errors.Is(err, errDeleting) {
				t.Fatalf("shell admission during cleanup = %v, want errDeleting", err)
			}
			release()
			if err := <-removed; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFamilyRemovalWaitsForAdmittedChildCreation(t *testing.T) {
	t.Parallel()
	// Creation waiting is shared by Delete and Archive. Exercise each creation
	// outcome once, rather than repeating the same barriers for both callers.
	for _, tc := range []struct {
		archive bool
		outcome string
	}{{false, "success"}, {true, "missing-plugin"}} {
		archive, outcome := tc.archive, tc.outcome
		t.Run(map[bool]string{false: "delete", true: "archive"}[archive]+"/"+outcome, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			parent := f.worktreeSession()
			options := ChildOptions{Name: "worker", Title: "Worker"}
			if outcome == "missing-plugin" {
				options.Settings = harness.Settings{Harness: "missing", Model: "m"}
			}
			wait, release := f.pauseGit(parent.Dir, "checkout")
			type creation struct {
				child store.Session
				err   error
			}
			created := make(chan creation, 1)
			go func() {
				child, err := f.rt.CreateChild(parent.ID, options)
				created <- creation{child, err}
			}()
			wait()
			var child store.Session
			f.read(func(tx *store.Tx) error {
				children, err := tx.Children(parent.ID)
				if err == nil && len(children) == 1 {
					child = children[0]
				}
				return err
			})
			removed := make(chan error, 1)
			go func() {
				if archive {
					removed <- f.rt.Archive(parent.ID)
				} else {
					removed <- f.rt.Delete(parent.ID)
				}
			}()
			f.until("the parent removal claim", func() bool {
				f.rt.mu.Lock()
				defer f.rt.mu.Unlock()
				return f.rt.deleting[parent.ID]
			})
			release()
			select {
			case result := <-created:
				if outcome == "success" {
					if result.err != nil || result.child.ID != child.ID {
						t.Fatalf("creation = %+v, %v", result.child, result.err)
					}
				} else if result.err == nil {
					t.Fatal("creation unexpectedly succeeded")
				} else if outcome == "missing-plugin" && !strings.Contains(result.err.Error(), `harness "missing" has no plugin`) {
					t.Fatalf("creation failed for the wrong reason: %v", result.err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("child creation did not finish")
			}
			select {
			case err := <-removed:
				if err != nil {
					t.Fatal("first removal request failed:", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("family removal did not finish")
			}
			for _, member := range []store.Session{parent, child} {
				if _, err := os.Stat(member.Dir); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("worktree %s survived: %v", member.Dir, err)
				}
				out, err := exec.Command("git", "-C", f.root, "branch", "--list", member.Branch).CombinedOutput()
				if err != nil || len(out) != 0 {
					t.Errorf("branch %s survived: %s, %v", member.Branch, out, err)
				}
			}
			family := f.family(parent.ID)
			want := 0
			if archive {
				want = 1
				if got := f.summary(parent.ID); got != "user:start|text:ack" {
					t.Errorf("archive changed the parent chat: %q", got)
				}
			}
			if len(family) != want {
				t.Fatalf("remaining family = %+v, want %d members", family, want)
			}
			for _, member := range family {
				if !member.Ended {
					t.Errorf("archived member %s is not ended", member.ID)
				}
			}
		})
	}
}

func TestEndChildKeepsWorkWrittenByItsStoppingShell(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "writer", "")
	ready := filepath.Join(t.TempDir(), "ready")
	f.setenv("HYPERCUBE_TEST_SHELL_READY", ready)
	f.rt.Terminals.Command = []string{"/bin/sh", "-c", `trap 'printf final > shutdown-write; exit' HUP
printf ready > "$HYPERCUBE_TEST_SHELL_READY"
while read line; do :; done`}
	if _, err := f.rt.Shell(child.ID, 80, 24); err != nil {
		t.Fatal(err)
	}
	f.until("the shell's signal handler", func() bool { _, err := os.Stat(ready); return err == nil })
	if _, err := f.rt.EndChild(parent.ID, child.Name); err == nil {
		t.Fatal("ending the child discarded work written while its shell stopped")
	}
	data, err := os.ReadFile(filepath.Join(child.Dir, "shutdown-write"))
	if err != nil || string(data) != "final" {
		t.Fatalf("shutdown work = %q, %v", data, err)
	}
	if f.storedSession(child.ID).Ended || !f.shellEnded(child.ID) {
		t.Fatal("cleanup must keep the child and stop its writers after a late refusal")
	}
	if err := os.Remove(filepath.Join(child.Dir, "shutdown-write")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.EndChild(parent.ID, child.Name); err != nil {
		t.Fatal("retrying cleanup:", err)
	}
}

func TestEndChildBlocksShellAdmissionDuringItsChecks(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "ending", "")
	wait, release := f.pauseStatus(child.Dir)
	ended := make(chan error, 1)
	go func() { _, err := f.rt.EndChild(parent.ID, child.Name); ended <- err }()
	wait()
	if _, err := f.rt.Shell(child.ID, 80, 24); !errors.Is(err, errDeleting) {
		t.Fatalf("shell admission during child cleanup = %v, want errDeleting", err)
	}
	release()
	if err := <-ended; err != nil {
		t.Fatal(err)
	}
	if !f.storedSession(child.ID).Ended {
		t.Fatal("the child did not end")
	}
}

// A refusal during the initial checks must release the claim without holding
// queued messages or suppressing the running agent's normal relaunch.
func TestRefusedRemovalKeepsTheFamilyWorking(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	parent := f.worktreeSession()
	if err := f.rt.Select(parent.ID, harness.Selection{Model: "m", Effort: "low"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent.Dir, "unfinished"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	wait, release := f.pauseStatus(parent.Dir)
	removed := make(chan error, 1)
	go func() { removed <- f.rt.Delete(parent.ID) }()
	wait()
	f.queue("user", parent.ID, "during the checks")
	f.until("delivery during the initial checks", func() bool { return len(f.queued(parent.ID)) == 0 })
	f.input(parent.ID, "!crash")
	f.until("normal relaunch during the checks", func() bool {
		return len(f.log(parent.ID, "start")) == 2 && f.idle(parent.ID)
	})
	release()
	if err := <-removed; err == nil {
		t.Fatal("removed a dirty worktree")
	}
	if _, err := f.rt.Shell(parent.ID, 80, 24); err != nil {
		t.Fatal("the refusal kept its shell admission claim:", err)
	}
	if got := f.storedSession(parent.ID); got.Ended || got.Activity == store.Exited {
		t.Fatalf("refused removal changed the family: %+v", got)
	}
}

func TestPartialFamilyRemovalRecordsEachCompletedChild(t *testing.T) {
	t.Parallel()
	for _, archive := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete", true: "archive"}[archive], func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			parent := f.worktreeSession()
			first := f.child(parent, "first", "")
			second := f.child(parent, "second", "")
			direct, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "direct", Title: "Direct child", Dir: ".", Message: ""})
			if err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			// Git checks the second worktree again after removing the first.
			// Its repository-local hook then introduces real work that must not
			// be lost. No process-global Git wrapper is needed.
			quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
			script := fmt.Sprintf("#!/bin/sh\nif [ ! -d %s ] && [ \"$PWD\" = %s ]; then\n printf preserved > %s\n git config --unset core.fsmonitor\nfi\nexit 1\n", quote(first.Dir), quote(second.Dir), quote(filepath.Join(second.Dir, "uncommitted")))
			hook := filepath.Join(bin, "fsmonitor")
			if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			f.git(f.root, "config", "core.fsmonitor", hook)
			remove := func() error {
				if archive {
					return f.rt.Archive(parent.ID)
				}
				return f.rt.Delete(parent.ID)
			}
			if err := remove(); err == nil {
				t.Fatal("expected the second worktree removal to fail")
			}
			if _, err := os.Stat(first.Dir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("first worktree was not removed: %v", err)
			}
			f.read(func(tx *store.Tx) error {
				s, err := tx.Session(first.ID)
				if archive {
					if err != nil || !s.Ended {
						t.Errorf("completed child was not archived: %+v, %v", s, err)
					}
				} else if !errors.Is(err, store.ErrNotFound) {
					t.Errorf("completed child was not deleted: %+v, %v", s, err)
				}
				return nil
			})
			for _, member := range []store.Session{parent, second, direct} {
				if f.storedSession(member.ID).Ended {
					t.Errorf("unfinished cleanup ended %s", member.Name)
				}
				if _, err := os.Stat(member.Dir); err != nil {
					t.Errorf("unfinished cleanup removed %s: %v", member.Dir, err)
				}
			}
			scratch := filepath.Join(second.Dir, "uncommitted")
			if err := remove(); err == nil {
				t.Fatal("retry removed the remaining dirty child")
			}
			if data, err := os.ReadFile(scratch); err != nil || string(data) != "preserved" {
				t.Fatalf("remaining child's work = %q, %v", data, err)
			}
			if err := os.Remove(scratch); err != nil {
				t.Fatal(err)
			}
			if err := remove(); err != nil {
				t.Fatal("retrying cleanup after fixing the remaining child:", err)
			}
			if archive {
				for _, member := range f.family(parent.ID) {
					if !member.Ended {
						t.Errorf("successful archive left %s active", member.Name)
					}
				}
			} else if members := f.family(parent.ID); len(members) != 0 {
				t.Errorf("successful delete left family: %+v", members)
			}
		})
	}
}

// pausedRefusal starts deleting a family whose dirty worktree refuses removal,
// runs during while the worktree check holds the claim, then waits for the
// refusal.
func (f *fixture) pausedRefusal(parent store.Session, during func()) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(parent.Dir, "unfinished"), []byte("keep"), 0o644); err != nil {
		f.t.Fatal(err)
	}
	wait, release := f.pauseStatus(parent.Dir)
	removed := make(chan error, 1)
	go func() { removed <- f.rt.Delete(parent.ID) }()
	wait()
	during()
	release()
	if err := <-removed; err == nil {
		f.t.Fatal("removed a dirty worktree")
	}
}

func TestRefusedRemovalWakesQueuedRecipients(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.idleAfter(time.Hour)
	parent := f.worktreeSession()
	child := f.child(parent, "worker", "")
	if err := f.rt.Select(parent.ID, harness.Selection{Model: "m", Effort: "low"}); err != nil {
		t.Fatal(err)
	}
	f.until("the child idle", func() bool { return f.idle(child.ID) })
	f.park(parent.ID)
	f.park(child.ID)
	f.pausedRefusal(parent, func() {
		f.queue("user", parent.ID, "to the parent")
		if _, err := f.rt.SendMessage(parent.ID, child.Name, "to the child"); err != nil {
			t.Fatal(err)
		}
	})
	f.until("both woken for their messages", func() bool {
		return f.awake(parent.ID) && f.awake(child.ID) && len(f.queued(parent.ID)) == 0 && len(f.queued(child.ID)) == 0
	})
}

func TestRefusedRemovalRestartsTheIdleTimeout(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.idleAfter(time.Hour)
	parent := f.worktreeSession()
	child := f.child(parent, "worker", "")
	if err := f.rt.Select(parent.ID, harness.Selection{Model: "m", Effort: "low"}); err != nil {
		t.Fatal(err)
	}
	f.until("the child idle", func() bool { return f.idle(child.ID) })
	f.pausedRefusal(parent, func() {
		f.idleAfter(50 * time.Millisecond)
		// A command reply during the checks cancels the claimed parent's timer.
		if err := f.rt.Select(parent.ID, harness.Selection{Model: "m", Effort: "low"}); err != nil {
			t.Fatal(err)
		}
	})
	f.untilParked(parent.ID)
	f.untilParked(child.ID)
}
