package session

import (
	"cmp"
	"context"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// live is one launch of an agent's plugin. It starts in Runtime.live with
// starting set, becomes ready when the plugin replies to start, and is over
// once it leaves Runtime.live: by end, when it fails to start or its plugin
// exits, or by detachLocked, when it is stopped. Events and exits from a launch
// that is over are ignored, and no further deliveries start.
type live struct {
	id, root string
	// prefix makes the plugin's item IDs unique across launches, since
	// plugins number items from the start of each one.
	prefix string
	// client is set once the plugin process exists.
	client   *harness.Client
	starting bool
	// launched is when the launch became ready.
	launched time.Time
	// quick counts the quick exits before this launch.
	quick int
	// cause is why the launch ended by itself. It is nil for a launch that is
	// running or was stopped.
	cause      error
	delivering bool
	attention  bool
	// pending holds items still streaming. They are written when done, or
	// when the launch ends.
	pending map[string]*pendingItem
}

type pendingItem struct {
	seq    int64
	item   harness.Item
	native string
}

// current reports whether l is its session's launch. The caller holds mu.
func (r *Runtime) current(l *live) bool { return r.live[l.id] == l }

// register adds a starting launch for the session, whose transcript position is
// loaded. The caller holds mu.
func (r *Runtime) register(id, root string, quick int) *live {
	r.gen++
	l := &live{id: id, root: root, prefix: strconv.FormatInt(r.gen, 36) + ":",
		starting: true, quick: quick, pending: map[string]*pendingItem{}}
	r.live[id] = l
	return l
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

// launch starts l's plugin and waits until its harness is ready. A launch that
// fails to start ends. launch returns why the launch ended, or errStopped if it
// was stopped first.
func (r *Runtime) launch(l *live) error {
	client, err := r.startPlugin(l)
	if err != nil {
		// end records the start's error before Kill makes the plugin exit.
		r.end(l, err)
		if client != nil {
			client.Kill()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.current(l) {
		return cmp.Or(l.cause, errStopped)
	}
	l.starting, l.launched = false, time.Now()
	if err := r.save(func(tx *write) error {
		s, err := tx.Session(l.id)
		if err != nil || s.Activity != store.Exited {
			return err
		}
		s.Activity = store.Idle
		return tx.SaveSession(s)
	}); err != nil {
		logf("marking %s ready: %v", l.id, err)
	}
	r.kickLocked(l.id)
	return nil
}

// startPlugin starts l's plugin and sends it start. It returns the plugin's
// client, if the process started, with any error.
func (r *Runtime) startPlugin(l *live) (*harness.Client, error) {
	path, args, err := r.startArgs(l.id)
	if err != nil {
		return nil, err
	}
	client, err := harness.Launch(path, func(ev harness.Event) { r.event(l, ev) }, func(err error) { r.end(l, err) })
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	running := r.current(l)
	if running {
		l.client = client
	}
	r.mu.Unlock()
	if !running {
		return client, errStopped
	}
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	return client, client.Do(ctx, harness.Command{Command: harness.CmdStart, Start: &args})
}

// startArgs returns the session's plugin and what it starts with.
func (r *Runtime) startArgs(id string) (path string, args harness.StartArgs, err error) {
	var s store.Session
	err = r.db.Read(func(tx *store.Tx) error {
		if s, err = tx.Session(id); err != nil {
			return err
		}
		var profiles []harness.Profile
		if s.Parent == "" {
			cfg, err := harness.LoadConfig()
			if err != nil {
				return err
			}
			profiles = cfg.ChildProfiles()
		}
		instructions, err := report.Instructions(tx, s, profiles)
		if err != nil {
			return err
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		args = harness.StartArgs{
			Hypercube: exe, Env: map[string]string{"HYPERCUBE_SESSION_ID": s.ID, "HYPERCUBE_URL": r.url},
			Session: s.ID, Model: s.Model, Effort: s.Effort, Conversation: s.Conversation,
			Dir: s.Dir, Instructions: instructions, StateDir: store.Dir(),
		}
		return nil
	})
	if err != nil {
		return "", args, err
	}
	path, err = harness.Find(r.PluginDir, s.Harness)
	return path, args, err
}

// end records that l's launch is over because it failed to start or its
// plugin exited: it saves l's unfinished items, notes why in the transcript,
// and marks the agent exited. An agent whose plugin exits after starting is
// launched again, resuming its conversation, unless its session stopped or it
// ended maxQuickExits times in a row within quickExit of starting. A launch
// that failed to start stays exited. end ignores a launch that was stopped or
// has ended already.
func (r *Runtime) end(l *live, cause error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.current(l) {
		return
	}
	delete(r.live, l.id)
	l.cause = cause
	note, relaunch, quick := "Could not start the agent: ", false, 0
	if !l.starting {
		note = "The agent exited: "
		if time.Since(l.launched) < quickExit {
			quick = l.quick + 1
		}
		relaunch = quick < maxQuickExits
	}
	err := r.save(func(tx *write) error {
		for _, p := range l.drain() {
			if err := tx.putItem(l.id, p.seq, p.item); err != nil {
				return err
			}
		}
		if err := r.note(tx, l, harness.KindError, note+cause.Error()); err != nil {
			return err
		}
		if err := markExited(tx, l.id); err != nil {
			return err
		}
		s, err := tx.Session(l.id)
		if err != nil {
			return err
		}
		running, err := tx.Running(s)
		relaunch = relaunch && running
		return err
	})
	if err != nil {
		logf("recording the end of %s: %v", l.id, err)
		return
	}
	if relaunch {
		next := r.register(l.id, l.root, quick)
		// A relaunch that fails ends too, which records why.
		go r.launch(next)
	}
}

func markExited(tx *write, id string) error {
	s, err := tx.Session(id)
	if err != nil {
		return err
	}
	s.Activity, s.Attention = store.Exited, false
	return tx.SaveSession(s)
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
