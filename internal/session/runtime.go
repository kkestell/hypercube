// Package session runs Hypercube's workspaces, sessions, and agents. A Runtime
// attaches to each agent's holder, records commands and reported output, and
// is the only writer of the database.
package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/terminal"
	"github.com/kkestell/hypercube/internal/workspace"
)

const (
	// startTimeout bounds a plugin's reply to start, which can include resuming
	// a long conversation.
	startTimeout   = time.Minute
	requestTimeout = 30 * time.Second
	// idleTimeout is how long a session stays eligible to park before its
	// agent ends.
	idleTimeout = 15 * time.Minute
	// An agent that ends maxQuickExits times in a row, each within quickExit of
	// starting, stays exited.
	quickExit     = 10 * time.Second
	maxQuickExits = 3
	// utilityTimeout is how long a utility model call may take.
	utilityTimeout = time.Minute
)

var (
	errStopped           = errors.New("the agent was stopped")
	errClosing           = errors.New("hypercube is shutting down")
	errDeleting          = errors.New("another change to the session is in progress")
	errEnded             = errors.New("the session has ended")
	errWorkspaceDeleting = errors.New("the workspace is being deleted")
	errWorkspaceCreating = errors.New("a session is being created in the workspace; retry when creation finishes")
)

// Programs are the executables a runtime starts. Plugins is the directory of
// harness plugins. MCP is the MCP bridge that each agent's harness runs.
// Holder is hypercube-hold, which runs agents' plugins and panel shells.
type Programs struct {
	Plugins string
	MCP     string
	Holder  string
}

// Runtime is the app's service for running agents.
//
// mu guards the launches in live, their fields other than send and sent, and
// the maps and fields after it up to refreshMu. It is never held while waiting on a plugin: a
// native request can call back into the MCP tools, which need it. A function that takes mu releases it
// before it returns; helpers that need it held say so.
type Runtime struct {
	// Terminals holds each session's shell. Archive, Delete, EndChild, and
	// Quit end them.
	Terminals terminal.Set
	// Env returns extra environment for a workspace's new agent launches and
	// panel shells. It is called with the runtime's lock held, so it never
	// blocks. Set it before Restore; nil adds nothing.
	Env func(workspace string) []string

	db       *store.DB
	config   config.File
	url      string
	programs Programs
	// startTimeout and requestTimeout bound replies, and idleTimeout parks
	// sessions; tests shorten them.
	startTimeout, requestTimeout, idleTimeout time.Duration
	// utilityTimeout bounds one utility model call.
	utilityTimeout time.Duration

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
	// childCreates tracks admitted creations through their return or rollback,
	// independently of launch completion. Family removal waits for these.
	childCreates map[string]chan struct{}
	// workspaceCreates counts admitted top-level creations through registration
	// or failure cleanup. workspaceDeleting excludes them during deletion.
	workspaceCreates  map[string]int
	workspaceDeleting map[string]bool
	// submissions holds, by session ID, the browser's ID for the first
	// prompt of each session whose creation has not yet returned.
	submissions map[string]string
	// merges holds each worktree session's merge status, by session ID.
	merges map[string]workspace.Merge
	// viewers counts the pages viewing each session, by session ID.
	viewers map[string]int
	// idle holds each parkable session's idle timer, by session ID.
	idle map[string]*idleTimer
	// harnesses is the last scan of the installed harnesses.
	harnesses []harness.Harness
	// utilityCancels holds the cancel of each session's pending utility call,
	// by session ID; utilityCalls counts those calls.
	utilityCancels map[string]context.CancelFunc
	utilityCalls   sync.WaitGroup
	// refreshMu serializes refreshMerges.
	refreshMu sync.Mutex
	feed      feed
}

// New returns a runtime for the database. url is where agents reach the
// backend.
func New(db *store.DB, preferences config.File, url string, programs Programs) *Runtime {
	return &Runtime{Terminals: terminal.Set{Dir: filepath.Join(db.Dir, "run"), Holder: programs.Holder}, db: db, config: preferences, url: url, programs: programs, live: map[string]*live{},
		deleting: map[string]bool{}, merges: map[string]workspace.Merge{}, opens: map[string][]FileOpen{}, seqs: map[string]int64{},
		workspaceCreates: map[string]int{}, workspaceDeleting: map[string]bool{}, submissions: map[string]string{}, childCreates: map[string]chan struct{}{},
		viewers: map[string]int{}, idle: map[string]*idleTimer{}, startTimeout: startTimeout, requestTimeout: requestTimeout, idleTimeout: idleTimeout,
		utilityTimeout: utilityTimeout, utilityCancels: map[string]context.CancelFunc{}}
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "hypercube: "+format+"\n", args...)
}
