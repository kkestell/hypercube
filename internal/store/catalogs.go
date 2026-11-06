package store

import (
	"database/sql"
	"errors"
)

// Catalog returns the saved JSON for a harness's models or a model's efforts.
func (t *Tx) Catalog(harness, dir, model string) (string, error) {
	var catalog string
	err := t.tx.QueryRow(`SELECT catalog FROM catalogs WHERE harness = ? AND dir = ? AND model = ?`, harness, dir, model).Scan(&catalog)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return catalog, err
}

// SaveCatalog replaces the last catalog returned by a harness.
func (t *Tx) SaveCatalog(harness, dir, model, catalog string) error {
	return t.exec(`INSERT INTO catalogs (harness, dir, model, catalog) VALUES (?, ?, ?, ?)
ON CONFLICT (harness, dir, model) DO UPDATE SET catalog = excluded.catalog`, harness, dir, model, catalog)
}
