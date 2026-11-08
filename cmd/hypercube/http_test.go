package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/kkestell/hypercube/internal/buildinfo"
	"github.com/kkestell/hypercube/internal/desktop"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

// openHTTPStore returns a database in a temporary state directory.
func openHTTPStore(t *testing.T) *store.DB {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// testHandler serves a runtime whose agents are never started.
func testHandler(t *testing.T, db *store.DB) http.Handler {
	t.Helper()
	return handler(db, session.New(db, "", session.Programs{}), &desktop.Desktop{})
}

func TestVersion(t *testing.T) {
	res := httptest.NewRecorder()
	testHandler(t, openHTTPStore(t)).ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/version", nil))
	if res.Code != http.StatusOK || res.Body.String() != buildinfo.String()+"\n" || res.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("version response = %d, %v, %s", res.Code, res.Header(), res.Body)
	}
}

func TestRoutesRefuseOtherSites(t *testing.T) {
	db := openHTTPStore(t)
	if err := db.Tx(func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: "/src/repo"}); err != nil {
			return err
		}
		return tx.AddSession(store.Session{ID: "s", Workspace: "w"})
	}); err != nil {
		t.Fatal(err)
	}
	h := testHandler(t, db)
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

func TestPageRefusesForeignFrames(t *testing.T) {
	res := httptest.NewRecorder()
	testHandler(t, openHTTPStore(t)).ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/", nil))
	if res.Code != http.StatusOK || res.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Fatalf("page response = %d, %v", res.Code, res.Header())
	}
}

func TestRouteFallthrough(t *testing.T) {
	h := testHandler(t, openHTTPStore(t))
	for _, tt := range []struct {
		method, path, allow string
		status              int
	}{
		{"GET", "/mcp", "", http.StatusNotFound},
		{"HEAD", "/mcp", "", http.StatusNotFound},
		{"POST", "/version", "GET, HEAD", http.StatusMethodNotAllowed},
		{"POST", "/mcp", "", http.StatusForbidden},
		{"OPTIONS", "/mcp", "GET, HEAD, POST", http.StatusMethodNotAllowed},
		{"PUT", "/mcp", "GET, HEAD, POST", http.StatusMethodNotAllowed},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://localhost:4280"+tt.path, nil)
			req.Header.Set("Origin", "http://localhost:4280")
			req.Header.Set("X-Hypercube-Session", "unknown")
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != tt.status || res.Header().Get("Allow") != tt.allow {
				t.Fatalf("response = %d, %v, %s", res.Code, res.Header(), res.Body)
			}
		})
	}
}
