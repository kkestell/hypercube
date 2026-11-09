package session

import (
	"encoding/json"
	"errors"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// ProposeChild records a task without launching an agent or blocking this
// turn, and returns the child's resolved settings, which the proposal saves.
func (r *Runtime) ProposeChild(parentID string, options ChildOptions) (harness.Settings, error) {
	if err := r.checkChildDir(parentID, options); err != nil {
		return harness.Settings{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	l, err := r.running(parentID)
	if err != nil {
		return harness.Settings{}, err
	}
	err = r.save(func(tx *write) error {
		_, child, err := prepareChild(tx.Tx, parentID, options, false)
		if err != nil {
			return err
		}
		options.Settings = harness.Settings{Harness: child.Harness, Model: child.Model, Effort: child.Effort}
		data, err := json.Marshal(options)
		if err != nil {
			return err
		}
		return tx.putItem(parentID, r.nextSeq(parentID), harness.Item{ID: l.prefix + "host-" + store.NewID(), Kind: harness.KindProposal, Text: options.Title, Input: data})
	})
	return options.Settings, err
}

func childProposal(tx *store.Tx, parentID, id string) (harness.Item, int64, error) {
	e, found, err := tx.Entry(parentID, id)
	if err != nil {
		return harness.Item{}, 0, err
	}
	if !found {
		return harness.Item{}, 0, errors.New("the child proposal was not found")
	}
	var item harness.Item
	if err := json.Unmarshal(e.Data, &item); err != nil {
		return item, 0, err
	}
	if item.Kind != harness.KindProposal || item.Done {
		return item, 0, errors.New("the child proposal is closed")
	}
	return item, e.Seq, nil
}

// useProposal closes a started proposal with its child's ID.
func useProposal(tx *write, parentID, id, child string) error {
	item, seq, err := childProposal(tx.Tx, parentID, id)
	if err != nil {
		return err
	}
	item.Done, item.Answer = true, child
	return tx.putItem(parentID, seq, item)
}

func reopenProposal(tx *write, parentID, id string) error {
	e, found, err := tx.Entry(parentID, id)
	if err != nil || !found {
		return err
	}
	var item harness.Item
	if err := json.Unmarshal(e.Data, &item); err != nil {
		return err
	}
	item.Done, item.Answer = false, ""
	return tx.putItem(parentID, e.Seq, item)
}

// StartProposal is a user action. Registration of the child closes the saved
// proposal in the same transaction, so concurrent clicks cannot start it twice.
func (r *Runtime) StartProposal(parentID, id string) (store.Session, error) {
	var options ChildOptions
	err := r.db.Read(func(tx *store.Tx) error {
		item, _, err := childProposal(tx, parentID, id)
		if err != nil {
			return err
		}
		return json.Unmarshal(item.Input, &options)
	})
	if err != nil {
		return store.Session{}, err
	}
	return r.createChild(parentID, options, approval{id: id})
}

func (r *Runtime) DismissProposal(parentID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.save(func(tx *write) error {
		parent, err := tx.Session(parentID)
		if err != nil {
			return err
		}
		if parent.Ended {
			return errEnded
		}
		item, seq, err := childProposal(tx.Tx, parentID, id)
		if err != nil {
			return err
		}
		item.Done, item.Answer = true, "Dismissed"
		return tx.putItem(parentID, seq, item)
	})
}
