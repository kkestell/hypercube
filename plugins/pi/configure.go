package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

// check reports a state whose model is not the requested one.
func (s sessionState) check(model string) error {
	selected := "no model"
	if s.Model != nil {
		selected = s.Model.Provider + "/" + s.Model.ID
	}
	if selected != model {
		return fmt.Errorf("Pi selected %s instead of requested %s; use a full provider/model ID from list_models", selected, model)
	}
	return nil
}

// levels lists the thinking levels of Pi's current model.
func (ag *agent) levels(ctx context.Context) ([]string, error) {
	var levels struct {
		Levels []string `json:"levels"`
	}
	err := ag.call(ctx, "get_available_thinking_levels", nil, &levels)
	return levels.Levels, err
}

// checkLevel rejects a thinking level the current model lacks, which Pi
// would otherwise lower silently. A model with levels needs one.
func (ag *agent) checkLevel(ctx context.Context, model, level string) error {
	levels, err := ag.levels(ctx)
	if err != nil {
		return err
	}
	switch {
	case level == "" && len(levels) > 0:
		return fmt.Errorf("Pi model %s needs a thinking level; see list_models", model)
	case level != "" && !slices.Contains(levels, level):
		return fmt.Errorf("Pi model %s does not accept thinking level %q; see list_models", model, level)
	}
	return nil
}

// configure selects the model, then the thinking level.
func (ag *agent) configure(selection harness.Selection) (*harness.Selection, error) {
	provider, model, ok := strings.Cut(selection.Model, "/")
	if !ok {
		return nil, fmt.Errorf("model %q is not provider/id; see list_models", selection.Model)
	}
	ctx, cancel := context.WithTimeout(ag.ctx, callTimeout)
	defer cancel()
	if err := ag.call(ctx, "set_model", map[string]any{"provider": provider, "modelId": model}, nil); err != nil {
		return nil, err
	}
	err := ag.checkLevel(ctx, selection.Model, selection.Effort)
	if err == nil && selection.Effort != "" {
		err = ag.call(ctx, "set_thinking_level", map[string]any{"level": selection.Effort}, nil)
	}
	// Read Pi's actual selection even when the effort failed after the model
	// was accepted.
	var state sessionState
	if readErr := ag.call(ctx, "get_state", nil, &state); readErr != nil {
		return nil, errors.Join(err, readErr)
	}
	ag.mu.Lock()
	ag.window = state.Model.ContextWindow
	ag.mu.Unlock()
	return &harness.Selection{Model: state.Model.Provider + "/" + state.Model.ID, Effort: state.ThinkingLevel}, err
}
