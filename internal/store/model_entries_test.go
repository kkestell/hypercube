package store

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestModelEntryCheck(t *testing.T) {
	for _, test := range []struct {
		name  string
		entry ModelEntry
		valid bool
	}{
		{"empty harness", ModelEntry{Model: "m", Title: "M"}, false},
		{"empty model", ModelEntry{Harness: "h", Title: "M"}, false},
		{"empty title", ModelEntry{Harness: "h", Model: "m"}, false},
		{"blank title", ModelEntry{Harness: "h", Model: "m", Title: " "}, false},
		{"blank level", ModelEntry{Harness: "h", Model: "m", Title: "M", Efforts: []string{" "}}, false},
		{"repeated level", ModelEntry{Harness: "h", Model: "m", Title: "M", Efforts: []string{"low", " low "}}, false},
		{"missing default", ModelEntry{Harness: "h", Model: "m", Title: "M", Efforts: []string{"low", "high"}}, false},
		{"unlisted default", ModelEntry{Harness: "h", Model: "m", Title: "M", Efforts: []string{"low", "high"}, DefaultEffort: "max"}, false},
		{"no levels with default", ModelEntry{Harness: "h", Model: "m", Title: "M", DefaultEffort: "high"}, false},
		{"one level", ModelEntry{Harness: " h ", Model: " m ", Title: " M ", Efforts: []string{" low "}}, true},
		{"no levels", ModelEntry{Harness: "h", Model: "m", Title: "M"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.entry.Check()
			if (err == nil) != test.valid {
				t.Fatalf("Check() = %v", err)
			}
			if test.valid && (test.entry.Harness != "h" || test.entry.Model != "m" || test.entry.Title != "M" || test.entry.Efforts == nil) {
				t.Fatalf("not normalized: %+v", test.entry)
			}
			if test.name == "one level" && test.entry.DefaultEffort != "low" {
				t.Fatalf("default = %q", test.entry.DefaultEffort)
			}
		})
	}
}

func TestModelEntries(t *testing.T) {
	db := open(t)
	tx(t, db, func(tx *Tx) error {
		list, err := tx.ModelEntries()
		if err != nil {
			return err
		}
		data, _ := json.Marshal(list)
		if string(data) != "[]" {
			t.Fatalf("empty = %s", data)
		}
		for _, e := range []ModelEntry{{Harness: "z", Model: "m", Title: "M"}, {Harness: "a", Model: "z", Title: "Z"}, {Harness: "a", Model: "a", Title: "A", Efforts: []string{"low", "high"}, DefaultEffort: "high"}} {
			if err := tx.SaveModelEntry(e); err != nil {
				return err
			}
		}
		list, err = tx.ModelEntries()
		if err != nil {
			return err
		}
		if len(list) != 3 || list[0].ID == "" || list[0].Model != "a" || list[0].Title != "A" || list[1].Model != "z" || list[2].Harness != "z" || !reflect.DeepEqual(list[0].Efforts, []string{"low", "high"}) {
			t.Fatalf("list = %+v", list)
		}
		e := list[0]
		e.Model = "edited"
		e.Title = "Edited title"
		if err := tx.SaveModelEntry(e); err != nil {
			return err
		}
		list, err = tx.ModelEntries()
		if err != nil {
			return err
		}
		if list[0].Title != "Edited title" {
			t.Fatalf("edited title = %q", list[0].Title)
		}
		if err := tx.SaveModelEntry(ModelEntry{ID: "missing", Harness: "a", Model: "m", Title: "M"}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown = %v", err)
		}
		if err := tx.SaveModelEntry(ModelEntry{Harness: "a", Model: "edited", Title: "Another title"}); !errors.Is(err, ErrDuplicateModel) {
			t.Fatalf("duplicate = %v", err)
		}
		e.Model = "z"
		if err := tx.SaveModelEntry(e); !errors.Is(err, ErrDuplicateModel) {
			t.Fatalf("duplicate edit = %v", err)
		}
		if err := tx.DeleteModelEntry(e.ID); err != nil {
			return err
		}
		list, err = tx.ModelEntries()
		if len(list) != 2 {
			t.Fatalf("after delete = %+v", list)
		}
		return err
	})
}
