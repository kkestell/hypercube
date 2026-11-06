// Package desktop is the backend's side of its connection to the Electron
// desktop app, which shows the window, the notifications, and each session's
// browser. The app connects to the backend's /desktop WebSocket, one JSON
// object per text frame: the backend sends requests and alerts, and the app
// replies to each request with its ID. The backend runs without the app; its
// calls then fail with ErrNoApp.
package desktop

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/coder/websocket"

	"github.com/kkestell/hypercube/internal/session"
)

// ErrNoApp is the error of browser calls made while the desktop app is not
// connected.
var ErrNoApp = errors.New("the browser needs the Hypercube app; open it")

// Desktop holds the desktop app's current connection. Its zero value has none.
type Desktop struct {
	mu      sync.Mutex
	current *conn
	closed  bool
}

// ServeHTTP accepts the app's connection and serves it until it ends. A newer
// connection replaces an older one.
func (d *Desktop) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	// The app's replies carry whole page values, and the app is a trusted local
	// process.
	ws.SetReadLimit(-1)
	c := &conn{ws: ws, done: make(chan struct{})}
	old, ok := d.attach(c)
	if !ok {
		ws.CloseNow()
		return
	}
	if old != nil {
		old.ws.CloseNow()
	}
	c.serve(r.Context())
	d.detach(c)
}

// attach makes c current and returns the connection it replaces. It fails
// after Close.
func (d *Desktop) attach(c *conn) (old *conn, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, false
	}
	old, d.current = d.current, c
	return old, true
}

func (d *Desktop) detach(c *conn) {
	d.mu.Lock()
	if d.current == c {
		d.current = nil
	}
	d.mu.Unlock()
}

// Shutdown ends the current connection, waits for it to finish, and refuses
// later ones. The HTTP server does not own upgraded connections, so the
// backend closes them here.
func (d *Desktop) Shutdown() {
	d.mu.Lock()
	d.closed = true
	c := d.current
	d.mu.Unlock()
	if c != nil {
		c.ws.CloseNow()
		<-c.done
	}
}

func (d *Desktop) conn() (*conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.current == nil {
		return nil, ErrNoApp
	}
	return d.current, nil
}

// Alert asks the app to notify the user of the alert, with a sound when sound
// is set. Without the app, the alert is dropped.
func (d *Desktop) Alert(a session.Alert, sound bool) error {
	c, err := d.conn()
	if err != nil {
		return nil
	}
	return c.send(map[string]any{"type": "alert", "session": a.Session, "title": a.Title, "subtitle": a.Subtitle, "text": a.Text, "sound": sound})
}

// Navigate loads url in the session's browser, creating it, and returns the
// page's URL and title once it has loaded.
func (d *Desktop) Navigate(ctx context.Context, session, url string) (string, string, error) {
	c, err := d.conn()
	if err != nil {
		return "", "", err
	}
	r, err := c.call(ctx, map[string]any{"type": "navigate", "session": session, "url": url})
	return r.URL, r.Title, err
}

// Eval runs script as the body of an async function in the session's page,
// and returns JSON.stringify of its result.
func (d *Desktop) Eval(ctx context.Context, session, script string) (string, error) {
	c, err := d.conn()
	if err != nil {
		return "", err
	}
	r, err := c.call(ctx, map[string]any{"type": "eval", "session": session, "script": script})
	return r.Result, err
}

// Close closes the session's browser, if it has one.
func (d *Desktop) Close(ctx context.Context, session string) error {
	c, err := d.conn()
	if err != nil {
		return err
	}
	_, err = c.call(ctx, map[string]any{"type": "close", "session": session})
	return err
}
