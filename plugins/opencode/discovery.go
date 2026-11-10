package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/kkestell/hypercube/internal/harness"
)

// launchCatalog starts a server in dir and waits until its native plugins have
// activated, so the model catalog is complete. The caller stops the agent.
func launchCatalog(ctx context.Context, dir string) (*agent, error) {
	s, err := launch(ctx, dir)
	if err != nil {
		return nil, err
	}
	ag := newAgent(func(harness.Event) {})
	ag.server = s
	if err := ag.subscribe(ctx); err != nil {
		ag.stop()
		return nil, err
	}
	if err := ag.awaitCatalog(ctx); err != nil {
		ag.stop()
		return nil, err
	}
	return ag, nil
}

// models reads the same native catalog used for configuration. Discovery never
// opens a conversation, submits a prompt, or changes the selected model.
func models(ctx context.Context, req harness.ModelsRequest) (harness.Catalog, error) {
	ag, err := launchCatalog(ctx, req.Dir)
	if err != nil {
		return harness.Catalog{}, err
	}
	defer ag.stop()
	s := ag.server
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
