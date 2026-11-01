// Package ui serves the browser view. Each page has two WebSockets: one
// carries Hypercube's data from the database, and one connects a tmux
// control-mode client for the selected session's terminals, carrying its
// output lines to the page and the page's commands to it. Browsers cap a
// host's HTTP connections at six; WebSockets are outside that cap, so open
// pages never stall each other's requests.
package ui

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/coder/websocket"
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
	db *store.DB
	mu sync.Mutex
	// snapshot is the latest view data, and changed is closed and replaced
	// when it changes.
	snapshot []byte
	changed  chan struct{}
}

// Handler serves the page, its data and terminal sockets, and its actions.
func Handler(db *store.DB) http.Handler {
	s := &server{db: db, changed: make(chan struct{})}
	go func() {
		if err := db.Watch(context.Background(), s.refresh); err != nil {
			fmt.Fprintln(os.Stderr, "hypercube: watching the database:", err)
		}
	}()
	files, _ := fs.Sub(static, "static")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(files))
	if revision := os.Getenv("HYPERCUBE_DEV_RELOAD"); revision != "" {
		devReload(mux, revision)
	}
	mux.HandleFunc("GET /state", s.state)
	mux.HandleFunc("GET /tmux", terminals)
	mux.HandleFunc("GET /leads", leads)
	mux.HandleFunc("POST /workspace", s.workspaceAction)
	mux.HandleFunc("POST /session", s.sessionAction)
	return localOnly(mux)
}

// devReload is enabled only by scripts/dev. A rebuilt server publishes its
// source revision; open pages wait through downtime and reload when it changes.
func devReload(mux *http.ServeMux, revision string) {
	mux.HandleFunc("GET /dev-reload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, revision)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		page, _ := static.ReadFile("static/index.html")
		encoded, _ := json.Marshal(revision)
		reload := fmt.Sprintf(`<script>
setInterval(async () => {
  try {
    const response = await fetch('/dev-reload', {cache: 'no-store'});
    if (response.ok && await response.text() !== %s) location.reload();
  } catch {}
}, 1000);
</script>`, encoded)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(bytes.Replace(page, []byte("</body>"), []byte(reload+"</body>"), 1))
	})
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

// state sends the view data: the current snapshot on connect, then each new
// one.
func (s *server) state(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := conn.CloseRead(r.Context())
	var sent []byte
	for {
		s.mu.Lock()
		data, changed := s.snapshot, s.changed
		s.mu.Unlock()
		if data != nil && !bytes.Equal(data, sent) {
			if conn.Write(ctx, websocket.MessageText, data) != nil {
				return
			}
			sent = data
		}
		select {
		case <-changed:
		case <-ctx.Done():
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
// server, and other origins' actions. websocket.Accept refuses other origins'
// sockets, so another page cannot type into a pane.
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

// terminals connects a control-mode client attached to the session's tmux
// session. Each binary message to the page is one of the client's output
// lines; each message from the page is one or more commands, one per line.
func terminals(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")
	if session == "" || !tmux.Running(session) {
		http.Error(w, "the session is not running", http.StatusConflict)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	// A paste arrives as one command.
	conn.SetReadLimit(16 << 20)
	// Closing either end stops the other: the socket's end cancels the
	// context, which kills tmux.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", "-C", "attach-session", "-t", "="+tmux.Name(session))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	go func() {
		defer cancel()
		for {
			_, commands, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if _, err := stdin.Write(append(bytes.TrimSuffix(commands, []byte("\n")), '\n')); err != nil {
				return
			}
		}
	}()
	lines := bufio.NewReader(stdout)
	for {
		line, err := lines.ReadBytes('\n')
		if err != nil {
			return
		}
		if conn.Write(ctx, websocket.MessageBinary, bytes.TrimSuffix(line, []byte("\n"))) != nil {
			return
		}
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
