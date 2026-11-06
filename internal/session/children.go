package session

import (
	"errors"
	"fmt"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// CreateChild launches a child in a new worktree, or directly in dir.
func (r *Runtime) CreateChild(parentID string, options ChildOptions) (store.Session, error) {
	return r.createChild(parentID, options, "")
}

func (r *Runtime) createChild(parentID string, options ChildOptions, proposal string) (store.Session, error) {
	var parent, child store.Session
	r.mu.Lock()
	if r.deleting[parentID] {
		r.mu.Unlock()
		return store.Session{}, errDeleting
	}
	if r.live[parentID] == nil || r.live[parentID].stopping || r.closing {
		r.mu.Unlock()
		return store.Session{}, errors.New("the session is not running")
	}
	var l *live
	err := r.save(func(tx *write) error {
		var err error
		parent, child, err = prepareChild(tx.Tx, parentID, options)
		if err != nil {
			return err
		}
		if err := tx.AddSession(child); err != nil {
			return err
		}
		if proposal != "" {
			item, seq, err := childProposal(tx.Tx, parentID, proposal)
			if err != nil {
				return err
			}
			item.Done, item.Answer = true, child.ID
			if err := tx.putItem(parentID, seq, item); err != nil {
				return err
			}
		}
		if options.Message != "" {
			if err := tx.Queue(store.Message{From: parentID, To: child.ID, Text: options.Message}); err != nil {
				return err
			}
		}
		l, err = r.decide(tx, child, parent.ID, 0)
		return err
	})
	if err == nil {
		r.live[child.ID] = l
	}
	r.mu.Unlock()
	if err != nil {
		return store.Session{}, err
	}
	discard := func(err error) error {
		r.mu.Lock()
		cleanup := r.save(func(tx *write) error {
			if err := tx.DeleteSession(child.ID); err != nil {
				return err
			}
			if proposal != "" {
				return reopenProposal(tx, parentID, proposal)
			}
			return nil
		})
		r.mu.Unlock()
		if cleanup != nil {
			return fmt.Errorf("%w; deleting the child session failed: %v", err, cleanup)
		}
		return err
	}
	if child.Branch != "" {
		if err := workspace.AddWorktree(parent.Dir, child.Dir, child.Branch); err != nil {
			// run never starts, so end the launch here and release anything
			// already stopping it.
			r.end(l, err)
			close(l.done)
			return store.Session{}, discard(err)
		}
	}
	if err := r.launch(l); err != nil {
		if child.Branch != "" {
			if cleanup := workspace.RemoveWorktree(parent.Dir, child.Dir, child.Branch); cleanup != nil {
				return store.Session{}, fmt.Errorf("%w; worktree cleanup failed: %v; child session %s was kept for cleanup", err, cleanup, child.Name)
			}
		}
		return store.Session{}, discard(err)
	}
	return child, nil
}
