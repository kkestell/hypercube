package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

func TestStandIn(t *testing.T) { harnesstest.Run() }

// dial opens one of the server's sockets with the page's origin.
func dial(t *testing.T, server *httptest.Server, path string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+path, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {server.URL}},
	})
}

// open returns a database in a temporary state directory.
func open(t *testing.T) *store.DB {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// handler returns the view's handler over a runtime whose agents are never
// started.
func handler(t *testing.T, db *store.DB) http.Handler {
	t.Helper()
	return Handler(t.Context(), db, session.New(db, ""))
}

func change(t *testing.T, db *store.DB, fn func(*store.Tx) error) {
	t.Helper()
	if err := db.Tx(fn); err != nil {
		t.Fatal(err)
	}
}

func TestLeads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "hypercube/config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{"profiles":[
		{"name":"first","role":"lead","description":"First lead","harness":"claude","model":"opus","effort":"high"},
		{"name":"worker","role":"work","description":"Worker","harness":"codex","model":"sol","effort":"high"},
		{"name":"second","role":"lead","description":"Second lead","harness":"codex","model":"sol","effort":"high"}
	]}`
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := handler(t, open(t))
	get := func() *httptest.ResponseRecorder {
		t.Helper()
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/leads", nil))
		return res
	}
	res := get()
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("leads response = %d %s", res.Code, res.Body)
	}
	type lead struct{ Name, Description string }
	var got []lead
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []lead{{"first", "First lead"}, {"second", "Second lead"}}) {
		t.Fatalf("leads = %+v", got)
	}
	// The same handler reads config edits instead of keeping a cached list.
	if err := os.WriteFile(path, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	res = get()
	if res.Code != http.StatusInternalServerError || !strings.Contains(res.Body.String(), path+": unexpected end of JSON input") {
		t.Fatalf("broken config response = %d %s", res.Code, res.Body)
	}
}

func TestIcon(t *testing.T) {
	res := httptest.NewRecorder()
	handler(t, open(t)).ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/favicon.svg", nil))
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("favicon response = %d, %v", res.Code, res.Header())
	}
	if len(Icon()) == 0 || !bytes.Equal(res.Body.Bytes(), Icon()) {
		t.Fatal("favicon differs from the menu bar icon")
	}
}

func TestDevReload(t *testing.T) {
	for _, revision := range []string{"", "build-1", "build-2"} {
		t.Run("revision="+revision, func(t *testing.T) {
			t.Setenv("HYPERCUBE_DEV_RELOAD", revision)
			handler := handler(t, open(t))
			page := httptest.NewRecorder()
			handler.ServeHTTP(page, httptest.NewRequest("GET", "http://localhost:4281/", nil))
			if page.Code != http.StatusOK {
				t.Fatalf("page status = %d", page.Code)
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4281/dev-reload", nil))
			if revision == "" {
				if strings.Contains(page.Body.String(), "/dev-reload") || res.Code != http.StatusNotFound {
					t.Fatal("production serves development reload")
				}
				return
			}
			encoded, _ := json.Marshal(revision)
			if !strings.Contains(page.Body.String(), "await response.text() !== "+string(encoded)) {
				t.Fatal("page does not check its source revision")
			}
			if res.Code != http.StatusOK || res.Body.String() != revision {
				t.Fatalf("revision response = %d %q", res.Code, res.Body.String())
			}
			if res.Header().Get("Cache-Control") != "no-store" || page.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("development responses must not be cached")
			}
		})
	}
}

func TestStateStreamsSnapshots(t *testing.T) {
	db := open(t)
	change(t, db, func(tx *store.Tx) error { return tx.AddWorkspace(store.Workspace{ID: "w", Path: "/src/repo"}) })
	server := httptest.NewServer(handler(t, db))
	defer server.Close()
	conn, _, err := dial(t, server, "/state")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	next := func() []store.ShownWorkspace {
		t.Helper()
		_, data, err := conn.Read(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var msg struct {
			Type       string
			Workspaces []store.ShownWorkspace
		}
		if err := json.Unmarshal(data, &msg); err != nil || msg.Type != "state" {
			t.Fatalf("message %s: %v", data, err)
		}
		return msg.Workspaces
	}
	if list := next(); len(list) != 1 || list[0].Name != "repo" || len(list[0].Sessions) != 0 {
		t.Fatalf("first snapshot = %+v", list)
	}
	change(t, db, func(tx *store.Tx) error {
		return tx.AddSession(store.Session{ID: "s", Workspace: "w", Title: "Fix it"})
	})
	if list := next(); len(list[0].Sessions) != 1 || list[0].Sessions[0].Title != "Fix it" || list[0].Sessions[0].Running {
		t.Fatalf("snapshot after a change = %+v", list)
	}
}

func TestRoutesRefuseOtherSites(t *testing.T) {
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		tx.AddWorkspace(store.Workspace{ID: "w", Path: "/src/repo"})
		tx.AddSession(store.Session{ID: "s", Workspace: "w"})
		return tx.AddAgent(store.Agent{ID: "a", Session: "s", Name: "lead", Role: store.Lead, Activity: store.Idle})
	})
	h := handler(t, db)
	send := func(method, host, origin, path string) int {
		req := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
		req.Header.Set("X-Hypercube-Agent", "a")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res.Code
	}
	for name, got := range map[string]int{
		"foreign host":          send("GET", "example.com", "", "/leads"),
		"rebound host":          send("POST", "example.com:4280", "http://example.com:4280", "/mcp"),
		"foreign origin":        send("POST", "localhost:4280", "http://example.com", "/mcp"),
		"no origin":             send("POST", "localhost:4280", "", "/mcp"),
		"foreign input":         send("POST", "localhost:4280", "http://example.com", "/agent?action=input&id=a"),
		"input without origin":  send("POST", "localhost:4280", "", "/agent?action=input&id=a"),
		"terminal foreign host": send("GET", "example.com", "", "/terminal?session=s"),
	} {
		if got != http.StatusForbidden {
			t.Errorf("%s: status %d", name, got)
		}
	}
	// The agents' own bridge sends its own address as the origin.
	if got := send("POST", "127.0.0.1:4280", "http://127.0.0.1:4280", "/mcp"); got != http.StatusOK {
		t.Errorf("the bridge: status %d", got)
	}
	// Another page cannot open the data or terminal sockets.
	server := httptest.NewServer(h)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, path := range []string{"/state", "/terminal?session=s"} {
		if _, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+path, &websocket.DialOptions{
			HTTPHeader: http.Header{"Origin": {"http://example.com"}},
		}); err == nil || res == nil || res.StatusCode != http.StatusForbidden {
			t.Fatalf("%s from a foreign origin: %v %v", path, res, err)
		}
	}
}

func TestActions(t *testing.T) {
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		tx.AddWorkspace(store.Workspace{ID: "w", Path: t.TempDir()})
		tx.AddSession(store.Session{ID: "s", Workspace: "w", Unseen: true})
		tx.AddSession(store.Session{ID: "t", Workspace: "w"})
		return tx.AddAgent(store.Agent{ID: "a", Session: "t", Name: "lead", Role: store.Lead, Activity: store.Exited})
	})
	server := httptest.NewServer(handler(t, db))
	defer server.Close()
	post := func(path string, want int) {
		t.Helper()
		req, _ := http.NewRequest("POST", server.URL+path, nil)
		req.Header.Set("Origin", server.URL)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("%s: status %d, want %d", path, res.StatusCode, want)
		}
	}
	post("/session?action=rm&id=s", http.StatusBadRequest)
	post("/agent?action=rm&id=a", http.StatusBadRequest)
	post("/agent?action=input&id=a", http.StatusConflict)
	post("/agent?action=input&id=missing", http.StatusConflict)
	post("/agent?action=interrupt&id=a", http.StatusConflict)
	post("/agent?action=answer&id=a&request=q&choice=yes", http.StatusConflict)
	post("/workspace?action=rm", http.StatusBadRequest)
	post("/session?action=rename&id=s&title=%20", http.StatusConflict)
	post("/session?action=rename&id=missing&title=x", http.StatusConflict)
	post("/session?action=rename&id=s&title=Parser", http.StatusOK)
	post("/session?action=seen&id=s", http.StatusOK)
	change(t, db, func(tx *store.Tx) error {
		if s, err := tx.Session("s"); err != nil || s.Title != "Parser" || s.Unseen {
			t.Errorf("session after rename and seen = %+v, %v", s, err)
		}
		return nil
	})
	post("/session?action=delete&id=s", http.StatusOK)
	change(t, db, func(tx *store.Tx) error {
		if list, _ := tx.Sessions("w"); len(list) != 1 || list[0].ID != "t" {
			t.Errorf("sessions after delete = %+v", list)
		}
		return nil
	})
	post("/workspace?action=delete&id=w", http.StatusOK)
	change(t, db, func(tx *store.Tx) error {
		if list, _ := tx.Workspaces(); len(list) != 0 {
			t.Errorf("workspaces after delete = %+v", list)
		}
		if _, err := tx.Agent("a"); err == nil {
			t.Error("the deleted workspace's agent remains")
		}
		return nil
	})
}

// app is the view over a runtime whose agents are the stand-in plugin.
type app struct {
	t       *testing.T
	server  *httptest.Server
	rt      *session.Runtime
	session store.Session
	lead    store.Agent
}

func newApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	plugins, workspace := filepath.Join(dir, "plugins"), filepath.Join(dir, "repo")
	logs := filepath.Join(dir, "logs")
	for _, d := range []string{plugins, workspace, logs, filepath.Join(dir, "config/hypercube")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("HYPERCUBE_TEST_LOGS", logs)
	harnesstest.Install(t, plugins, "test")
	config := `{"profiles":[
		{"name":"lead","role":"lead","description":"Lead","harness":"test","model":"m","effort":"low"},
		{"name":"worker","role":"work","description":"Worker","harness":"test","model":"m","effort":"low"}]}`
	if err := os.WriteFile(filepath.Join(dir, "config/hypercube/config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rt := session.New(db, "")
	rt.PluginDir = plugins
	rt.Terminals.Command = []string{"/bin/cat"}
	t.Cleanup(rt.Shutdown)
	w, err := session.AddWorkspace(db, workspace)
	if err != nil {
		t.Fatal(err)
	}
	s, err := rt.Create(w.ID, "lead")
	if err != nil {
		t.Fatal(err)
	}
	a := &app{t: t, rt: rt, session: s}
	a.server = httptest.NewServer(Handler(t.Context(), db, rt))
	t.Cleanup(a.server.Close)
	if err := db.Read(func(tx *store.Tx) error { var err error; a.lead, err = tx.Lead(s.ID); return err }); err != nil {
		t.Fatal(err)
	}
	return a
}

func (a *app) post(path, body string, want int) {
	a.t.Helper()
	req, _ := http.NewRequest("POST", a.server.URL+path, strings.NewReader(body))
	req.Header.Set("Origin", a.server.URL)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != want {
		a.t.Fatalf("%s: status %d, want %d", path, res.StatusCode, want)
	}
}

// page follows the data socket as the browser does, keeping the transcripts
// it has been sent.
type page struct {
	t       *testing.T
	conn    *websocket.Conn
	items   map[string][]harness.Item
	resets  int
	lastMsg string
}

func (a *app) page() *page {
	conn, _, err := dial(a.t, a.server, "/state")
	if err != nil {
		a.t.Fatal(err)
	}
	a.t.Cleanup(func() { conn.CloseNow() })
	return &page{t: a.t, conn: conn, items: map[string][]harness.Item{}}
}

func (p *page) send(msg string) {
	p.t.Helper()
	if err := p.conn.Write(p.t.Context(), websocket.MessageText, []byte(msg)); err != nil {
		p.t.Fatal(err)
	}
}

// until reads messages until check passes.
func (p *page) until(what string, check func() bool) {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(p.t.Context(), 5*time.Second)
	defer cancel()
	for !check() {
		_, data, err := p.conn.Read(ctx)
		if err != nil {
			p.t.Fatalf("waiting for %s: %v (last message %s)", what, err, p.lastMsg)
		}
		p.lastMsg = string(data)
		var msg struct {
			Type  string
			Agent string
			Reset bool
			Items []harness.Item
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			p.t.Fatal(err)
		}
		if msg.Type != "transcript" {
			continue
		}
		if msg.Reset {
			p.items[msg.Agent] = nil
			p.resets++
		}
		for _, it := range msg.Items {
			i := slices.IndexFunc(p.items[msg.Agent], func(have harness.Item) bool { return have.ID == it.ID })
			if i < 0 {
				p.items[msg.Agent] = append(p.items[msg.Agent], it)
			} else {
				p.items[msg.Agent][i] = it
			}
		}
	}
}

func (p *page) find(agent string, match func(harness.Item) bool) (harness.Item, bool) {
	for _, it := range p.items[agent] {
		if match(it) {
			return it, true
		}
	}
	return harness.Item{}, false
}

func TestTranscriptStreamsToTheSelectedSession(t *testing.T) {
	a := newApp(t)
	p := a.page()
	// Nothing is sent until the page selects a session.
	p.send(`{"select":"` + a.session.ID + `"}`)
	p.until("the first snapshot", func() bool { return p.resets == 1 })

	a.post("/agent?action=input&id="+a.lead.ID, "!stream", http.StatusOK)
	// The streamed text replaces itself in place instead of repeating, and the
	// tool call nests under it.
	p.until("the streamed turn", func() bool {
		text, ok := p.find(a.lead.ID, func(it harness.Item) bool { return it.Kind == harness.KindText })
		tool, found := p.find(a.lead.ID, func(it harness.Item) bool { return it.Kind == harness.KindTool })
		return ok && found && text.Text == "Hello!" && text.Done && tool.Done && tool.Output == "ok" && tool.Parent == text.ID
	})
	var kinds []string
	for _, it := range p.items[a.lead.ID] {
		kinds = append(kinds, it.Kind)
	}
	if !slices.Equal(kinds, []string{"user", "text", "tool"}) {
		t.Fatalf("transcript kinds = %v", kinds)
	}
	if p.resets != 1 {
		t.Fatalf("the page was sent %d snapshots, want 1", p.resets)
	}

	// A new connection gets the whole transcript as its first snapshot.
	q := a.page()
	q.send(`{"select":"` + a.session.ID + `"}`)
	q.until("the whole transcript", func() bool { return len(q.items[a.lead.ID]) == 3 })

	// Selecting nothing stops the transcripts.
	p.send(`{"select":""}`)
	a.post("/agent?action=input&id="+a.lead.ID, "again", http.StatusOK)
	q.until("the second turn", func() bool { return len(q.items[a.lead.ID]) == 5 })
	if len(p.items[a.lead.ID]) != 3 {
		t.Fatalf("an unselected page received items: %d", len(p.items[a.lead.ID]))
	}
}

func TestAnswerAndInterruptRoutes(t *testing.T) {
	a := newApp(t)
	p := a.page()
	p.send(`{"select":"` + a.session.ID + `"}`)
	a.post("/agent?action=input&id="+a.lead.ID, "!ask", http.StatusOK)
	var request harness.Item
	p.until("the request", func() bool {
		var ok bool
		request, ok = p.find(a.lead.ID, func(it harness.Item) bool { return it.Kind == harness.KindAttention })
		return ok
	})
	if request.Text != "Proceed?" || !slices.Equal(request.Choices, []string{"yes", "no"}) || request.Done {
		t.Fatalf("request = %+v", request)
	}
	a.post("/agent?action=answer&id="+a.lead.ID+"&request=missing&choice=yes", "", http.StatusConflict)
	a.post("/agent?action=answer&id="+a.lead.ID+"&request="+url.QueryEscape(request.ID)+"&choice=yes", " now", http.StatusOK)
	p.until("the answer", func() bool {
		it, _ := p.find(a.lead.ID, func(it harness.Item) bool { return it.Kind == harness.KindAttention })
		return it.Done && it.Answer == "yes now"
	})
	// A finished request cannot be answered again.
	a.post("/agent?action=answer&id="+a.lead.ID+"&request="+url.QueryEscape(request.ID)+"&choice=no", "", http.StatusConflict)

	a.post("/agent?action=input&id="+a.lead.ID, "!work", http.StatusOK)
	a.post("/agent?action=input&id="+a.lead.ID, "  ", http.StatusConflict)
	a.post("/agent?action=interrupt&id="+a.lead.ID, "", http.StatusOK)
}

func TestTerminalSocket(t *testing.T) {
	a := newApp(t)
	path := "/terminal?session=" + a.session.ID + "&cols=100&rows=30"
	conn, _, err := dial(t, a.server, path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	ctx := t.Context()
	if err := conn.Write(ctx, websocket.MessageBinary, []byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"cols":90,"rows":20}`)); err != nil {
		t.Fatal(err)
	}
	var got string
	for !strings.Contains(got, "hello") {
		kind, data, err := conn.Read(ctx)
		if err != nil || kind != websocket.MessageBinary {
			t.Fatalf("read %q: %v", got, err)
		}
		got += string(data)
	}

	// A second connection gets the earlier output at once, and the shell ends
	// with an exit message to every connection.
	other, _, err := dial(t, a.server, path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.CloseNow()
	if _, data, err := other.Read(ctx); err != nil || !strings.Contains(string(data), "hello") {
		t.Fatalf("replay %q: %v", data, err)
	}
	if err := conn.Write(ctx, websocket.MessageBinary, []byte{4}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*websocket.Conn{conn, other} {
		for {
			kind, data, err := c.Read(ctx)
			if err != nil {
				t.Fatalf("waiting for the exit: %v", err)
			}
			if kind == websocket.MessageText {
				if string(data) != `{"exit":0}` {
					t.Fatalf("exit message = %s", data)
				}
				break
			}
		}
	}

	// A stopped session has no shell.
	if err := a.rt.Stop(a.session.ID); err != nil {
		t.Fatal(err)
	}
	if _, res, err := dial(t, a.server, path); err == nil || res == nil || res.StatusCode != http.StatusConflict {
		t.Fatalf("terminal of a stopped session: %v %v", res, err)
	}
}
