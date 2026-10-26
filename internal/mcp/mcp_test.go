package mcp

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestToolsFor(t *testing.T) {
	if got := toolsFor(store.State{}); &got[0] != &tools[0] {
		t.Error("toolsFor changed the tools of a Git workspace")
	}
	required := func(list []tool) []string {
		for _, tl := range list {
			if tl.Name == "create_agent" {
				return tl.InputSchema["required"].([]string)
			}
		}
		return nil
	}
	if req := required(toolsFor(store.State{NoGit: true})); !slices.Equal(req, []string{"name", "dir"}) {
		t.Errorf("outside Git, create_agent requires %q", req)
	}
	if req := required(tools); !slices.Equal(req, []string{"name"}) {
		t.Errorf("toolsFor changed the shared tools: create_agent requires %q", req)
	}
}

// team returns the state path of a workspace, outside Git if noGit is set,
// with an orchestrator in orchDir and, if workerDir is set, a worker working
// directly in it.
func team(t *testing.T, noGit bool, orchDir, workerDir string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "hypercube"), 0o755)
	os.WriteFile(filepath.Join(dir, "hypercube", "config.json"), []byte(`{
  "orchestrator": {"harness": "claude", "model": "opus", "effort": "high"},
  "worker": {"harness": "codex", "model": "gpt-6.1-sol", "effort": "medium"}
}`), 0o644)
	path := filepath.Join(dir, "w.json")
	store.Update(path, func(s *store.State) error {
		s.NoGit = noGit
		s.Agents = []store.Agent{{ID: "a", Name: "orchestrator", Role: store.Orchestrator, Dir: orchDir}}
		if workerDir != "" {
			s.Agents = append(s.Agents, store.Agent{ID: "b", Name: "w1", Role: store.Worker, Dir: workerDir, Target: "%999"})
		}
		return nil
	})
	return path
}

func TestCreateAgentNoGit(t *testing.T) {
	path := team(t, true, t.TempDir(), "")
	for _, args := range []map[string]string{{"name": "w1"}, {"name": "w1", "dir": "missing"}} {
		if _, err := createAgent("a", path, args); err == nil {
			t.Errorf("createAgent(%v) succeeded", args)
		}
		if s, _ := store.Load(path); len(s.Agents) != 1 {
			t.Errorf("after createAgent(%v): %+v", args, s.Agents)
		}
	}
}

// TestRemoveDirWorker removes a worker without a worktree, outside Git and
// in a Git workspace, and checks that its directory is left alone.
func TestRemoveDirWorker(t *testing.T) {
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("TMUX", "")
	for _, noGit := range []bool{true, false} {
		workerDir := t.TempDir()
		file := filepath.Join(workerDir, "keep")
		os.WriteFile(file, nil, 0o644)
		path := team(t, noGit, t.TempDir(), workerDir)

		if _, err := removeAgent("a", path, "w1"); err != nil {
			t.Fatal(err)
		}
		if s, _ := store.Load(path); len(s.Agents) != 1 {
			t.Errorf("noGit %v: after removeAgent: %+v", noGit, s.Agents)
		}
		if _, err := os.Stat(file); err != nil {
			t.Errorf("noGit %v: removeAgent touched the worker's directory: %v", noGit, err)
		}
	}
}
