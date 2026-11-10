package main

import (
	"context"
	"os"

	"github.com/kkestell/hypercube/internal/harness"
)

// generate sends one prompt to a private OpenCode server started in an empty
// temporary directory and returns the answer. No session runs, so no
// conversation is saved and the Hypercube MCP server, instructions, and
// permissions do not apply. The selection is checked against the models the
// server offers before the prompt is sent.
func generate(ctx context.Context, req harness.GenerateRequest) (string, error) {
	dir, err := os.MkdirTemp("", "hypercube-utility-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	ag, err := launchCatalog(ctx, dir)
	if err != nil {
		return "", err
	}
	defer ag.stop()
	var available []modelInfo
	if err := ag.server.call(ctx, "GET", "/api/model", nil, &available); err != nil {
		return "", err
	}
	m, err := choose(available, harness.Selection{Model: req.Model, Effort: req.Effort})
	if err != nil {
		return "", err
	}
	body := map[string]any{"prompt": req.Prompt, "model": modelRef{ProviderID: m.ProviderID, ID: m.ID, Variant: req.Effort}}
	var out struct {
		Text string `json:"text"`
	}
	if err := ag.server.call(ctx, "POST", "/api/experimental/generate/text", body, &out); err != nil {
		return "", err
	}
	return out.Text, nil
}
