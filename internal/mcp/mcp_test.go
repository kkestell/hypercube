package mcp

import (
	"bytes"
	"encoding/json"
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
	git := create(toolsFor(false, profiles)).InputSchema
	if req := git["required"].([]string); !slices.Equal(req, []string{"name", "profile"}) {
		t.Errorf("in Git, create_agent requires %q", req)
	}
	if enum := git["properties"].(map[string]any)["profile"].(map[string]any)["enum"]; !slices.Equal(enum.([]string), []string{"sol", "haiku"}) {
		t.Errorf("profile enum = %v", enum)
	}
	if req := create(toolsFor(true, profiles)).InputSchema["required"].([]string); !slices.Equal(req, []string{"name", "profile", "dir"}) {
		t.Errorf("outside Git, create_agent requires %q", req)
	}
}

// team returns a database holding a workspace, outside Git if noGit is set,
// with session session-1, whose lead "a" works in leadDir and, if workerDir is
// set, whose worker "b" works directly in it.
func team(t *testing.T, noGit bool, leadDir, workerDir string) *store.DB {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	os.MkdirAll(filepath.Join(dir, "hypercube"), 0o755)
	os.WriteFile(filepath.Join(dir, "hypercube", "config.json"), []byte(`{
  "profiles": [
    {"name": "opus", "role": "lead", "description": "leading.", "harness": "one", "model": "opus", "effort": "high"},
    {"name": "sol", "role": "work", "description": "coding.", "harness": "two", "model": "gpt-6.1-sol", "effort": "medium"}
  ]
}`), 0o644)
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	change(t, db, func(tx *store.Tx) error {
		tx.AddWorkspace(store.Workspace{ID: "w", Path: leadDir, NoGit: noGit})
		tx.AddSession(store.Session{ID: "session-1", Workspace: "w", Running: true})
		tx.AddAgent(store.Agent{ID: "a", Session: "session-1", Name: "lead", Role: store.Lead, Dir: leadDir, Activity: store.Idle})
		if workerDir != "" {
			return tx.AddAgent(store.Agent{ID: "b", Session: "session-1", Name: "w1", Role: store.Worker, Dir: workerDir, Pane: "%999", Activity: store.Idle})
		}
		return nil
	})
	return db
}

func change(t *testing.T, db *store.DB, fn func(*store.Tx) error) {
	t.Helper()
	if err := db.Tx(fn); err != nil {
		t.Fatal(err)
	}
}

func agents(t *testing.T, db *store.DB, session string) (list []store.Agent) {
	t.Helper()
	change(t, db, func(tx *store.Tx) error { var err error; list, err = tx.Agents(session); return err })
	return list
}

func TestToolsListWorkers(t *testing.T) {
	team(t, false, t.TempDir(), "")
	t.Setenv("HYPERCUBE_AGENT_ID", "a")
	var out bytes.Buffer
	if err := Serve(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result struct {
			Tools []tool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	for _, tl := range response.Result.Tools {
		if tl.Name != "create_agent" {
			continue
		}
		enum := tl.InputSchema["properties"].(map[string]any)["profile"].(map[string]any)["enum"].([]any)
		if len(enum) != 1 || enum[0] != "sol" {
			t.Fatalf("worker profile enum = %v", enum)
		}
		return
	}
	t.Fatal("create_agent missing")
}

// create_agent refuses bad arguments, a lead profile, and a stopped session,
// adding no agent.
func TestCreateAgentRefusals(t *testing.T) {
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("TMUX", "")
	db := team(t, true, t.TempDir(), "")
	for _, args := range []map[string]string{{"name": "w1", "profile": "sol"}, {"name": "w1", "profile": "sol", "dir": "missing"}, {"name": "w1", "dir": "."}, {"name": "w1", "profile": "nope", "dir": "."}, {"name": "w1", "profile": "opus", "dir": "."}, {"name": "w1", "profile": "sol", "dir": "."}} {
		if _, err := createAgent(db, "a", args); err == nil {
			t.Errorf("createAgent(%v) succeeded", args)
		}
		if list := agents(t, db, "session-1"); len(list) != 1 {
			t.Errorf("after createAgent(%v): %+v", args, list)
		}
	}
}

// remove_agent refuses a stopped session and leaves the worker.
func TestRemoveAgentStoppedSession(t *testing.T) {
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("TMUX", "")
	workerDir := t.TempDir()
	db := team(t, false, t.TempDir(), workerDir)
	if _, err := removeAgent(db, "a", "w1"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("removeAgent in a stopped session: %v", err)
	}
	if list := agents(t, db, "session-1"); len(list) != 2 {
		t.Errorf("after removeAgent: %+v", list)
	}
}

func TestSessionBoundaries(t *testing.T) {
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("TMUX", "")
	db := team(t, false, t.TempDir(), t.TempDir())
	change(t, db, func(tx *store.Tx) error {
		tx.AddSession(store.Session{ID: "other", Workspace: "w", Title: "Fix the parser"})
		tx.AddAgent(store.Agent{ID: "other-lead", Session: "other", Name: "lead", Role: store.Lead, Activity: store.Idle})
		tx.AddAgent(store.Agent{ID: "other-worker", Session: "other", Name: "w1", Role: store.Worker, Activity: store.Idle})
		tx.AddWorkspace(store.Workspace{ID: "far", Path: "/elsewhere"})
		tx.AddSession(store.Session{ID: "far", Workspace: "far"})
		return tx.AddAgent(store.Agent{ID: "far-lead", Session: "far", Name: "lead", Role: store.Lead, Activity: store.Idle})
	})
	for _, send := range [][2]string{{"a", "other/w1"}, {"b", "other/w1"}, {"b", "other/lead"}, {"a", "far/lead"}} {
		if _, err := sendMessage(db, send[0], send[1], "wrong session"); err == nil {
			t.Fatalf("%s reached %s across sessions", send[0], send[1])
		}
	}
	if _, err := removeAgent(db, "a", "other/w1"); err == nil {
		t.Fatal("removed another session's worker")
	}
	if _, err := removeAgent(db, "b", "w1"); err == nil {
		t.Fatal("worker removed an agent")
	}
	// Session "other" is stopped; session-1 runs.
	for _, send := range [][3]string{{"other-lead", "w1", "stopped"}, {"a", "other/lead", "stopped"}, {"other-lead", "session-1/lead", ""}} {
		reply, err := sendMessage(db, send[0], send[1], "allowed")
		if err != nil {
			t.Fatal(err)
		}
		if stopped := strings.Contains(reply, "stopped"); stopped != (send[2] == "stopped") {
			t.Errorf("reply to %s: %q", send[1], reply)
		}
	}
	change(t, db, func(tx *store.Tx) error {
		for id, want := range map[string]int{"other-worker": 1, "other-lead": 1, "a": 1, "far-lead": 0} {
			if m, _ := tx.Queued(id); len(m) != want {
				t.Errorf("%s has %d messages, want %d", id, len(m), want)
			}
		}
		return nil
	})
	if listed, _ := call(db, "a", "list_agents", nil); !strings.Contains(listed, "other/lead") || !strings.Contains(listed, `"session_title": "Fix the parser"`) || strings.Contains(listed, "other/w1") || strings.Contains(listed, "far/") {
		t.Fatal("lead's list_agents", listed)
	}
	if listed, _ := call(db, "b", "list_agents", nil); strings.Contains(listed, "other/") {
		t.Fatal("worker's list_agents", listed)
	}
}

func TestSetStatus(t *testing.T) {
	db := team(t, false, t.TempDir(), t.TempDir())
	if _, err := call(db, "b", "set_status", map[string]string{"text": "  Reading\nthe card view. "}); err != nil {
		t.Fatal(err)
	}
	if a := agents(t, db, "session-1")[1]; a.Status != "Reading the card view." || a.StatusAt == 0 {
		t.Errorf("worker = %+v", a)
	}
	if _, err := call(db, "a", "set_status", map[string]string{"text": "Planning."}); err == nil {
		t.Error("the lead set a status")
	}
	if _, err := call(db, "b", "set_status", map[string]string{"text": " "}); err == nil {
		t.Error("an empty status was accepted")
	}
}
