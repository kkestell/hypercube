package session

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kkestell/hypercube/internal/store"
)

// Alert is a request or a reply the user has not yet been told about.
type Alert struct {
	// Key identifies the alert: "request:<session>:<item>" or "reply:<session>:<seq>".
	Key     string
	Session string
	Title   string
	// Subtitle is the workspace name, then the parent's title for a child.
	Subtitle string
	Text     string
}

// WatchAlerts calls notify for each attention request and each finished turn's
// reply that appears after it starts, until ctx ends. Requests and replies
// present when it starts are not alerted.
func (r *Runtime) WatchAlerts(ctx context.Context, notify func(Alert)) error {
	w := newAlertWatch(r)
	changes, cancel := r.Subscribe()
	defer func() { cancel() }()
	if err := w.read(nil); err != nil {
		return err
	}
	for {
		select {
		case change, ok := <-changes:
			if !ok {
				changes, cancel = r.Subscribe()
			} else if change.Session != "" {
				// A streamed item changes neither requests nor replies; the
				// write that does publishes an empty Change.
				continue
			}
			if err := w.read(notify); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// alertWatch remembers what has been alerted: the open requests, each
// session's last reply position, and which sessions were busy at the last read.
type alertWatch struct {
	runtime  *Runtime
	requests map[string]bool
	replies  map[string]int64
	busy     map[string]bool
}

func newAlertWatch(r *Runtime) *alertWatch {
	return &alertWatch{runtime: r, requests: map[string]bool{}, replies: map[string]int64{}, busy: map[string]bool{}}
}

// read records the current requests and replies and sends the new ones to
// notify. A nil notify only records them.
func (w *alertWatch) read(notify func(Alert)) error {
	requests := w.runtime.Requests()
	var alerts []Alert
	err := w.runtime.db.Read(func(tx *store.Tx) error {
		for _, req := range requests {
			key := "request:" + req.Session + ":" + req.Item.ID
			if w.requests[key] {
				continue
			}
			w.requests[key] = true
			a, err := w.alert(tx, req.Session, key, req.Item.Text)
			if err != nil {
				return err
			}
			alerts = append(alerts, a)
		}
		workspaces, err := tx.Workspaces()
		if err != nil {
			return err
		}
		for _, ws := range workspaces {
			tops, err := tx.Sessions(ws.ID)
			if err != nil {
				return err
			}
			for _, top := range tops {
				children, err := tx.Children(top.ID)
				if err != nil {
					return err
				}
				for _, s := range append([]store.Session{top}, children...) {
					wasBusy := w.busy[s.ID]
					w.busy[s.ID] = s.Activity == store.Busy
					if s.Activity != store.Idle || !(s.Unseen || wasBusy) {
						continue
					}
					text, seq, err := tx.LastText(s.ID)
					if err != nil {
						return err
					}
					if seq <= w.replies[s.ID] {
						continue
					}
					w.replies[s.ID] = seq
					a, err := w.alert(tx, s.ID, "reply:"+s.ID+":"+strconv.FormatInt(seq, 10), text)
					if err != nil {
						return err
					}
					alerts = append(alerts, a)
				}
			}
		}
		return nil
	})
	if err != nil || notify == nil {
		return err
	}
	for _, a := range alerts {
		notify(a)
	}
	return nil
}

// alert describes a request or reply from a session, with the workspace and
// parent it belongs to.
func (w *alertWatch) alert(tx *store.Tx, id, key, text string) (Alert, error) {
	s, err := tx.Session(id)
	if err != nil {
		return Alert{}, err
	}
	root, err := tx.Root(id)
	if err != nil {
		return Alert{}, err
	}
	ws, err := tx.Workspace(root.Workspace)
	if err != nil {
		return Alert{}, err
	}
	subtitle := []string{filepath.Base(ws.Path)}
	if s.Parent != "" {
		parent, err := tx.Session(s.Parent)
		if err != nil {
			return Alert{}, err
		}
		subtitle = append(subtitle, parent.Title)
	}
	return Alert{Key: key, Session: id, Title: s.Title, Subtitle: strings.Join(subtitle, " › "), Text: text}, nil
}
