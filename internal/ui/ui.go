// Package ui serves the browser view: the page, its data WebSocket, its
// actions, files, previews, and panel terminals. The data WebSocket carries
// Hypercube's data from the database, and the transcripts of the session the
// page selects. Browsers cap a host's HTTP connections at six; WebSockets are
// outside that cap, so open pages never stall each other's requests.
package ui

import (
	"net/http"

	"github.com/kkestell/hypercube/internal/github"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

type server struct {
	db *store.DB
	rt *session.Runtime
	gh *github.Watcher
}

// Handler serves the browser view's routes. gh supplies the GitHub status of
// workspaces.
func Handler(db *store.DB, rt *session.Runtime, gh *github.Watcher) http.Handler {
	s := &server{db: db, rt: rt, gh: gh}
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(assets()))
	mux.HandleFunc("GET /state", s.state)
	mux.HandleFunc("GET /config", getConfig)
	mux.HandleFunc("POST /config", saveConfig)
	mux.HandleFunc("GET /harnesses", s.harnesses)
	mux.HandleFunc("GET /search-sessions", s.searchSessions)
	mux.HandleFunc("GET /grants", s.grants)
	mux.HandleFunc("POST /grants", s.deleteGrant)
	mux.HandleFunc("POST /workspace", s.workspaceAction)
	mux.HandleFunc("POST /session", s.sessionAction)
	mux.HandleFunc("GET /terminal", s.terminal)
	mux.HandleFunc("POST /files/tree", s.listFileTree)
	mux.HandleFunc("GET /mention-files", s.mentionFiles)
	mux.HandleFunc("GET /search-files", s.searchFiles)
	mux.HandleFunc("GET /file", s.readFile)
	mux.HandleFunc("GET /preview/{session}/{path...}", s.readPreview)
	mux.HandleFunc("GET /workspace-preview/{workspace}/{path...}", s.readPreview)
	mux.HandleFunc("GET /changes", s.listChanges)
	mux.HandleFunc("GET /diff", s.readDiff)
	mux.HandleFunc("GET /image", s.readImage)
	mux.HandleFunc("GET /transcript-image", s.readTranscriptImage)
	return mux
}

// refused answers with the reason an action failed, and reports whether it did.
func refused(w http.ResponseWriter, err error) bool {
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
	}
	return err != nil
}
