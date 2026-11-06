package session

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

var childName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ChildOptions are the settings and assignment of a new child. A proposal
// saves these settings so its Start action uses exactly what the user saw.
type ChildOptions struct {
	Name    string          `json:"name"`
	Title   string          `json:"title"`
	Profile harness.Profile `json:"profile"`
	Dir     string          `json:"dir,omitempty"`
	Message string          `json:"message,omitempty"`
	Fork    bool            `json:"fork,omitempty"`
}

// prepareChild validates the requested context and directory before any files
// or processes are created. Proposals and direct starts share these checks.
func prepareChild(tx *store.Tx, parentID string, options ChildOptions) (store.Session, store.Session, error) {
	name, title, profile, dir := options.Name, options.Title, options.Profile, options.Dir
	if !childName.MatchString(name) {
		return store.Session{}, store.Session{}, errors.New("name must be lowercase letters, digits, and hyphens")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return store.Session{}, store.Session{}, errors.New("a session needs a title")
	}
	parent, err := tx.Session(parentID)
	if err != nil {
		return store.Session{}, store.Session{}, err
	}
	if parent.Parent != "" {
		return store.Session{}, store.Session{}, errors.New("only a top-level session can do this")
	}
	if options.Fork {
		if profile.Harness != parent.Harness {
			return store.Session{}, store.Session{}, errors.New("a fork must use the parent's harness; choose fresh context to change harness")
		}
		if parent.Conversation == "" {
			return store.Session{}, store.Session{}, errors.New("the parent has no saved conversation to fork yet")
		}

	}
	child := store.Session{ID: store.NewID(), Workspace: parent.Workspace, Parent: parent.ID, Name: name, Title: title,
		Harness: profile.Harness, Model: profile.Model, Effort: profile.Effort, Activity: store.Exited}
	if options.Fork {
		child.Fork = parent.Conversation
	}
	w, err := tx.Workspace(parent.Workspace)
	if err != nil {
		return parent, child, err
	}
	children, err := tx.Children(parent.ID)
	if err != nil {
		return parent, child, err
	}
	for _, c := range children {
		if c.Name == name {
			return parent, child, fmt.Errorf("a child session with the name %s already exists; choose another", name)
		}
	}
	switch {
	case dir != "":
		child.Dir, err = workspace.Resolve(parent.Dir, dir)
		if err != nil {
			return parent, child, err
		}
	case w.NoGit:
		return parent, child, errors.New("dir is required: this workspace is not a Git checkout, so child sessions cannot have worktrees")
	default:
		child.Dir, child.Branch = workspace.WorktreeDir(parent.Dir, parent.ID, name), workspace.ChildBranch(parent.ID, name)
	}
	return parent, child, nil
}
