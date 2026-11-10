package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestWorkspaceDeletionClaimsMembershipBeforeCheckingWorktrees(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := f.worktreeSession()
	f.rt.Terminals.Command = []string{"/bin/sleep", "300"}
	if _, err := f.rt.WorkspaceShell(f.workspace.ID, 80, 24); err != nil {
		t.Fatal(err)
	}
	// The configure reply follows the first turn's merge refresh, so only
	// deletion can reach the status barrier.
	if err := f.rt.Select(s.ID, harness.Selection{Model: "m", Effort: "low"}); err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(s.Dir, "unfinished")
	if err := os.WriteFile(scratch, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	wait, release := f.pauseStatus(s.Dir)
	removed := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		removed <- f.rt.DeleteWorkspace(f.workspace.ID)
	}()
	t.Cleanup(func() { release(); <-done })
	wait()
	if _, err := f.rt.WorkspaceShell(f.workspace.ID, 80, 24); !errors.Is(err, errWorkspaceDeleting) {
		t.Fatalf("shell during deletion = %v", err)
	}
	if _, err := f.rt.Create(f.workspace.ID, lead, true, "late"); !errors.Is(err, errWorkspaceDeleting) {
		t.Fatalf("creation during deletion = %v, want errWorkspaceDeleting", err)
	}
	if err := f.rt.DeleteWorkspace(f.workspace.ID); !errors.Is(err, errWorkspaceDeleting) {
		t.Fatalf("concurrent deletion = %v, want errWorkspaceDeleting", err)
	}
	release()
	if err := <-removed; err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("deletion of dirty worktree = %v", err)
	}
	if f.shellEnded(workspaceShellOwner(f.workspace.ID)) {
		t.Fatal("refused deletion stopped workspace shell")
	}
	if got := f.storedSession(s.ID); got.Dir != s.Dir || got.Ended {
		t.Fatalf("refused deletion changed the session: %+v", got)
	}
	if data, err := os.ReadFile(scratch); err != nil || string(data) != "keep" {
		t.Fatalf("refused deletion changed the worktree: %q, %v", data, err)
	}
	late, err := f.rt.Create(f.workspace.ID, lead, true, "after refusal")
	if err != nil {
		t.Fatal("creation after deletion refused:", err)
	}
	if err := os.Remove(scratch); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.DeleteWorkspace(f.workspace.ID); err != nil {
		t.Fatal("retrying deletion:", err)
	}
	for _, member := range []store.Session{s, late} {
		if len(f.family(member.ID)) != 0 {
			t.Errorf("session %s remains", member.ID)
		}
		if _, err := os.Stat(member.Dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("worktree %s remains: %v", member.Dir, err)
		}
	}
}

func TestWorkspaceDeletionRefusesUntilEveryCreationFinishes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	wait, release := f.pauseWorktreeAdd()
	var s store.Session
	created := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var err error
		s, err = f.rt.Create(f.workspace.ID, lead, true, "paused creation")
		created <- err
	}()
	t.Cleanup(func() { release(); <-done })
	wait()
	if err := f.rt.DeleteWorkspace(f.workspace.ID); !errors.Is(err, errWorkspaceCreating) {
		t.Fatalf("deletion before registration = %v, want errWorkspaceCreating", err)
	}
	// A second creation can finish without releasing the first admission.
	second, err := f.rt.Create(f.workspace.ID, lead, false, "second creation")
	if err != nil {
		t.Fatal("concurrent creation:", err)
	}
	if err := f.rt.DeleteWorkspace(f.workspace.ID); !errors.Is(err, errWorkspaceCreating) {
		t.Fatalf("deletion while one creation remains = %v, want errWorkspaceCreating", err)
	}
	other, err := f.rt.AddWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.Create(other.ID, lead, false, "unrelated workspace"); err != nil {
		t.Fatal("creation in another workspace:", err)
	}
	if err := f.rt.DeleteWorkspace(other.ID); err != nil {
		t.Fatal("deletion in another workspace:", err)
	}
	release()
	if err := <-created; err != nil {
		t.Fatal(err)
	}
	if got := f.storedSession(s.ID); got.Dir != s.Dir || got.Base == "" {
		t.Fatalf("created session = %+v", got)
	}
	if _, err := os.Stat(s.Dir); err != nil {
		t.Fatal("created worktree:", err)
	}
	if err := f.rt.DeleteWorkspace(f.workspace.ID); err != nil {
		t.Fatal("deletion after both creations:", err)
	}
	if len(f.family(s.ID)) != 0 || len(f.family(second.ID)) != 0 {
		t.Fatal("deletion left created sessions")
	}
	if _, err := os.Stat(s.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deletion left the created worktree: %v", err)
	}
}
