package session

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harnessclient"
	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// live is an attachment to the durable launch named by prefix. The reader
// owns its end; stopping never removes it before its last output is applied.
type live struct {
	// Runtime.mu guards these fields.
	id, root  string
	workspace string
	prefix    string // the launch ID and ":", which namespaces item IDs
	client    *harnessclient.Client
	ready     int64 // Unix milliseconds of the start reply; zero while starting
	position  int64 // the output offset applied so far
	quick     int   // consecutive launches that exited soon after starting
	cause     error // why the launch ended, once known
	stopping  bool
	attention bool // the last saved attention flag
	pending   map[string]*pendingItem
	commands  map[int64]store.Command
	waiters   map[int64]chan error

	// send serializes writes to client and guards sent, the last command
	// written.
	send sync.Mutex
	sent int64
	// started is closed once the plugin has accepted start.
	started chan struct{}
	// done is closed when the launch is over, by run or, when run never
	// starts, by CreateChild.
	done chan struct{}
}
type pendingItem struct {
	seq  int64
	item harness.Item
}

func (r *Runtime) current(l *live) bool { return r.live[l.id] == l }

// decide records a launch and its start line together. The caller holds mu.
func (r *Runtime) decide(tx *write, s store.Session, root string, quick int) (*live, error) {
	args, err := r.startArgs(tx.Tx, s)
	if err != nil {
		return nil, err
	}
	s.Launch, s.Position, s.Ready, s.Quick = store.NewID(), 0, 0, quick
	s.Activity, s.Attention = store.Exited, false
	s.InputPending = false
	if err = tx.SaveSession(s); err != nil {
		return nil, err
	}
	l := &live{id: s.ID, root: root, workspace: s.Workspace, prefix: s.Launch + ":", pending: map[string]*pendingItem{}, commands: map[int64]store.Command{}, waiters: map[int64]chan error{}, started: make(chan struct{}), done: make(chan struct{}), quick: quick}
	if err = r.loadSeq(tx.Tx, s.ID); err != nil {
		return nil, err
	}
	if _, err = r.record(tx, l, harness.Command{Command: harness.CmdStart, Start: &args}, 0, 0, nil); err != nil {
		return nil, err
	}
	return l, nil
}

// loadSeq reads the session's last transcript position, unless the runtime has
// it already: the runtime's position can be ahead of the database while an
// item is being written. The caller holds mu.
func (r *Runtime) loadSeq(tx *store.Tx, id string) error {
	if _, ok := r.seqs[id]; ok {
		return nil
	}
	last, err := tx.LastSeq(id)
	if err != nil {
		return err
	}
	r.seqs[id] = last
	return nil
}

// nextSeq returns the session's next transcript position. The caller holds mu.
func (r *Runtime) nextSeq(id string) int64 {
	r.seqs[id]++
	return r.seqs[id]
}

// launch attaches the reader and waits for the plugin to start.
func (r *Runtime) launch(l *live) error {
	go r.run(l)
	return r.awaitStart(l)
}

// awaitStart returns once the launch has started, or with why it ended
// before starting. run enforces the start timeout.
func (r *Runtime) awaitStart(l *live) error {
	select {
	case <-l.started:
		return nil
	case <-l.done:
	}
	select {
	case <-l.started:
		return nil
	default:
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return cmp.Or(l.cause, errStopped)
}

func (r *Runtime) run(l *live) {
	defer close(l.done)
	r.mu.Lock()
	if r.detached {
		r.mu.Unlock()
		return
	}
	var client *harnessclient.Client
	var err error
	dir := r.launchDir(l)
	if _, statErr := os.Stat(dir); errors.Is(statErr, os.ErrNotExist) {
		if l.stopping {
			err = errStopped
		} else {
			var path string
			path, err = r.pluginPath(l.id)
			if err == nil {
				client, err = harnessclient.Launch(dir, path)
			}
		}
	} else {
		client, err = harnessclient.Attach(dir)
	}
	stop := err == nil && l.stopping
	if err == nil {
		l.client = client
	}
	r.mu.Unlock()
	if err != nil {
		r.end(l, err)
		return
	}
	if stop {
		_ = client.Stop()
	}
	go r.send(l)
	r.mu.Lock()
	for id, c := range l.commands {
		if c.Message != 0 {
			go r.watchDelivery(l, id)
		}
	}
	// A message can be queued and saved just before the app stops, so an
	// attached launch delivers what is already queued.
	r.kickLocked(l.id)
	r.mu.Unlock()
	timer := time.AfterFunc(r.startTimeout, func() {
		r.mu.Lock()
		late := r.current(l) && l.ready == 0 && !r.detached
		if late {
			l.cause = errors.New("the plugin did not start in time")
		}
		r.mu.Unlock()
		if late {
			_ = client.Stop()
		}
	})
	defer timer.Stop()
	reason := client.Read(l.position, func(rec harness.Record, next int64) error {
		toParent, err := r.apply(l, rec, next)
		if err != nil {
			return err
		}
		// A finished turn can have committed or merged work.
		if rec.Event != nil && rec.Event.State == harness.Idle {
			r.refreshMerges(l.workspace)
		}
		// A child's report or failure wakes a parked parent.
		if toParent {
			r.Deliver(l.root)
		}
		return nil
	})
	if errors.Is(reason, hold.ErrClosed) {
		return
	}
	r.end(l, reason)
}

func (r *Runtime) launchDir(l *live) string {
	return filepath.Join(store.Dir(), "run", strings.TrimSuffix(l.prefix, ":"))
}
func (r *Runtime) pluginPath(id string) (string, error) {
	var s store.Session
	err := r.db.Read(func(tx *store.Tx) error { var err error; s, err = tx.Session(id); return err })
	if err != nil {
		return "", err
	}
	return harnessclient.Find(r.programs.Plugins, s.Harness)
}

// startArgs builds the arguments recorded with the launch's start command.
// The caller holds mu.
func (r *Runtime) startArgs(tx *store.Tx, s store.Session) (args harness.StartArgs, err error) {
	var guidance string
	if s.Parent == "" {
		cfg, err := config.Load()
		if err != nil {
			return args, err
		}
		guidance = cfg.ChildGuidance
	}
	instructions, err := report.Instructions(tx, s, r.installedLocked(), guidance)
	if err != nil {
		return args, err
	}
	args = harness.StartArgs{MCP: r.programs.MCP, Env: map[string]string{"HYPERCUBE_SESSION_ID": s.ID, "HYPERCUBE_URL": r.url}, Session: s.ID, Model: s.Model, Effort: s.Effort, Conversation: s.Conversation, Fork: s.Fork, Dir: s.Dir, Instructions: instructions, StateDir: store.Dir()}
	if s.Fork != "" {
		parent, err := tx.Session(s.Parent)
		if err != nil {
			return args, err
		}
		args.ForkDir = parent.Dir
	}
	return args, nil
}

// end applies the final output before clearing this launch and deciding its
// successor in the same transaction. Removing the holder waits for it to exit,
// so it happens outside mu.
func (r *Runtime) end(l *live, cause error) {
	r.mu.Lock()
	next, ended := r.endLocked(l, cause)
	r.mu.Unlock()
	if !ended {
		return
	}
	if l.client != nil {
		_ = l.client.Close()
		if err := l.client.Remove(); err != nil {
			logf("removing holder: %v", err)
		}
	} else {
		_ = os.RemoveAll(r.launchDir(l))
	}
	if next != nil {
		go r.launch(next)
	}
}

// endLocked records the end and registers the next launch, if any. It reports
// whether the end was recorded. The caller holds mu.
func (r *Runtime) endLocked(l *live, cause error) (*live, bool) {
	if !r.current(l) || r.detached {
		return nil, false
	}
	var next *live
	err := r.save(func(tx *write) error {
		for _, p := range l.drain() {
			if err := tx.putItem(l.id, p.seq, p.item); err != nil {
				return err
			}
		}
		s, err := tx.Session(l.id)
		if err != nil {
			return err
		}
		suppress := r.closing || l.stopping
		lost := errors.Is(cause, hold.ErrLost)
		failed := l.ready == 0
		reason := cmp.Or(l.cause, cause, errStopped)
		if !suppress && !lost {
			note := "The agent exited: "
			if failed {
				note = "Could not start the agent: "
			}
			if err = r.note(tx, l, harness.KindError, note+reason.Error()); err != nil {
				return err
			}
		}
		quick := 0
		if l.ready > 0 && time.Since(time.UnixMilli(l.ready)) < quickExit {
			quick = l.quick + 1
		}
		s.Launch, s.Position, s.Ready, s.Quick = "", 0, 0, quick
		s.Activity, s.Attention = store.Exited, false
		if err = tx.SaveSession(s); err != nil {
			return err
		}
		if err = tx.DeleteCommands(l.id); err != nil {
			return err
		}
		if !suppress && !lost && !failed && quick < maxQuickExits {
			next, err = r.decide(tx, s, l.root, quick)
		}
		return err
	})
	if err != nil {
		logf("recording the end of %s: %v", l.id, err)
		return nil, false
	}
	delete(r.live, l.id)
	l.cause = cmp.Or(l.cause, cause, errStopped)
	for _, ch := range l.waiters {
		select {
		case ch <- l.cause:
		default:
		}
	}
	if next != nil {
		r.live[next.id] = next
	}
	r.kickFamilyLocked(l.root)
	return next, true
}

// drain takes the launch's unfinished items as finished items.
func (l *live) drain() []*pendingItem {
	items := make([]*pendingItem, 0, len(l.pending))
	for id, p := range l.pending {
		p.item.Done = true
		items = append(items, p)
		delete(l.pending, id)
	}
	slices.SortFunc(items, func(a, b *pendingItem) int { return cmp.Compare(a.seq, b.seq) })
	return items
}

// note adds an item written by Hypercube to l's session's transcript. The caller
// holds mu.
func (r *Runtime) note(tx *write, l *live, kind, text string) error {
	return tx.putItem(l.id, r.nextSeq(l.id), harness.Item{ID: l.prefix + "host-" + store.NewID(), Kind: kind, Text: text, Done: true})
}
