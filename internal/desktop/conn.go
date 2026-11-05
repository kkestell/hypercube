package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/coder/websocket"
)

// errClosed is the error of calls still waiting when their connection ended.
var errClosed = errors.New("the Hypercube app closed")

// conn is one connection from the app.
type conn struct {
	ws *websocket.Conn
	// done closes when serve returns.
	done chan struct{}

	mu      sync.Mutex
	next    int
	pending map[int]chan reply // calls awaiting a reply, by request ID
	closed  bool
}

type reply struct {
	ID     int    `json:"id"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Result string `json:"result"`
	Error  string `json:"error"`
	// err is Error as an error, or errClosed when the connection ended.
	err error
}

// serve delivers the app's replies until the connection ends or the app sends
// something that is not a reply, then closes the connection and fails the
// calls still waiting.
func (c *conn) serve(ctx context.Context) {
	defer close(c.done)
	defer c.fail()
	defer c.ws.CloseNow()
	for {
		kind, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		var rep reply
		if kind != websocket.MessageText || json.Unmarshal(data, &rep) != nil {
			return
		}
		if rep.Error != "" {
			rep.err = errors.New(rep.Error)
		}
		c.mu.Lock()
		done, ok := c.pending[rep.ID]
		delete(c.pending, rep.ID)
		c.mu.Unlock()
		// A call that gave up has no waiter; its late reply is dropped.
		if ok {
			done <- rep
		}
	}
}

// fail ends the connection's calls: those waiting, and those that come later.
func (c *conn) fail() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for _, done := range c.pending {
		done <- reply{err: errClosed}
	}
	c.pending = nil
}

// send writes one message. A write ends only when the connection does, so a
// call that gives up cannot cancel it and close the connection.
func (c *conn) send(message map[string]any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if err := c.ws.Write(context.Background(), websocket.MessageText, data); err != nil {
		return fmt.Errorf("sending to the Hypercube app: %w", err)
	}
	return nil
}

// call sends the request with a new ID and waits for its reply or for ctx to
// end.
func (c *conn) call(ctx context.Context, message map[string]any) (reply, error) {
	done := make(chan reply, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return reply{}, errClosed
	}
	c.next++
	id := c.next
	if c.pending == nil {
		c.pending = map[int]chan reply{}
	}
	c.pending[id] = done
	c.mu.Unlock()
	forget := func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}
	message["id"] = id
	if err := c.send(message); err != nil {
		forget()
		return reply{}, err
	}
	select {
	case r := <-done:
		return r, r.err
	case <-ctx.Done():
		forget()
		return reply{}, ctx.Err()
	}
}
