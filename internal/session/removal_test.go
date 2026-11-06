package session

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// pauseStatus holds the first completed Git status in dir. Both pipe ends
// stay open in the test so cleanup can release the command even after failure.
func pauseStatus(t *testing.T, dir string) (wait, release func()) {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	barrier := t.TempDir()
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
	script := `#!/bin/sh
if [ "$1" = status ] && [ "$PWD" = "$HYPERCUBE_TEST_STATUS_DIR" ] && mkdir "$HYPERCUBE_TEST_STATUS_BARRIER/claimed" 2>/dev/null; then
 "$HYPERCUBE_TEST_GIT" "$@"
 result=$?
 printf 'ready\n' > "$HYPERCUBE_TEST_STATUS_BARRIER/reached"
 read response < "$HYPERCUBE_TEST_STATUS_BARRIER/release"
 exit "$result"
fi
exec "$HYPERCUBE_TEST_GIT" "$@"
`
	if err := os.WriteFile(filepath.Join(barrier, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HYPERCUBE_TEST_GIT", git)
	t.Setenv("HYPERCUBE_TEST_STATUS_DIR", dir)
	t.Setenv("HYPERCUBE_TEST_STATUS_BARRIER", barrier)
	t.Setenv("PATH", barrier+":"+os.Getenv("PATH"))
	reached := make(chan error, 1)
	go func() {
		var b [6]byte
		_, err := pipes[0].Read(b[:])
		reached <- err
	}()
	var once sync.Once
	release = func() { once.Do(func() { _, _ = pipes[1].Write([]byte("continue\n")) }) }
	t.Cleanup(func() {
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
			t.Fatal("Git status did not reach the barrier")
		}
	}, release
}

func TestFamilyRemovalClaimsMembershipBeforeCheckingWorktrees(t *testing.T) {
	for _, archive := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete", true: "archive"}[archive], func(t *testing.T) {
			f := newFixture(t)
			parent := f.worktreeSession()
			// The configure reply follows the first turn's merge refresh, so
			// the barrier can only catch this test's cleanup.
			if err := f.rt.SelectProfile(parent.ID, "lead"); err != nil {
				t.Fatal(err)
			}
			wait, release := pauseStatus(t, parent.Dir)
			removed := make(chan error, 1)
			go func() {
				if archive {
					removed <- f.rt.Archive(parent.ID)
				} else {
					removed <- f.rt.Delete(parent.ID)
				}
			}()
			wait()
			_, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "late", Title: "Late child", Profile: harness.Profile{Harness: "test"}, Dir: "", Message: ""})
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

func TestEndChildKeepsWorkWrittenByItsStoppingShell(t *testing.T) {
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "writer", "")
	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv("HYPERCUBE_TEST_SHELL_READY", ready)
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
	f := newFixture(t)
	parent := f.session()
	child := f.child(parent, "ending", "")
	wait, release := pauseStatus(t, child.Dir)
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
	f := newFixture(t)
	parent := f.worktreeSession()
	if err := f.rt.SelectProfile(parent.ID, "lead"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent.Dir, "unfinished"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	wait, release := pauseStatus(t, parent.Dir)
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
	for _, archive := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete", true: "archive"}[archive], func(t *testing.T) {
			f := newFixture(t)
			parent := f.worktreeSession()
			first := f.child(parent, "first", "")
			second := f.child(parent, "second", "")
			direct, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: "direct", Title: "Direct child", Profile: harness.Profile{Harness: "test"}, Dir: ".", Message: ""})
			if err != nil {
				t.Fatal(err)
			}
			git, err := exec.LookPath("git")
			if err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			script := `#!/bin/sh
if [ "$1" = worktree ] && [ "$2" = remove ] && [ "$4" = "$HYPERCUBE_TEST_DENY_DIR" ]; then
 printf preserved > "$HYPERCUBE_TEST_DENY_DIR/uncommitted"
 echo 'test worktree removal failure' >&2
 exit 1
fi
exec "$HYPERCUBE_TEST_REAL_GIT" "$@"
`
			if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HYPERCUBE_TEST_REAL_GIT", git)
			t.Setenv("HYPERCUBE_TEST_DENY_DIR", second.Dir)
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
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
			t.Setenv("HYPERCUBE_TEST_DENY_DIR", "")
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
	wait, release := pauseStatus(f.t, parent.Dir)
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
	f := newFixture(t)
	f.idleAfter(time.Hour)
	parent := f.worktreeSession()
	child := f.child(parent, "worker", "")
	if err := f.rt.SelectProfile(parent.ID, "lead"); err != nil {
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
	f := newFixture(t)
	f.idleAfter(time.Hour)
	parent := f.worktreeSession()
	child := f.child(parent, "worker", "")
	if err := f.rt.SelectProfile(parent.ID, "lead"); err != nil {
		t.Fatal(err)
	}
	f.until("the child idle", func() bool { return f.idle(child.ID) })
	f.pausedRefusal(parent, func() {
		f.idleAfter(50 * time.Millisecond)
		// A command reply during the checks cancels the claimed parent's timer.
		if err := f.rt.SelectProfile(parent.ID, "lead"); err != nil {
			t.Fatal(err)
		}
	})
	f.untilParked(parent.ID)
	f.untilParked(child.ID)
}
