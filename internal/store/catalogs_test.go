package store

import (
	"errors"
	"testing"
)

func TestCatalog(t *testing.T) {
	db := open(t)
	if err := db.Read(func(tx *Tx) error { _, err := tx.Catalog("test", "/repo", ""); return err }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing catalog: %v", err)
	}
	for _, data := range []string{`{"models":[]}`, `{"models":[{"id":"m"}]}`} {
		tx(t, db, func(tx *Tx) error { return tx.SaveCatalog("test", "/repo", "", data) })
		if err := db.Read(func(tx *Tx) error {
			got, err := tx.Catalog("test", "/repo", "")
			if err == nil && got != data {
				t.Fatalf("catalog = %s, want %s", got, data)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}
