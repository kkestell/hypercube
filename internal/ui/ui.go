// Package ui serves the browser view. Each page has two connections: a stream
// of Hypercube's data from the database, and a tmux control-mode client for
// the selected session's terminals, whose output the server streams to the
// page and whose commands the page sends through the server.
package ui

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/tmux"
)

//go:embed static
var static embed.FS

// Icon returns the Hypercube SVG shared by the browser and menu bar.
func Icon() []byte {
	icon, _ := static.ReadFile("static/favicon.svg")
	return icon
}

type server struct {
	db      *store.DB
	mu      sync.Mutex
	clients map[string]io.Writer
	// snapshot is the latest view data, and changed is closed and replaced
	// when it changes.
	snapshot []byte
	changed  chan struct{}
}

// Handler serves the page, its data and terminal streams, and its actions.
func Handler(db *store.DB) http.Handler {
	s := &server{db: db, clients: map[string]io.Writer{}, changed: make(chan struct{})}
	go func() {
		if err := db.Watch(context.Background(), s.refresh); err != nil {
			fmt.Fprintln(os.Stderr, "hypercube: watching the database:", err)
		}
	}()
	files, _ := fs.Sub(static, "static")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(files))
	mux.HandleFunc("GET /state", s.state)
	mux.HandleFunc("GET /events", s.events)
	mux.HandleFunc("POST /send", s.send)
	mux.HandleFunc("GET /leads", leads)
	mux.HandleFunc("POST /workspace", s.workspaceAction)
	mux.HandleFunc("POST /session", s.sessionAction)
	return localOnly(mux)
}

// refresh reads the view data after a database change and wakes the pages
// when it differs from what they have.
func (s *server) refresh() {
	list, err := store.Snapshot(s.db)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube: reading the view data:", err)
		return
	}
	data, _ := json.Marshal(list)
	s.mu.Lock()
	defer s.mu.Unlock()
	if bytes.Equal(data, s.snapshot) {
		return
	}
	s.snapshot = data
	close(s.changed)
	s.changed = make(chan struct{})
}

// state streams the view data: the current snapshot on connect, then each
// new one.
func (s *server) state(w http.ResponseWriter, r *http.Request) {
	flusher := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	var sent []byte
	for {
		s.mu.Lock()
		data, changed := s.snapshot, s.changed
		s.mu.Unlock()
		if data != nil && !bytes.Equal(data, sent) {
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher.Flush() != nil {
				return
			}
			sent = data
		}
		select {
		case <-changed:
		case <-r.Context().Done():
			return
		}
	}
}

// leads rereads the profiles so config edits appear in the next session menu.
func leads(w http.ResponseWriter, r *http.Request) {
	cfg, err := harness.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type lead struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	var list []lead
	for _, p := range cfg.Leads() {
		list = append(list, lead{p.Name, p.Description})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// localOnly refuses other hosts' names, so a rebound DNS name cannot reach the
// server, and other origins' commands, so another page cannot type into a pane.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || host != "localhost" && host != "127.0.0.1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("Origin") != "http://"+r.Host {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// events attaches a control-mode client to the session's tmux session and
// streams its output lines. Lines are base64 encoded because pane output can
// split a UTF-8 sequence. The first event names the client for send.
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")
	if session == "" || !tmux.Running(session) {
		http.Error(w, "the session is not running", http.StatusConflict)
		return
	}
	cmd := exec.CommandContext(r.Context(), "tmux", "-C", "attach-session", "-t", "="+tmux.Name(session))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := cmd.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id := rand.Text()
	s.mu.Lock()
	s.clients[id] = stdin
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, id)
		s.mu.Unlock()
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	flusher := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "event: id\ndata: %s\n\n", id)
	if flusher.Flush() != nil {
		return
	}
	lines := bufio.NewReader(stdout)
	for {
		line, err := lines.ReadBytes('\n')
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", base64.StdEncoding.EncodeToString(bytes.TrimSuffix(line, []byte("\n"))))
		if lines.Buffered() == 0 && flusher.Flush() != nil {
			return
		}
	}
}

// send writes the body's tmux commands, one per line, to the named client.
func (s *server) send(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	client := s.clients[r.URL.Query().Get("id")]
	if client == nil {
		http.Error(w, "no such client", http.StatusNotFound)
		return
	}
	if _, err := client.Write(append(bytes.TrimSuffix(body, []byte("\n")), '\n')); err != nil {
		http.Error(w, err.Error(), http.StatusGone)
	}
}

// workspaceAction adds a workspace in a folder the user picks and returns its
// ID, or deletes a workspace.
func (s *server) workspaceAction(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch q.Get("action") {
	case "new":
		// activate brings the picker in front of the browser.
		out, err := exec.Command("osascript", "-e", "activate", "-e", `POSIX path of (choose folder with prompt "Choose the workspace's folder:")`).Output()
		var exit *exec.ExitError
		if errors.As(err, &exit) && strings.Contains(string(exit.Stderr), "(-128)") {
			return // The user cancelled.
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		added, err := session.AddWorkspace(s.db, strings.TrimSpace(string(out)))
		if refused(w, err) {
			return
		}
		fmt.Fprint(w, added.ID)
	case "delete":
		refused(w, session.DeleteWorkspace(s.db, q.Get("id")))
	default:
		http.Error(w, "unknown workspace action", http.StatusBadRequest)
	}
}

// sessionAction creates a session in a workspace and returns its ID, or
// starts, stops, deletes, renames, or marks seen a session.
func (s *server) sessionAction(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	var err error
	switch q.Get("action") {
	case "new":
		var created store.Session
		created, err = session.Create(s.db, q.Get("workspace"), q.Get("lead"))
		if !refused(w, err) {
			fmt.Fprint(w, created.ID)
		}
		return
	case "start":
		err = session.Start(s.db, id)
	case "stop":
		err = session.Stop(s.db, id)
	case "delete":
		err = session.Delete(s.db, id)
	case "rename":
		err = session.Rename(s.db, id, q.Get("title"))
	case "seen":
		err = session.Seen(s.db, id)
	default:
		http.Error(w, "unknown session action", http.StatusBadRequest)
		return
	}
	refused(w, err)
}

// refused answers with the reason an action failed, and reports whether it did.
func refused(w http.ResponseWriter, err error) bool {
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
	}
	return err != nil
}
