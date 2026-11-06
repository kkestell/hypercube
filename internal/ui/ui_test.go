package ui

import (
	"bytes"
	"cmp"
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

	"github.com/kkestell/hypercube/internal/buildinfo"
	"github.com/kkestell/hypercube/internal/desktop"
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
	return Handler(db, session.New(db, ""), &desktop.Desktop{})
}

func change(t *testing.T, db *store.DB, fn func(*store.Tx) error) {
	t.Helper()
	if err := db.Tx(fn); err != nil {
		t.Fatal(err)
	}
}

func TestConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "hypercube/config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{"profiles":[
		{"name":"first","description":"First lead","harness":"claude","model":"opus","effort":"high"},
		{"name":"worker","description":"Worker","harness":"codex","model":"sol","effort":"high"},
		{"name":"second","description":"Second lead","harness":"codex","model":"sol","effort":"high"}
	]}`
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := handler(t, open(t))
	get := func() *httptest.ResponseRecorder {
		t.Helper()
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/config", nil))
		return res
	}
	res := get()
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("profiles response = %d %s", res.Code, res.Body)
	}
	var got harness.Config
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []harness.Profile{
		{Name: "first", Description: "First lead", Harness: "claude", Model: "opus", Effort: "high"},
		{Name: "worker", Description: "Worker", Harness: "codex", Model: "sol", Effort: "high"},
		{Name: "second", Description: "Second lead", Harness: "codex", Model: "sol", Effort: "high"},
	}
	if got.DefaultProfile != "first" || !slices.Equal(got.Profiles, want) {
		t.Fatalf("profiles = %+v", got)
	}
	// A malformed config is reported instead of returning an empty list.
	if err := os.WriteFile(path, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	res = get()
	if res.Code != http.StatusInternalServerError || !strings.Contains(res.Body.String(), path+": unexpected end of JSON input") {
		t.Fatalf("broken config response = %d %s", res.Code, res.Body)
	}
}

func TestFavicon(t *testing.T) {
	res := httptest.NewRecorder()
	handler(t, open(t)).ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/favicon.svg", nil))
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("favicon response = %d, %v", res.Code, res.Header())
	}
}

func TestVersion(t *testing.T) {
	res := httptest.NewRecorder()
	handler(t, open(t)).ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/version", nil))
	if res.Code != http.StatusOK || res.Body.String() != buildinfo.String()+"\n" || res.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("version response = %d, %v, %s", res.Code, res.Header(), res.Body)
	}
}

func TestRoutesRefuseOtherSites(t *testing.T) {
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		tx.AddWorkspace(store.Workspace{ID: "w", Path: "/src/repo"})
		tx.AddSession(store.Session{ID: "s", Workspace: "w"})
		return nil
	})
	h := handler(t, db)
	send := func(method, host, origin, path string) int {
		req := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
		req.Header.Set("X-Hypercube-Session", "s")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res.Code
	}
	for name, got := range map[string]int{
		"foreign host":          send("GET", "example.com", "", "/config"),
		"rebound host":          send("POST", "example.com:4280", "http://example.com:4280", "/mcp"),
		"foreign input":         send("POST", "localhost:4280", "http://example.com", "/session?action=input&id=a"),
		"input without origin":  send("POST", "localhost:4280", "", "/session?action=input&id=a"),
		"terminal foreign host": send("GET", "example.com", "", "/terminal?session=s"),
	} {
		if got != http.StatusForbidden {
			t.Errorf("%s: status %d", name, got)
		}
	}
	// An agent's request names no origin. The origin check lets it through to
	// the MCP handler, which refuses this stand-in for lacking a content type.
	if got := send("POST", "127.0.0.1:4280", "", "/mcp"); got == http.StatusForbidden {
		t.Errorf("the bridge: status %d", got)
	}
	// Another page cannot open the data or terminal sockets.
	server := httptest.NewServer(h)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, path := range []string{"/state", "/terminal?session=s", "/desktop"} {
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
		return nil
	})
	server := httptest.NewServer(handler(t, db))
	defer server.Close()
	post := func(path string, want int) {
		t.Helper()
		req, _ := http.NewRequest("POST", server.URL+path, strings.NewReader(`{"text":"hi"}`))
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
	post("/session?action=rm&id=a", http.StatusBadRequest)
	post("/session?action=input&id=a", http.StatusConflict)
	post("/session?action=input&id=missing", http.StatusConflict)
	post("/session?action=interrupt&id=a", http.StatusConflict)
	post("/session?action=answer&id=a&request=q&choice=yes", http.StatusConflict)
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
		if _, err := tx.Session("s"); err == nil {
			t.Error("the deleted workspace's agent remains")
		}
		return nil
	})
	dir := t.TempDir()
	post("/workspace?action=new&path="+url.QueryEscape(dir), http.StatusOK)
	change(t, db, func(tx *store.Tx) error {
		if list, _ := tx.Workspaces(); len(list) != 1 {
			t.Errorf("workspaces after adding %s = %+v", dir, list)
		}
		return nil
	})
}

// app is the view over a runtime whose agents are the stand-in plugin.
type app struct {
	t       *testing.T
	server  *httptest.Server
	db      *store.DB
	rt      *session.Runtime
	session store.Session
}

func newApp(t *testing.T) *app {
	t.Helper()
	dir, err := os.MkdirTemp("", "hc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
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
		{"name":"lead","description":"Lead","harness":"test","model":"m","effort":"low"},
		{"name":"worker","description":"Worker","harness":"test","model":"m","effort":"low"}]}`
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
	t.Cleanup(rt.Quit)
	w, err := rt.AddWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	s, err := rt.Create(w.ID, "lead", false, "start")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		items, err := rt.Transcript(s.ID)
		if err != nil {
			t.Fatal(err)
		}
		shown := slices.DeleteFunc(items, func(it session.TranscriptItem) bool { return it.Kind == harness.KindState })
		if len(shown) == 2 && shown[1].Done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("creation reply did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	a := &app{t: t, db: db, rt: rt, session: s}
	a.server = httptest.NewServer(Handler(db, rt, &desktop.Desktop{}))
	t.Cleanup(a.server.Close)
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
	t        *testing.T
	conn     *websocket.Conn
	items    map[string][]session.TranscriptItem // without state markers
	last     map[string]int64                    // the newest position received, markers included
	state    stateMessage
	messages []transcriptMessage
	opens    []openMessage
	resets   int
	lastMsg  string
}

func (a *app) page() *page {
	conn, _, err := dial(a.t, a.server, "/state")
	if err != nil {
		a.t.Fatal(err)
	}
	a.t.Cleanup(func() { conn.CloseNow() })
	return &page{t: a.t, conn: conn, items: map[string][]session.TranscriptItem{}, last: map[string]int64{}}
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
		var msg transcriptMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			p.t.Fatal(err)
		}
		if msg.Type == "state" {
			p.state = stateMessage{}
			if err := json.Unmarshal(data, &p.state); err != nil {
				p.t.Fatal(err)
			}
			if p.state.Build != buildID() {
				p.t.Fatalf("build = %q", p.state.Build)
			}
			continue
		}
		if msg.Type == "open" {
			var open openMessage
			if err := json.Unmarshal(data, &open); err != nil {
				p.t.Fatal(err)
			}
			p.opens = append(p.opens, open)
			continue
		}
		if msg.Type != "transcript" {
			p.t.Fatalf("unknown message: %s", data)
		}
		if msg.Session == "" {
			p.t.Fatalf("transcript without session: %s", data)
		}
		for i, item := range msg.Items {
			if item.Seq <= 0 || i > 0 && item.Seq <= msg.Items[i-1].Seq {
				p.t.Fatalf("unordered items: %s", data)
			}
		}
		p.messages = append(p.messages, msg)
		if msg.Reset {
			p.items[msg.Session] = nil
			p.resets++
		}
		for _, it := range msg.Items {
			p.last[msg.Session] = max(p.last[msg.Session], it.Seq)
			if it.Kind == harness.KindState {
				continue
			}
			i := slices.IndexFunc(p.items[msg.Session], func(have session.TranscriptItem) bool { return have.ID == it.ID })
			if i < 0 {
				p.items[msg.Session] = append(p.items[msg.Session], it)
			} else {
				p.items[msg.Session][i] = it
			}
		}
		slices.SortFunc(p.items[msg.Session], func(a, b session.TranscriptItem) int { return cmp.Compare(a.Seq, b.Seq) })
	}
}

func (p *page) find(agent string, match func(harness.Item) bool) (harness.Item, bool) {
	for _, it := range p.items[agent] {
		if match(it.Item) {
			return it.Item, true
		}
	}
	return harness.Item{}, false
}

// input posts the user's text to the session as an input action.
func (a *app) input(text string, want int) {
	a.t.Helper()
	body, _ := json.Marshal(map[string]any{"text": text})
	a.post("/session?action=input&id="+a.session.ID, string(body), want)
}

func TestTranscriptStreamsToTheSelectedSession(t *testing.T) {
	a := newApp(t)
	child := a.child("first")
	p := a.page()
	p.send(`{"select":"` + a.session.ID + `"}`)
	p.until("one reset per agent", func() bool { return p.resets == 1 })
	if len(p.items[a.session.ID]) != 2 || len(p.items[child.ID]) != 0 {
		t.Fatalf("reset items = %+v", p.items)
	}
	for _, message := range p.messages {
		if !message.Reset || message.Session != a.session.ID {
			t.Fatalf("reset = %+v", message)
		}
	}
	p.messages = nil
	known := p.last[a.session.ID]

	a.input("!work", http.StatusOK)
	p.until("streaming text", func() bool {
		item, ok := p.find(a.session.ID, func(it harness.Item) bool { return it.Text == "working" })
		return ok && !item.Done
	})
	a.input("!finish", http.StatusOK)
	p.until("replaced text", func() bool {
		item, ok := p.find(a.session.ID, func(it harness.Item) bool { return it.Text == "finished" })
		return ok && item.Done && len(p.items[a.session.ID]) == 5
	})
	a.input("!stream", http.StatusOK)
	p.until("the streamed turn", func() bool {
		text, ok := p.find(a.session.ID, func(it harness.Item) bool { return it.Kind == harness.KindText && it.Text == "Hello!" })
		tool, found := p.find(a.session.ID, func(it harness.Item) bool { return it.Kind == harness.KindTool })
		return ok && found && text.Done && tool.Done && tool.Output == "ok" && tool.Parent == text.ID && len(p.items[a.session.ID]) == 8
	})
	for _, message := range p.messages {
		if message.Reset || message.Session != a.session.ID {
			t.Fatalf("update = %+v", message)
		}
		for _, item := range message.Items {
			if item.Seq <= known {
				t.Fatalf("unchanged item in update: %+v", item)
			}
		}
	}

	// A child's transcript arrives only when it is selected.
	later := a.child("later")
	if err := a.rt.Input(later.ID, "!work"); err != nil {
		t.Fatal(err)
	}

	q := a.page()
	q.send(`{"select":"` + a.session.ID + `"}`)
	q.until("the whole transcript", func() bool { return q.resets == 1 })
	if len(q.items[a.session.ID]) != 8 || len(q.items[later.ID]) != 0 {
		t.Fatalf("reconnected items = %+v", q.items)
	}
	first, _ := json.Marshal(p.items[a.session.ID])
	second, _ := json.Marshal(q.items[a.session.ID])
	if !bytes.Equal(first, second) {
		t.Fatalf("reconnected transcript differs: %s / %s", first, second)
	}

	p.send(`{"select":""}`)
	a.input("again", http.StatusOK)
	q.until("the second turn", func() bool { return len(q.items[a.session.ID]) == 10 })
	if err := a.rt.Rename(a.session.ID, "Unselected"); err != nil {
		t.Fatal(err)
	}
	p.until("the state after deselection", func() bool { return p.session(a.session.ID).Title == "Unselected" })
	if len(p.items[a.session.ID]) != 8 {
		t.Fatalf("unselected page received items: %d", len(p.items[a.session.ID]))
	}
}

func (a *app) child(name string) store.Session {
	a.t.Helper()
	child, err := a.rt.CreateChild(a.session.ID, session.ChildOptions{Name: name, Title: "Title", Profile: harness.Profile{Harness: "test", Model: "m", Effort: "low"}, Dir: ".", Message: ""})
	if err != nil {
		a.t.Fatal(err)
	}
	return child
}

func (p *page) session(id string) shownSession {
	for _, workspace := range p.state.Workspaces {
		for _, session := range workspace.Sessions {
			if session.ID == id {
				return session
			}
			for _, child := range session.Children {
				if child.ID == id {
					return child
				}
			}
		}
	}
	return shownSession{}
}

func TestSnapshotOrder(t *testing.T) {
	db := open(t)
	seedSnapshot(t, db, "b", "/src/zeta", "s1")
	seedSnapshot(t, db, "a", "/src/alpha", "s2")
	seedSnapshot(t, db, "b", "/src/zeta", "s3", "w")
	seedSnapshot(t, db, "b", "/src/zeta", "s4")
	seedSnapshot(t, db, "b", "/src/zeta", "s5")
	change(t, db, func(tx *store.Tx) error {
		for i, id := range []string{"s1", "s3", "s4", "s5"} {
			if i >= 1 {
				s, err := tx.Session(id)
				if err != nil {
					return err
				}
				s.Title = "Title " + id
				if err := tx.SaveSession(s); err != nil {
					return err
				}
			}
		}
		return nil
	})
	list, _, err := snapshot(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "zeta" {
		t.Fatalf("workspaces = %+v", list)
	}
	z := list[1].Sessions
	if len(z) != 4 || z[0].ID != "s5" || z[1].ID != "s4" || z[2].ID != "s3" || z[3].ID != "s1" || z[2].Title != "Title s3" || len(z[2].Children) != 1 || z[2].Children[0].Parent != "s3" {
		t.Fatalf("sessions = %+v", z)
	}
}

func seedSnapshot(t *testing.T, db *store.DB, workspace, path, id string, children ...string) {
	t.Helper()
	change(t, db, func(tx *store.Tx) error {
		if _, err := tx.Workspace(workspace); err != nil {
			if err := tx.AddWorkspace(store.Workspace{ID: workspace, Path: path}); err != nil {
				return err
			}
		}
		if err := tx.AddSession(store.Session{ID: id, Workspace: workspace}); err != nil {
			return err
		}
		for _, name := range children {
			if err := tx.AddSession(store.Session{ID: id + "-" + name, Workspace: workspace, Parent: id, Name: name, Activity: store.Idle}); err != nil {
				return err
			}
		}
		return nil
	})
}
