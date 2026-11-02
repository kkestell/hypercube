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
	return Handler(db, session.New(db, ""))
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
	type parent struct{ Name, Description, Harness, Model, Effort string }
	var got []parent
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []parent{{"first", "First lead", "claude", "opus", "high"}, {"second", "Second lead", "codex", "sol", "high"}}) {
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

func TestStateStreamsSnapshots(t *testing.T) {
	a := newApp(t)
	p := a.page()
	p.until("the first state", func() bool { return len(p.state.Workspaces) == 1 })
	if p.state.Build != buildID() || p.state.Requests == nil {
		t.Fatalf("first state = %+v", p.state)
	}
	w, err := a.rt.AddWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p.until("the new workspace", func() bool { return len(p.state.Workspaces) == 2 })
	s, err := a.rt.Create(w.ID, "lead", "Fix it")
	if err != nil {
		t.Fatal(err)
	}
	p.until("the new session", func() bool {
		for _, workspace := range p.state.Workspaces {
			if workspace.ID != w.ID {
				continue
			}
			return len(workspace.Sessions) == 1 && workspace.Sessions[0].ID == s.ID &&
				workspace.Sessions[0].Title == "Fix it" && workspace.Sessions[0].Running &&
				workspace.Sessions[0].Harness == "test"
		}
		return false
	})
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
		"foreign host":          send("GET", "example.com", "", "/leads"),
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
	t.Cleanup(rt.Quit)
	w, err := rt.AddWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	s, err := rt.Create(w.ID, "lead", "start")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		items, err := rt.Transcript(s.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) == 2 && items[1].Done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("creation reply did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	a := &app{t: t, db: db, rt: rt, session: s}
	a.server = httptest.NewServer(Handler(db, rt))
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
	items    map[string][]session.TranscriptItem
	state    stateMessage
	messages []transcriptMessage
	resets   int
	lastMsg  string
}

func (a *app) page() *page {
	conn, _, err := dial(a.t, a.server, "/state")
	if err != nil {
		a.t.Fatal(err)
	}
	a.t.Cleanup(func() { conn.CloseNow() })
	return &page{t: a.t, conn: conn, items: map[string][]session.TranscriptItem{}}
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

func TestImageInput(t *testing.T) {
	a := newApp(t)
	p := a.page()
	p.send(`{"select":"` + a.session.ID + `"}`)
	path := "/session?action=input&id=" + a.session.ID
	a.post(path, "not json", http.StatusBadRequest)
	a.post(path, `{"images":[{"mime_type":"image/png","data":"bm90IGEgcG5n"}]}`, http.StatusConflict)
	a.post(path, `{"images":[{"mime_type":"image/svg+xml","data":"PHN2Zy8+"}]}`, http.StatusConflict)
	a.post(path, `{"text":" "}`, http.StatusConflict)
	a.post(path, `{"images":[{"mime_type":"image/png","data":"iVBORw0KGgo="}]}`, http.StatusOK)
	p.until("the image prompt", func() bool {
		it, ok := p.find(a.session.ID, func(it harness.Item) bool { return it.Kind == harness.KindUser && len(it.Images) > 0 })
		return ok && len(it.Images) == 1 && it.Images[0].Data == "iVBORw0KGgo="
	})
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
			if item.Seq <= 2 {
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
	child, err := a.rt.CreateChild(a.session.ID, name, "Title", harness.Profile{Harness: "test", Model: "m", Effort: "low"}, ".", "")
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

func TestRepliesAndRequestsReachEveryPage(t *testing.T) {
	a := newApp(t)
	child := a.child("worker")
	pages := []*page{a.page(), a.page()}
	for _, p := range pages {
		p.until("the parent's reply", func() bool {
			s := p.session(a.session.ID)
			return s.Unseen && s.Reply == "ack" && s.ReplySeq == 2
		})
	}
	a.input("!work", http.StatusOK)
	for _, p := range pages {
		p.until("the reply to clear while the parent works", func() bool {
			s := p.session(a.session.ID)
			return s.Unseen && s.Reply == "" && s.ReplySeq == 0 && s.Activity == store.Busy
		})
	}
	a.input("!finish", http.StatusOK)
	for _, p := range pages {
		p.until("the new reply", func() bool {
			s := p.session(a.session.ID)
			return s.Reply == "finished" && s.ReplySeq > 2
		})
	}
	a.input("!ask", http.StatusOK)
	if err := a.rt.Input(child.ID, "!ask"); err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		p.until("requests from parent and child", func() bool { return len(p.state.Requests) == 2 })
		if p.state.Requests[0].Session == p.state.Requests[1].Session {
			t.Fatalf("requests = %+v", p.state.Requests)
		}
	}
	for _, request := range pages[0].state.Requests {
		a.post("/session?action=answer&id="+request.Session+"&request="+url.QueryEscape(request.Item.ID)+"&choice=yes", "", http.StatusOK)
	}
	for _, p := range pages {
		p.until("answered requests to clear", func() bool { return len(p.state.Requests) == 0 })
	}
	a.post("/session?action=seen&id="+a.session.ID, "", http.StatusOK)
	for _, p := range pages {
		p.until("seen replies to clear", func() bool { s := p.session(a.session.ID); return !s.Unseen && s.Reply == "" && s.ReplySeq == 0 })
	}
	for _, input := range []string{"!work", "!finish"} {
		if err := a.rt.Input(child.ID, input); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range pages {
		p.until("the child's reply", func() bool {
			s := p.session(child.ID)
			return s.Unseen && s.Reply == "finished" && s.ReplySeq > 0
		})
	}
	a.post("/session?action=seen&id="+child.ID, "", http.StatusOK)
	for _, p := range pages {
		p.until("the child's reply to clear", func() bool { s := p.session(child.ID); return !s.Unseen && s.Reply == "" && s.ReplySeq == 0 })
	}
}

func TestAnswerAndInterruptRoutes(t *testing.T) {
	a := newApp(t)
	p := a.page()
	p.send(`{"select":"` + a.session.ID + `"}`)
	a.input("!ask", http.StatusOK)
	var request harness.Item
	p.until("the request", func() bool {
		var ok bool
		request, ok = p.find(a.session.ID, func(it harness.Item) bool { return it.Kind == harness.KindAttention })
		return ok
	})
	if request.Text != "Proceed?" || !slices.Equal(request.Choices, []string{"yes", "no"}) || request.Done {
		t.Fatalf("request = %+v", request)
	}
	a.post("/session?action=answer&id="+a.session.ID+"&request=missing&choice=yes", "", http.StatusConflict)
	a.post("/session?action=answer&id="+a.session.ID+"&request="+url.QueryEscape(request.ID)+"&choice=yes", " now", http.StatusOK)
	p.until("the answer", func() bool {
		it, _ := p.find(a.session.ID, func(it harness.Item) bool { return it.Kind == harness.KindAttention })
		return it.Done && it.Answer == "yes now"
	})
	// A finished request cannot be answered again.
	a.post("/session?action=answer&id="+a.session.ID+"&request="+url.QueryEscape(request.ID)+"&choice=no", "", http.StatusConflict)

	a.input("!work", http.StatusOK)
	a.input("  ", http.StatusConflict)
	a.post("/session?action=interrupt&id="+a.session.ID, "", http.StatusOK)
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

// A session is created by its first prompt: a refused prompt makes no session.
func TestNewSessionTakesItsFirstPrompt(t *testing.T) {
	a := newApp(t)
	count := func() (n int) {
		t.Helper()
		if err := a.db.Read(func(tx *store.Tx) error {
			list, err := tx.Sessions(a.session.Workspace)
			n = len(list)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	path := "/session?action=new&workspace=" + a.session.Workspace + "&lead=lead"
	a.post(path, `{`, http.StatusBadRequest)
	a.post(path, `{"text":" "}`, http.StatusConflict)
	if count() != 1 {
		t.Fatalf("a refused prompt left %d sessions", count())
	}
	req, _ := http.NewRequest("POST", a.server.URL+path, strings.NewReader(`{"text":"Fix the parser\nand the lexer"}`))
	req.Header.Set("Origin", a.server.URL)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || count() != 2 {
		t.Fatalf("new session: status %d, %d sessions", res.StatusCode, count())
	}
	var created struct{ Session string }
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("new session content type = %q", res.Header.Get("Content-Type"))
	}
	var title string
	var parent store.Session
	if err := a.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(created.Session)
		title = s.Title
		if err == nil {
			parent = s
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	items, err := a.rt.Transcript(parent.ID)
	if err != nil || title != "Fix the parser" || len(items) == 0 || items[0].Text != "Fix the parser\nand the lexer" {
		t.Fatalf("title %q, transcript %+v, %v", title, items, err)
	}
}

func TestSnapshotOrder(t *testing.T) {
	db := open(t)
	seedSnapshot(t, db, "b", "/src/zeta", "s1", "lead")
	seedSnapshot(t, db, "a", "/src/alpha", "s2", "lead")
	seedSnapshot(t, db, "b", "/src/zeta", "s3", "lead", "w")
	seedSnapshot(t, db, "b", "/src/zeta", "s4", "lead")
	seedSnapshot(t, db, "b", "/src/zeta", "s5", "lead")
	change(t, db, func(tx *store.Tx) error {
		for i, id := range []string{"s1", "s3", "s4", "s5"} {
			if err := tx.TouchSession(id, int64(i+1)); err != nil {
				return err
			}
			if i >= 1 {
				s, err := tx.Session(id)
				if err != nil {
					return err
				}
				s.Title = "Title " + id
				s.Running = i < 2
				if err := tx.SaveSession(s); err != nil {
					return err
				}
			}
		}
		return nil
	})
	list, err := snapshot(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "zeta" {
		t.Fatalf("workspaces = %+v", list)
	}
	z := list[1].Sessions
	if len(z) != 4 || z[0].ID != "s3" || z[1].ID != "s1" || z[2].ID != "s5" || z[3].ID != "s4" || z[0].Title != "Title s3" || len(z[0].Children) != 1 || z[0].Children[0].Parent != "s3" {
		t.Fatalf("sessions = %+v", z)
	}
}

func seedSnapshot(t *testing.T, db *store.DB, workspace, path, id string, names ...string) {
	t.Helper()
	change(t, db, func(tx *store.Tx) error {
		if _, err := tx.Workspace(workspace); err != nil {
			if err := tx.AddWorkspace(store.Workspace{ID: workspace, Path: path}); err != nil {
				return err
			}
		}
		if err := tx.AddSession(store.Session{ID: id, Workspace: workspace, Running: true}); err != nil {
			return err
		}
		for _, name := range names[1:] {
			if err := tx.AddSession(store.Session{ID: id + "-" + name, Workspace: workspace, Parent: id, Name: name, Activity: store.Idle}); err != nil {
				return err
			}
		}
		return nil
	})
}
