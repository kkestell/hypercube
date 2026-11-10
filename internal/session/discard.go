package session

import (
	"errors"
	"fmt"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// Discard stops a top-level worktree session's family and returns its
// worktree and its children's worktrees to the commit it started from. Their
// uncommitted and untracked files and the commits made since are deleted. The
// family stays active.
func (r *Runtime) Discard(id string) error {
	s, root, children, err := r.claimRemoval(id)
	if err != nil {
		return err
	}
	defer r.releaseRemoval(id)
	switch {
	case s.Parent != "":
		return errors.New("only a top-level session's changes can be discarded")
	case s.Ended:
		return errors.New("the session is archived")
	case s.Base == "":
		return errors.New("only a worktree session's changes can be discarded")
	}
	r.waitChildCreations(children)
	err = r.db.Read(func(tx *store.Tx) error {
		var err error
		children, err = tx.Children(id)
		return err
	})
	if err != nil {
		return err
	}
	r.mu.Lock()
	lives := r.selectLaunches(func(l *live) bool { return l.root == id })
	r.mu.Unlock()
	r.stopLaunches(lives)
	// Trees already reset stay reset; running the action again finishes the rest.
	for _, tree := range familyWorktrees(root, s, children) {
		if err := workspace.Discard(tree.session.Dir, tree.session.Branch, s.Base); err != nil {
			return fmt.Errorf("%s: %w", treeName(tree.session), err)
		}
	}
	r.refreshMerges(s.Workspace)
	return nil
}
