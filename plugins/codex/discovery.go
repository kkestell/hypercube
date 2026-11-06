package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/kkestell/hypercube/internal/harness"
)

// nativeModel is one entry of Codex's model/list.
type nativeModel struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	DisplayName string `json:"displayName"`
	IsDefault   bool   `json:"isDefault"`
	Efforts     []struct {
		Effort string `json:"reasoningEffort"`
	} `json:"supportedReasoningEfforts"`
	DefaultEffort string `json:"defaultReasoningEffort"`
}

func (m nativeModel) efforts() []string {
	var efforts []string
	for _, e := range m.Efforts {
		efforts = append(efforts, e.Effort)
	}
	return efforts
}

// initialize opens the connection. Experimental methods include the
// questions Codex asks the user.
func (ag *agent) initialize(ctx context.Context) error {
	if err := ag.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "hypercube", "version": "1"},
		"capabilities": map[string]any{"experimentalApi": true},
	}, nil); err != nil {
		return fmt.Errorf("incompatible Codex app-server; update Codex: %w", err)
	}
	return ag.rpc.Notify("initialized", map[string]any{})
}

// listModels reads every page of model/list, including models Codex hides
// from its own picker, which a session may still name.
func (ag *agent) listModels(ctx context.Context) ([]nativeModel, error) {
	var models []nativeModel
	params := map[string]any{"includeHidden": true}
	for {
		var page struct {
			Data       []nativeModel `json:"data"`
			NextCursor *string       `json:"nextCursor"`
		}
		if err := ag.call(ctx, "model/list", params, &page); err != nil {
			return nil, fmt.Errorf("listing Codex models; update Codex: %w", err)
		}
		models = append(models, page.Data...)
		if page.NextCursor == nil || *page.NextCursor == "" {
			return models, nil
		}
		params["cursor"] = *page.NextCursor
	}
}

// resolve checks a selection against model/list. An empty effort becomes the
// model's default effort.
func (ag *agent) resolve(ctx context.Context, s harness.Selection) (harness.Selection, error) {
	models, err := ag.listModels(ctx)
	if err != nil {
		return s, err
	}
	m, err := find(models, s.Model)
	if err != nil {
		return s, err
	}
	if s.Effort == "" {
		s.Effort = m.DefaultEffort
	} else if !slices.Contains(m.efforts(), s.Effort) {
		return s, fmt.Errorf("Codex model %s does not accept effort %q; see list_models", s.Model, s.Effort)
	}
	return s, nil
}

func find(models []nativeModel, id string) (nativeModel, error) {
	i := slices.IndexFunc(models, func(m nativeModel) bool { return m.ID == id || m.Model == id })
	if i < 0 {
		return nativeModel{}, fmt.Errorf("Codex does not offer model %q; see list_models", id)
	}
	return models[i], nil
}

// models lists every model Codex accepts in the directory, including those
// it hides from its own picker, without starting a thread.
func models(ctx context.Context, req harness.ModelsRequest) (harness.Catalog, error) {
	ag, err := launch(req.Dir, func(harness.Event) {})
	if err != nil {
		return harness.Catalog{}, err
	}
	defer ag.stop()
	if err := ag.initialize(ctx); err != nil {
		return harness.Catalog{}, err
	}
	list, err := ag.listModels(ctx)
	if err != nil {
		return harness.Catalog{}, err
	}
	var c harness.Catalog
	for _, m := range list {
		c.Models = append(c.Models, harness.Model{ID: m.ID, Label: m.DisplayName})
		if m.IsDefault {
			c.Default = m.ID
		}
	}
	if req.Model != "" {
		m, err := find(list, req.Model)
		if err != nil {
			return harness.Catalog{}, err
		}
		c.Efforts, c.DefaultEffort = m.efforts(), m.DefaultEffort
	}
	return c, nil
}
