// Package ui serves the browser view and the app's routes. The page's data
// WebSocket carries Hypercube's data from the database. Browsers cap a host's
// HTTP connections at six; WebSockets are outside that cap, so open pages never
// stall each other's requests. Agents reach the tools through /mcp.
package ui

import (
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
	"github.com/kkestell/hypercube/internal/mcp"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
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
	rt *session.Runtime
	mu sync.Mutex
	// snapshot is the latest view data, and changed is closed and replaced
	// when it changes.
	snapshot []byte
	changed  chan struct{}
}

// Handler serves the page, its data socket, its actions, and the agents' tools.
// It watches the database until ctx ends.
func Handler(ctx context.Context, db *store.DB, rt *session.Runtime) http.Handler {
	s := &server{db: db, rt: rt, changed: make(chan struct{})}
	go func() {
		if err := db.Watch(ctx, s.refresh); err != nil {
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
	mux.HandleFunc("GET /leads", leads)
	mux.Handle("POST /mcp", mcp.Handler(db, rt))
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
// sockets, so another page cannot reach the app's routes.
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
		refused(w, s.rt.DeleteWorkspace(q.Get("id")))
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
		created, err = s.rt.Create(q.Get("workspace"), q.Get("lead"))
		if !refused(w, err) {
			fmt.Fprint(w, created.ID)
		}
		return
	case "start":
		err = s.rt.Start(id)
	case "stop":
		err = s.rt.Stop(id)
	case "delete":
		err = s.rt.Delete(id)
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
