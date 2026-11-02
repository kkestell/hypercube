package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness/harnesstest"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStandIn(t *testing.T) { harnesstest.Run() }

// family returns a database holding a workspace, outside Git if noGit is set,
// with top-level session session-1 working in parentDir and, if childDir is
// set, its child "w1" (ID "b") working directly in it.
func family(t *testing.T, noGit bool, parentDir, childDir string) (*store.DB, *session.Runtime) {
	t.Helper()
	dir, err := os.MkdirTemp("", "hc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	if err := os.MkdirAll(filepath.Join(dir, "hypercube"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := []byte(`{
  "profiles": [
    {"name": "opus", "role": "lead", "description": "leading.", "harness": "one", "model": "opus", "effort": "high"},
    {"name": "sol", "role": "general", "description": "coding.", "harness": "two", "model": "gpt-6.1-sol", "effort": "medium"}
  ]
}`)
	if err := os.WriteFile(filepath.Join(dir, "hypercube", "config.json"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: parentDir, NoGit: noGit}); err != nil {
			return err
		}
		if err := tx.AddSession(store.Session{ID: "session-1", Workspace: "w", Running: true, Dir: parentDir, Activity: store.Idle}); err != nil {
			return err
		}
		if childDir != "" {
			return tx.AddSession(store.Session{ID: "b", Workspace: "w", Parent: "session-1", Name: "w1", Dir: childDir, Activity: store.Idle})
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

func children(t *testing.T, db *store.DB, session string) (list []store.Session) {
	t.Helper()
	change(t, db, func(tx *store.Tx) error { var err error; list, err = tx.Children(session); return err })
	return list
}

// post sends a JSON-RPC request to the handler as the session.
func post(handler http.Handler, session, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "http://localhost/mcp", strings.NewReader(body))
	req.Header.Set(SessionHeader, session)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

// serve runs the app's /mcp route on a test server.
func serve(t *testing.T, db *store.DB, rt *session.Runtime) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(Handler(db, rt))
	t.Cleanup(server.Close)
	return server
}

// connect returns an SDK client session with the app's /mcp route as the session.
func connect(t *testing.T, url, id string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	s, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             url,
		HTTPClient:           &http.Client{Transport: identity(id)},
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
		db, rt := family(t, noGit, t.TempDir(), "")
		server := httptest.NewServer(Handler(db, rt))
		list := tools(t, connect(t, server.URL, "session-1"))
		server.Close()
		for _, name := range []string{"send_message", "list_sessions", "create_session", "end_session"} {
			if list[name] == nil {
				t.Errorf("noGit=%v: %s missing", noGit, name)
			}
		}
		create := schemaOf(t, list["create_session"])
		want := []any{"name", "title", "profile"}
		if noGit {
			want = append(want, "dir")
		}
		if got := create["required"]; !slices.Equal(got.([]any), want) {
			t.Errorf("noGit=%v: create_session requires %v", noGit, got)
		}
		// Only child profiles are offered.
		if enum := create["properties"].(map[string]any)["profile"].(map[string]any)["enum"]; !slices.Equal(enum.([]any), []any{"sol"}) {
			t.Errorf("noGit=%v: profile enum = %v", noGit, enum)
		}
	}
}

func TestProtocol(t *testing.T) {
	db, rt := family(t, false, t.TempDir(), "")
	server := httptest.NewServer(Handler(db, rt))
	defer server.Close()
	s := connect(t, server.URL, "session-1")
	if v := s.InitializeResult().ProtocolVersion; v == "" || v == "unknown" {
		t.Errorf("negotiated protocol version %q", v)
	}
	if name := s.InitializeResult().ServerInfo.Name; name != "hypercube" {
		t.Errorf("server name %q", name)
	}
	// A client asking for an unsupported version gets a supported one.
	res := post(Handler(db, rt), "session-1", `{"jsonrpc":"2.0","id":7,"method":"initialize","params":{"protocolVersion":"1999-01-01","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`)
	if !strings.Contains(res.Body.String(), `"id":7`) || strings.Contains(res.Body.String(), "1999-01-01") {
		t.Errorf("initialize = %s", res.Body)
	}
	// Unknown methods are refused, not answered with an empty success.
	res = post(Handler(db, rt), "session-1", `{"jsonrpc":"2.0","id":8,"method":"no/such"}`)
	if res.Code != http.StatusBadRequest {
		t.Errorf("unknown method = %d %s", res.Code, res.Body)
	}
	if _, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "no_such_tool"}); err == nil {
		t.Error("an unknown tool succeeded")
	}
	if text, failed := callTool(t, s, "send_message", map[string]any{"to": "missing", "text": 5}); !failed {
		t.Error(text)
	}
}

func TestHandlerChecksTheSession(t *testing.T) {
	db, rt := family(t, false, t.TempDir(), "")
	handler := Handler(db, rt)
	for _, id := range []string{"", "nobody"} {
		if res := post(handler, id, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); res.Code != http.StatusForbidden {
			t.Errorf("session %q: status %d", id, res.Code)
		}
	}
}

// The bridge forwards each request to the app with the session ID and
// answers in the app's words.
func TestBridge(t *testing.T) {
	db, rt := family(t, false, t.TempDir(), t.TempDir())
	server := serve(t, db, rt)
	run := func(id, url string) []map[string]any {
		t.Helper()
		t.Setenv("HYPERCUBE_SESSION_ID", id)
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
	if items, _ := rt.Transcript("session-1"); len(items) != 1 || items[0].Text != "Reading" {
		t.Error("status did not reach parent transcript")
	}
	// A refused or unreachable app answers each request with an error.
	for name, got := range map[string][]map[string]any{"unknown agent": run("nobody", server.URL), "bad app URL": run("b", ":invalid")} {
		if len(got) != 3 || got[0]["error"] == nil || got[0]["id"] != 1.0 || got[1]["error"] == nil {
			t.Errorf("%s: answers = %v", name, got)
		}
	}
	t.Setenv("HYPERCUBE_URL", "")
	if err := Bridge(strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Error("the bridge ran without the app's address")
	}
}

// create_session refuses bad arguments, a lead profile, and a stopped session,
// adding no agent.
func TestCreateSessionRefusals(t *testing.T) {
	db, rt := family(t, true, t.TempDir(), "")
	parent := connect(t, serve(t, db, rt).URL, "session-1")
	for _, args := range []map[string]any{
		{"name": "w1", "title": "One", "profile": "sol"},
		{"name": "w1", "title": "One", "dir": "."},
		{"name": "W 1", "title": "One", "profile": "sol", "dir": "."},
		{"name": "w1", "title": "One", "profile": "nope", "dir": "."},
		{"name": "w1", "title": "One", "profile": "opus", "dir": "."},
		{"name": "w1", "profile": "sol", "dir": "."},
		{"name": "w1", "title": "  ", "profile": "sol", "dir": "."},
		{"name": "w1", "title": "One", "profile": "sol", "dir": "."},
	} {
		if text, failed := callTool(t, parent, "create_session", args); !failed {
			t.Errorf("create_session(%v) = %q", args, text)
		}
		if list := children(t, db, "session-1"); len(list) != 0 {
			t.Errorf("after create_session(%v): %+v", args, list)
		}
	}
}

func TestCreateSessionRefusesMissingDirectory(t *testing.T) {
	db, rt := family(t, true, t.TempDir(), "")
	plugins := t.TempDir()
	harnesstest.Install(t, plugins, "one")
	t.Setenv("HYPERCUBE_TEST_LOGS", t.TempDir())
	rt.PluginDir = plugins
	t.Cleanup(rt.Quit)
	change(t, db, func(tx *store.Tx) error {
		parent, err := tx.Session("session-1")
		if err != nil {
			return err
		}
		parent.Harness, parent.Model, parent.Effort = "one", "opus", "high"
		return tx.SaveSession(parent)
	})
	if err := rt.Start("session-1"); err != nil {
		t.Fatal(err)
	}
	parent := connect(t, serve(t, db, rt).URL, "session-1")
	text, failed := callTool(t, parent, "create_session", map[string]any{"name": "w1", "title": "One", "profile": "sol", "dir": "missing"})
	if !failed || !strings.Contains(text, "missing: no such file or directory") {
		t.Fatalf("create_session with a missing directory: %q %v", text, failed)
	}
	if list := children(t, db, "session-1"); len(list) != 0 {
		t.Errorf("after create_session: %+v", list)
	}
}

// set_title changes the caller's display title and nothing else.
func TestSetTitle(t *testing.T) {
	db, rt := family(t, false, t.TempDir(), t.TempDir())
	parent := connect(t, serve(t, db, rt).URL, "session-1")
	if text, failed := callTool(t, parent, "set_title", map[string]any{"title": "  Fix the parser "}); failed {
		t.Fatal(text)
	}
	if text, failed := callTool(t, parent, "set_title", map[string]any{"title": " "}); !failed {
		t.Errorf("set_title accepted a blank title: %q", text)
	}
	change(t, db, func(tx *store.Tx) error {
		s, err := tx.Session("session-1")
		if s.Title != "Fix the parser" {
			t.Errorf("title = %q", s.Title)
		}
		return err
	})
	if tools(t, connect(t, serve(t, db, rt).URL, "b"))["set_title"] != nil {
		t.Error("a child can set its title")
	}
}

// end_session refuses in a stopped session and leaves the child.
func TestEndSessionStoppedSession(t *testing.T) {
	db, rt := family(t, false, t.TempDir(), t.TempDir())
	parent := connect(t, serve(t, db, rt).URL, "session-1")
	if text, failed := callTool(t, parent, "end_session", map[string]any{"name": "w1"}); !failed || !strings.Contains(text, "not running") {
		t.Fatalf("end_session in a stopped session: %q %v", text, failed)
	}
	if list := children(t, db, "session-1"); len(list) != 1 {
		t.Errorf("after end_session: %+v", list)
	}
}

func TestSessionBoundaries(t *testing.T) {
	db, rt := family(t, false, t.TempDir(), t.TempDir())
	change(t, db, func(tx *store.Tx) error {
		for _, s := range []store.Session{{ID: "peer", Workspace: "w", Title: "Fix parser"}, {ID: "peer-child", Workspace: "w", Parent: "peer", Name: "child"}} {
			if err := tx.AddSession(s); err != nil {
				return err
			}
		}
		return nil
	})
	url := serve(t, db, rt).URL
	parent := connect(t, url, "session-1")
	child := connect(t, url, "b")
	parentTools, childTools := tools(t, parent), tools(t, child)
	if len(parentTools) != 5 || parentTools["set_status"] != nil || len(childTools) != 3 || childTools["create_session"] != nil || childTools["end_session"] != nil {
		t.Fatalf("tools: %v / %v", parentTools, childTools)
	}
	listed, failed := callTool(t, parent, "list_sessions", nil)
	if failed || !strings.Contains(listed, `"to": "w1"`) || !strings.Contains(listed, `"to": "peer"`) || strings.Contains(listed, "peer-child") {
		t.Fatal(listed)
	}
	listed, failed = callTool(t, child, "list_sessions", nil)
	if failed || !strings.Contains(listed, `"to": "session-1"`) || strings.Contains(listed, "peer") {
		t.Fatal(listed)
	}
	if text, failed := callTool(t, child, "send_message", map[string]any{"to": "peer", "text": "no"}); !failed {
		t.Fatal(text)
	}
	if text, failed := callTool(t, parent, "send_message", map[string]any{"to": "peer", "text": "yes"}); failed || !strings.Contains(text, "stopped") {
		t.Fatal(text)
	}
	if text, failed := callTool(t, child, "send_message", map[string]any{"to": "session-1", "text": "yes"}); failed {
		t.Fatal(text)
	}
	if text, failed := callTool(t, child, "set_status", map[string]any{"text": " Reading\nthe code. "}); failed {
		t.Fatal(text)
	}
	items, _ := rt.Transcript("session-1")
	if len(items) != 1 || items[0].Text != "Reading the code." || items[0].From != "b" {
		t.Fatalf("status=%+v", items)
	}
	if text, failed := callTool(t, child, "set_status", map[string]any{"text": " "}); !failed {
		t.Fatal(text)
	}
}

func TestBridgeWaitsForRestartingApp(t *testing.T) {
	db, rt := family(t, false, t.TempDir(), t.TempDir())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	t.Setenv("HYPERCUBE_SESSION_ID", "b")
	t.Setenv("HYPERCUBE_URL", "http://"+addr)
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Bridge(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"set_status","arguments":{"text":"after restart"}}}`+"\n"), &out)
	}()
	// The first refused attempt has to wait for the replacement app.
	select {
	case err := <-done:
		t.Fatalf("bridge gave up before app started: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	listener, err = net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: Handler(db, rt)}
	go server.Serve(listener)
	defer server.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bridge did not resume")
	}
	if !strings.Contains(out.String(), "Status set.") {
		t.Fatal(out.String())
	}
	items, err := rt.Transcript("session-1")
	if err != nil || len(items) != 1 || items[0].Text != "after restart" {
		t.Fatalf("%+v: %v", items, err)
	}
}
