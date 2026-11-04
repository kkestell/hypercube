// dev-seed prepares the preview workspace before the dev app accepts browsers.
package main

import (
	"errors"
	"log"
	"os"

	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: dev-seed <repo>")
	}
	if err := seed(os.Args[1]); err != nil {
		log.Fatal(err)
	}
}

func seed(repo string) error {
	db, err := store.Open()
	if err != nil {
		return err
	}
	defer db.Close()
	w, err := session.New(db, "").AddWorkspace(repo)
	if err != nil {
		return err
	}
	return db.Tx(func(tx *store.Tx) error {
		if _, err := tx.Session("dev-preview"); !errors.Is(err, store.ErrNotFound) {
			return err
		}
		// A stopped session makes Files available without starting a model.
		return tx.AddSession(store.Session{
			ID: "dev-preview", Workspace: w.ID, Title: "Preview session",
			Harness: "claude", Model: "haiku", Effort: "low",
			Dir: w.Path, Activity: store.Idle,
		})
	})
}
