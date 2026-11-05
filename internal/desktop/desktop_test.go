package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	return func() bool { _, err := d.conn(); return err == nil }
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

// respond answers the request with the JSON reply, which carries its ID.
func respond(t *testing.T, ws *websocket.Conn, m map[string]any, reply string) {
	t.Helper()
	line := fmt.Sprintf(`{"id":%d,%s}`, int(m["id"].(float64)), reply)
	if err := ws.Write(t.Context(), websocket.MessageText, []byte(line)); err != nil {
		t.Fatal(err)
	}
}

// async runs fn on its own goroutine and returns what it returned.
func async[T any](fn func() T) func() T {
	done := make(chan T, 1)
	go func() { done <- fn() }()
	return func() T { return <-done }
}

func TestRequests(t *testing.T) {
	d := &Desktop{}
	ws := dial(t, serve(t, d))
	until(t, "the connection", connected(d))
	ctx := t.Context()

	wait := async(func() error {
		url, title, err := d.Navigate(ctx, "s1", "example.com")
		if url != "https://example.com/" || title != "Example" {
			t.Errorf("Navigate = %q, %q", url, title)
		}
		return err
	})
	m := request(t, ws)
	if m["type"] != "navigate" || m["session"] != "s1" || m["url"] != "example.com" {
		t.Errorf("navigate request = %v", m)
	}
	respond(t, ws, m, `"url":"https://example.com/","title":"Example"`)
	if err := wait(); err != nil {
		t.Fatal(err)
	}

	wait = async(func() error {
		_, _, err := d.Navigate(ctx, "s1", "x")
		return err
	})
	respond(t, ws, request(t, ws), `"error":"x is not a URL"`)
	if err := wait(); err == nil || err.Error() != "x is not a URL" {
		t.Errorf("Navigate error = %v", err)
	}

	wait = async(func() error {
		text, err := d.Eval(ctx, "s1", "return 1")
		if text != "1" {
			t.Errorf("Eval = %q", text)
		}
		return err
	})
	m = request(t, ws)
	if m["type"] != "eval" || m["session"] != "s1" || m["script"] != "return 1" {
		t.Errorf("eval request = %v", m)
	}
	respond(t, ws, m, `"result":"1"`)
	if err := wait(); err != nil {
		t.Fatal(err)
	}

	wait = async(func() error { return d.Close(ctx, "s1") })
	m = request(t, ws)
	if m["type"] != "close" || m["session"] != "s1" {
		t.Errorf("close request = %v", m)
	}
	respond(t, ws, m, `"x":0`)
	if err := wait(); err != nil {
		t.Fatal(err)
	}

	if err := d.Alert(session.Alert{Session: "s1", Title: "T", Subtitle: "W", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	m = request(t, ws)
	if m["type"] != "alert" || m["session"] != "s1" || m["title"] != "T" || m["subtitle"] != "W" || m["text"] != "hello" {
		t.Errorf("alert = %v", m)
	}
}

func TestLargeEvalResult(t *testing.T) {
	d := &Desktop{}
	ws := dial(t, serve(t, d))
	until(t, "the connection", connected(d))
	// Page values can far exceed a WebSocket library's default message size.
	big := strings.Repeat("x", 4<<20)
	wait := async(func() string {
		text, err := d.Eval(t.Context(), "s1", "return big")
		if err != nil {
			t.Error(err)
		}
		return text
	})
	respond(t, ws, request(t, ws), `"result":"`+big+`"`)
	if got := wait(); got != big {
		t.Errorf("result has %d bytes, want %d", len(got), len(big))
	}
}

func TestCancelledCallDropsLateReply(t *testing.T) {
	d := &Desktop{}
	ws := dial(t, serve(t, d))
	until(t, "the connection", connected(d))
	ctx, cancel := context.WithCancel(t.Context())
	wait := async(func() error { return d.Close(ctx, "s1") })
	first := request(t, ws)
	cancel()
	if err := wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled call = %v", err)
	}
	// The cancelled call's reply arrives before the next call's, and must not
	// be taken for it.
	wait = async(func() error { return d.Close(t.Context(), "s2") })
	second := request(t, ws)
	respond(t, ws, first, `"error":"late"`)
	respond(t, ws, second, `"x":0`)
	if err := wait(); err != nil {
		t.Fatalf("second call = %v", err)
	}
}

func TestWithoutTheApp(t *testing.T) {
	d := &Desktop{}
	ctx := t.Context()
	if _, _, err := d.Navigate(ctx, "s1", "example.com"); !errors.Is(err, ErrNoApp) {
		t.Errorf("Navigate = %v, want ErrNoApp", err)
	}
	if _, err := d.Eval(ctx, "s1", "return 1"); !errors.Is(err, ErrNoApp) {
		t.Errorf("Eval = %v, want ErrNoApp", err)
	}
	if err := d.Close(ctx, "s1"); !errors.Is(err, ErrNoApp) {
		t.Errorf("Close = %v, want ErrNoApp", err)
	}
	if err := d.Alert(session.Alert{}); err != nil {
		t.Errorf("Alert = %v, want the alert dropped", err)
	}
}

func TestDisconnectFailsPendingCalls(t *testing.T) {
	d := &Desktop{}
	ws := dial(t, serve(t, d))
	until(t, "the connection", connected(d))
	wait := async(func() error { return d.Close(t.Context(), "s1") })
	request(t, ws)
	ws.CloseNow()
	if err := wait(); !errors.Is(err, errClosed) {
		t.Errorf("pending call = %v, want errClosed", err)
	}
	until(t, "the connection to clear", func() bool { return !connected(d)() })
	if err := d.Close(t.Context(), "s1"); !errors.Is(err, ErrNoApp) {
		t.Errorf("later call = %v, want ErrNoApp", err)
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
	wait := async(func() error { return d.Close(t.Context(), "s1") })
	respond(t, newer, request(t, newer), `"x":0`)
	if err := wait(); err != nil {
		t.Fatalf("call on the newer connection = %v", err)
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
