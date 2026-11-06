package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/terminal"
)

// Restore adopts durable launches. A session whose agent was lost stays parked
// until it is used or has queued messages.
func (r *Runtime) Restore() {
	var families [][]store.Session
	known := map[string]bool{}
	err := r.db.Read(func(tx *store.Tx) error {
		roots, err := tx.Sessions("")
		if err != nil {
			return err
		}
		for _, root := range roots {
			children, err := tx.Children(root.ID)
			if err != nil {
				return err
			}
			family := append([]store.Session{root}, children...)
			families = append(families, family)
			for _, s := range family {
				if s.Launch != "" {
					known[s.Launch] = true
				}
				if !s.Ended {
					known[terminal.Name(s.ID)] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		logf("restore sessions: %v", err)
		return
	}
	run := filepath.Join(store.Dir(), "run")
	entries, _ := os.ReadDir(run)
	for _, e := range entries {
		if !known[e.Name()] {
			h, err := hold.Open(filepath.Join(run, e.Name()))
			if err == nil {
				_ = h.Stop()
				_ = h.Remove()
			}
		}
	}
	workspaces := map[string]bool{}
	for _, family := range families {
		r.restoreFamily(family)
		workspaces[family[0].Workspace] = true
	}
	for id := range workspaces {
		r.refreshMerges(id)
	}
}

func (r *Runtime) restoreFamily(family []store.Session) {
	run := filepath.Join(store.Dir(), "run")
	root := family[0]
	var lost []*live
	var attached []*live
	r.mu.Lock()
	for _, s := range family {
		if s.Launch == "" {
			continue
		}
		dir := filepath.Join(run, s.Launch)
		h, err := hold.Open(dir)
		missing := errors.Is(err, os.ErrNotExist)
		alive := err == nil && h.Alive()
		exited := false
		if err == nil {
			_, exitErr := h.Exit()
			exited = exitErr == nil
		}
		l, err := r.attach(s, root.ID)
		if err != nil {
			logf("restoring %s: %v", s.ID, err)
			continue
		}
		r.live[s.ID] = l
		l.stopping = s.Ended
		// The holder writes its exit record before it releases its lock, so
		// checking the lock first leaves no gap between the two.
		if !missing && !alive && !exited {
			lost = append(lost, l)
		}
		attached = append(attached, l)
	}
	r.mu.Unlock()
	for _, l := range attached {
		go r.run(l)
	}
	// A lost launch ends without a successor, leaving its session parked.
	for _, l := range lost {
		<-l.done
	}
	r.mu.Lock()
	for _, l := range attached {
		r.settleLocked(l.id)
	}
	r.mu.Unlock()
	for _, s := range family {
		if s.Ended {
			continue
		}
		if err := r.Terminals.Adopt(s.ID); err != nil {
			logf("adopting the shell of %s: %v", s.ID, err)
		}
		r.Deliver(s.ID)
	}
}

func (r *Runtime) attach(s store.Session, root string) (*live, error) {
	l := &live{id: s.ID, root: root, workspace: s.Workspace, prefix: s.Launch + ":", position: s.Position, ready: s.Ready, quick: s.Quick, attention: s.Attention, pending: map[string]*pendingItem{}, commands: map[int64]store.Command{}, waiters: map[int64]chan error{}, started: make(chan struct{}), done: make(chan struct{})}
	if s.Ready != 0 {
		close(l.started)
	}
	err := r.db.Read(func(tx *store.Tx) error {
		if err := r.loadSeq(tx, s.ID); err != nil {
			return err
		}
		entries, err := tx.Transcript(s.ID)
		if err != nil {
			return err
		}
		for _, e := range entries {
			var item harness.Item
			if err = json.Unmarshal(e.Data, &item); err != nil {
				return err
			}
			if !item.Done && item.Kind != harness.KindProposal && !item.Offer && strings.HasPrefix(item.ID, l.prefix) {
				l.pending[item.ID] = &pendingItem{seq: e.Seq, item: item}
			}
		}
		commands, err := tx.Commands(s.ID)
		if err != nil {
			return err
		}
		for _, c := range commands {
			l.commands[c.ID] = c
		}
		return nil
	})
	return l, err
}
