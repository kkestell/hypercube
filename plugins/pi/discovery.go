package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

// models starts pi without a saved session or Hypercube's extension, and
// reads its models and, for a requested model, that model's thinking levels.
// A requested model is passed at launch, so Pi's saved defaults are unchanged.
func models(ctx context.Context, req harness.ModelsRequest) (harness.Catalog, error) {
	argv := []string{"--mode", "rpc", "--no-session"}
	if req.Model != "" {
		provider, model, ok := strings.Cut(req.Model, "/")
		if !ok {
			return harness.Catalog{}, fmt.Errorf("model %q is not provider/id", req.Model)
		}
		argv = append(argv, "--provider", provider, "--model", model)
	}
	ag, err := launch(req.Dir, argv, func(harness.Event) {})
	if err != nil {
		return harness.Catalog{}, err
	}
	defer ag.stop()
	var available struct {
		Models []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Provider string `json:"provider"`
		} `json:"models"`
	}
	if err := ag.call(ctx, "get_available_models", nil, &available); err != nil {
		return harness.Catalog{}, err
	}
	var c harness.Catalog
	for _, m := range available.Models {
		c.Models = append(c.Models, harness.Model{ID: m.Provider + "/" + m.ID, Label: m.Name, Provider: m.Provider})
	}
	var state sessionState
	if err := ag.call(ctx, "get_state", nil, &state); err != nil {
		return harness.Catalog{}, err
	}
	if req.Model == "" {
		if state.Model != nil {
			c.Default = state.Model.Provider + "/" + state.Model.ID
		}
		if !slices.ContainsFunc(c.Models, func(m harness.Model) bool { return m.ID == c.Default }) {
			c.Default = ""
		}
		return c, nil
	}
	if err := state.check(req.Model); err != nil {
		return harness.Catalog{}, err
	}
	if c.Efforts, err = ag.levels(ctx); err != nil {
		return harness.Catalog{}, err
	}
	if slices.Contains(c.Efforts, state.ThinkingLevel) {
		c.DefaultEffort = state.ThinkingLevel
	}
	return c, nil
}
