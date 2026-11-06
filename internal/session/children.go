package session

import (
	"errors"
	"fmt"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// CreateChild launches a child in a new worktree, or directly in dir.
func (r *Runtime) CreateChild(parentID string, options ChildOptions) (store.Session, error) {
	return r.createChild(parentID, options, approval{})
}

// approval names the user's saved permission for a child, if any: a proposal
// the user started, or an offer the user accepted. Registering the child uses
// it up in the same transaction, and a cleaned-up failed start restores it.
type approval struct {
	id    string
	offer bool
}

func (a approval) use(tx *write, parentID string, options ChildOptions, child store.Session) error {
	switch {
	case a.id == "":
		return nil
	case a.offer:
		return useOffer(tx, parentID, a.id, options)
	default:
		return useProposal(tx, parentID, a.id, child.ID)
	}
}

func (a approval) restore(tx *write, parentID string) error {
	switch {
	case a.id == "":
		return nil
	case a.offer:
		return reopenOffer(tx, parentID, a.id)
	default:
		return reopenProposal(tx, parentID, a.id)
	}
}

func (r *Runtime) createChild(parentID string, options ChildOptions, approved approval) (store.Session, error) {
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
		if err := approved.use(tx, parentID, options, child); err != nil {
			return err
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
			return approved.restore(tx, parentID)
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
