package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// The answers to an offer.
const (
	offerNew  = "New session"
	offerHere = "Do it here"
)

// OfferChild asks the user whether a new task gets its own child session, and
// returns the offer's ID. The question stays open until the user answers it;
// the answer reaches the parent as a message.
func (r *Runtime) OfferChild(parentID, title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", errors.New("a session needs a title")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	l, err := r.running(parentID)
	if err != nil {
		return "", err
	}
	item := harness.Item{ID: l.prefix + "host-" + store.NewID(), Kind: harness.KindAttention, Text: title, Choices: []string{offerNew, offerHere}, Offer: true}
	err = r.save(func(tx *write) error {
		parent, err := tx.Session(parentID)
		if err != nil {
			return err
		}
		if parent.Parent != "" {
			return errors.New("only a top-level session can do this")
		}
		return tx.putItem(parentID, r.nextSeq(parentID), item)
	})
	return item.ID, err
}

// answerOffer records the user's choice and queues it for the parent, which
// the caller then delivers. It reports whether id names an offer. The caller
// holds mu.
func (r *Runtime) answerOffer(parentID, id, choice string) (bool, error) {
	isOffer := false
	err := r.save(func(tx *write) error {
		item, seq, found, err := offer(tx.Tx, parentID, id)
		if err != nil || !found {
			return err
		}
		isOffer = true
		parent, err := tx.Session(parentID)
		if err != nil {
			return err
		}
		if parent.Ended {
			return errEnded
		}
		if item.Done {
			return errors.New("the request is finished")
		}
		if !slices.Contains(item.Choices, choice) {
			return fmt.Errorf("choose %s or %s", offerNew, offerHere)
		}
		item.Done, item.Answer = true, choice
		if err := tx.putItem(parentID, seq, item); err != nil {
			return err
		}
		accepted := ""
		if choice == offerNew {
			accepted = item.ID
		}
		return tx.Queue(store.Message{To: parentID, Text: report.OfferAnswer(item.Text, accepted)})
	})
	return isOffer, err
}

func offer(tx *store.Tx, parentID, id string) (harness.Item, int64, bool, error) {
	e, found, err := tx.Entry(parentID, id)
	if err != nil || !found {
		return harness.Item{}, 0, false, err
	}
	var item harness.Item
	if err := json.Unmarshal(e.Data, &item); err != nil {
		return item, 0, false, err
	}
	return item, e.Seq, item.Kind == harness.KindAttention && item.Offer, nil
}

// StartOffer starts the child of an offer the user accepted, with the brief
// and settings the parent supplies. Registration uses the offer up.
func (r *Runtime) StartOffer(parentID, id string, options ChildOptions) (store.Session, error) {
	return r.createChild(parentID, options, approval{id: id, offer: true})
}

// useOffer marks an accepted offer as used by the child's settings.
func useOffer(tx *write, parentID, id string, options ChildOptions) error {
	item, seq, found, err := offer(tx.Tx, parentID, id)
	if err != nil {
		return err
	}
	switch {
	case !found:
		return errors.New("no offer has this offer_id")
	case !item.Done || item.Answer != offerNew:
		return errors.New("the user has not chosen a new session for this offer")
	case len(item.Input) > 0:
		return errors.New("this offer has already started a child session")
	case strings.TrimSpace(options.Title) != item.Text:
		return fmt.Errorf("the title must be the offer's title %q", item.Text)
	case options.Fork:
		return errors.New("an offered child session starts with fresh context")
	case strings.TrimSpace(options.Message) == "":
		return errors.New("an offered child session needs its brief in message")
	}
	if item.Input, err = json.Marshal(options); err != nil {
		return err
	}
	return tx.putItem(parentID, seq, item)
}

// reopenOffer lets an accepted offer start a child again after a failed start.
func reopenOffer(tx *write, parentID, id string) error {
	item, seq, found, err := offer(tx.Tx, parentID, id)
	if err != nil || !found {
		return err
	}
	item.Input = nil
	return tx.putItem(parentID, seq, item)
}
