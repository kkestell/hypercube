package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
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
func team(t *testing.T, noGit bool, leadDir, workerDir string) (*store.DB, *session.Runtime) {
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
			return tx.AddAgent(store.Agent{ID: "b", Session: "session-1", Name: "w1", Role: store.Worker, Dir: workerDir, Activity: store.Idle})
		}
		return nil
	})
	return db, session.New(db, "")
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

// post sends a JSON-RPC request to the handler as the agent.
func post(handler http.Handler, agent, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "http://localhost/mcp", strings.NewReader(body))
	req.Header.Set(AgentHeader, agent)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func TestToolsListWorkers(t *testing.T) {
	db, rt := team(t, false, t.TempDir(), "")
	res := post(Handler(db, rt), "a", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var response struct {
		Result struct {
			Tools []tool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
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

func TestHandlerChecksTheAgent(t *testing.T) {
	db, rt := team(t, false, t.TempDir(), "")
	handler := Handler(db, rt)
	for _, agent := range []string{"", "nobody"} {
		if res := post(handler, agent, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); res.Code != http.StatusForbidden {
			t.Errorf("agent %q: status %d", agent, res.Code)
		}
	}
	if res := post(handler, "a", `{`); res.Code != http.StatusBadRequest {
		t.Errorf("bad JSON: status %d", res.Code)
	}
	if res := post(handler, "a", `{"jsonrpc":"2.0","method":"notifications/initialized"}`); res.Code != http.StatusNoContent {
		t.Errorf("notification: status %d", res.Code)
	}
	res := post(handler, "a", `{"jsonrpc":"2.0","id":7,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if !strings.Contains(res.Body.String(), `"protocolVersion":"2025-06-18"`) || !strings.Contains(res.Body.String(), `"id":7`) {
		t.Errorf("initialize = %s", res.Body)
	}
}

// The bridge forwards each request to the app with the agent's identity and
// answers in the app's words.
func TestBridge(t *testing.T) {
	db, rt := team(t, false, t.TempDir(), t.TempDir())
	var origins []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origins = append(origins, r.Header.Get("Origin"))
		Handler(db, rt).ServeHTTP(w, r)
	}))
	defer server.Close()
	run := func(agent, url string) []map[string]any {
		t.Helper()
		t.Setenv("HYPERCUBE_AGENT_ID", agent)
		t.Setenv("HYPERCUBE_URL", url)
		in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"v"}}
not json
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"set_status","arguments":{"text":"Reading"}}}
`
		var out bytes.Buffer
		if err := Bridge(strings.NewReader(in), &out); err != nil {
			t.Fatal(err)
		}
		var answers []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
			var answer map[string]any
			if err := json.Unmarshal([]byte(line), &answer); err != nil {
				t.Fatalf("bridge wrote %q", line)
			}
			answers = append(answers, answer)
		}
		return answers
	}
	answers := run("b", server.URL)
	if len(answers) != 2 || answers[0]["id"] != 1.0 || answers[1]["id"] != 2.0 || answers[1]["result"].(map[string]any)["isError"] != false {
		t.Fatalf("answers = %v", answers)
	}
	if a := agents(t, db, "session-1")[1]; a.Status != "Reading" {
		t.Errorf("the tool did not run for the agent: %+v", a)
	}
	if len(origins) != 2 || origins[0] != server.URL {
		t.Errorf("origins = %v", origins)
	}
	// A refused or unreachable app answers each request with an error.
	for name, got := range map[string][]map[string]any{"unknown agent": run("nobody", server.URL), "no app": run("b", "http://127.0.0.1:1")} {
		if len(got) != 2 || got[0]["error"] == nil || got[0]["id"] != 1.0 {
			t.Errorf("%s: answers = %v", name, got)
		}
	}
	t.Setenv("HYPERCUBE_URL", "")
	if err := Bridge(strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Error("the bridge ran without the app's address")
	}
}

// create_agent refuses bad arguments, a lead profile, and a stopped session,
// adding no agent.
func TestCreateAgentRefusals(t *testing.T) {
	db, rt := team(t, true, t.TempDir(), "")
	for _, args := range []map[string]string{{"name": "w1", "profile": "sol"}, {"name": "w1", "profile": "sol", "dir": "missing"}, {"name": "w1", "dir": "."}, {"name": "w1", "profile": "nope", "dir": "."}, {"name": "w1", "profile": "opus", "dir": "."}, {"name": "w1", "profile": "sol", "dir": "."}} {
		if _, err := createAgent(rt, "a", args); err == nil {
			t.Errorf("createAgent(%v) succeeded", args)
		}
		if list := agents(t, db, "session-1"); len(list) != 1 {
			t.Errorf("after createAgent(%v): %+v", args, list)
		}
	}
}

// remove_agent refuses a stopped session and leaves the worker.
func TestRemoveAgentStoppedSession(t *testing.T) {
	workerDir := t.TempDir()
	db, rt := team(t, false, t.TempDir(), workerDir)
	if _, err := removeAgent(rt, "a", "w1"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("removeAgent in a stopped session: %v", err)
	}
	if list := agents(t, db, "session-1"); len(list) != 2 {
		t.Errorf("after removeAgent: %+v", list)
	}
}

func TestSessionBoundaries(t *testing.T) {
	db, rt := team(t, false, t.TempDir(), t.TempDir())
	change(t, db, func(tx *store.Tx) error {
		tx.AddSession(store.Session{ID: "other", Workspace: "w", Title: "Fix the parser"})
		tx.AddAgent(store.Agent{ID: "other-lead", Session: "other", Name: "lead", Role: store.Lead, Activity: store.Idle})
		tx.AddAgent(store.Agent{ID: "other-worker", Session: "other", Name: "w1", Role: store.Worker, Activity: store.Idle})
		tx.AddWorkspace(store.Workspace{ID: "far", Path: "/elsewhere"})
		tx.AddSession(store.Session{ID: "far", Workspace: "far"})
		return tx.AddAgent(store.Agent{ID: "far-lead", Session: "far", Name: "lead", Role: store.Lead, Activity: store.Idle})
	})
	for _, send := range [][2]string{{"a", "other/w1"}, {"b", "other/w1"}, {"b", "other/lead"}, {"a", "far/lead"}} {
		if _, err := sendMessage(db, rt, send[0], send[1], "wrong session"); err == nil {
			t.Fatalf("%s reached %s across sessions", send[0], send[1])
		}
	}
	if _, err := removeAgent(rt, "a", "other/w1"); err == nil {
		t.Fatal("removed another session's worker")
	}
	if _, err := removeAgent(rt, "b", "w1"); err == nil {
		t.Fatal("worker removed an agent")
	}
	// Session "other" is stopped; session-1 runs.
	for _, send := range [][3]string{{"other-lead", "w1", "stopped"}, {"a", "other/lead", "stopped"}, {"other-lead", "session-1/lead", ""}} {
		reply, err := sendMessage(db, rt, send[0], send[1], "allowed")
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
	if listed, _ := call(db, rt, "a", "list_agents", nil); !strings.Contains(listed, "other/lead") || !strings.Contains(listed, `"session_title": "Fix the parser"`) || strings.Contains(listed, "other/w1") || strings.Contains(listed, "far/") {
		t.Fatal("lead's list_agents", listed)
	}
	if listed, _ := call(db, rt, "b", "list_agents", nil); strings.Contains(listed, "other/") {
		t.Fatal("worker's list_agents", listed)
	}
}

func TestSetStatus(t *testing.T) {
	db, rt := team(t, false, t.TempDir(), t.TempDir())
	if _, err := call(db, rt, "b", "set_status", map[string]string{"text": "  Reading\nthe card view. "}); err != nil {
		t.Fatal(err)
	}
	if a := agents(t, db, "session-1")[1]; a.Status != "Reading the card view." || a.StatusAt == 0 {
		t.Errorf("worker = %+v", a)
	}
	if _, err := call(db, rt, "a", "set_status", map[string]string{"text": "Planning."}); err == nil {
		t.Error("the lead set a status")
	}
	if _, err := call(db, rt, "b", "set_status", map[string]string{"text": " "}); err == nil {
		t.Error("an empty status was accepted")
	}
}
