package desktop

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/kkestell/hypercube/internal/session"
)

// until waits for check, which polls the Desktop's state.
func until(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// connected reports whether the Desktop has a connection.
func connected(d *Desktop) func() bool {
	return func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.current != nil
	}
}

func serve(t *testing.T, d *Desktop) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(d)
	t.Cleanup(server.Close)
	return server
}

// dial connects a stand-in for the Electron app to the server.
func dial(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.CloseNow() })
	ws.SetReadLimit(-1)
	return ws
}

// request reads the next message the Desktop sent.
func request(t *testing.T, ws *websocket.Conn) map[string]any {
	t.Helper()
	_, data, err := ws.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAlert(t *testing.T) {
	d := &Desktop{}
	ws := dial(t, serve(t, d))
	until(t, "the connection", connected(d))
	if err := d.Alert(session.Alert{Session: "s1", Title: "T", Subtitle: "W", Text: "hello"}, true); err != nil {
		t.Fatal(err)
	}
	m := request(t, ws)
	if m["type"] != "alert" || m["session"] != "s1" || m["title"] != "T" || m["subtitle"] != "W" || m["text"] != "hello" || m["sound"] != true {
		t.Errorf("alert = %v", m)
	}
}

func TestWithoutTheApp(t *testing.T) {
	if err := (&Desktop{}).Alert(session.Alert{}, true); err != nil {
		t.Errorf("Alert = %v, want the alert dropped", err)
	}
}

func TestNewerConnectionReplacesOlder(t *testing.T) {
	d := &Desktop{}
	server := serve(t, d)
	old := dial(t, server)
	until(t, "the first connection", connected(d))
	newer := dial(t, server)
	// The replaced connection is closed.
	if _, _, err := old.Read(t.Context()); err == nil {
		t.Fatal("the older connection stayed open")
	}
	if err := d.Alert(session.Alert{Text: "newer"}, false); err != nil {
		t.Fatal(err)
	}
	if m := request(t, newer); m["text"] != "newer" {
		t.Errorf("alert on newer connection = %v", m)
	}
	// The older connection's end leaves the newer one current.
	if !connected(d)() {
		t.Error("the older connection's end cleared the newer one")
	}
}

func TestShutdownClosesTheConnection(t *testing.T) {
	d := &Desktop{}
	server := serve(t, d)
	ws := dial(t, server)
	until(t, "the connection", connected(d))
	d.Shutdown()
	if _, _, err := ws.Read(t.Context()); err == nil {
		t.Fatal("the connection stayed open")
	}
	// A late connection is refused.
	late, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err == nil {
		defer late.CloseNow()
		if _, _, err := late.Read(t.Context()); err == nil {
			t.Fatal("a connection after Shutdown stayed open")
		}
	}
	if connected(d)() {
		t.Error("a connection was current after Shutdown")
	}
}
