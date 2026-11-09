package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
)

type listModelsArgs struct {
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
	Query   string `json:"query,omitempty"`
}

const listModelsDescription = `List the choices for a child session's harness, model, and effort, as your directory offers them. Without harness, list the installed harnesses. With harness, list its models and its default model; query keeps the models whose ID, label, or provider contains it. With model, also list that model's efforts; a model with efforts needs one, and a model without efforts takes none. This starts no session and changes nothing.`

// listModels asks the harness's plugin for its models in dir.
func listModels(ctx context.Context, rt *session.Runtime, dir string, a listModelsArgs) (string, error) {
	var v any
	if a.Harness == "" {
		list, err := rt.ScanHarnesses(ctx)
		if err != nil {
			return "", err
		}
		v = list
	} else {
		c, err := rt.Models(ctx, a.Harness, dir, a.Model)
		if err != nil {
			return "", err
		}
		if q := strings.ToLower(strings.TrimSpace(a.Query)); q != "" {
			var kept []harness.Model
			for _, m := range c.Models {
				if strings.Contains(strings.ToLower(m.ID+" "+m.Label+" "+m.Provider+" "+m.Resolved), q) {
					kept = append(kept, m)
				}
			}
			c.Models = kept
		}
		v = c
	}
	b, err := json.MarshalIndent(v, "", "  ")
	return string(b), err
}
