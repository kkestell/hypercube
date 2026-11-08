package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestWorkspaceShellSurvivesRestoreWithoutSessions(t *testing.T) {
	f := newFixture(t)
	marker := filepath.Join(t.TempDir(), "shell-directory")
	t.Setenv("HYPERCUBE_TEST_SHELL_DIRECTORY", marker)
	f.rt.Terminals.Command = []string{"/bin/sh", "-c", `pwd > "$HYPERCUBE_TEST_SHELL_DIRECTORY"; exec /bin/cat`}
	sh, err := f.rt.WorkspaceShell(f.workspace.ID, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	f.until("shell working directory", func() bool { data, _ := os.ReadFile(marker); return strings.TrimSpace(string(data)) == f.root })
	again, err := f.rt.WorkspaceShell(f.workspace.ID, 100, 30)
	if err != nil || again != sh {
		t.Fatalf("reconnect replaced shell: %v", err)
	}
	owner := workspaceShellOwner(f.workspace.ID)
	f.restart()
	if f.shellEnded(owner) {
		t.Fatal("restoration removed a workspace shell with no sessions")
	}
	// A restored running holder must be reused, even if the launch command changes.
	f.rt.Terminals.Command = []string{"/does/not/exist"}
	if _, err := f.rt.WorkspaceShell(f.workspace.ID, 80, 24); err != nil {
		t.Fatal("restored shell:", err)
	}
	if err := f.rt.DeleteWorkspace(f.workspace.ID); err != nil {
		t.Fatal(err)
	}
	if !f.shellEnded(owner) {
		t.Fatal("workspace deletion left its shell")
	}
	if _, err := f.rt.WorkspaceShell(f.workspace.ID, 80, 24); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted workspace shell = %v", err)
	}
}

func TestWorkspaceShellEndsOnQuit(t *testing.T) {
	f := newFixture(t)
	f.rt.Terminals.Command = []string{"/bin/sleep", "300"}
	if _, err := f.rt.WorkspaceShell(f.workspace.ID, 80, 24); err != nil {
		t.Fatal(err)
	}
	f.rt.Quit()
	if !f.shellEnded(workspaceShellOwner(f.workspace.ID)) {
		t.Fatal("quit left workspace shell")
	}
	if _, err := f.rt.WorkspaceShell(f.workspace.ID, 80, 24); !errors.Is(err, errClosing) {
		t.Fatalf("shell after quit = %v", err)
	}
}

func TestWorkspaceShellIsAdoptedBeforeServingDeletion(t *testing.T) {
	f := newFixture(t)
	f.rt.Terminals.Command = []string{"/bin/sleep", "300"}
	if _, err := f.rt.WorkspaceShell(f.workspace.ID, 80, 24); err != nil {
		t.Fatal(err)
	}
	f.rt.Detach()
	f.rt = f.newRuntime()
	err := f.rt.Restore(func() {
		if err := f.rt.DeleteWorkspace(f.workspace.ID); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !f.shellEnded(workspaceShellOwner(f.workspace.ID)) {
		t.Fatal("restoration orphaned the deleted workspace's shell")
	}
}

func TestSessionShellIsAdoptedBeforeServingDeletion(t *testing.T) {
	f := newFixture(t)
	s := f.session()
	f.park(s.ID)
	f.rt.Terminals.Command = []string{"/bin/sleep", "300"}
	if _, err := f.rt.Shell(s.ID, 80, 24); err != nil {
		t.Fatal(err)
	}
	f.rt.Detach()
	f.rt = f.newRuntime()
	err := f.rt.Restore(func() {
		if err := f.rt.Delete(s.ID); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !f.shellEnded(s.ID) {
		t.Fatal("restoration orphaned the deleted session's shell")
	}
}
