package session

import (
	"errors"
	"regexp"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// running returns the session's launch if it is ready. The caller holds mu.
func (r *Runtime) running(id string) (*live, error) {
	l := r.live[id]
	if l == nil || l.ready == 0 || l.stopping || r.closing {
		return nil, errors.New("the session is not running")
	}
	return l, nil
}

// checkPrompt refuses a prompt with no text and no image, and bad images.
func checkPrompt(text string, images []harness.Image) error {
	if strings.TrimSpace(text) == "" && len(images) == 0 {
		return errors.New("a prompt needs text or an image")
	}
	return harness.CheckImages(images)
}

// Input sends the user's prompt, text and images, to the agent at once, even
// while it works. A parked session wakes first.
func (r *Runtime) Input(id, text string, images ...harness.Image) error {
	return r.input(id, "", text, images...)
}

var submissionID = regexp.MustCompile(`^submission-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// CheckSubmissionID confines browser correlation IDs to their own namespace.
// Empty IDs preserve input from clients that do not supply correlation.
func CheckSubmissionID(id string) error {
	if id != "" && !submissionID.MatchString(id) {
		return errors.New("invalid submission ID")
	}
	return nil
}

// InputSubmission correlates an HTTP prompt with its provisional browser row.
// It does not change when the plugin acknowledges input or make retries unique.
func (r *Runtime) InputSubmission(id, submission, text string, images ...harness.Image) error {
	if err := CheckSubmissionID(submission); err != nil {
		return err
	}
	return r.input(id, submission, text, images...)
}

func (r *Runtime) input(id, submission, text string, images ...harness.Image) error {
	if err := checkPrompt(text, images); err != nil {
		return err
	}
	l, err := r.wake(id)
	if err != nil {
		return err
	}
	if err := r.awaitStart(l); err != nil {
		return err
	}
	r.mu.Lock()
	l, err = r.running(id)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	item := &harness.Item{ID: l.prefix + "host-" + store.NewID(), Kind: harness.KindUser, Text: text, Images: images, Done: true}
	if submission != "" {
		item.ID = submission
	}
	command, err := r.recordRequest(l, harness.Command{Command: harness.CmdInput, Text: text, Images: images}, item)
	r.mu.Unlock()
	if err != nil {
		return err
	}
	return r.request(l, command)
}

// Interrupt stops the agent's current turn. The turn stays in the transcript
// and is not sent again.
func (r *Runtime) Interrupt(id string) error {
	r.mu.Lock()
	l, err := r.running(id)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	command, err := r.recordRequest(l, harness.Command{Command: harness.CmdInterrupt}, nil)
	r.mu.Unlock()
	if err != nil {
		return err
	}
	return r.request(l, command)
}

// Answer replies to an attention request with selected choices, typed text,
// or both. A request that has finished cannot be answered. Hypercube answers
// its own offers; the plugin answers the rest.
func (r *Runtime) Answer(id, requestID string, choices []string, text string) error {
	r.mu.Lock()
	choice := ""
	if len(choices) == 1 {
		choice = choices[0]
	}
	if isOffer, err := r.answerOffer(id, requestID, choice); isOffer || err != nil {
		r.mu.Unlock()
		if isOffer && err == nil {
			r.Deliver(id)
		}
		return err
	}
	l, err := r.running(id)
	var p *pendingItem
	if err == nil {
		p = l.pending[requestID]
	}
	if p == nil || p.item.Kind != harness.KindAttention {
		r.mu.Unlock()
		return errors.New("the request is finished")
	}
	if err := harness.CheckAnswer(p.item, choices, text); err != nil {
		r.mu.Unlock()
		return err
	}
	command, err := r.recordRequest(l, harness.Command{Command: harness.CmdAnswer, Request: strings.TrimPrefix(requestID, l.prefix), Choices: choices, Text: text}, nil)
	r.mu.Unlock()
	if err != nil {
		return err
	}
	return r.request(l, command)
}
