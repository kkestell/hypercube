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
	case "list":
		reply = s.list()
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

// spawn starts a pane in a new session, or in a new window of req.Session, or in
// an existing window req.Window.
func (s *Server) spawn(req client.Request) (client.Reply, error) {
	if len(req.Argv) == 0 {
		return client.Reply{}, fmt.Errorf("spawn needs a command")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var sess *session
	var w *window
	switch {
	case req.Window != "":
		sess, w = s.findWindow(req.Window)
		if w == nil {
			return client.Reply{}, fmt.Errorf("unknown window %q", req.Window)
		}
		if req.Session != "" && req.Session != sess.id {
			return client.Reply{}, fmt.Errorf("window %s is not in session %s", w.id, req.Session)
		}
	case req.Session != "":
		if sess = s.findSession(req.Session); sess == nil {
			return client.Reply{}, fmt.Errorf("unknown session %q", req.Session)
		}
	}

	p, err := s.startPane(s.nextID('p'), req.Dir, req.Argv, req.Env)
	if err != nil {
		return client.Reply{}, err
	}
	if w != nil {
		w.panes = append(w.panes, p)
	} else {
		w = &window{id: s.nextID('w'), panes: []*pane{p}}
		if sess == nil {
			sess = &session{id: s.nextID('s')}
			s.sessions = append(s.sessions, sess)
		}
		sess.windows = append(sess.windows, w)
	}
	return client.Reply{Session: sess.id, Window: w.id, Pane: p.id}, nil
}

func (s *Server) list() client.Reply {
	s.mu.Lock()
	defer s.mu.Unlock()
	var reply client.Reply
	for _, sess := range s.sessions {
		cs := client.Session{ID: sess.id}
		for _, w := range sess.windows {
			cw := client.Window{ID: w.id}
			for _, p := range w.panes {
				cw.Panes = append(cw.Panes, client.Pane{ID: p.id, Alive: p.alive, Status: p.status})
			}
			cs.Windows = append(cs.Windows, cw)
		}
		reply.Sessions = append(reply.Sessions, cs)
	}
	return reply
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

// findSession must be called with the mutex held.
func (s *Server) findSession(id string) *session {
	for _, sess := range s.sessions {
		if sess.id == id {
			return sess
		}
	}
	return nil
}

// findWindow returns the window and its session. It must be called with the
// mutex held.
func (s *Server) findWindow(id string) (*session, *window) {
	for _, sess := range s.sessions {
		for _, w := range sess.windows {
			if w.id == id {
				return sess, w
			}
		}
	}
	return nil, nil
}
