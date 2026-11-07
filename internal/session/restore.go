package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/terminal"
)

// Restore registers durable launches before serve starts the app's services,
// then adopts their readers. Services remain available to attaching agents. A
// session whose agent was lost stays parked until used or sent a message.
// Pass nil for serve when no services need to start.
func (r *Runtime) Restore(serve func()) error {
	var families [][]store.Session
	var workspaceRecords []store.Workspace
	known := map[string]bool{}
	err := r.db.Read(func(tx *store.Tx) error {
		var err error
		workspaceRecords, err = tx.Workspaces()
		if err != nil {
			return err
		}
		for _, workspace := range workspaceRecords {
			known[terminal.Name(workspaceShellOwner(workspace.ID))] = true
		}
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
		return fmt.Errorf("restore sessions: %w", err)
	}
	attached, lost, err := r.registerRestored(families)
	if err != nil {
		return err
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
	// Workspace deletion can be served immediately, so its holders must already
	// belong to the terminal set before requests can remove their owners.
	for _, workspace := range workspaceRecords {
		if err := r.Terminals.Adopt(workspaceShellOwner(workspace.ID)); err != nil {
			logf("adopting workspace shell %s: %v", workspace.ID, err)
		}
	}
	if serve != nil {
		serve()
	}
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
	workspaces := map[string]bool{}
	for _, family := range families {
		for _, s := range family {
			if s.Ended {
				continue
			}
			if err := r.Terminals.Adopt(s.ID); err != nil {
				logf("adopting the shell of %s: %v", s.ID, err)
			}
			r.Deliver(s.ID)
		}
		workspaces[family[0].Workspace] = true
	}
	for id := range workspaces {
		r.refreshMerges(id)
	}
	return nil
}

// registerRestored installs every saved launch before any lifecycle request
// can decide a new launch from the same session snapshot.
func (r *Runtime) registerRestored(families [][]store.Session) (attached, lost []*live, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := filepath.Join(store.Dir(), "run")
	for _, family := range families {
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
			l, err := r.attach(s, family[0].ID)
			if err != nil {
				return nil, nil, fmt.Errorf("restoring %s: %w", s.ID, err)
			}
			r.live[s.ID] = l
			l.stopping = s.Ended
			// The holder writes its exit record before releasing its lock.
			if !missing && !alive && !exited {
				lost = append(lost, l)
			}
			attached = append(attached, l)
		}
	}
	return attached, lost, nil
}

func (r *Runtime) attach(s store.Session, root string) (*live, error) {
	l := &live{
		id: s.ID, root: root, workspace: s.Workspace, prefix: s.Launch + ":",
		position: s.Position, ready: s.Ready, quick: s.Quick, attention: s.Attention,
		pending: map[string]*pendingItem{}, commands: map[int64]store.Command{}, waiters: map[int64]chan error{},
		started: make(chan struct{}), done: make(chan struct{}),
	}
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
