package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ModelEntry lists a harness's native model ID and ordered effort levels.
// Sessions keep their own settings, not a reference to this entry.
type ModelEntry struct {
	ID            string   `json:"id"`
	Harness       string   `json:"harness"`
	Model         string   `json:"model"`
	Efforts       []string `json:"efforts"`
	DefaultEffort string   `json:"default_effort"`
}

// ErrDuplicateModel identifies an already listed harness and model.
var ErrDuplicateModel = errors.New("model already listed")

// Check trims fields and validates the entry without asking a harness.
func (e *ModelEntry) Check() error {
	e.ID = strings.TrimSpace(e.ID)
	e.Harness = strings.TrimSpace(e.Harness)
	e.Model = strings.TrimSpace(e.Model)
	e.DefaultEffort = strings.TrimSpace(e.DefaultEffort)
	if e.Harness == "" || e.Model == "" {
		return errors.New("harness and model are required")
	}
	seen := make(map[string]bool)
	if e.Efforts == nil {
		e.Efforts = []string{}
	}
	for i, level := range e.Efforts {
		level = strings.TrimSpace(level)
		if level == "" {
			return errors.New("effort levels must not be empty")
		}
		if seen[level] {
			return fmt.Errorf("effort level %q repeats", level)
		}
		seen[level], e.Efforts[i] = true, level
	}
	if len(e.Efforts) == 1 {
		e.DefaultEffort = e.Efforts[0]
	}
	if len(e.Efforts) == 0 {
		if e.DefaultEffort != "" {
			return errors.New("a model without effort levels cannot have a default effort")
		}
	} else if !seen[e.DefaultEffort] {
		return errors.New("default effort must be one of the effort levels")
	}
	return nil
}

// ModelEntries lists entries by harness, then model.
func (t *Tx) ModelEntries() ([]ModelEntry, error) {
	rows, err := t.tx.Query(`SELECT id, harness, model, efforts, default_effort FROM model_entries ORDER BY harness, model`)
	if err != nil {
		return nil, err
	}
	list, err := collect(rows, func(row interface{ Scan(...any) error }) (ModelEntry, error) {
		var e ModelEntry
		var levels string
		if err := row.Scan(&e.ID, &e.Harness, &e.Model, &levels, &e.DefaultEffort); err != nil {
			return e, err
		}
		err := json.Unmarshal([]byte(levels), &e.Efforts)
		return e, err
	})
	if list == nil && err == nil {
		list = []ModelEntry{}
	}
	return list, err
}

// SaveModelEntry adds an entry without an ID, or replaces an existing entry.
func (t *Tx) SaveModelEntry(e ModelEntry) error {
	if err := e.Check(); err != nil {
		return err
	}
	if e.ID != "" {
		var found bool
		if err := t.tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM model_entries WHERE id = ?)`, e.ID).Scan(&found); err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
	}
	var duplicate bool
	if err := t.tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM model_entries WHERE harness = ? AND model = ? AND id != ?)`, e.Harness, e.Model, e.ID).Scan(&duplicate); err != nil {
		return err
	}
	if duplicate {
		return fmt.Errorf("%s already lists model %q: %w", e.Harness, e.Model, ErrDuplicateModel)
	}
	if e.ID == "" {
		e.ID = NewID()
	}
	levels, err := json.Marshal(e.Efforts)
	if err != nil {
		return err
	}
	return t.exec(`INSERT INTO model_entries (id, harness, model, efforts, default_effort) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET harness = excluded.harness, model = excluded.model, efforts = excluded.efforts, default_effort = excluded.default_effort`, e.ID, e.Harness, e.Model, string(levels), e.DefaultEffort)
}

// DeleteModelEntry removes an entry without changing session settings.
func (t *Tx) DeleteModelEntry(id string) error {
	return t.exec(`DELETE FROM model_entries WHERE id = ?`, id)
}
