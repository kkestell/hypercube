package session

import (
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/terminal"
)

// Workspace owners occupy a separate namespace in the existing terminal set.
func workspaceShellOwner(id string) string { return "w-" + id }

// WorkspaceShell lazily opens a shell in the registered main checkout.
func (r *Runtime) WorkspaceShell(id string, cols, rows int) (*terminal.Shell, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing {
		return nil, errClosing
	}
	if r.workspaceDeleting[id] {
		return nil, errWorkspaceDeleting
	}
	var dir string
	err := r.db.Read(func(tx *store.Tx) error {
		workspace, err := tx.Workspace(id)
		dir = workspace.Path
		return err
	})
	if err != nil {
		return nil, err
	}
	return r.Terminals.Open(workspaceShellOwner(id), dir, cols, rows)
}

// Shell opens the session's shell in its own directory. The shell outlives the
// session's agent; Archive, Delete, EndChild, and Quit end it.
func (r *Runtime) Shell(id string, cols, rows int) (*terminal.Shell, error) {
	// Open only forks the shell's holder; it does not wait for the shell.
	// Keep admission under the lifecycle lock so cleanup sees every shell
	// admitted before its claim and refuses every request after it.
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing {
		return nil, errClosing
	}
	var dir string
	err := r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		if s.Ended {
			return errEnded
		}
		if r.deleting[id] || r.deleting[s.Parent] {
			return errDeleting
		}
		dir = s.Dir
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.Terminals.Open(id, dir, cols, rows)
}

// stopShells ends the root's shell and all its children's shells.
func (r *Runtime) stopShells(id string) {
	var children []store.Session
	if err := r.db.Read(func(tx *store.Tx) error { var err error; children, err = tx.Children(id); return err }); err != nil {
		logf("reading child shells: %v", err)
	}
	r.Terminals.Stop(id)
	for _, c := range children {
		r.Terminals.Stop(c.ID)
	}
}
