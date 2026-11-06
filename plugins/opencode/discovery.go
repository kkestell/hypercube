package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

// deleteTimeout bounds deleting the session that discovery opened.
const deleteTimeout = 10 * time.Second

// models opens a session without Hypercube's tools in the directory, reads its
// model setting and, for a requested model, selects it to read its efforts.
// The session gets no prompt and is deleted, also when discovery fails or is
// cancelled; a failed deletion fails discovery.
func models(ctx context.Context, req harness.ModelsRequest) (catalog harness.Catalog, err error) {
	ag, err := launch(harness.StartArgs{Dir: req.Dir}, func(harness.Event) {})
	if err != nil {
		return harness.Catalog{}, err
	}
	defer ag.stop()
	init, err := ag.initialize(ctx)
	if err != nil {
		return harness.Catalog{}, err
	}
	if init.AgentCapabilities.SessionCapabilities.Delete == nil {
		return harness.Catalog{}, errors.New("this OpenCode cannot delete ACP sessions; update OpenCode")
	}
	var opened sessionResult
	if err := ag.call(ctx, "session/new", map[string]any{"cwd": req.Dir, "mcpServers": []any{}}, &opened); err != nil {
		return harness.Catalog{}, err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), deleteTimeout)
		defer cancel()
		if deleteErr := ag.call(ctx, "session/delete", map[string]any{"sessionId": opened.SessionID}, nil); deleteErr != nil {
			catalog, err = harness.Catalog{}, errors.Join(err, fmt.Errorf("deleting OpenCode's discovery session %s: %w", opened.SessionID, deleteErr))
		}
	}()
	ag.session, ag.options = opened.SessionID, opened.ConfigOptions
	model := findOption(ag.options, "model")
	if model == nil {
		return harness.Catalog{}, errors.New("OpenCode offers no model setting")
	}
	c := harness.Catalog{Models: catalogModels(model)}
	if slices.Contains(values(model), model.CurrentValue) {
		c.Default = model.CurrentValue
	}
	if req.Model == "" {
		return c, nil
	}
	options, err := ag.choose(ctx, ag.options, "model", req.Model)
	if err != nil {
		return harness.Catalog{}, err
	}
	if effort := findOption(options, "effort"); effort != nil {
		c.Efforts, c.DefaultEffort = values(effort), effort.CurrentValue
	}
	return c, nil
}

// catalogModels lists a model setting's values. A group names the values'
// provider; otherwise the provider is the ID's first part.
func catalogModels(o *configOption) []harness.Model {
	var models []harness.Model
	add := func(v configValue, provider string) {
		if provider == "" {
			provider, _, _ = strings.Cut(v.Value, "/")
		}
		models = append(models, harness.Model{ID: v.Value, Label: v.Name, Provider: provider})
	}
	for _, v := range o.Options {
		if v.Value != "" {
			add(v, "")
		}
		for _, grouped := range v.Options {
			add(grouped, v.Name)
		}
	}
	return models
}
