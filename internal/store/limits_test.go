package store

import (
	"reflect"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func limits(t *testing.T, db *DB) (got []Limit) {
	t.Helper()
	if err := db.Read(func(tx *Tx) (err error) { got, err = tx.Limits(); return }); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestLimitsKeepTheNewestReadingOfEachWindow(t *testing.T) {
	db := open(t)
	if got := limits(t, db); got == nil || len(got) != 0 {
		t.Fatalf("limits of an empty database = %#v", got)
	}
	tx(t, db, func(tx *Tx) error {
		for _, r := range []struct {
			harness string
			limit   harness.Limit
		}{
			{"codex", harness.Limit{Minutes: 10080, Used: 48, Resets: 300, At: 10}},
			{"claude", harness.Limit{Minutes: 10080, Used: 18, Resets: 200, At: 10}},
			{"claude", harness.Limit{Minutes: 300, Used: 40, Resets: 100, At: 10}},
			// A newer reading replaces the window; the other windows stay.
			{"claude", harness.Limit{Minutes: 300, Used: 42, Resets: 110, At: 20}},
			// Readings arrive out of order: an older or equally old one changes nothing.
			{"claude", harness.Limit{Minutes: 300, Used: 41, Resets: 105, At: 15}},
			{"claude", harness.Limit{Minutes: 300, Used: 43, Resets: 120, At: 20}},
		} {
			if err := tx.PutLimit(r.harness, r.limit); err != nil {
				return err
			}
		}
		return nil
	})
	want := []Limit{
		{"claude", harness.Limit{Minutes: 300, Used: 42, Resets: 110, At: 20}},
		{"claude", harness.Limit{Minutes: 10080, Used: 18, Resets: 200, At: 10}},
		{"codex", harness.Limit{Minutes: 10080, Used: 48, Resets: 300, At: 10}},
	}
	if got := limits(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("limits = %+v, want %+v", got, want)
	}
}

func TestOpenAddsLimitsToAnExistingDatabase(t *testing.T) {
	db := open(t)
	if _, err := db.sql.Exec(`DROP TABLE limits`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if got := limits(t, reopen(t)); len(got) != 0 {
		t.Fatalf("limits = %+v", got)
	}
}
