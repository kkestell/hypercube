package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/kkestell/hypercube/internal/harness"
)

// models reads the same native catalog used for configuration. Discovery never
// opens a conversation, submits a prompt, or changes the selected model.
func models(ctx context.Context, req harness.ModelsRequest) (harness.Catalog, error) {
	s, err := launch(ctx, req.Dir)
	if err != nil {
		return harness.Catalog{}, err
	}
	ag := newAgent(func(harness.Event) {})
	ag.server = s
	defer ag.stop()
	if err := ag.subscribe(ctx); err != nil {
		return harness.Catalog{}, err
	}
	if err := ag.awaitCatalog(ctx); err != nil {
		return harness.Catalog{}, err
	}
	var available []modelInfo
	if err := s.call(ctx, "GET", "/api/model", nil, &available); err != nil {
		return harness.Catalog{}, err
	}
	var def *modelInfo
	if err := s.call(ctx, "GET", "/api/model/default", nil, &def); err != nil {
		return harness.Catalog{}, err
	}
	var c harness.Catalog
	for _, model := range available {
		if model.Enabled {
			c.Models = append(c.Models, harness.Model{ID: model.key(), Label: model.ProviderID + "/" + model.Name, Provider: model.ProviderID})
		}
	}
	if def != nil {
		c.Default = def.key()
	}
	if req.Model != "" {
		i := slices.IndexFunc(available, func(m modelInfo) bool { return m.Enabled && m.key() == req.Model })
		if i < 0 {
			return harness.Catalog{}, fmt.Errorf("OpenCode does not offer model %q; see list_models", req.Model)
		}
		c.Efforts = available[i].efforts()
	}
	return c, nil
}
