package ui

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
	"time"

	"github.com/coder/websocket"

	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

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
		var list []store.ShownWorkspace
		if err := json.Unmarshal(data, &list); err != nil {
			t.Fatal(err)
		}
		return list
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
		"foreign host":   send("GET", "example.com", "", "/leads"),
		"rebound host":   send("POST", "example.com:4280", "http://example.com:4280", "/mcp"),
		"foreign origin": send("POST", "localhost:4280", "http://example.com", "/mcp"),
		"no origin":      send("POST", "localhost:4280", "", "/mcp"),
	} {
		if got != http.StatusForbidden {
			t.Errorf("%s: status %d", name, got)
		}
	}
	// The agents' own bridge sends its own address as the origin.
	if got := send("POST", "127.0.0.1:4280", "http://127.0.0.1:4280", "/mcp"); got != http.StatusOK {
		t.Errorf("the bridge: status %d", got)
	}
	// Another page cannot open the data socket.
	server := httptest.NewServer(h)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/state", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {"http://example.com"}},
	}); err == nil || res == nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin: %v %v", res, err)
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
