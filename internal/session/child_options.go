package session

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

var childName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ChildOptions are the settings and assignment of a new child. Settings
// overrides the parent's harness, model, and effort where set. A proposal
// saves the resolved settings so its Start action uses exactly what the user
// saw.
type ChildOptions struct {
	Name     string           `json:"name"`
	Title    string           `json:"title"`
	Settings harness.Settings `json:"settings"`
	Dir      string           `json:"dir,omitempty"`
	Message  string           `json:"message,omitempty"`
	Fork     bool             `json:"fork,omitempty"`
}

// prepareChild validates the requested context and directory before any files
// or processes are created. Proposals and direct starts share these checks.
// With exact, the options' settings are complete and used as they are.
func prepareChild(tx *store.Tx, parentID string, options ChildOptions, exact bool) (store.Session, store.Session, error) {
	name, title, dir := options.Name, options.Title, options.Dir
	if !childName.MatchString(name) {
		return store.Session{}, store.Session{}, errors.New("name must be lowercase letters, digits, and hyphens")
	}
	var err error
	if title, err = report.CheckTitle(title); err != nil {
		return store.Session{}, store.Session{}, err
	}
	parent, err := tx.Session(parentID)
	if err != nil {
		return store.Session{}, store.Session{}, err
	}
	if parent.Parent != "" {
		return store.Session{}, store.Session{}, errors.New("only a top-level session can do this")
	}
	settings := options.Settings
	if !exact {
		if settings, err = childSettings(parent, settings); err != nil {
			return store.Session{}, store.Session{}, err
		}
	}
	if err := settings.Check(); err != nil {
		return store.Session{}, store.Session{}, err
	}
	if options.Fork {
		if settings.Harness != parent.Harness {
			return store.Session{}, store.Session{}, errors.New("a fork must use the parent's harness; choose fresh context to change harness")
		}
		if parent.Conversation == "" {
			return store.Session{}, store.Session{}, errors.New("the parent has no saved conversation to fork yet")
		}
	}
	child := store.Session{ID: store.NewID(), Workspace: parent.Workspace, Parent: parent.ID, Name: name, Title: title,
		Harness: settings.Harness, Model: settings.Model, Effort: settings.Effort, Activity: store.Exited}
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
	case options.Fork:
		child.Dir, child.Branch, err = forkDir(parent, name, dir)
	case dir != "":
		child.Dir, err = workspace.Resolve(parent.Dir, dir)
	case w.NoGit:
		err = errors.New("dir is required: this workspace is not a Git checkout, so child sessions cannot have worktrees")
	default:
		child.Dir, child.Branch = workspace.WorktreeDir(parent.Dir, parent.ID, name), workspace.ChildBranch(parent.ID, name)
	}
	return parent, child, err
}

// forkDir places a fork as its parent works: a parent in the original
// checkout shares its folder, uncommitted files included, and a parent in its
// own worktree gives the fork another worktree from the parent's HEAD. A dir
// saved with an older proposal must name that same placement.
func forkDir(parent store.Session, name, dir string) (string, string, error) {
	if parent.Base != "" {
		if dir != "" {
			return "", "", errors.New("a fork of a session in its own worktree gets a new worktree; remove the directory and propose it again")
		}
		return workspace.WorktreeDir(parent.Dir, parent.ID, name), workspace.ChildBranch(parent.ID, name), nil
	}
	if dir != "" {
		resolved, err := workspace.Resolve(parent.Dir, dir)
		if err != nil {
			return "", "", err
		}
		if resolved != parent.Dir {
			return "", "", fmt.Errorf("a fork works in its parent's folder %s; remove the directory and propose it again", parent.Dir)
		}
	}
	return parent.Dir, "", nil
}
