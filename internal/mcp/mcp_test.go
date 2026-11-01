package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

// withAgent adds the agent's identity to every request.
type withAgent string

func (a withAgent) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set(AgentHeader, string(a))
	return http.DefaultTransport.RoundTrip(r)
}

// connect returns an SDK client session with the app's /mcp route as the agent.
func connect(t *testing.T, url, agent string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	s, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             url,
		HTTPClient:           &http.Client{Transport: withAgent(agent)},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// callTool runs a tool and returns its text and whether it failed.
func callTool(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

func tools(t *testing.T, s *mcp.ClientSession) map[string]*mcp.Tool {
	t.Helper()
	res, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range res.Tools {
		byName[tl.Name] = tl
	}
	return byName
}

// schemaOf returns a tool's input schema as decoded JSON.
func schemaOf(t *testing.T, tl *mcp.Tool) map[string]any {
	t.Helper()
	b, err := json.Marshal(tl.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(b, &schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestTools(t *testing.T) {
	for _, noGit := range []bool{false, true} {
		db, rt := team(t, noGit, t.TempDir(), "")
		server := httptest.NewServer(Handler(db, rt))
		list := tools(t, connect(t, server.URL, "a"))
		server.Close()
		for _, name := range []string{"send_message", "list_agents", "set_status", "create_agent", "remove_agent"} {
			if list[name] == nil {
				t.Errorf("noGit=%v: %s missing", noGit, name)
			}
		}
		create := schemaOf(t, list["create_agent"])
		want := []any{"name", "profile"}
		if noGit {
			want = append(want, "dir")
		}
		if got := create["required"]; !slices.Equal(got.([]any), want) {
			t.Errorf("noGit=%v: create_agent requires %v", noGit, got)
		}
		// Only worker profiles are offered.
		if enum := create["properties"].(map[string]any)["profile"].(map[string]any)["enum"]; !slices.Equal(enum.([]any), []any{"sol"}) {
			t.Errorf("noGit=%v: profile enum = %v", noGit, enum)
		}
	}
}

func TestProtocol(t *testing.T) {
	db, rt := team(t, false, t.TempDir(), "")
	server := httptest.NewServer(Handler(db, rt))
	defer server.Close()
	s := connect(t, server.URL, "a")
	if v := s.InitializeResult().ProtocolVersion; v == "" || v == "unknown" {
		t.Errorf("negotiated protocol version %q", v)
	}
	if name := s.InitializeResult().ServerInfo.Name; name != "hypercube" {
		t.Errorf("server name %q", name)
	}
	// A client asking for an unsupported version gets a supported one.
	res := post(Handler(db, rt), "a", `{"jsonrpc":"2.0","id":7,"method":"initialize","params":{"protocolVersion":"1999-01-01","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`)
	if !strings.Contains(res.Body.String(), `"id":7`) || strings.Contains(res.Body.String(), "1999-01-01") {
		t.Errorf("initialize = %s", res.Body)
	}
	// Unknown methods are refused, not answered with an empty success.
	res = post(Handler(db, rt), "a", `{"jsonrpc":"2.0","id":8,"method":"no/such"}`)
	if res.Code != http.StatusBadRequest {
		t.Errorf("unknown method = %d %s", res.Code, res.Body)
	}
	if _, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "no_such_tool"}); err == nil {
		t.Error("an unknown tool succeeded")
	}
	// Tool failures are results marked as errors.
	if text, failed := callTool(t, s, "set_status", map[string]any{"text": 5}); !failed {
		t.Errorf("a number for a string argument = %q", text)
	}
	if text, failed := callTool(t, s, "set_status", map[string]any{"text": "Planning."}); !failed || !strings.Contains(text, "only workers") {
		t.Errorf("the lead set a status: %q %v", text, failed)
	}
}

func TestHandlerChecksTheAgent(t *testing.T) {
	db, rt := team(t, false, t.TempDir(), "")
	handler := Handler(db, rt)
	for _, agent := range []string{"", "nobody"} {
		if res := post(handler, agent, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); res.Code != http.StatusForbidden {
			t.Errorf("agent %q: status %d", agent, res.Code)
		}
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
		in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"set_status","arguments":{"text":"Reading"}}}
{"jsonrpc":"2.0","id":3,"method":"no/such"}
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
	if len(answers) != 3 || answers[0]["id"] != 1.0 || answers[1]["id"] != 2.0 || answers[1]["result"].(map[string]any)["isError"] == true || answers[2]["id"] != 3.0 || answers[2]["error"] == nil {
		t.Fatalf("answers = %v", answers)
	}
	if a := agents(t, db, "session-1")[1]; a.Status != "Reading" {
		t.Errorf("the tool did not run for the agent: %+v", a)
	}
	for _, o := range origins {
		if o != server.URL {
			t.Errorf("origins = %v", origins)
			break
		}
	}
	// A refused or unreachable app answers each request with an error.
	for name, got := range map[string][]map[string]any{"unknown agent": run("nobody", server.URL), "no app": run("b", "http://127.0.0.1:1")} {
		if len(got) != 3 || got[0]["error"] == nil || got[0]["id"] != 1.0 || got[1]["error"] == nil {
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
	for _, args := range []createAgentArgs{{Name: "w1", Profile: "sol"}, {Name: "w1", Profile: "sol", Dir: "missing"}, {Name: "w1", Dir: "."}, {Name: "w1", Profile: "nope", Dir: "."}, {Name: "w1", Profile: "opus", Dir: "."}, {Name: "w1", Profile: "sol", Dir: "."}} {
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
	if listed, _ := listAgents(db, "a"); !strings.Contains(listed, "other/lead") || !strings.Contains(listed, `"session_title": "Fix the parser"`) || strings.Contains(listed, "other/w1") || strings.Contains(listed, "far/") {
		t.Fatal("lead's list_agents", listed)
	}
	if listed, _ := listAgents(db, "b"); strings.Contains(listed, "other/") {
		t.Fatal("worker's list_agents", listed)
	}
}

func TestSetStatus(t *testing.T) {
	db, _ := team(t, false, t.TempDir(), t.TempDir())
	if _, err := setStatus(db, "b", "  Reading\nthe card view. "); err != nil {
		t.Fatal(err)
	}
	if a := agents(t, db, "session-1")[1]; a.Status != "Reading the card view." || a.StatusAt == 0 {
		t.Errorf("worker = %+v", a)
	}
	if _, err := setStatus(db, "a", "Planning."); err == nil {
		t.Error("the lead set a status")
	}
	if _, err := setStatus(db, "b", " "); err == nil {
		t.Error("an empty status was accepted")
	}
}
