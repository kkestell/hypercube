package main

import (
	"fmt"
	"net"
	"net/http"

	"github.com/kkestell/hypercube/internal/buildinfo"
	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/desktop"
	"github.com/kkestell/hypercube/internal/github"
	"github.com/kkestell/hypercube/internal/mcp"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/ui"
)

// handler serves the backend: the browser view, the agents' tools at /mcp,
// the desktop app's connection at /desktop, and the build identity at
// /version. localOnly guards every route.
func handler(db *store.DB, preferences config.File, rt *session.Runtime, d *desktop.Desktop, gh *github.Watcher) http.Handler {
	browser := ui.Handler(db, preferences, rt, gh)
	mux := http.NewServeMux()
	mux.Handle("POST /mcp", mcp.Handler(db, rt))
	// The browser fallback has a GET / route. Preserve the single mux's Allow
	// header for unsupported MCP methods, and its browser fallback for GET/HEAD.
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			browser.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}))
	mux.Handle("GET /desktop", d)
	mux.HandleFunc("GET /version", version)
	mux.Handle("/", browser)
	return localOnly(mux)
}

// version answers with the build identity, which the desktop app and
// installers compare with the installed executable's.
func version(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintln(w, buildinfo.String())
}

// localOnly refuses other hosts' names, so a rebound DNS name cannot reach the
// server, and other origins' requests, so another page cannot reach the app's
// routes. Actions must name their origin. The /mcp route is for agents, not
// pages; it requires a known agent ID in a header, which another page cannot
// send.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || host != "localhost" && host != "127.0.0.1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		if r.URL.Path != "/mcp" && origin != "http://"+r.Host && (r.Method != http.MethodGet || origin != "") {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		// Framed pages send actions with their own origin, so the origin
		// check alone cannot prevent another site from hiding our controls.
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}
