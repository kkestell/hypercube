package session

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestAddWorkspaceReclassifiesWithoutMoving(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	w, err := f.rt.AddWorkspace(dir)
	if err != nil || !w.NoGit || w.Path != dir {
		t.Fatalf("plain workspace = %+v, %v", w, err)
	}
	var saved store.Workspace
	f.read(func(tx *store.Tx) error {
		var err error
		saved, err = tx.Workspace(w.ID)
		return err
	})
	if got, err := f.rt.AddWorkspace(dir); err != nil || got != saved {
		t.Fatalf("duplicate plain workspace = %+v, %v; want %+v", got, err, saved)
	}
	if err := f.db.Tx(func(tx *store.Tx) error {
		return tx.AddSession(store.Session{ID: "plain-session", Workspace: w.ID, Dir: dir, Activity: store.Exited})
	}); err != nil {
		t.Fatal(err)
	}
	beforeSession := f.storedSession("plain-session")
	f.git(dir, "init", "-q")
	for _, noGit := range []bool{false, true} {
		if noGit {
			// Remove the test repository's marker without touching its contents.
			if err := os.Rename(filepath.Join(dir, ".git"), filepath.Join(dir, "old-git")); err != nil {
				t.Fatal(err)
			}
		}
		want := saved
		want.NoGit = noGit
		got, err := f.rt.AddWorkspace(dir)
		if err != nil || got != want {
			t.Fatalf("re-add (NoGit=%v) = %+v, %v; want %+v", noGit, got, err, want)
		}
		f.read(func(tx *store.Tx) error {
			persisted, err := tx.Workspace(w.ID)
			if persisted != want {
				t.Errorf("persisted workspace = %+v, want %+v", persisted, want)
			}
			return err
		})
		if got := f.storedSession("plain-session"); !reflect.DeepEqual(got, beforeSession) {
			t.Errorf("session changed: %+v, want %+v", got, beforeSession)
		}
	}
}

func TestAddWorkspaceRepairsSavedSubfolderFirst(t *testing.T) {
	f := newFixture(t)
	sub := filepath.Join(f.root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := store.Workspace{ID: "sub", Path: sub, NoGit: true, Created: 123}
	if err := f.db.Tx(func(tx *store.Tx) error {
		if err := tx.AddWorkspace(stale); err != nil {
			return err
		}
		return tx.AddSession(store.Session{ID: "sub-session", Workspace: stale.ID, Dir: sub, Activity: store.Exited})
	}); err != nil {
		t.Fatal(err)
	}
	before := f.storedSession("sub-session")
	var savedRoot store.Workspace
	f.read(func(tx *store.Tx) error {
		var err error
		savedRoot, err = tx.Workspace(f.workspace.ID)
		return err
	})
	stale.NoGit = false
	if got, err := f.rt.AddWorkspace(sub); err != nil || got != stale {
		t.Fatalf("re-add subfolder = %+v, %v; want %+v", got, err, stale)
	}
	f.read(func(tx *store.Tx) error {
		list, err := tx.Workspaces()
		byID := map[string]store.Workspace{}
		for _, w := range list {
			byID[w.ID] = w
		}
		if len(list) != 2 || byID[savedRoot.ID] != savedRoot || byID[stale.ID] != stale {
			t.Errorf("workspaces after repair = %+v", list)
		}
		return err
	})
	if got := f.storedSession("sub-session"); !reflect.DeepEqual(got, before) {
		t.Errorf("session changed: %+v, want %+v", got, before)
	}
	// A newly added subfolder still selects the existing checkout root.
	other := filepath.Join(f.root, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := f.rt.AddWorkspace(other); err != nil || got.ID != f.workspace.ID || got.Path != f.root || got.NoGit {
		t.Errorf("new subfolder = %+v, %v", got, err)
	}
}

func TestAddWorkspaceFailureDoesNotSave(t *testing.T) {
	f := newFixture(t)
	newDir := t.TempDir()
	f.git(newDir, "init", "-q")
	if err := f.db.Tx(func(tx *store.Tx) error { return tx.SetWorkspaceNoGit(f.workspace.ID, true) }); err != nil {
		t.Fatal(err)
	}
	var before []store.Workspace
	f.read(func(tx *store.Tx) error {
		var err error
		before, err = tx.Workspaces()
		return err
	})
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\necho test-git-refusal >&2\nexit 128\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, dir := range []string{newDir, f.root} {
		if _, err := f.rt.AddWorkspace(dir); err == nil || !strings.Contains(err.Error(), "test-git-refusal") {
			t.Errorf("AddWorkspace(%s) = %v, want refusal", dir, err)
		}
		f.read(func(tx *store.Tx) error {
			got, err := tx.Workspaces()
			if !reflect.DeepEqual(got, before) {
				t.Errorf("failed add changed saved data: %+v, want %+v", got, before)
			}
			return err
		})
	}
}
