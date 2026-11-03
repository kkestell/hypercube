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

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// live is an attachment to the durable launch named by prefix. The reader
// owns its end; stopping never removes it before its last output is applied.
// send serializes senders and guards sent; run closes done when it returns.
type live struct {
	id, root, prefix string
	client           *harness.Client
	ready            int64
	position         int64
	quick            int
	cause            error
	stopping         bool
	attention        bool
	pending          map[string]*pendingItem
	commands         map[int64]store.Command
	waiters          map[int64]chan error
	send             sync.Mutex
	sent             int64
	done             chan struct{}
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
	if err = tx.SaveSession(s); err != nil {
		return nil, err
	}
	l := &live{id: s.ID, root: root, prefix: s.Launch + ":", pending: map[string]*pendingItem{}, commands: map[int64]store.Command{}, waiters: map[int64]chan error{}, done: make(chan struct{}), quick: quick}
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

// sessionLive reports whether any member of the family has a launch. The
// caller holds mu.
func (r *Runtime) sessionLive(session string) bool {
	for _, l := range r.live {
		if l.root == session {
			return true
		}
	}
	return false
}

// launch attaches the reader and waits for the durable start command's reply.
func (r *Runtime) launch(l *live) error {
	r.mu.Lock()
	var start int64
	for id, c := range l.commands {
		if c.Command == harness.CmdStart {
			start = id
		}
	}
	r.mu.Unlock()
	go r.run(l)
	if start == 0 {
		return nil
	}
	err := r.wait(l, start, r.startTimeout)
	if err != nil {
		<-l.done
	}
	return err
}

func (r *Runtime) run(l *live) {
	defer close(l.done)
	r.mu.Lock()
	if r.detached {
		r.mu.Unlock()
		return
	}
	var client *harness.Client
	var err error
	dir := r.launchDir(l)
	if _, statErr := os.Stat(dir); errors.Is(statErr, os.ErrNotExist) {
		if l.stopping {
			err = errStopped
		} else {
			var path string
			path, err = r.pluginPath(l.id)
			if err == nil {
				client, err = harness.Launch(dir, path)
			}
		}
	} else {
		client, err = harness.Attach(dir)
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
	reason := client.Read(l.position, func(rec harness.Record, next int64) error { return r.apply(l, rec, next) })
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
	return harness.Find(r.PluginDir, s.Harness)
}

// startArgs builds the arguments recorded with the launch's start command.
func (r *Runtime) startArgs(tx *store.Tx, s store.Session) (args harness.StartArgs, err error) {
	var profiles []harness.Profile
	if s.Parent == "" {
		cfg, err := harness.LoadConfig()
		if err != nil {
			return args, err
		}
		profiles = cfg.Profiles
	}
	instructions, err := report.Instructions(tx, s, profiles)
	if err != nil {
		return args, err
	}
	exe, err := os.Executable()
	if err != nil {
		return args, err
	}
	args = harness.StartArgs{Hypercube: exe, Env: map[string]string{"HYPERCUBE_SESSION_ID": s.ID, "HYPERCUBE_URL": r.url}, Session: s.ID, Model: s.Model, Effort: s.Effort, Conversation: s.Conversation, Dir: s.Dir, Instructions: instructions, StateDir: store.Dir()}
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
		running, err := tx.Running(s)
		if err != nil {
			return err
		}
		suppress := r.closing || l.stopping || !running || r.deleting[l.root]
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
