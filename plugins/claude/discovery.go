package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// nativeModel is one model that Claude Code's initialize response offers.
// Value is what --model and apply_flag_settings accept; ResolvedModel is the
// model an alias such as "opus" stands for.
type nativeModel struct {
	Value                 string   `json:"value"`
	ResolvedModel         string   `json:"resolvedModel"`
	DisplayName           string   `json:"displayName"`
	SupportsEffort        bool     `json:"supportsEffort"`
	SupportedEffortLevels []string `json:"supportedEffortLevels"`
}

// defaultModel is Claude Code's own entry for the model it uses by default.
const defaultModel = "default"

// initialized reads the models from the response to initialize.
func initialized(response json.RawMessage) ([]nativeModel, error) {
	var r struct {
		Models []nativeModel `json:"models"`
	}
	if len(response) > 0 {
		if err := json.Unmarshal(response, &r); err != nil {
			return nil, fmt.Errorf("unreadable initialize response: %w", err)
		}
	}
	if len(r.Models) == 0 {
		return nil, errors.New("Claude Code reported no models; update Claude Code")
	}
	return r.Models, nil
}

// efforts lists the efforts Claude Code accepts for the model.
func (m nativeModel) efforts() []string {
	if !m.SupportsEffort {
		return nil
	}
	return m.SupportedEffortLevels
}

// check finds the selected model, by its value or the model it resolves to,
// and checks the effort against it. An empty effort leaves Claude's default.
func check(models []nativeModel, s harness.Selection) (nativeModel, error) {
	i := slices.IndexFunc(models, func(m nativeModel) bool { return m.Value == s.Model || m.ResolvedModel == s.Model })
	if i < 0 {
		return nativeModel{}, fmt.Errorf("Claude Code does not offer model %q; see list_models", s.Model)
	}
	m := models[i]
	if s.Effort != "" && !slices.Contains(m.efforts(), s.Effort) {
		return m, fmt.Errorf("Claude Code model %s does not accept effort %q; see list_models", s.Model, s.Effort)
	}
	return m, nil
}

// models starts claude without a prompt or Hypercube's tools, reads the
// models its initialize response offers in the directory, and stops it.
func models(ctx context.Context, req harness.ModelsRequest) (harness.Catalog, error) {
	cmd := exec.Command("claude", "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose")
	cmd.Dir = req.Dir
	p, err := plugin.StartProcess("Claude Code", cmd)
	if err != nil {
		return harness.Catalog{}, err
	}
	defer p.Stop()
	c := &claude{proc: p, stdin: p.Stdin, emit: func(harness.Event) {},
		blocks: map[string]int{}, open: map[string]harness.Item{}, tools: map[string]harness.Item{}}
	ready := make(chan error, 1)
	go c.read(ready)
	if err := c.initialize(ctx, ready); err != nil {
		return harness.Catalog{}, err
	}
	var catalog harness.Catalog
	for _, m := range c.models {
		model := harness.Model{ID: m.Value, Label: m.DisplayName}
		if m.ResolvedModel != m.Value {
			model.Resolved = m.ResolvedModel
		}
		catalog.Models = append(catalog.Models, model)
		if m.Value == defaultModel {
			catalog.Default = defaultModel
		}
	}
	if req.Model != "" {
		m, err := check(c.models, harness.Selection{Model: req.Model})
		if err != nil {
			return harness.Catalog{}, err
		}
		catalog.Efforts = m.efforts()
	}
	return catalog, nil
}
