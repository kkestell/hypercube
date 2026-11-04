package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/kkestell/hypercube/internal/harness"
)

// configOption is a session setting. A setting with a list of values has the
// type "select".
type configOption struct {
	CurrentValue string        `json:"currentValue"`
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	Options      []configValue `json:"options"`
}

// configValue is one value of a setting, or a group of values.
type configValue struct {
	Value   string        `json:"value"`
	Options []configValue `json:"options"`
}

func (ag *agent) configure(selection harness.Selection) (*harness.Selection, error) {
	ctx, cancel := context.WithTimeout(ag.ctx, startTimeout)
	defer cancel()
	return ag.selectSettings(ctx, selection)
}

// selectSettings uses the options returned after the model change: selecting
// a model can change both the available efforts and the current effort.
func (ag *agent) selectSettings(ctx context.Context, selection harness.Selection) (*harness.Selection, error) {
	options, err := ag.choose(ctx, ag.options, "model", selection.Model)
	if err != nil {
		return nil, err
	}
	ag.options = options
	selected := &harness.Selection{Model: selection.Model, Effort: "default"}
	effort := findOption(options, "effort")
	if effort != nil {
		selected.Effort = effort.CurrentValue
	}
	if effort != nil || selection.Effort != "default" {
		options, err = ag.choose(ctx, options, "effort", selection.Effort)
		if err != nil {
			return selected, fmt.Errorf("model %s: %w", selection.Model, err)
		}
		ag.options = options
		selected.Effort = selection.Effort
	}
	return selected, nil
}

// findOption returns the session setting with a list of values that has the
// given ID.
func findOption(options []configOption, id string) *configOption {
	for i, o := range options {
		if o.Type == "select" && o.ID == id {
			return &options[i]
		}
	}
	return nil
}

// values lists a setting's values, whether listed directly or in groups.
func values(o *configOption) []string {
	var values []string
	for _, v := range o.Options {
		if v.Value != "" {
			values = append(values, v.Value)
		}
		for _, grouped := range v.Options {
			values = append(values, grouped.Value)
		}
	}
	return values
}

// choose sets a session option to a value OpenCode offers and returns the
// options that follow, which depend on the model.
func (ag *agent) choose(ctx context.Context, options []configOption, id, value string) ([]configOption, error) {
	o := findOption(options, id)
	if o == nil {
		return nil, fmt.Errorf("OpenCode offers no %s setting", id)
	}
	if !slices.Contains(values(o), value) {
		hint := ""
		if id == "model" {
			hint = "; use a provider/model ID from `opencode models`"
		}
		return nil, fmt.Errorf("OpenCode does not offer %s %q%s", id, value, hint)
	}
	var result sessionResult
	err := ag.call(ctx, "session/set_config_option", map[string]any{"sessionId": ag.session, "configId": id, "value": value}, &result)
	return result.ConfigOptions, err
}
