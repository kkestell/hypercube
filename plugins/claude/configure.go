package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

// configuration is the flag override awaiting its native response.
type configuration struct {
	id    string
	model string
	reply chan error
}

// configure waits for the response to this flag override, while the stream
// reader continues translating the conversation.
// An empty effort clears the effort override, so the model's default applies.
func (c *claude) configure(selection harness.Selection) (*harness.Selection, error) {
	// Claude accepts any model name here and fails only on the next prompt.
	if _, err := check(c.models, selection); err != nil {
		return nil, err
	}
	var effort any
	if selection.Effort != "" {
		effort = selection.Effort
	}
	c.writing.Lock()
	c.counter++
	id := fmt.Sprintf("configure-%d", c.counter)
	c.writing.Unlock()
	reply := make(chan error, 1)
	c.mu.Lock()
	c.configuring = &configuration{id: id, model: selection.Model, reply: reply}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.configuring = nil
		c.mu.Unlock()
	}()
	if err := c.send(map[string]any{"type": "control_request", "request_id": id,
		"request": map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{
			"model": selection.Model, "effortLevel": effort,
		}}}); err != nil {
		return nil, err
	}
	timer := time.NewTimer(startTimeout)
	defer timer.Stop()
	select {
	case err := <-reply:
		if err != nil {
			return nil, err
		}
		return &selection, nil
	case <-c.proc.Done():
		return nil, c.proc.Err()
	case <-timer.C:
		return nil, errors.New("claude did not answer configure")
	}
}
