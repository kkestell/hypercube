// Package desktop sends notifications to the Electron app over its /desktop
// WebSocket. The backend runs without the app; alerts are then dropped.
package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/coder/websocket"

	"github.com/kkestell/hypercube/internal/session"
)

// conn is one connection from the app.
type conn struct {
	ws   *websocket.Conn
	done chan struct{} // closed when the HTTP handler finishes
}

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
	c := &conn{ws: ws, done: make(chan struct{})}
	old, ok := d.attach(c)
	if !ok {
		ws.CloseNow()
		return
	}
	if old != nil {
		old.ws.CloseNow()
	}
	defer close(c.done)
	defer ws.CloseNow()
	<-ws.CloseRead(r.Context()).Done()
	d.detach(c)
}

// attach makes c current and returns the connection it replaces. It fails
// after Shutdown.
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

// Alert asks the app to notify the user of the alert, with a sound when sound
// is set. Without the app, the alert is dropped.
func (d *Desktop) Alert(a session.Alert, sound bool) error {
	d.mu.Lock()
	c := d.current
	d.mu.Unlock()
	if c == nil {
		return nil
	}
	data, err := json.Marshal(map[string]any{"type": "alert", "session": a.Session, "title": a.Title, "subtitle": a.Subtitle, "text": a.Text, "sound": sound})
	if err != nil {
		return err
	}
	if err := c.ws.Write(context.Background(), websocket.MessageText, data); err != nil {
		return fmt.Errorf("sending to the Hypercube app: %w", err)
	}
	return nil
}
