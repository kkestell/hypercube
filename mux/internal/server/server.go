// Package server holds the mux's sessions, windows, and panes and serves them
// over a Unix socket.
package server

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/kkestell/hypercube/mux/client"
)

type session struct {
	id      string
	windows []*window
}

type window struct {
	id    string
	panes []*pane
}

// Server guards the session tree and every terminal with one mutex.
type Server struct {
	mu       sync.Mutex
	counters map[byte]int // one per kind of ID: 's', 'w', 'p'
	sessions []*session
	attached map[*attachment]struct{}
}

func New() *Server {
	return &Server{counters: map[byte]int{}, attached: map[*attachment]struct{}{}}
}

// ListenAndServe serves the socket at path until the listener fails. It fails
// if another server is already listening there.
func (s *Server) ListenAndServe(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if c, err := client.Dial(path); err == nil {
		c.Close()
		return fmt.Errorf("a server is already listening on %s", path)
	}
	os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go s.serve(conn)
	}
}

func (s *Server) serve(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	for {
		var req client.Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		if req.Type == "attach" {
			s.attach(dec, enc, req)
			return
		}
		if err := enc.Encode(s.handle(req)); err != nil {
			return
		}
	}
}

func (s *Server) handle(req client.Request) client.Reply {
	var reply client.Reply
	var err error
	switch req.Type {
	case "spawn":
		reply, err = s.spawn(req)
	case "capture":
		reply, err = s.capture(req)
	default:
		err = fmt.Errorf("unknown request type %q", req.Type)
	}
	if err != nil {
		return client.Reply{Error: err.Error()}
	}
	reply.OK = true
	return reply
}

// nextID must be called with the mutex held.
func (s *Server) nextID(kind byte) string {
	s.counters[kind]++
	return fmt.Sprintf("%c%d", kind, s.counters[kind])
}

func (s *Server) spawn(req client.Request) (client.Reply, error) {
	if len(req.Argv) == 0 {
		return client.Reply{}, fmt.Errorf("spawn needs a command")
	}
	// Spawning into an existing session or window is task 2.
	if req.Session != "" {
		return client.Reply{}, fmt.Errorf("unknown session %q", req.Session)
	}
	if req.Window != "" {
		return client.Reply{}, fmt.Errorf("unknown window %q", req.Window)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.startPane(s.nextID('p'), req.Dir, req.Argv, req.Env)
	if err != nil {
		return client.Reply{}, err
	}
	w := &window{id: s.nextID('w'), panes: []*pane{p}}
	sess := &session{id: s.nextID('s'), windows: []*window{w}}
	s.sessions = append(s.sessions, sess)
	return client.Reply{Session: sess.id, Window: w.id, Pane: p.id}, nil
}

func (s *Server) capture(req client.Request) (client.Reply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.findPane(req.Pane)
	if p == nil {
		return client.Reply{}, fmt.Errorf("unknown pane %q", req.Pane)
	}
	text, err := p.capture()
	return client.Reply{Text: text}, err
}

// findPane must be called with the mutex held.
func (s *Server) findPane(id string) *pane {
	for _, sess := range s.sessions {
		for _, w := range sess.windows {
			for _, p := range w.panes {
				if p.id == id {
					return p
				}
			}
		}
	}
	return nil
}
