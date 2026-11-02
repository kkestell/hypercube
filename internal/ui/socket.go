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
}

type transcriptMessage struct {
	Type    string                   `json:"type"`
	Session string                   `json:"session"`
	Agent   string                   `json:"agent"`
	Reset   bool                     `json:"reset"`
	Items   []session.TranscriptItem `json:"items"`
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

	d := &dataSocket{ctx: ctx, conn: conn, db: s.db, rt: s.rt, releaseView: func() {},
		batch: map[string]map[string]session.TranscriptItem{}, stale: true}
	defer func() { d.releaseView() }()
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
	// stale marks workspaces out of date with the database.
	stale bool
	sent  []byte

	selected    string
	releaseView func()
	// batch holds the selected session's changed items by agent and item ID.
	batch map[string]map[string]session.TranscriptItem
}

// sendState sends the view data and requests if they differ from the last
// state sent, rereading the view data first when it is stale.
func (d *dataSocket) sendState() error {
	if d.stale {
		workspaces, err := snapshot(d.db)
		if err != nil {
			return err
		}
		d.workspaces, d.stale = workspaces, false
	}
	data, err := json.Marshal(stateMessage{"state", buildID(), d.workspaces, d.rt.Requests()})
	if err != nil || bytes.Equal(data, d.sent) {
		return err
	}
	if err := d.conn.Write(d.ctx, websocket.MessageText, data); err != nil {
		return err
	}
	d.sent = data
	return nil
}

// selectSession holds the session open for viewing and sends each of its
// agents' whole transcript.
func (d *dataSocket) selectSession(id string) error {
	release := d.rt.ViewSession(id)
	d.releaseView()
	d.releaseView, d.selected = release, id
	clear(d.batch)
	if id == "" {
		return nil
	}
	var agents []store.Agent
	if err := d.db.Read(func(tx *store.Tx) error {
		var err error
		agents, err = tx.Agents(id)
		return err
	}); err != nil {
		return err
	}
	for _, a := range agents {
		items, err := d.rt.Transcript(a.ID)
		if err != nil {
			return err
		}
		if err := d.sendTranscript(a.ID, true, items); err != nil {
			return err
		}
	}
	return nil
}

// add records a change for the next batch.
func (d *dataSocket) add(change session.Change) {
	switch {
	case change.Agent == "":
		d.stale = true
	case change.Session == d.selected:
		if d.batch[change.Agent] == nil {
			d.batch[change.Agent] = map[string]session.TranscriptItem{}
		}
		d.batch[change.Agent][change.Item.ID] = session.TranscriptItem{Item: change.Item, Seq: change.Seq}
	}
}

// flush sends the state if it changed, then each agent's batched items in
// transcript order.
func (d *dataSocket) flush() error {
	if err := d.sendState(); err != nil {
		return err
	}
	for agent, changed := range d.batch {
		items := make([]session.TranscriptItem, 0, len(changed))
		for _, item := range changed {
			items = append(items, item)
		}
		slices.SortFunc(items, func(a, b session.TranscriptItem) int { return cmp.Compare(a.Seq, b.Seq) })
		if err := d.sendTranscript(agent, false, items); err != nil {
			return err
		}
	}
	clear(d.batch)
	return nil
}

func (d *dataSocket) sendTranscript(agent string, reset bool, items []session.TranscriptItem) error {
	data, err := json.Marshal(transcriptMessage{"transcript", d.selected, agent, reset, items})
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
