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
}

type transcriptMessage struct {
	Type    string                   `json:"type"`
	Session string                   `json:"session"`
	Reset   bool                     `json:"reset"`
	Items   []session.TranscriptItem `json:"items"`
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

	d := &dataSocket{ctx: ctx, conn: conn, db: s.db, rt: s.rt,
		batch: map[string]session.TranscriptItem{}, stale: true}
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
			if tick == nil {
				timer.Reset(coalesce)
				tick = timer.C
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

	workspaces []shownWorkspace
	limits     []store.Limit
	// stale marks workspaces and limits out of date with the database.
	stale bool
	sent  []byte

	selected string
	// batch holds the selected session's changed items by item ID.
	batch map[string]session.TranscriptItem
}

// sendState sends the view data and requests if they differ from the last
// state sent, rereading the view data first when it is stale.
func (d *dataSocket) sendState() error {
	if d.stale {
		workspaces, limits, err := snapshot(d.db, d.rt.Merges())
		if err != nil {
			return err
		}
		d.workspaces, d.limits, d.stale = workspaces, limits, false
	}
	data, err := json.Marshal(stateMessage{"state", buildID(), d.workspaces, d.rt.Requests(), d.limits})
	if err != nil || bytes.Equal(data, d.sent) {
		return err
	}
	if err := d.conn.Write(d.ctx, websocket.MessageText, data); err != nil {
		return err
	}
	d.sent = data
	return nil
}

// selectSession selects a session and sends its whole transcript.
func (d *dataSocket) selectSession(id string) error {
	d.selected = id
	clear(d.batch)
	if id == "" {
		return nil
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

// add records a change for the next batch.
func (d *dataSocket) add(change session.Change) {
	switch {
	case change.Session == "":
		d.stale = true
	case change.Session == d.selected:
		d.batch[change.Item.ID] = session.TranscriptItem{Item: change.Item, Seq: change.Seq}
	}
}

// flush sends the state if it changed, then the batched items in
// transcript order.
func (d *dataSocket) flush() error {
	if err := d.sendState(); err != nil {
		return err
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
	data, err := json.Marshal(transcriptMessage{"transcript", d.selected, reset, items})
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
