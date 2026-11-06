package session

import (
	"errors"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// SelectProfile changes a ready session's model and effort within its harness.
func (r *Runtime) SelectProfile(id, name string) error {
	cfg, err := harness.LoadConfig()
	if err != nil {
		return err
	}
	profile, err := cfg.Profile(name)
	if err != nil {
		return err
	}
	r.mu.Lock()
	l, err := r.running(id)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	err = r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		if s.Harness != profile.Harness {
			return errors.New("a session cannot change harness")
		}
		return nil
	})
	var command int64
	if err == nil {
		command, err = r.recordRequest(l, harness.Command{Command: harness.CmdConfigure,
			Selection: &harness.Selection{Model: profile.Model, Effort: profile.Effort}}, nil)
	}
	r.mu.Unlock()
	if err != nil {
		return err
	}
	return r.request(l, command)
}
