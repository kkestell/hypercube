package session

import (
	"errors"
	"fmt"
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

// Merge asks a top-level worktree session's agent to commit its remaining work
// and merge its branch into target, the branch the user saw. It refuses when
// the checkout is no longer on target, and publishes the current target so the
// browser shows where a merge would go now.
func (r *Runtime) Merge(id, target string) error {
	var s store.Session
	var root string
	err := r.db.Read(func(tx *store.Tx) error {
		var err error
		if s, err = tx.Session(id); err != nil {
			return err
		}
		w, err := tx.Workspace(s.Workspace)
		root = w.Path
		return err
	})
	if err != nil {
		return err
	}
	if s.Ended {
		return errors.New("the session has ended")
	}
	if s.Base == "" {
		return errors.New("only a top-level worktree session can be merged")
	}
	live, err := workspace.Branch(root)
	if err != nil {
		return err
	}
	if live != target {
		r.refreshMerges(s.Workspace)
		return fmt.Errorf("the checkout is now on %s, not %s", live, target)
	}
	return r.input(id, "", fmt.Sprintf("Commit any remaining work, then merge your branch into %s in the checkout at %s, resolving any conflicts, and report the result.", target, root))
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
