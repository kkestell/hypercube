package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
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
	// its launch, stays ended.
	quickExit     = 10 * time.Second
	maxQuickExits = 3
)

var (
	errStopped = errors.New("the agent was stopped")
	errClosing = errors.New("hypercube is shutting down")
)

// Runtime is the app's service for running agents. It owns every plugin
// process, delivers queued messages and the user's input to them, and records
// what they report. It is the only writer of the database.
//
// mu guards the live agents and their transcripts in memory. It is never held
// while waiting on a plugin: a native request can call back into the MCP tools,
// which need it.
type Runtime struct {
	// PluginDir holds the harness plugins. Empty means beside the running
	// executable.
	PluginDir string
	// Terminals holds each running session's shell. Stop, Delete, and Shutdown
	// end them.
	Terminals terminal.Set

	db  *store.DB
	url string

	mu   sync.Mutex
	live map[string]*live // by agent ID
	// seqs holds each agent's last transcript position. It outlives launches,
	// because a launch that has ended can still record a late item.
	seqs    map[string]int64
	gen     int64
	closing bool
	// changed is closed and replaced whenever an agent's transcript changes.
	changed chan struct{}
}

// live is one launch of an agent's plugin. Events and exits from a launch that
// is no longer in Runtime.live are ignored.
type live struct {
	id, session, role string
	// prefix makes the plugin's item IDs unique across launches, since
	// plugins number items from the start of each one.
	prefix string
	client *harness.Client
	// starting holds until the plugin replies to start. exit records an exit
	// during that time, which launch reports.
	starting bool
	exit     error
	launched time.Time
	// quick counts the quick exits before this launch.
	quick      int
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

// New returns a runtime for the database. url is where agents reach the app.
func New(db *store.DB, url string) *Runtime {
	return &Runtime{db: db, url: url, live: map[string]*live{}, seqs: map[string]int64{}, gen: time.Now().UnixMilli(),
		changed: make(chan struct{})}
}

// Changes returns a channel that closes the next time an agent's transcript
// changes, including items still streaming. Callers read the transcripts after
// getting the channel, so no change is missed.
func (r *Runtime) Changes() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.changed
}

// touchLocked wakes the watchers of Changes. The caller holds mu.
func (r *Runtime) touchLocked() {
	close(r.changed)
	r.changed = make(chan struct{})
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "hypercube: "+format+"\n", args...)
}

func (r *Runtime) current(l *live) bool { return r.live[l.id] == l }

func (r *Runtime) register(a store.Agent, quick int) *live {
	r.gen++
	l := &live{id: a.ID, session: a.Session, role: a.Role, prefix: strconv.FormatInt(r.gen, 36) + ":",
		starting: true, launched: time.Now(), quick: quick, pending: map[string]*pendingItem{}}
	r.live[a.ID] = l
	if _, ok := r.seqs[a.ID]; !ok {
		err := r.db.Read(func(tx *store.Tx) error {
			last, err := tx.LastSeq(a.ID)
			r.seqs[a.ID] = last
			return err
		})
		if err != nil {
			delete(r.seqs, a.ID)
			logf("reading the transcript of %s: %v", a.ID, err)
		}
	}
	return l
}

// nextSeq returns the agent's next transcript position. The caller holds mu.
func (r *Runtime) nextSeq(agent string) int64 {
	r.seqs[agent]++
	return r.seqs[agent]
}

func (r *Runtime) sessionLive(session string) bool {
	for _, l := range r.live {
		if l.session == session {
			return true
		}
	}
	return false
}

func entry(seq int64, item harness.Item) store.Entry {
	data, _ := json.Marshal(item)
	return store.Entry{Seq: seq, Item: item.ID, Data: data}
}

// drain takes the launch's unfinished items as finished entries.
func (l *live) drain() []store.Entry {
	var entries []store.Entry
	for id, p := range l.pending {
		p.item.Done = true
		entries = append(entries, entry(p.seq, p.item))
		delete(l.pending, id)
	}
	slices.SortFunc(entries, func(a, b store.Entry) int { return int(a.Seq - b.Seq) })
	return entries
}

// note adds an item written by Hypercube to l's agent's transcript. The caller
// holds mu.
func (r *Runtime) note(tx *store.Tx, l *live, kind, text string) error {
	return tx.PutEntry(l.id, entry(r.nextSeq(l.id), harness.Item{ID: l.prefix + "host-" + store.NewID(), Kind: kind, Text: text, Done: true}))
}

// launch starts the plugin for l, which the caller registered, and waits for
// its harness to be ready. Failure leaves l unregistered.
func (r *Runtime) launch(l *live) error {
	var start harness.StartArgs
	var harnessName string
	err := r.db.Read(func(tx *store.Tx) error {
		a, err := tx.Agent(l.id)
		if err != nil {
			return err
		}
		var profiles []harness.Profile
		if a.Role == store.Lead {
			cfg, err := harness.LoadConfig()
			if err != nil {
				return err
			}
			profiles = cfg.Workers()
		}
		instructions, err := report.Instructions(tx, a, profiles)
		if err != nil {
			return err
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		harnessName = a.Harness
		start = harness.StartArgs{
			Hypercube: exe, Env: map[string]string{"HYPERCUBE_AGENT_ID": a.ID, "HYPERCUBE_URL": r.url},
			Agent: a.ID, Role: a.Role, Model: a.Model, Effort: a.Effort, Conversation: a.Conversation,
			Dir: a.Dir, Instructions: instructions, StateDir: store.Dir(),
		}
		return nil
	})
	var client *harness.Client
	if err == nil {
		var path string
		if path, err = harness.Find(r.PluginDir, harnessName); err == nil {
			client, err = harness.Launch(path, func(ev harness.Event) { r.event(l, ev) }, func(err error) { r.exited(l, err) })
		}
	}
	if err == nil {
		r.mu.Lock()
		if r.closing || !r.current(l) {
			r.mu.Unlock()
			client.Kill()
			return errStopped
		}
		l.client = client
		r.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
		err = client.Do(ctx, harness.Command{Command: harness.CmdStart, Start: &start})
		cancel()
	}
	r.mu.Lock()
	l.starting = false
	if err == nil && l.exit != nil {
		err = l.exit
	}
	// A launch stopped by Stop, Delete, or shutdown reports only that, so its
	// caller does not record or relaunch it.
	if r.closing || !r.current(l) {
		err = errStopped
	}
	if err != nil {
		if r.current(l) {
			delete(r.live, l.id)
		}
		r.mu.Unlock()
		if client != nil {
			client.Kill()
		}
		return err
	}
	defer r.mu.Unlock()
	l.launched = time.Now()
	if err := r.db.Tx(func(tx *store.Tx) error {
		a, err := tx.Agent(l.id)
		if err != nil || a.Activity != store.Exited {
			return err
		}
		a.Activity = store.Idle
		return tx.SaveAgent(a)
	}); err != nil {
		logf("marking %s ready: %v", l.id, err)
	}
	r.kickLocked(l.id)
	return nil
}

// failed records that l could not start, leaving the agent ended.
func (r *Runtime) failed(l *live, cause error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur := r.live[l.id]; cur != nil && cur != l {
		return
	}
	err := r.db.Tx(func(tx *store.Tx) error {
		if err := r.note(tx, l, harness.KindError, "Could not start the agent: "+cause.Error()); err != nil {
			return err
		}
		return r.markEnded(tx, l.id)
	})
	if err != nil {
		logf("recording the failed start of %s: %v", l.id, err)
	}
}

func (r *Runtime) markEnded(tx *store.Tx, id string) error {
	a, err := tx.Agent(id)
	if err != nil {
		return err
	}
	a.Activity, a.Attention = store.Exited, false
	return tx.SaveAgent(a)
}

// exited handles the end of l's plugin.
func (r *Runtime) exited(l *live, reason error) {
	r.mu.Lock()
	if l.starting {
		l.exit = reason
		r.mu.Unlock()
		return
	}
	if !r.current(l) {
		r.mu.Unlock()
		return
	}
	delete(r.live, l.id)
	r.ended(l, reason)
}

// ended records that l's launch is over and relaunches the agent, resuming its
// conversation, unless it ended three times in a row within ten seconds of
// launching. The caller holds mu, and ended releases it.
func (r *Runtime) ended(l *live, reason error) {
	if cur := r.live[l.id]; cur != nil && cur != l {
		r.mu.Unlock()
		return
	}
	quick := l.quick + 1
	if time.Since(l.launched) >= quickExit {
		quick = 0
	}
	relaunch := quick < maxQuickExits && !r.closing
	err := r.db.Tx(func(tx *store.Tx) error {
		for _, e := range l.drain() {
			if err := tx.PutEntry(l.id, e); err != nil {
				return err
			}
		}
		if err := r.note(tx, l, harness.KindError, "The agent ended: "+reason.Error()); err != nil {
			return err
		}
		a, err := tx.Agent(l.id)
		if err != nil {
			return err
		}
		a.Activity, a.Attention = store.Exited, false
		if err := tx.SaveAgent(a); err != nil {
			return err
		}
		s, err := tx.Session(a.Session)
		relaunch = relaunch && s.Running
		return err
	})
	if err != nil {
		logf("recording the end of %s: %v", l.id, err)
		relaunch = false
	}
	var next *live
	if relaunch {
		var a store.Agent
		a.ID, a.Session, a.Role = l.id, l.session, l.role
		next = r.register(a, quick)
	}
	r.mu.Unlock()
	if next != nil {
		go func() {
			if err := r.launch(next); err != nil && !errors.Is(err, errStopped) {
				r.mu.Lock()
				r.ended(next, err)
			}
		}()
	}
}

// event applies an event from l's plugin.
func (r *Runtime) event(l *live, ev harness.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.current(l) {
		return
	}
	defer r.touchLocked()
	var write []store.Entry
	var failures []string
	for _, it := range ev.Items {
		native := it.ID
		it.ID = l.prefix + it.ID
		if it.Parent != "" {
			it.Parent = l.prefix + it.Parent
		}
		p := l.pending[it.ID]
		if p == nil {
			p = &pendingItem{seq: r.nextSeq(l.id), native: native}
			l.pending[it.ID] = p
			if it.Kind == harness.KindError {
				failures = append(failures, it.Text)
			}
		} else if it.Append {
			merged := p.item
			merged.Text += it.Text
			merged.Done = it.Done
			it = merged
		}
		p.item = it
		if it.Done {
			delete(l.pending, it.ID)
			write = append(write, entry(p.seq, it))
		}
	}
	attention := false
	for _, p := range l.pending {
		attention = attention || p.item.Kind == harness.KindAttention
	}
	changed := ev.Conversation != "" || ev.State != "" || ev.Usage != nil || attention != l.attention
	if len(write) == 0 && len(failures) == 0 && !changed {
		return
	}
	err := r.db.Tx(func(tx *store.Tx) error {
		for _, e := range write {
			if err := tx.PutEntry(l.id, e); err != nil {
				return err
			}
		}
		if !changed && len(failures) == 0 {
			return nil
		}
		a, err := tx.Agent(l.id)
		if err != nil {
			return err
		}
		if changed {
			s, err := tx.Session(a.Session)
			if err != nil {
				return err
			}
			report.Apply(&a, &s, ev)
			a.Attention = attention
			if err := tx.SaveAgent(a); err != nil {
				return err
			}
			if err := tx.SaveSession(s); err != nil {
				return err
			}
		}
		for _, text := range failures {
			if err := report.Failure(tx, a, text); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logf("recording an event from %s: %v", l.id, err)
		return
	}
	l.attention = attention
	if len(failures) > 0 {
		r.kickSessionLocked(l.session)
	} else if ev.State == harness.Idle {
		r.kickLocked(l.id)
	}
}

// Deliver sends the agent's queued messages, if it is running and its session
// is not waiting for the user. Callers queue a message, then call Deliver.
func (r *Runtime) Deliver(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.kickLocked(id)
}

func (r *Runtime) kickSessionLocked(session string) {
	for id, l := range r.live {
		if l.session == session {
			r.kickLocked(id)
		}
	}
}

func (r *Runtime) kickLocked(id string) {
	l := r.live[id]
	if l == nil || l.starting || l.delivering {
		return
	}
	l.delivering = true
	go r.deliver(l, l.client)
}

// deliver sends l's queued messages one at a time, removing each after the
// plugin accepts it. Messages go out whether the agent is idle or working; the
// plugin steers or queues them natively. A message the plugin refuses stays
// queued until the next delivery attempt.
func (r *Runtime) deliver(l *live, client *harness.Client) {
	for {
		r.mu.Lock()
		m, text, from, seq, ok := r.next(l)
		if !ok {
			l.delivering = false
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()
		sent := request(client, harness.Command{Command: harness.CmdInput, Text: text})
		r.mu.Lock()
		var err error
		if sent != nil {
			err = r.db.Tx(func(tx *store.Tx) error {
				return r.note(tx, l, harness.KindError, "Delivering a message failed: "+sent.Error())
			})
		} else {
			err = r.db.Tx(func(tx *store.Tx) error {
				if err := tx.Dequeue(m.ID); err != nil {
					return err
				}
				item := harness.Item{ID: l.prefix + "host-" + store.NewID(), Kind: harness.KindUser, Text: text, Done: true}
				if from != "" {
					item.Text, item.From = m.Text, from
				}
				return tx.PutEntry(l.id, entry(seq, item))
			})
		}
		if err != nil {
			logf("delivering to %s: %v", l.id, err)
		}
		if sent != nil || err != nil {
			l.delivering = false
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()
	}
}

// next returns l's oldest queued message, its text as the model sees it, the
// address of the agent that sent it if an agent did, and a transcript position
// for it. It reports false when there is nothing to send.
func (r *Runtime) next(l *live) (m store.Message, text, from string, seq int64, ok bool) {
	if !r.current(l) {
		return m, "", "", 0, false
	}
	err := r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(l.session)
		if err != nil || s.AwaitingUser {
			return err
		}
		queued, err := tx.Queued(l.id)
		if err != nil || len(queued) == 0 {
			return err
		}
		m, ok = queued[0], true
		text = report.Format(tx, queued[:1])
		if a, err := tx.Agent(m.From); err == nil {
			from = a.Address()
		}
		return nil
	})
	if err != nil {
		logf("reading messages for %s: %v", l.id, err)
		return m, "", "", 0, false
	}
	if ok {
		seq = r.nextSeq(l.id)
	}
	return m, text, from, seq, ok
}

func request(client *harness.Client, cmd harness.Command) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return client.Do(ctx, cmd)
}

// ready returns the running agent's live launch.
func (r *Runtime) ready(agent string) (*live, error) {
	l := r.live[agent]
	if l == nil || l.starting {
		return nil, errors.New("the agent is not running")
	}
	return l, nil
}

// Input sends the user's prompt, text and images, to the agent at once, even
// while it works. It also releases the session's queued messages.
func (r *Runtime) Input(agent, text string, images ...harness.Image) error {
	if strings.TrimSpace(text) == "" && len(images) == 0 {
		return errors.New("a prompt needs text or an image")
	}
	if err := harness.CheckImages(images); err != nil {
		return err
	}
	r.mu.Lock()
	l, err := r.ready(agent)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	var before store.Agent
	if err := r.db.Read(func(tx *store.Tx) error { var err error; before, err = tx.Agent(agent); return err }); err != nil {
		r.mu.Unlock()
		return err
	}
	seq, client := r.nextSeq(agent), l.client
	r.mu.Unlock()
	if err := request(client, harness.Command{Command: harness.CmdInput, Text: text, Images: images}); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	err = r.db.Tx(func(tx *store.Tx) error {
		s, err := tx.Session(l.session)
		if err != nil {
			return err
		}
		titled := text
		if strings.TrimSpace(text) == "" {
			titled = "Image"
		}
		report.Prompt(before, &s, titled)
		if err := tx.SaveSession(s); err != nil {
			return err
		}
		return tx.PutEntry(l.id, entry(seq, harness.Item{ID: l.prefix + "host-" + store.NewID(), Kind: harness.KindUser, Text: text, Images: images, Done: true}))
	})
	if err != nil {
		return err
	}
	r.kickSessionLocked(l.session)
	return nil
}

// Interrupt stops the agent's current turn. The turn stays in the transcript
// and is not sent again.
func (r *Runtime) Interrupt(agent string) error {
	r.mu.Lock()
	l, err := r.ready(agent)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	client := l.client
	r.mu.Unlock()
	return request(client, harness.Command{Command: harness.CmdInterrupt})
}

// Answer replies to an attention request with one of its choices, typed text,
// or both. A request that has finished cannot be answered.
func (r *Runtime) Answer(agent, requestID, choice, text string) error {
	r.mu.Lock()
	l, err := r.ready(agent)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	p := l.pending[requestID]
	if p == nil || p.item.Kind != harness.KindAttention {
		r.mu.Unlock()
		return errors.New("the request is finished")
	}
	client, native := l.client, p.native
	r.mu.Unlock()
	return request(client, harness.Command{Command: harness.CmdAnswer, Request: native, Choice: choice, Text: text})
}

// Shell returns the running session's shell, starting it in the workspace
// folder with the given size if the session has none.
func (r *Runtime) Shell(session string, cols, rows int) (*terminal.Shell, error) {
	var dir string
	err := r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(session)
		if err != nil {
			return err
		}
		if !s.Running {
			return errors.New("the session is not running")
		}
		w, err := tx.Workspace(s.Workspace)
		dir = w.Path
		return err
	})
	if err != nil {
		return nil, err
	}
	return r.Terminals.Open(session, dir, cols, rows)
}

// Transcript returns the agent's items in order: those saved, then those still
// streaming.
func (r *Runtime) Transcript(agent string) ([]harness.Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var entries []store.Entry
	if err := r.db.Read(func(tx *store.Tx) error {
		var err error
		entries, err = tx.Transcript(agent)
		return err
	}); err != nil {
		return nil, err
	}
	if l := r.live[agent]; l != nil {
		for _, p := range l.pending {
			entries = append(entries, entry(p.seq, p.item))
		}
		slices.SortFunc(entries, func(a, b store.Entry) int { return int(a.Seq - b.Seq) })
	}
	items := make([]harness.Item, 0, len(entries))
	for _, e := range entries {
		var item harness.Item
		if err := json.Unmarshal(e.Data, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// detach removes the session's running agents from the runtime, saving their
// unfinished items, and returns them for stopClients. With ended set it also
// marks the agents ended.
func (r *Runtime) detach(session string, ended bool) []*live {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.detachLocked(func(l *live) bool { return l.session == session }, ended)
}

func (r *Runtime) detachLocked(match func(*live) bool, ended bool) []*live {
	var lives []*live
	for id, l := range r.live {
		if match(l) {
			lives = append(lives, l)
			delete(r.live, id)
		}
	}
	err := r.db.Tx(func(tx *store.Tx) error {
		for _, l := range lives {
			for _, e := range l.drain() {
				if err := tx.PutEntry(l.id, e); err != nil {
					return err
				}
			}
			if ended {
				if err := r.markEnded(tx, l.id); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		logf("saving agents' transcripts: %v", err)
	}
	return lives
}

// stopClients ends the plugins of detached agents, in parallel.
func stopClients(lives []*live) {
	var wg sync.WaitGroup
	for _, l := range lives {
		if l.client != nil {
			wg.Go(func() { l.client.Shutdown(shutdownWait) })
		}
	}
	wg.Wait()
}

// Shutdown ends every agent's plugin for the app's exit. Sessions keep the
// running mark, so Restore starts them again.
func (r *Runtime) Shutdown() {
	r.mu.Lock()
	r.closing = true
	lives := r.detachLocked(func(*live) bool { return true }, true)
	r.mu.Unlock()
	r.Terminals.Close()
	stopClients(lives)
}
