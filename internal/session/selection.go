package session

import (
	"context"
	"errors"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harnessclient"
	"github.com/kkestell/hypercube/internal/store"
)

// ScanHarnesses asks each plugin about its native harness and keeps the
// answer for the instructions of sessions that start later.
func (r *Runtime) ScanHarnesses(ctx context.Context) ([]harness.Harness, error) {
	list, err := harnessclient.Describe(ctx, r.programs.Plugins)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.harnesses = list
	r.mu.Unlock()
	return list, nil
}

// installedLocked names the installed harnesses of the last scan. The caller
// holds mu.
func (r *Runtime) installedLocked() []string {
	var ids []string
	for _, h := range r.harnesses {
		if h.Installed {
			ids = append(ids, h.ID)
		}
	}
	return ids
}

// Models asks the harness for the models it offers in dir and, with model,
// that model's efforts.
func (r *Runtime) Models(ctx context.Context, harnessID, dir, model string) (harness.Catalog, error) {
	return harnessclient.Models(ctx, r.programs.Plugins, harnessID, harness.ModelsRequest{Dir: dir, Model: model})
}

// Select changes a ready session's model and effort within its harness. The
// plugin checks them with its native harness.
func (r *Runtime) Select(id string, selection harness.Selection) error {
	if strings.TrimSpace(selection.Model) == "" {
		return errors.New("missing model")
	}
	r.mu.Lock()
	l, err := r.running(id)
	var command int64
	if err == nil {
		command, err = r.recordRequest(l, harness.Command{Command: harness.CmdConfigure, Selection: &selection}, nil)
	}
	r.mu.Unlock()
	if err != nil {
		return err
	}
	return r.request(l, command)
}

// childSettings resolves a child's settings from its parent's current ones
// and the requested overrides. Another harness needs its own model, and
// another model starts with its harness's default effort unless one is given.
func childSettings(parent store.Session, o harness.Settings) (harness.Settings, error) {
	s := harness.Settings{Harness: parent.Harness, Model: parent.Model, Effort: parent.Effort}
	if o.Harness != "" && o.Harness != parent.Harness {
		if o.Model == "" {
			return s, errors.New("a child with another harness needs a model; see list_models")
		}
		return o, nil
	}
	if o.Model != "" && o.Model != parent.Model {
		s.Model, s.Effort = o.Model, ""
	}
	if o.Effort != "" {
		s.Effort = o.Effort
	}
	return s, nil
}
