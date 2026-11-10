package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harnessclient"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
)

// nameSession renames a new session in the background from its first message,
// using the utility model, when the user has chosen one. A session whose
// message has no text, or whose naming fails, keeps its title; failures go to
// the log. The call gets the workspace's launch environment, like an agent.
func (r *Runtime) nameSession(id, workspace, message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	c, err := r.config.Load()
	if err != nil {
		logf("naming session %s: %v", id, err)
		return
	}
	if c.UtilitySelection == nil {
		return
	}
	utility := *c.UtilitySelection
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		return
	}
	env := r.env(workspace)
	ctx, cancel := context.WithTimeout(context.Background(), r.utilityTimeout)
	r.utilityCancels[id] = cancel
	r.utilityCalls.Add(1)
	r.mu.Unlock()
	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.utilityCancels, id)
			r.mu.Unlock()
			cancel()
			r.utilityCalls.Done()
		}()
		answer, err := harnessclient.Generate(ctx, r.programs.Plugins, utility.Harness,
			harness.GenerateRequest{Model: utility.Model, Effort: utility.Effort, Prompt: report.TitlePrompt(message)}, env)
		if err == nil {
			err = r.applyTitle(id, answer)
		}
		if err != nil {
			logf("naming session %s: %v", id, err)
		}
	}()
}

// applyTitle sets the session's title from the model's answer unless the user
// has renamed the session. A session deleted meanwhile is not an error.
func (r *Runtime) applyTitle(id, answer string) error {
	title, err := report.GeneratedTitle(answer)
	if err != nil {
		return err
	}
	err = r.update(id, func(s *store.Session) {
		if !s.Renamed {
			s.Title = title
		}
	})
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("saving the title: %w", err)
	}
	return nil
}

// cancelUtilityCallsLocked cancels the pending utility calls. The caller holds
// mu; a cancel does not call back into the runtime.
func (r *Runtime) cancelUtilityCallsLocked() {
	for _, cancel := range r.utilityCancels {
		cancel()
	}
}
