package mcp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestToolsFor(t *testing.T) {
	if got := toolsFor(store.State{}); &got[0] != &tools[0] {
		t.Error("toolsFor changed the tools of a Git workspace")
	}
	for _, tl := range toolsFor(store.State{NoGit: true}) {
		if tl.Name == "create_agent" {
			if req := tl.InputSchema["required"].([]string); !slices.Equal(req, []string{"name", "dir"}) {
				t.Errorf("create_agent requires %q", req)
			}
		}
	}
	for _, tl := range tools {
		if tl.Name == "create_agent" && strings.Contains(tl.Description, "dir") {
			t.Error("toolsFor changed the shared tools")
		}
	}
}

// noGit returns the state path of a workspace outside Git with an
// orchestrator in orchDir and, if workerDir is set, a worker in it.
func noGit(t *testing.T, orchDir, workerDir string) string {
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
		s.NoGit = true
		s.Agents = []store.Agent{{ID: "a", Name: "orchestrator", Role: store.Orchestrator, Dir: orchDir}}
		if workerDir != "" {
			s.Agents = append(s.Agents, store.Agent{ID: "b", Name: "w1", Role: store.Worker, Dir: workerDir, Target: "%999"})
		}
		return nil
	})
	return path
}

func TestCreateAgentNoGit(t *testing.T) {
	path := noGit(t, t.TempDir(), "")
	for _, args := range []map[string]string{{"name": "w1"}, {"name": "w1", "dir": "missing"}} {
		if _, err := createAgent("a", path, args); err == nil {
			t.Errorf("createAgent(%v) succeeded", args)
		}
		if s, _ := store.Load(path); len(s.Agents) != 1 {
			t.Errorf("after createAgent(%v): %+v", args, s.Agents)
		}
	}
}

func TestRemoveAgentNoGit(t *testing.T) {
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("TMUX", "")
	workerDir := t.TempDir()
	file := filepath.Join(workerDir, "keep")
	os.WriteFile(file, nil, 0o644)
	path := noGit(t, t.TempDir(), workerDir)

	if _, err := removeAgent("a", path, "w1"); err != nil {
		t.Fatal(err)
	}
	if s, _ := store.Load(path); len(s.Agents) != 1 {
		t.Errorf("after removeAgent: %+v", s.Agents)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("removeAgent touched the worker's directory: %v", err)
	}
}
