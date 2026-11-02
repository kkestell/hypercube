package session

import (
	"errors"
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
// while it works. It also releases the session's queued messages.
func (r *Runtime) Input(id, text string, images ...harness.Image) error {
	if err := checkPrompt(text, images); err != nil {
		return err
	}
	r.mu.Lock()
	l, err := r.running(id)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	if err := r.recordActivity(l.root); err != nil {
		r.mu.Unlock()
		return err
	}
	item := &harness.Item{ID: l.prefix + "host-" + store.NewID(), Kind: harness.KindUser, Text: text, Images: images, Done: true}
	command, err := r.recordRequest(l, harness.Command{Command: harness.CmdInput, Text: text, Images: images}, item, true)
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
	if err := r.recordActivity(l.root); err != nil {
		r.mu.Unlock()
		return err
	}
	command, err := r.recordRequest(l, harness.Command{Command: harness.CmdInterrupt}, nil, false)
	r.mu.Unlock()
	if err != nil {
		return err
	}
	return r.request(l, command)
}

// Answer replies to an attention request with one of its choices, typed text,
// or both. A request that has finished cannot be answered.
func (r *Runtime) Answer(id, requestID, choice, text string) error {
	r.mu.Lock()
	l := r.live[id]
	if l == nil || l.ready == 0 || l.stopping || r.closing {
		r.mu.Unlock()
		return errors.New("the request is finished")
	}
	p := l.pending[requestID]
	if p == nil || p.item.Kind != harness.KindAttention {
		r.mu.Unlock()
		return errors.New("the request is finished")
	}
	if err := r.recordActivity(l.root); err != nil {
		r.mu.Unlock()
		return err
	}
	command, err := r.recordRequest(l, harness.Command{Command: harness.CmdAnswer, Request: strings.TrimPrefix(requestID, l.prefix), Choice: choice, Text: text}, nil, false)
	r.mu.Unlock()
	if err != nil {
		return err
	}
	return r.request(l, command)
}
