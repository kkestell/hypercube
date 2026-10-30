package mcp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// shortState points the mux socket at a state directory with no server, apart
// from the user's. A socket path must stay under 104 bytes on macOS.
func shortState(t *testing.T) {
	t.Helper()
	short, err := os.MkdirTemp("/tmp", "hc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(short) })
	t.Setenv("XDG_STATE_HOME", short)
}

// team returns the state path of a workspace, outside Git if noGit is set,
// with a lead in leadDir and, if workerDir is set, a worker working
// directly in it.
func team(t *testing.T, noGit bool, leadDir, workerDir string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "hypercube"), 0o755)
	os.WriteFile(filepath.Join(dir, "hypercube", "config.json"), []byte(`{
  "lead": {"harness": "claude", "model": "opus", "effort": "high"},
  "profiles": [{"name": "sol", "description": "coding.", "harness": "opencode", "model": "gpt-6.1-sol", "effort": "medium"}]
}`), 0o644)
	path := filepath.Join(dir, "w.json")
	store.Update(path, func(s *store.State) error {
		s.NoGit = noGit
		s.Agents = []store.Agent{{ID: "a", Name: "lead", Role: store.Lead, Dir: leadDir}}
		if workerDir != "" {
			s.Agents = append(s.Agents, store.Agent{ID: "b", Name: "w1", Role: store.Worker, Dir: workerDir, Target: "p1-0000"})
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
	shortState(t)
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

func TestSessionBoundaries(t *testing.T) {
	shortState(t)
	path := team(t, false, t.TempDir(), t.TempDir())
	if err := store.Update(path, func(s *store.State) error {
		s.Agents = append(s.Agents,
			store.Agent{ID: "other-lead", Name: "lead", Role: store.Lead, Team: "other"},
			store.Agent{ID: "other-worker", Name: "w1", Role: store.Worker, Team: "other"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, send := range [][2]string{{"a", "other/w1"}, {"b", "other/w1"}, {"b", "other/lead"}} {
		if _, err := sendMessage(send[0], path, send[1], "wrong session"); err == nil {
			t.Fatalf("%s reached %s across sessions", send[0], send[1])
		}
	}
	if _, err := removeAgent("a", path, "other/w1"); err == nil {
		t.Fatal("removed another session's worker")
	}
	if _, err := removeAgent("b", path, "w1"); err == nil {
		t.Fatal("worker removed an agent")
	}
	for _, send := range [][2]string{{"other-lead", "w1"}, {"a", "other/lead"}, {"other-lead", "default/lead"}} {
		if _, err := sendMessage(send[0], path, send[1], "allowed"); err != nil {
			t.Fatal(err)
		}
	}
	s, err := store.Load(path)
	if err != nil || len(s.Agents) != 4 || len(s.Messages) != 3 || s.Messages[0].To != "other-worker" || s.Messages[1].To != "other-lead" || s.Messages[2].To != "a" {
		t.Fatal(s, err)
	}
	if listed, _ := call("a", path, "list_agents", nil); !strings.Contains(listed, "other/lead") || strings.Contains(listed, "other/w1") {
		t.Fatal("lead's list_agents", listed)
	}
	if listed, _ := call("b", path, "list_agents", nil); strings.Contains(listed, "other/") {
		t.Fatal("worker's list_agents", listed)
	}
}
