package session

import (
	"maps"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// refreshMerges measures each of the workspace's worktree sessions against
// the checkout and publishes the results. It runs in a launch's reader, where
// a returned error would end the launch, so a failure to measure a session is
// stored as that session's status instead.
func (r *Runtime) refreshMerges(workspaceID string) {
	// Holding refreshMu across the whole refresh keeps an older refresh from
	// overwriting a newer one.
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	var root string
	var sessions []store.Session
	err := r.db.Read(func(tx *store.Tx) error {
		w, err := tx.Workspace(workspaceID)
		if err != nil {
			return err
		}
		root = w.Path
		sessions, err = tx.Sessions(workspaceID)
		return err
	})
	if err != nil {
		logf("reading worktree sessions: %v", err)
		return
	}
	merges := map[string]workspace.Merge{}
	for _, s := range sessions {
		if s.Base == "" || s.Ended {
			continue
		}
		m, err := workspace.MergeStatus(root, s.Dir, s.Branch, s.Base)
		if err != nil {
			m = workspace.Merge{Branch: s.Branch, Error: err.Error()}
		}
		merges[s.ID] = m
	}
	if len(merges) == 0 {
		return
	}
	r.mu.Lock()
	maps.Copy(r.merges, merges)
	r.mu.Unlock()
	r.feed.publish(Change{})
}

// forgetMerge drops the merge status of a session whose worktree and branch are
// gone. Taking refreshMu lets a refresh that read the session before its
// removal publish first.
func (r *Runtime) forgetMerge(id string) {
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	r.mu.Lock()
	delete(r.merges, id)
	r.mu.Unlock()
	r.feed.publish(Change{})
}

// Merges returns each worktree session's last measured merge status, by
// session ID.
func (r *Runtime) Merges() map[string]workspace.Merge {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maps.Clone(r.merges)
}
