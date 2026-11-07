package main

import (
	"context"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

// input acknowledges native admission, not the completion of an agent turn.
// OpenCode delivers steer input at the next step boundary, after running tools.
func (ag *agent) input(text string, images []harness.Image) error {
	files := make([]map[string]string, 0, len(images))
	for _, image := range images {
		files = append(files, map[string]string{"uri": "data:" + image.MIMEType + ";base64," + image.Data})
	}
	ctx, cancel := context.WithTimeout(ag.ctx, startTimeout)
	defer cancel()
	path := sessionPath(ag.session)
	// Preserve ACP's slash commands rather than turning them into model text.
	if strings.HasPrefix(text, "/") {
		word := strings.Fields(text)[0]
		name := strings.TrimPrefix(word, "/")
		if name == "compact" {
			return ag.server.call(ctx, "POST", path+"/compact", map[string]any{"delivery": "steer"}, nil)
		}
		var commands []struct {
			Name string `json:"name"`
		}
		if err := ag.server.call(ctx, "GET", "/api/command", nil, &commands); err != nil {
			return err
		}
		for _, command := range commands {
			if command.Name == name {
				return ag.server.call(ctx, "POST", path+"/command", map[string]any{"name": name, "text": strings.TrimSpace(text[len(word):]), "files": files, "delivery": "steer"}, nil)
			}
		}
	}
	return ag.server.call(ctx, "POST", path+"/prompt", map[string]any{
		"text": text, "files": files, "delivery": "steer",
	}, nil)
}

func (ag *agent) interrupt() error {
	ctx, cancel := context.WithTimeout(ag.ctx, startTimeout)
	defer cancel()
	return ag.server.call(ctx, "POST", sessionPath(ag.session)+"/interrupt?resume=false", nil, nil)
}
