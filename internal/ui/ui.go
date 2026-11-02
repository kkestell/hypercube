// Package ui serves the browser view and the app's routes. The page's data
// WebSocket carries Hypercube's data from the database, and the transcripts of
// the session the page selects. Browsers cap a host's HTTP connections at six;
// WebSockets are outside that cap, so open pages never stall each other's
// requests. Agents reach the tools through /mcp.
package ui

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/mcp"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

type server struct {
	db *store.DB
	rt *session.Runtime
}

// Handler serves the page, its data socket, its actions, and the agents' tools.
func Handler(db *store.DB, rt *session.Runtime) http.Handler {
	s := &server{db: db, rt: rt}
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(assets()))
	mux.HandleFunc("GET /state", s.state)
	mux.HandleFunc("GET /leads", leads)
	mux.Handle("POST /mcp", mcp.Handler(db, rt))
	mux.HandleFunc("POST /workspace", s.workspaceAction)
	mux.HandleFunc("POST /session", s.sessionAction)
	mux.HandleFunc("POST /agent", s.agentAction)
	mux.HandleFunc("GET /terminal", s.terminal)
	mux.HandleFunc("GET /files", s.listFiles)
	mux.HandleFunc("GET /file", s.readFile)
	return localOnly(mux)
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
		Harness     string `json:"harness"`
		Model       string `json:"model"`
		Effort      string `json:"effort"`
	}
	var list []lead
	for _, p := range cfg.Leads() {
		list = append(list, lead{p.Name, p.Description, p.Harness, p.Model, p.Effort})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// localOnly refuses other hosts' names, so a rebound DNS name cannot reach the
// server, and other origins' requests, so another page cannot reach the app's
// routes. Actions must name their origin. A socket's origin is checked here
// rather than by websocket.Accept, because the terminal route starts a shell
// before accepting.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || host != "localhost" && host != "127.0.0.1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "http://"+r.Host && (r.Method != http.MethodGet || origin != "") {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// refused answers with the reason an action failed, and reports whether it did.
func refused(w http.ResponseWriter, err error) bool {
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
	}
	return err != nil
}
