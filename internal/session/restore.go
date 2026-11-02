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

// Restore adopts durable launches before starting families that lost every
// process. Only those families await the user's next prompt.
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
				if root.Running && !s.Ended {
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
	for _, family := range families {
		r.restoreFamily(family)
	}
}

func (r *Runtime) restoreFamily(family []store.Session) {
	run := filepath.Join(store.Dir(), "run")
	root := family[0]
	allLost := true
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
		// The holder writes its exit record before it releases its lock, so
		// checking the lock first leaves no gap between the two.
		holderLost := !missing && !alive && !exited
		allLost = allLost && holderLost
		l, err := r.attach(s, root.ID)
		if err != nil {
			logf("restoring %s: %v", s.ID, err)
			continue
		}
		r.live[s.ID] = l
		if !root.Running || s.Ended {
			l.stopping = true
		}
		if holderLost {
			lost = append(lost, l)
		}
		attached = append(attached, l)
	}
	r.mu.Unlock()
	// Read ended and lost holders fully before deciding how the family resumes.
	for _, l := range attached {
		go r.run(l)
	}
	for _, l := range lost {
		<-l.done
	}
	if !root.Running {
		return
	}

	if allLost {
		if err := r.Start(root.ID); err != nil {
			logf("restore %s: %v", root.ID, err)
		}
	} else {
		for _, l := range lost {
			if l.stopping {
				continue
			}
			r.mu.Lock()
			var next *live
			err := r.save(func(tx *write) error {
				current, err := tx.Session(l.id)
				if err != nil {
					return err
				}
				next, err = r.decide(tx, current, root.ID, current.Quick)
				return err
			})
			if err == nil {
				r.live[l.id] = next
				go r.launch(next)
			} else {
				logf("restore %s: %v", l.id, err)
			}
			r.mu.Unlock()
		}
	}
	for _, s := range family {
		if !s.Ended {
			if err := r.Terminals.Adopt(s.ID); err != nil {
				logf("adopting the shell of %s: %v", s.ID, err)
			}
		}
	}
}

func (r *Runtime) attach(s store.Session, root string) (*live, error) {
	l := &live{id: s.ID, root: root, prefix: s.Launch + ":", position: s.Position, ready: s.Ready, quick: s.Quick, attention: s.Attention, pending: map[string]*pendingItem{}, commands: map[int64]store.Command{}, waiters: map[int64]chan error{}, done: make(chan struct{})}
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
			if !item.Done && strings.HasPrefix(item.ID, l.prefix) {
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
