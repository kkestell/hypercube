package plugin

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestParseFormKeepsTheSchemaOrder(t *testing.T) {
	keys, fields, err := ParseForm(json.RawMessage(
		`{"b":{"type":"string","title":"B"},"a":{"type":"boolean"},"c":{"type":"number","description":"C"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"b", "a", "c"}; !slices.Equal(keys, want) {
		t.Errorf("keys = %v, want %v", keys, want)
	}
	if fields["b"].Title != "B" || fields["a"].Type != "boolean" || fields["c"].Description != "C" {
		t.Errorf("fields = %+v", fields)
	}
}

func TestParseFormRejectsBadProperties(t *testing.T) {
	for name, properties := range map[string]string{
		"repeated key": `{"a":{"type":"string"},"a":{"type":"string"}}`,
		"an array":     `[]`,
		"a string":     `"a"`,
		"null":         `null`,
		"empty":        ``,
		"bad field":    `{"a":{"type":1}}`,
		"truncated":    `{"a":`,
	} {
		if _, _, err := ParseForm(json.RawMessage(properties)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestFieldOptions(t *testing.T) {
	var field Field
	for _, test := range []struct {
		name, schema string
		want         []Option
	}{
		{"one of then enum",
			`{"type":"string","oneOf":[{"const":"a","title":"A"}],"enum":["b"]}`,
			[]Option{{Const: "a", Title: "A"}, {Const: "b"}}},
		{"items any of then items enum then enum",
			`{"type":"array","items":{"anyOf":[{"const":"a","description":"first"}],"enum":["b"]},"enum":["c"]}`,
			[]Option{{Const: "a", Description: "first"}, {Const: "b"}, {Const: "c"}}},
		{"an array ignores one of",
			`{"type":"array","oneOf":[{"const":"a"}]}`,
			nil},
		{"none", `{"type":"string"}`, nil},
	} {
		field = Field{}
		if err := json.Unmarshal([]byte(test.schema), &field); err != nil {
			t.Fatal(err)
		}
		if got := field.Options(); !slices.Equal(got, test.want) {
			t.Errorf("%s: Options() = %+v, want %+v", test.name, got, test.want)
		}
	}
}

func TestOptionNotes(t *testing.T) {
	for _, test := range []struct {
		name         string
		labels, note []string
		want         string
	}{
		{"notes", []string{"A", "B", "C"}, []string{"first", "", "third"},
			"Pick\n\n- A: first\n- C: third"},
		{"no notes", []string{"A"}, []string{""}, "Pick"},
		{"no options", nil, nil, "Pick"},
	} {
		if got := OptionNotes("Pick", test.labels, test.note); got != test.want {
			t.Errorf("%s: got %q, want %q", test.name, got, test.want)
		}
	}
}
