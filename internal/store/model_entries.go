package store

import (
	"encoding/json"
	"slices"

	"github.com/kkestell/hypercube/internal/config"
)

// moveModelEntries moves the model list from the model_entries table, where
// earlier builds saved it, to config.json, then drops the table. The config is
// saved before the drop commits, and rows the config already lists are
// skipped, so an interrupted move neither loses nor repeats an entry.
func (d *DB) moveModelEntries(preferences config.File) error {
	return d.Tx(func(tx *Tx) error {
		var found bool
		if err := tx.tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'model_entries')`).Scan(&found); err != nil || !found {
			return err
		}
		rows, err := tx.tx.Query(`SELECT harness, model, title, efforts, default_effort FROM model_entries ORDER BY harness, model`)
		if err != nil {
			return err
		}
		entries, err := collect(rows, func(row interface{ Scan(...any) error }) (config.ModelEntry, error) {
			var e config.ModelEntry
			var levels string
			if err := row.Scan(&e.Harness, &e.Model, &e.Title, &levels, &e.DefaultEffort); err != nil {
				return e, err
			}
			return e, json.Unmarshal([]byte(levels), &e.Efforts)
		})
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			cfg, err := preferences.Load()
			if err != nil {
				return err
			}
			for _, e := range entries {
				if !slices.ContainsFunc(cfg.Models, func(m config.ModelEntry) bool { return m.Harness == e.Harness && m.Model == e.Model }) {
					cfg.Models = append(cfg.Models, e)
				}
			}
			if err := preferences.Save(cfg); err != nil {
				return err
			}
		}
		return tx.exec(`DROP TABLE model_entries`)
	})
}
