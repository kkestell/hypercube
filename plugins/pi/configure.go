package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

type sessionState struct {
	Model *struct {
		Provider      string `json:"provider"`
		ID            string `json:"id"`
		ContextWindow int    `json:"contextWindow"`
	} `json:"model"`
	ThinkingLevel string `json:"thinkingLevel"`
	SessionFile   string `json:"sessionFile"`
	SessionID     string `json:"sessionId"`
}

func (ag *agent) configure(selection harness.Selection) (*harness.Selection, error) {
	provider, model, ok := strings.Cut(selection.Model, "/")
	if !ok {
		return nil, fmt.Errorf("model %q is not provider/id; see pi --list-models", selection.Model)
	}
	ctx, cancel := context.WithTimeout(ag.ctx, callTimeout)
	defer cancel()
	if err := ag.call(ctx, "set_model", map[string]any{"provider": provider, "modelId": model}, nil); err != nil {
		return nil, err
	}
	err := ag.call(ctx, "set_thinking_level", map[string]any{"level": selection.Effort}, nil)
	// Pi clamps unsupported thinking levels. Read its actual selection even
	// when the effort call failed after the model was accepted.
	var state sessionState
	if readErr := ag.call(ctx, "get_state", nil, &state); readErr != nil {
		return nil, errors.Join(err, readErr)
	}
	ag.mu.Lock()
	ag.window = state.Model.ContextWindow
	ag.mu.Unlock()
	return &harness.Selection{Model: state.Model.Provider + "/" + state.Model.ID, Effort: state.ThinkingLevel}, err
}
