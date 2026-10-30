package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// deleteWorkspace refuses, changing nothing, while a worker's branch is unmerged or its
// worktree is dirty, and otherwise removes worktrees, branches, and the state.
func TestDelete(t *testing.T) {
	tmp, _ := os.MkdirTemp("/tmp", "hc")
	defer os.RemoveAll(tmp)
	t.Setenv("TMUX_TMPDIR", tmp)
	t.Setenv("TMUX", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(root, "init", "-q", "-b", "main")
	git(root, "commit", "-q", "--allow-empty", "-m", "init")
	dir, branch := workspace.WorktreeDir(root, "w1"), workspace.WorkerBranch("w1")
	if err := workspace.AddWorktree(root, dir, branch); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	name := workspace.Name(root)
	path := store.Path(name)
	store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{
			{ID: "a", Name: "orchestrator", Role: store.Orchestrator, Dir: root},
			{ID: "b", Name: "w1", Role: store.Worker, Dir: dir, Branch: branch},
			{ID: "c", Name: "w2", Role: store.Worker, Dir: other},
		}
		return nil
	})

	git(dir, "commit", "-q", "--allow-empty", "-m", "work")
	if err := deleteWorkspace(root, name); err == nil {
		t.Fatal("delete deleted an unmerged branch")
	}
	git(root, "merge", "-q", branch)
	os.WriteFile(filepath.Join(dir, "scratch"), nil, 0o644)
	if err := deleteWorkspace(root, name); err == nil {
		t.Fatal("delete deleted a dirty worktree")
	}
	if s, _ := store.Load(path); len(s.Agents) != 3 {
		t.Errorf("refused delete changed the state: %+v", s.Agents)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("refused delete removed the worktree: %v", err)
	}

	os.Remove(filepath.Join(dir, "scratch"))
	if err := deleteWorkspace(root, name); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, filepath.Join(store.Dir(), name+".lock"), dir} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s remains", p)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("delete touched a worker's own directory: %v", err)
	}
}

// Saved teams keep their sessions and queue; unsupported state and a running
// previous host are refused.
func TestSavedStateCompatibility(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "w.json")
	for _, source := range []string{
		`{"agents":[{"id":"o","role":"orchestrator","session_id":"saved"}],"messages":[{"from":"user","to":"o","text":"kept"}],"tasks":null}`,
		`{"agents":[{"id":"o","role":"orchestrator"},{"id":"other","role":"orchestrator","team":"docs"}],"messages":[],"tasks":[]}`,
		`{"agents":[{"id":"o","role":"orchestrator"}],"tasks":[{"name":"nightly","schedule":"0 3 * * *","prompt":"check"}]}`,
	} {
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		s, err := store.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		err = oneTeam(&s)
		if strings.Contains(source, `"session_id"`) {
			if err != nil || s.Agents[0].SessionID != "saved" || len(s.Messages) != 1 {
				t.Fatal(s, err)
			}
		} else if err == nil {
			t.Fatal("unsupported saved state accepted")
		}
	}
	hostPath := strings.TrimSuffix(path, ".json") + ".host.lock"
	lock, err := store.LockFile(hostPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := legacyStopped(path); err == nil {
		t.Fatal("a running previous host was accepted")
	}
}

func TestDeleteMissingCheckout(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("TMUX", "")
	root := filepath.Join(t.TempDir(), "gone")
	path := store.Path(workspace.Name(root))
	if err := store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "o", Role: store.Orchestrator, Dir: root}, {ID: "w", Role: store.Worker, Dir: filepath.Join(root, ".hypercube/w"), Branch: "hypercube/w"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := stopOrDelete("delete", []string{root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("missing checkout's state remains", err)
	}
}
