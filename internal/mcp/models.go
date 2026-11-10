package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
)

type listModelsArgs struct {
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
	Query   string `json:"query,omitempty"`
}

const listModelsDescription = `List the choices for a child session's harness, model, and effort, as your directory offers them. Without harness or query, list the installed harnesses. With harness, list its models and its default model. Query keeps the models whose ID, label, or provider contains it; without harness, it searches every installed harness and lists the matches by harness. With harness and model, also list that model's efforts; a model with efforts needs one, and a model without efforts takes none. This starts no session and changes nothing.`

// harnessMatches is one harness's models that match a query.
type harnessMatches struct {
	Harness string          `json:"harness"`
	Models  []harness.Model `json:"models,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// listModels asks the harnesses' plugins for their models in dir.
func listModels(ctx context.Context, rt *session.Runtime, dir string, a listModelsArgs) (string, error) {
	q := strings.ToLower(strings.TrimSpace(a.Query))
	var v any
	switch {
	case a.Harness != "":
		c, err := rt.Models(ctx, a.Harness, dir, a.Model)
		if err != nil {
			return "", err
		}
		if q != "" {
			c.Models = matching(c.Models, q)
		}
		v = c
	case a.Model != "":
		return "", errors.New("model needs a harness; pass query alone to find a model's harness")
	default:
		list, err := rt.ScanHarnesses(ctx)
		if err != nil {
			return "", err
		}
		if q == "" {
			v = list
			break
		}
		found := []harnessMatches{}
		for _, h := range list {
			if !h.Installed {
				continue
			}
			c, err := rt.Models(ctx, h.ID, dir, "")
			if err != nil {
				found = append(found, harnessMatches{Harness: h.ID, Error: err.Error()})
			} else if m := matching(c.Models, q); len(m) > 0 {
				found = append(found, harnessMatches{Harness: h.ID, Models: m})
			}
		}
		v = found
	}
	b, err := json.MarshalIndent(v, "", "  ")
	return string(b), err
}

// matching keeps the models whose ID, label, provider, or resolved model
// contains the lowercase query q.
func matching(models []harness.Model, q string) []harness.Model {
	var kept []harness.Model
	for _, m := range models {
		if strings.Contains(strings.ToLower(m.ID+" "+m.Label+" "+m.Provider+" "+m.Resolved), q) {
			kept = append(kept, m)
		}
	}
	return kept
}
