package ui

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/coder/websocket"
	"github.com/kkestell/hypercube/internal/github"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

// coalesce lets a burst of streamed text become one update.
const coalesce = 50 * time.Millisecond

type stateMessage struct {
	Type       string            `json:"type"`
	Build      string            `json:"build"`
	Workspaces []shownWorkspace  `json:"workspaces"`
	Requests   []session.Request `json:"requests"`
	Limits     []store.Limit     `json:"limits"`
	Accounts   []github.Account  `json:"accounts"`
}

type transcriptMessage struct {
	Type    string      `json:"type"`
	Session string      `json:"session"`
	Reset   bool        `json:"reset"`
	Items   []shownItem `json:"items"`
}

// shownItem is a transcript item with links to its images in place of their
// data, which can be megabytes per screenshot.
type shownItem struct {
	session.TranscriptItem
	Images []string `json:"images,omitempty"`
}

type openMessage struct {
	Type    string `json:"type"`
	Session string `json:"session"`
	session.FileOpen
}

// state sends view data and the selected session's transcript changes.
func (s *server) state(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// Subscribing before the first reads means a change made during them
	// arrives again afterwards and replaces its item in place.
	changes, unsubscribe := s.rt.Subscribe()
	defer unsubscribe()
	selections := make(chan string)
	go readSelections(ctx, cancel, conn, selections)

	d := &dataSocket{ctx: ctx, conn: conn, db: s.db, rt: s.rt, gh: s.gh,
		batch: map[string]session.TranscriptItem{}}
	defer d.endView()
	githubChanged := s.gh.Changed()
	uiChanged := buildChanged()
	if d.sendState() != nil {
		return
	}
	timer := time.NewTimer(coalesce)
	timer.Stop()
	defer timer.Stop()
	var tick <-chan time.Time
	for {
		select {
		case id := <-selections:
			if d.selectSession(id) != nil {
				return
			}
		case change, ok := <-changes:
			if !ok {
				return
			}
			d.add(change)
			if tick == nil && (d.stale || len(d.batch) > 0) {
				timer.Reset(coalesce)
				tick = timer.C
			}
		case <-githubChanged:
			githubChanged = s.gh.Changed()
			d.stale = true
			if tick == nil {
				timer.Reset(coalesce)
				tick = timer.C
			}
		case <-uiChanged:
			uiChanged = buildChanged()
			if d.sendState() != nil {
				return
			}
		case <-tick:
			tick = nil
			if d.flush() != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// dataSocket is one open data socket: what it last sent, the session it
// selects, and the changes waiting for the next batch.
type dataSocket struct {
	ctx  context.Context
	conn *websocket.Conn
	db   *store.DB
	rt   *session.Runtime
	gh   *github.Watcher

	// stale marks the state as possibly changed since it was last sent.
	stale bool
	sent  []byte

	selected string
	// release ends the view of the selected session, which keeps it awake.
	release func()
	// batch holds the selected session's changed items by item ID.
	batch map[string]session.TranscriptItem
}

// sendState reads the view data and requests and sends them if they differ
// from the last state sent.
func (d *dataSocket) sendState() error {
	d.stale = false
	workspaces, limits, err := snapshot(d.db, d.rt.Merges(), d.gh.Statuses())
	if err != nil {
		return err
	}
	requests, err := d.rt.Requests()
	if err != nil {
		return err
	}
	data, err := json.Marshal(stateMessage{"state", buildID(), workspaces, requests, limits, d.gh.Accounts()})
	if err != nil || bytes.Equal(data, d.sent) {
		return err
	}
	if err := d.conn.Write(d.ctx, websocket.MessageText, data); err != nil {
		return err
	}
	d.sent = data
	return nil
}

// selectSession selects and views a session, which wakes it, and sends its
// whole transcript.
func (d *dataSocket) selectSession(id string) error {
	d.endView()
	d.selected = id
	clear(d.batch)
	if id == "" {
		return nil
	}
	var err error
	if d.release, err = d.rt.View(id); err != nil {
		return err
	}
	items, err := d.rt.Transcript(id)
	if err != nil {
		return err
	}
	if err := d.sendTranscript(true, items); err != nil {
		return err
	}
	return d.sendOpens()
}

func (d *dataSocket) endView() {
	if d.release != nil {
		d.release()
		d.release = nil
	}
}

// add records a change for the next batch. Durable writes and queued file
// opens publish a change without a session. A streamed request also changes
// the requests; other streamed items concern only a page that selects their
// session.
func (d *dataSocket) add(change session.Change) {
	if change.Session == "" || change.Item.Kind == harness.KindAttention {
		d.stale = true
	}
	if change.Session != "" && change.Session == d.selected {
		d.batch[change.Item.ID] = session.TranscriptItem{Item: change.Item, Seq: change.Seq}
	}
}

// flush sends the state and file opens if they may have changed, with the
// batched items in transcript order between them.
func (d *dataSocket) flush() error {
	stale := d.stale
	if stale {
		if err := d.sendState(); err != nil {
			return err
		}
	}
	if len(d.batch) > 0 {
		items := make([]session.TranscriptItem, 0, len(d.batch))
		for _, item := range d.batch {
			items = append(items, item)
		}
		slices.SortFunc(items, func(a, b session.TranscriptItem) int { return cmp.Compare(a.Seq, b.Seq) })
		if err := d.sendTranscript(false, items); err != nil {
			return err
		}
	}
	clear(d.batch)
	if !stale {
		return nil
	}
	return d.sendOpens()
}

func (d *dataSocket) sendOpens() error {
	for _, open := range d.rt.TakeFileOpens(d.selected) {
		data, err := json.Marshal(openMessage{"open", d.selected, open})
		if err != nil {
			return err
		}
		if err := d.conn.Write(d.ctx, websocket.MessageText, data); err != nil {
			return err
		}
	}
	return nil
}

func (d *dataSocket) sendTranscript(reset bool, items []session.TranscriptItem) error {
	shown := make([]shownItem, len(items))
	for i, item := range items {
		shown[i] = shownItem{item, imageLinks(d.selected, item.ID, item.Item.Images)}
	}
	data, err := json.Marshal(transcriptMessage{"transcript", d.selected, reset, shown})
	if err != nil {
		return err
	}
	return d.conn.Write(d.ctx, websocket.MessageText, data)
}

func readSelections(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, selections chan<- string) {
	defer cancel()
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var msg struct {
			Select *string `json:"select"`
		}
		if kind != websocket.MessageText || json.Unmarshal(data, &msg) != nil || msg.Select == nil {
			continue
		}
		select {
		case selections <- *msg.Select:
		case <-ctx.Done():
			return
		}
	}
}
