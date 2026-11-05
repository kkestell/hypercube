// Package session runs Hypercube's workspaces, sessions, and agents. A Runtime
// attaches to each agent's holder, records commands and reported output, and
// is the only writer of the database.
package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/terminal"
	"github.com/kkestell/hypercube/internal/workspace"
)

const (
	// startTimeout bounds a plugin's reply to start, which can include resuming
	// a long conversation.
	startTimeout   = time.Minute
	requestTimeout = 30 * time.Second
	// An agent that ends maxQuickExits times in a row, each within quickExit of
	// starting, stays exited.
	quickExit     = 10 * time.Second
	maxQuickExits = 3
)

var (
	errStopped  = errors.New("the agent was stopped")
	errClosing  = errors.New("hypercube is shutting down")
	errDeleting = errors.New("the session is being deleted")
)

// Runtime is the app's service for running agents.
//
// mu guards the launches in live, their fields other than send and sent, and
// the maps and fields after it up to refreshMu. It is never held while waiting on a plugin: a
// native request can call back into the MCP tools, which need it. A function that takes mu releases it
// before it returns; helpers that need it held say so.
type Runtime struct {
	// PluginDir holds the harness plugins. Empty means beside the running
	// executable.
	PluginDir string
	// Terminals holds each running session's shell. Stop, Delete, and Quit
	// end them.
	Terminals terminal.Set

	db  *store.DB
	url string
	// startTimeout and requestTimeout bound replies; tests shorten them.
	startTimeout, requestTimeout time.Duration

	mu    sync.Mutex
	live  map[string]*live      // by session ID
	opens map[string][]FileOpen // file requests waiting for a viewing page
	// seqs holds each session's last transcript position. It outlives launches,
	// because a launch that has ended can still record a late item.
	seqs map[string]int64
	// closing is set by Quit or Detach, after which no agent launches.
	closing  bool
	detached bool
	// deleting claims family or child cleanup, blocking starts, children, and
	// shell admission until the initial checks refuse or cleanup finishes.
	deleting map[string]bool
	// merges holds each worktree session's merge status, by session ID.
	merges map[string]workspace.Merge
	// refreshMu serializes refreshMerges.
	refreshMu sync.Mutex
	feed      feed
}

// New returns a runtime for the database. url is where agents reach the app.
func New(db *store.DB, url string) *Runtime {
	return &Runtime{Terminals: terminal.Set{Dir: filepath.Join(store.Dir(), "run")}, db: db, url: url, live: map[string]*live{},
		deleting: map[string]bool{}, merges: map[string]workspace.Merge{}, opens: map[string][]FileOpen{}, seqs: map[string]int64{}, startTimeout: startTimeout, requestTimeout: requestTimeout}
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "hypercube: "+format+"\n", args...)
}
