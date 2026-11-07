package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kkestell/hypercube/internal/harness"
)

type modelRef struct {
	ProviderID string `json:"providerID"`
	ID         string `json:"id"`
	Variant    string `json:"variant,omitempty"`
}

func (m modelRef) key() string {
	if m.ProviderID == "" {
		return ""
	}
	return m.ProviderID + "/" + m.ID
}

type modelInfo struct {
	ProviderID string `json:"providerID"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Variants   []struct {
		ID string `json:"id"`
	} `json:"variants"`
	Limit struct {
		Context int `json:"context"`
	} `json:"limit"`
}

// Native catalog reads can precede initial plugin activation. Observe its
// completion rather than polling with a startup delay, as the ACP client did.
func (ag *agent) awaitCatalog(ctx context.Context) error {
	for {
		var plugins []json.RawMessage
		if err := ag.server.call(ctx, "GET", "/api/plugin", nil, &plugins); err != nil {
			return err
		}
		if len(plugins) > 0 {
			return nil
		}
		select {
		case <-ag.catalogChanged:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (m modelInfo) key() string { return m.ProviderID + "/" + m.ID }

func (m modelInfo) efforts() []string {
	if len(m.Variants) == 0 {
		return nil
	}
	var out []string
	for _, variant := range m.Variants {
		out = append(out, variant.ID)
	}
	if !slices.Contains(out, "default") {
		out = append(out, "default")
	}
	return out
}

func (ag *agent) configure(selection harness.Selection) (*harness.Selection, error) {
	ctx, cancel := context.WithTimeout(ag.ctx, startTimeout)
	defer cancel()
	return ag.selectSettings(ctx, selection)
}

func (ag *agent) selectSettings(ctx context.Context, selection harness.Selection) (*harness.Selection, error) {
	var models []modelInfo
	if err := ag.server.call(ctx, "GET", "/api/model", nil, &models); err != nil {
		return nil, err
	}
	i := slices.IndexFunc(models, func(m modelInfo) bool { return m.Enabled && m.key() == selection.Model })
	if i < 0 {
		return nil, fmt.Errorf("OpenCode does not offer model %q; see list_models", selection.Model)
	}
	m := models[i]
	efforts := m.efforts()
	effort := selection.Effort
	if effort == "" {
		effort = "default"
		if selection.Model == ag.selected.Model && slices.Contains(efforts, ag.selected.Effort) {
			effort = ag.selected.Effort
		}
	}
	if len(efforts) == 0 && effort == "default" {
		effort = ""
	}
	if effort != "" && !slices.Contains(efforts, effort) {
		return nil, fmt.Errorf("model %s: OpenCode does not offer effort %q; see list_models", selection.Model, effort)
	}
	ref := modelRef{ProviderID: m.ProviderID, ID: m.ID, Variant: effort}
	if err := ag.server.call(ctx, "POST", sessionPath(ag.session)+"/model", map[string]any{"model": ref}, nil); err != nil {
		return nil, err
	}
	ag.selected = harness.Selection{Model: m.key(), Effort: effort}
	ag.mu.Lock()
	ag.window = m.Limit.Context
	ag.mu.Unlock()
	selected := ag.selected
	return &selected, nil
}
