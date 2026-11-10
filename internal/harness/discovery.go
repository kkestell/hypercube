package harness

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// A plugin answers three one-shot commands besides `serve`. Each
// writes one JSON value to stdout and exits; a failure exits nonzero with the
// reason on stderr.
//
// `describe` reads nothing and writes a Description. It starts no native
// agent.
//
// `models` reads a ModelsRequest from stdin and writes a Catalog: the models
// the native harness offers in the request's directory. With a model, the
// catalog also lists that model's efforts.
//
// `generate` reads a GenerateRequest from stdin, sends Prompt to the model
// once, without tools, project instructions, or a saved native conversation,
// and writes a Generation. A plugin that cannot fails with the reason.

// Settings is a complete harness, model, and effort combination. An empty
// Effort leaves the effort to the harness.
type Settings struct {
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Effort  string `json:"effort"`
}

// Check reports a missing harness or model.
func (s Settings) Check() error {
	switch {
	case strings.TrimSpace(s.Harness) == "":
		return errors.New("missing harness")
	case strings.TrimSpace(s.Model) == "":
		return errors.New("missing model")
	}
	return nil
}

// GenerateRequest is the input of `generate`.
type GenerateRequest struct {
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
	Prompt string `json:"prompt"`
}

// Generation is a plugin's answer to `generate`: the model's plain text.
type Generation struct {
	Text string `json:"text"`
}

// Description is a plugin's answer to `describe`. Installed reports whether
// the native harness's executable is on the backend's PATH; it does not mean
// the harness is signed in.
type Description struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
}

// ModelsRequest asks for the models offered in Dir, an absolute directory,
// and, with Model, that model's efforts.
type ModelsRequest struct {
	Dir   string `json:"dir"`
	Model string `json:"model,omitempty"`
}

// Model is one model a harness offers. ID is the native ID a session passes
// back. Resolved is the model an alias stands for, when the harness reports
// it.
type Model struct {
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
	Provider string `json:"provider,omitempty"`
	Resolved string `json:"resolved,omitempty"`
}

// Catalog is a plugin's answer to `models`. Default is the harness's default
// model when it reports one. For a requested Model, Efforts lists its efforts
// in native order, empty when the model has no effort setting.
type Catalog struct {
	Models  []Model  `json:"models"`
	Default string   `json:"default,omitempty"`
	Model   string   `json:"model,omitempty"`
	Efforts []string `json:"efforts,omitempty"`
}

// Check validates a catalog received from a plugin for the request.
func (c Catalog) Check(req ModelsRequest) error {
	ids := map[string]bool{}
	for _, m := range c.Models {
		switch {
		case strings.TrimSpace(m.ID) == "":
			return errors.New("a model has no ID")
		case ids[m.ID]:
			return fmt.Errorf("model %q is listed twice", m.ID)
		}
		ids[m.ID] = true
	}
	if c.Default != "" && !ids[c.Default] {
		return fmt.Errorf("default model %q is not listed", c.Default)
	}
	if c.Model != req.Model {
		return fmt.Errorf("the catalog is for model %q, not %q", c.Model, req.Model)
	}
	if req.Model == "" && (len(c.Efforts) > 0) {
		return errors.New("efforts without a requested model")
	}
	for i, e := range c.Efforts {
		if strings.TrimSpace(e) == "" || slices.Contains(c.Efforts[:i], e) {
			return fmt.Errorf("model %q has an empty or repeated effort", req.Model)
		}
	}
	return nil
}

// Harness is an installed plugin and what it said about its native harness.
// Error is why the plugin could not describe it.
type Harness struct {
	ID string `json:"id"`
	Description
	Error string `json:"error,omitempty"`
}
