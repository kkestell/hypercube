package plugin

import (
	"slices"
	"testing"
)

func TestNotes(t *testing.T) {
	for _, test := range []struct {
		name  string
		notes []string
		want  []string
	}{
		{"notes", []string{"first", "", "third"}, []string{"first", "", "third"}},
		{"no notes", []string{"", ""}, nil},
		{"no options", nil, nil},
	} {
		if got := Notes(test.notes); !slices.Equal(got, test.want) {
			t.Errorf("%s: got %q, want %q", test.name, got, test.want)
		}
	}
}
