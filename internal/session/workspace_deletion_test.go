package session

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestWorkspaceDeletionClaimsMembershipBeforeCheckingWorktrees(t *testing.T) {
	f := newFixture(t)
	s := f.worktreeSession()
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
	f := newFixture(t)
	wait, release := f.pauseGit(f.root, "worktree", "add")
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

func TestWorkspaceDeletionRefusesUntilFailedCreationCleansUp(t *testing.T) {
	f := newFixture(t)
	// Git cannot read a directory as its exclude file. Setup therefore fails
	// after creating the worktree, and must remove it and its branch.
	if err := os.Mkdir(filepath.Join(f.root, ".worktreeinclude"), 0o755); err != nil {
		t.Fatal(err)
	}
	wait, release := f.pauseGit(f.root, "worktree", "remove")
	created := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := f.rt.Create(f.workspace.ID, lead, true, "failed creation")
		created <- err
	}()
	t.Cleanup(func() { release(); <-done })
	wait()
	if err := f.rt.DeleteWorkspace(f.workspace.ID); !errors.Is(err, errWorkspaceCreating) {
		t.Fatalf("deletion during failure cleanup = %v, want errWorkspaceCreating", err)
	}
	release()
	if err := <-created; err == nil {
		t.Fatal("creation with invalid worktree setup succeeded")
	}
	cmd := exec.Command("git", "-C", f.root, "for-each-ref", "--format=%(refname)", "refs/heads/hypercube/")
	if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 {
		t.Fatalf("failed creation left branches: %q, %v", out, err)
	}
	if entries, err := os.ReadDir(filepath.Join(f.root, ".hypercube")); err != nil || len(entries) != 0 {
		t.Fatalf("failed creation left worktrees: %v, %v", entries, err)
	}
	if _, err := f.rt.Create(f.workspace.ID, lead, false, "after failed creation"); err != nil {
		t.Fatal("creation after failure:", err)
	}
	if err := f.rt.DeleteWorkspace(f.workspace.ID); err != nil {
		t.Fatal("deletion after failure cleanup:", err)
	}
}
