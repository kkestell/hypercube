package mcp

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
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
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: parentDir, NoGit: noGit}); err != nil {
			return err
		}
		if err := tx.AddSession(store.Session{ID: "session-1", Workspace: "w", Dir: parentDir, Activity: store.Idle}); err != nil {
			return err
		}
		if childDir != "" {
			return tx.AddSession(store.Session{ID: "b", Workspace: "w", Parent: "session-1", Name: "w1", Dir: childDir, Activity: store.Idle})
		}
		return nil
	})
	plugins := filepath.Join(dir, "plugins")
	if err := os.Mkdir(plugins, 0o755); err != nil {
		t.Fatal(err)
	}
	harnesstest.Install(t, plugins, "one")
	harnesstest.Install(t, plugins, "two")
	return db, session.New(db, "", session.Programs{Plugins: plugins})
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
		HTTPClient:           &http.Client{Transport: withSession(id)},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

type withSession string

func (s withSession) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set(SessionHeader, string(s))
	return http.DefaultTransport.RoundTrip(r)
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
		for _, name := range []string{"send_message", "list_sessions", "list_models", "create_session", "end_session", "open_file"} {
			if list[name] == nil {
				t.Errorf("noGit=%v: %s missing", noGit, name)
			}
		}
		create := schemaOf(t, list["create_session"])
		if got := create["required"]; !slices.Equal(got.([]any), []any{"title"}) {
			t.Errorf("noGit=%v: create_session requires %v", noGit, got)
		}
		// Settings are free overrides, checked by the harness.
		properties := create["properties"].(map[string]any)
		for _, name := range []string{"harness", "model", "effort"} {
			if p := properties[name].(map[string]any); p["enum"] != nil {
				t.Errorf("noGit=%v: %s enum = %v", noGit, name, p["enum"])
			}
		}
		if properties["profile"] != nil {
			t.Errorf("noGit=%v: create_session takes a profile", noGit)
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

// create_session refuses bad arguments and a parked session,
// adding no agent.
func TestCreateSessionRefusals(t *testing.T) {
	db, rt := family(t, true, t.TempDir(), "")
	parent := connect(t, serve(t, db, rt).URL, "session-1")
	for _, args := range []map[string]any{
		{"name": "w1", "title": "One", "harness": "two", "model": "gpt-6.1-sol", "dir": "."},
		{"name": "w1", "title": "One", "harness": "two", "model": "gpt-6.1-sol", "dir": ".", "requested": true},
		{"initiation": "own_initiative", "name": "w1", "title": "One", "dir": "."},
		{"initiation": "own_initiative", "name": "W 1", "title": "One", "harness": "two", "model": "gpt-6.1-sol", "dir": "."},
		{"initiation": "own_initiative", "name": "w1", "title": "One", "harness": "two", "dir": "."},
		{"initiation": "own_initiative", "name": "w1", "harness": "two", "model": "gpt-6.1-sol", "dir": "."},
		{"initiation": "own_initiative", "name": "w1", "title": "  ", "harness": "two", "model": "gpt-6.1-sol", "dir": "."},
		{"initiation": "own_initiative", "name": "w1", "title": "One", "harness": "two", "model": "gpt-6.1-sol", "dir": ".", "fork": true},
		{"initiation": "offer", "title": "One", "model": "gpt-6.1-sol"},
		{"initiation": "offer", "title": "One"},
		{"initiation": "offer", "title": "One", "offer_id": "x"},
		{"offer_id": "x", "name": "w1", "title": "One", "harness": "two", "model": "gpt-6.1-sol", "dir": "."},
	} {
		if text, failed := callTool(t, parent, "create_session", args); !failed {
			t.Errorf("create_session(%v) = %q", args, text)
		}
		if list := children(t, db, "session-1"); len(list) != 0 {
			t.Errorf("after create_session(%v): %+v", args, list)
		}
	}
}

// started runs session-1 with the stand-in plugin and connects to its tools.
func started(t *testing.T, db *store.DB, rt *session.Runtime) *mcp.ClientSession {
	t.Helper()
	t.Setenv("HYPERCUBE_TEST_LOGS", t.TempDir())
	t.Cleanup(rt.Quit)
	change(t, db, func(tx *store.Tx) error {
		parent, err := tx.Session("session-1")
		if err != nil {
			return err
		}
		parent.Harness, parent.Model, parent.Effort = "one", "opus", "high"
		return tx.SaveSession(parent)
	})
	if err := rt.Input("session-1", "start"); err != nil {
		t.Fatal(err)
	}
	return connect(t, serve(t, db, rt).URL, "session-1")
}

func TestCreateSessionRefusesMissingDirectory(t *testing.T) {
	db, rt := family(t, true, t.TempDir(), "")
	parent := started(t, db, rt)
	text, failed := callTool(t, parent, "create_session", map[string]any{"initiation": "own_initiative", "name": "w1", "title": "One", "harness": "two", "model": "gpt-6.1-sol", "dir": "missing"})
	if !failed || !strings.Contains(text, "missing: no such file or directory") {
		t.Fatalf("create_session with a missing directory: %q %v", text, failed)
	}
	if list := children(t, db, "session-1"); len(list) != 0 {
		t.Errorf("after create_session: %+v", list)
	}
}

// An explicitly requested child starts without an offer or proposal.
func TestRequestedSessionStartsWithoutAnOffer(t *testing.T) {
	db, rt := family(t, true, t.TempDir(), "")
	parent := started(t, db, rt)
	args := map[string]any{"initiation": "user_asked", "name": "reviewer", "title": "Review", "dir": "."}
	for _, brief := range []string{"", "  "} {
		args["message"] = brief
		if text, failed := callTool(t, parent, "create_session", args); !failed {
			t.Fatalf("a requested child started without a brief: %q", text)
		}
	}
	args["message"] = "Review the change without editing it. Report contradictions and gaps."
	if text, failed := callTool(t, parent, "create_session", args); failed {
		t.Fatal(text)
	}
	list := children(t, db, "session-1")
	if len(list) != 1 || list[0].Name != "reviewer" || list[0].Harness != "one" || list[0].Model != "opus" {
		t.Fatalf("children = %+v", list)
	}
	items, err := rt.Transcript("session-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Offer || it.Kind == harness.KindProposal {
			t.Fatalf("requested delegation created a confirmation: %+v", it)
		}
	}
}

// An offer starts nothing until the user chooses New session and the agent
// supplies the brief; a suggestion starts nothing until the user's Start.
func TestCreateSessionForms(t *testing.T) {
	db, rt := family(t, true, t.TempDir(), "")
	parent := started(t, db, rt)
	text, failed := callTool(t, parent, "create_session", map[string]any{"initiation": "offer", "title": "Task"})
	if failed {
		t.Fatal(text)
	}
	items, err := rt.Transcript("session-1")
	if err != nil {
		t.Fatal(err)
	}
	var offer string
	for _, it := range items {
		if it.Offer && !it.Done {
			offer = it.ID
		}
	}
	if offer == "" || !strings.Contains(text, offer) || len(children(t, db, "session-1")) != 0 {
		t.Fatalf("offer %q: %q", offer, text)
	}
	fill := map[string]any{"offer_id": offer, "name": "w1", "title": "Task", "harness": "two", "model": "gpt-6.1-sol", "dir": ".", "message": "the brief"}
	if text, failed := callTool(t, parent, "create_session", fill); !failed {
		t.Fatalf("an unanswered offer started a child: %q", text)
	}
	if err := rt.Answer("session-1", offer, []string{"New session"}, ""); err != nil {
		t.Fatal(err)
	}
	for _, brief := range []any{nil, "  "} {
		unbriefed := maps.Clone(fill)
		if brief == nil {
			delete(unbriefed, "message")
		} else {
			unbriefed["message"] = brief
		}
		if text, failed := callTool(t, parent, "create_session", unbriefed); !failed {
			t.Fatalf("an offer started a child without a brief: %q", text)
		}
	}
	if text, failed := callTool(t, parent, "create_session", fill); failed {
		t.Fatal(text)
	}
	if text, failed := callTool(t, parent, "create_session", fill); !failed {
		t.Fatalf("an offer started a second child: %q", text)
	}
	if list := children(t, db, "session-1"); len(list) != 1 || list[0].Harness != "two" || list[0].Model != "gpt-6.1-sol" {
		t.Fatalf("children = %+v", list)
	}
	suggestion := map[string]any{"initiation": "own_initiative", "name": "w2", "title": "Later", "harness": "two", "model": "gpt-6.1-sol", "dir": "."}
	if text, failed := callTool(t, parent, "create_session", suggestion); failed {
		t.Fatal(text)
	}
	if list := children(t, db, "session-1"); len(list) != 1 {
		t.Fatalf("a suggestion started a child: %+v", list)
	}
}

// read_session shows a child's transcript, newest last, a page at a time.
func TestReadSession(t *testing.T) {
	db, rt := family(t, false, t.TempDir(), t.TempDir())
	long := strings.Repeat("x", 30000)
	items := []harness.Item{
		{ID: "1", Kind: harness.KindUser, Text: "the brief", From: "session-1"},
		{ID: "2", Kind: harness.KindText, Text: long},
		{ID: "3", Kind: harness.KindReasoning, Text: "thinking"},
		{ID: "4", Kind: harness.KindTool, Tool: "shell"},
		{ID: "5", Kind: harness.KindText, Parent: "4", Text: "a native subagent"},
		{ID: "6", Kind: harness.KindText, Text: "Done: " + long},
		{ID: "7", Kind: harness.KindState, State: harness.Idle, Final: []string{"6"}},
	}
	change(t, db, func(tx *store.Tx) error {
		for i, it := range items {
			data, err := json.Marshal(it)
			if err != nil {
				return err
			}
			if _, err := tx.PutEntry("b", store.Entry{Seq: int64(i + 1), Item: it.ID, Data: data}); err != nil {
				return err
			}
		}
		return nil
	})
	parent := connect(t, serve(t, db, rt).URL, "session-1")
	last, failed := callTool(t, parent, "read_session", map[string]any{"to": "w1"})
	if failed || !strings.HasPrefix(last, "Earlier entries remain: call read_session with cursor 4 ") || !strings.HasSuffix(last, "Tool: shell\n\nAssistant: Done: "+long+"\n\n(turn finished)") {
		t.Fatalf("last page = %.200q", last)
	}
	first, failed := callTool(t, parent, "read_session", map[string]any{"to": "w1", "cursor": 4})
	if failed || first != "Message from session-1: the brief\n\nAssistant: "+long {
		t.Fatalf("first page = %.200q", first)
	}
	if text, failed := callTool(t, parent, "read_session", map[string]any{"to": "nobody"}); !failed {
		t.Fatalf("read an unknown session: %q", text)
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

// end_session refuses in a parked session and leaves the child.
func TestEndSessionParkedSession(t *testing.T) {
	db, rt := family(t, false, t.TempDir(), t.TempDir())
	parent := connect(t, serve(t, db, rt).URL, "session-1")
	if text, failed := callTool(t, parent, "end_session", map[string]any{"name": "w1"}); !failed || !strings.Contains(text, "not running") {
		t.Fatalf("end_session in a parked session: %q %v", text, failed)
	}
	if list := children(t, db, "session-1"); len(list) != 1 {
		t.Errorf("after end_session: %+v", list)
	}
}

func TestSessionBoundaries(t *testing.T) {
	dir := t.TempDir()
	db, rt := family(t, false, dir, t.TempDir())
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "far", Path: "/other"}); err != nil {
			return err
		}
		for _, s := range []store.Session{
			{ID: "peer", Workspace: "w", Title: "Fix parser"},
			{ID: "peer-child", Workspace: "w", Parent: "peer", Name: "child"},
			{ID: "far", Workspace: "far"},
			{ID: "far-child", Workspace: "far", Parent: "far", Name: "child"},
			{ID: "parked", Workspace: "far"},
			{ID: "parked-child", Workspace: "far", Parent: "parked", Name: "child"},
			{ID: "ended", Workspace: "w", Parent: "session-1", Name: "ended", Ended: true},
		} {
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
	if len(parentTools) != 8 || parentTools["open_file"] == nil || childTools["list_models"] != nil || childTools["open_file"] == nil || len(childTools) != 3 || childTools["create_session"] != nil || childTools["end_session"] != nil {
		t.Fatalf("tools: %v / %v", parentTools, childTools)
	}
	if !strings.Contains(parentTools["send_message"].Description, "across all workspaces") || !strings.Contains(childTools["send_message"].Description, "only session you can message") {
		t.Error("tool descriptions do not describe messaging scope")
	}
	checkList := func(client *mcp.ClientSession, want map[string][2]string) {
		t.Helper()
		listed, failed := callTool(t, client, "list_sessions", nil)
		var entries []map[string]any
		if err := json.Unmarshal([]byte(listed), &entries); failed || err != nil {
			t.Fatalf("list_sessions: %s %v", listed, err)
		}
		if len(entries) != len(want) {
			t.Fatalf("list_sessions: %s, want %v", listed, want)
		}
		for _, e := range entries {
			identity, ok := want[e["to"].(string)]
			if !ok || e["workspace"] != identity[0] || e["parent"] != identity[1] {
				t.Errorf("unexpected recipient: %v", e)
			}
			for _, field := range []string{"to", "title", "harness", "activity", "directory", "branch", "workspace", "parent"} {
				if _, ok := e[field]; !ok {
					t.Errorf("missing %s: %v", field, e)
				}
			}
			if _, ok := e["ended"]; ok {
				t.Error("redundant ended field:", e)
			}
		}
	}
	checkList(parent, map[string][2]string{"w1": {dir, "session-1"}, "peer": {dir, ""}, "far": {"/other", ""}, "parked": {"/other", ""}})
	checkList(child, map[string][2]string{"session-1": {dir, ""}})
	checkList(connect(t, url, "parked-child"), map[string][2]string{"parked": {"/other", ""}})
	for _, to := range []string{"peer", "far", "peer-child", "far-child", "w1"} {
		if text, failed := callTool(t, child, "send_message", map[string]any{"to": to, "text": "no"}); !failed {
			t.Fatal(text)
		}
	}
	for _, to := range []string{"peer-child", "far-child", "ended"} {
		if text, failed := callTool(t, parent, "send_message", map[string]any{"to": to, "text": "no"}); !failed {
			t.Fatal(text)
		}
	}
	for _, to := range []string{"peer", "far", "parked"} {
		if text, failed := callTool(t, parent, "send_message", map[string]any{"to": to, "text": "yes"}); failed || text != "Message queued for "+to+"." {
			t.Fatal(text)
		}
	}
	if text, failed := callTool(t, child, "send_message", map[string]any{"to": "session-1", "text": "yes"}); failed {
		t.Fatal(text)
	}
	change(t, db, func(tx *store.Tx) error {
		for _, id := range []string{"peer-child", "far-child", "ended"} {
			messages, err := tx.Queued(id)
			if err != nil {
				return err
			}
			if len(messages) != 0 {
				t.Errorf("refused messages queued for %s: %+v", id, messages)
			}
		}
		return nil
	})
	change(t, db, func(tx *store.Tx) error {
		s, err := tx.Session("session-1")
		if err != nil {
			return err
		}
		s.Ended = true
		return tx.SaveSession(s)
	})
	checkList(child, map[string][2]string{})
}

// list_models reports the harnesses and their choices without starting a
// session.
func TestListModels(t *testing.T) {
	db, rt := family(t, true, t.TempDir(), "")
	parent := started(t, db, rt)
	for _, tc := range []struct {
		args map[string]any
		want []string
	}{
		{map[string]any{}, []string{`"id": "one"`, `"id": "two"`, `"installed": true`}},
		{map[string]any{"harness": "one"}, []string{`"id": "m"`, `"id": "plain"`, `"default": "m"`}},
		{map[string]any{"harness": "one", "query": "PLAIN"}, []string{`"id": "plain"`}},
		{map[string]any{"harness": "one", "model": "m"}, []string{`"efforts": [`, `"default_effort": "low"`}},
	} {
		text, failed := callTool(t, parent, "list_models", tc.args)
		for _, want := range tc.want {
			if failed || !strings.Contains(text, want) {
				t.Errorf("list_models(%v) = %s, want %s", tc.args, text, want)
			}
		}
	}
	if text, _ := callTool(t, parent, "list_models", map[string]any{"harness": "one", "query": "plain"}); strings.Contains(text, `"id": "m"`) {
		t.Errorf("query kept other models: %s", text)
	}
	if text, failed := callTool(t, parent, "list_models", map[string]any{"harness": "one", "model": "gone"}); !failed || !strings.Contains(text, "not offered") {
		t.Errorf("list_models for an unknown model = %q", text)
	}
	if list := children(t, db, "session-1"); len(list) != 0 {
		t.Errorf("list_models started sessions: %+v", list)
	}
}
