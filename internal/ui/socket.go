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
	changes, unsubscribe := s.rt.Subscribe()
	defer unsubscribe()
	selections := make(chan string)
	go readSelections(ctx, cancel, conn, selections)

	workspaces, err := snapshot(s.db)
	if err != nil {
		return
	}
	var sent []byte
	sendState := func() error {
		data, err := json.Marshal(stateMessage{"state", buildID(), workspaces, s.rt.Requests()})
		if err != nil || bytes.Equal(data, sent) {
			return err
		}
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			return err
		}
		sent = data
		return nil
	}
	if sendState() != nil {
		return
	}

	var selected string
	releaseView := func() {}
	defer func() { releaseView() }()
	batch := map[string]map[string]session.TranscriptItem{}
	stale := false
	timer := time.NewTimer(coalesce)
	timer.Stop()
	defer timer.Stop()
	var tick <-chan time.Time
	for {
		select {
		case id := <-selections:
			nextView := s.rt.ViewSession(id)
			releaseView()
			releaseView, selected = nextView, id
			if id != "" {
				var agents []store.Agent
				if err := s.db.Read(func(tx *store.Tx) error {
					var err error
					agents, err = tx.Agents(id)
					return err
				}); err != nil {
					return
				}
				for _, a := range agents {
					items, err := s.rt.Transcript(a.ID)
					if err != nil || writeTranscript(ctx, conn, id, a.ID, true, items) != nil {
						return
					}
				}
			}
			clear(batch)
		case change, ok := <-changes:
			if !ok {
				return
			}
			if change.Agent == "" {
				stale = true
			} else if change.Session == selected {
				if batch[change.Agent] == nil {
					batch[change.Agent] = map[string]session.TranscriptItem{}
				}
				batch[change.Agent][change.Item.ID] = session.TranscriptItem{Item: change.Item, Seq: change.Seq}
			}
			if tick == nil {
				timer.Reset(coalesce)
				tick = timer.C
			}
		case <-tick:
			tick = nil
			if stale {
				workspaces, err = snapshot(s.db)
				if err != nil {
					return
				}
				stale = false
			}
			if sendState() != nil {
				return
			}
			for agent, pending := range batch {
				items := make([]session.TranscriptItem, 0, len(pending))
				for _, item := range pending {
					items = append(items, item)
				}
				slices.SortFunc(items, func(a, b session.TranscriptItem) int { return cmp.Compare(a.Seq, b.Seq) })
				if writeTranscript(ctx, conn, selected, agent, false, items) != nil {
					return
				}
			}
			clear(batch)
		case <-ctx.Done():
			return
		}
	}
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

func writeTranscript(ctx context.Context, conn *websocket.Conn, selected, agent string, reset bool, items []session.TranscriptItem) error {
	data, err := json.Marshal(transcriptMessage{"transcript", selected, agent, reset, items})
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}
