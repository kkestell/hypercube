package mcp

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestToolsFor(t *testing.T) {
	create := func(list []tool) tool {
		for _, tl := range list {
			if tl.Name == "create_agent" {
				return tl
			}
		}
		return tool{}
	}
	profiles := []harness.Profile{{Name: "sol"}, {Name: "haiku"}}
	git := create(toolsFor(store.State{}, profiles)).InputSchema
	if req := git["required"].([]string); !slices.Equal(req, []string{"name", "profile"}) {
		t.Errorf("in Git, create_agent requires %q", req)
	}
	if enum := git["properties"].(map[string]any)["profile"].(map[string]any)["enum"]; !slices.Equal(enum.([]string), []string{"sol", "haiku"}) {
		t.Errorf("profile enum = %v", enum)
	}
	if req := create(toolsFor(store.State{NoGit: true}, profiles)).InputSchema["required"].([]string); !slices.Equal(req, []string{"name", "profile", "dir"}) {
		t.Errorf("outside Git, create_agent requires %q", req)
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
  "profiles": [{"name": "sol", "description": "coding.", "harness": "codex", "model": "gpt-6.1-sol", "effort": "medium"}]
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
	for _, args := range []map[string]string{{"name": "w1", "profile": "sol"}, {"name": "w1", "profile": "sol", "dir": "missing"}, {"name": "w1", "dir": "."}, {"name": "w1", "profile": "nope", "dir": "."}} {
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
