// Package session runs Hypercube's workspaces, sessions, and agents. A Runtime
// owns each agent's plugin process, delivers queued messages and the user's
// input to it, records what it reports, and is the only writer of the
// database.
package session

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/terminal"
)

const (
	// startTimeout bounds a plugin's reply to start, which can include resuming
	// a long conversation.
	startTimeout   = time.Minute
	requestTimeout = 30 * time.Second
	// shutdownWait is how long a plugin has to end after shutdown before its
	// process group is killed.
	shutdownWait = 3 * time.Second
	// An agent that ends maxQuickExits times in a row, each within quickExit of
	// starting, stays ended.
	quickExit     = 10 * time.Second
	maxQuickExits = 3
	idleTimeout   = 10 * time.Minute
)

var (
	errStopped  = errors.New("the agent was stopped")
	errClosing  = errors.New("hypercube is shutting down")
	errDeleting = errors.New("the session is being deleted")
)

// Runtime is the app's service for running agents.
//
// mu guards the launches in live, their fields, and the maps and fields after
// it. It is never held while waiting on a plugin: a native request can call
// back into the MCP tools, which need it. A function that takes mu releases it
// before it returns; helpers that need it held say so.
type Runtime struct {
	// PluginDir holds the harness plugins. Empty means beside the running
	// executable.
	PluginDir string
	// Terminals holds each running session's shell. Stop, Delete, and Shutdown
	// end them.
	Terminals terminal.Set

	db  *store.DB
	url string

	mu      sync.Mutex
	live    map[string]*live // by agent ID
	viewers map[string]int   // open pages selecting each session
	// seqs holds each agent's last transcript position. It outlives launches,
	// because a launch that has ended can still record a late item.
	seqs map[string]int64
	gen  int64
	// closing is set by Shutdown, after which no agent launches.
	closing bool
	// deleting holds the sessions Delete is ending, which cannot start.
	deleting map[string]bool
	feed     feed
}

// New returns a runtime for the database. url is where agents reach the app.
func New(db *store.DB, url string) *Runtime {
	return &Runtime{db: db, url: url, live: map[string]*live{}, viewers: map[string]int{},
		deleting: map[string]bool{}, seqs: map[string]int64{}, gen: time.Now().UnixMilli()}
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "hypercube: "+format+"\n", args...)
}
