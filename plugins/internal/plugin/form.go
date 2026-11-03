package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Field is a property of an elicitation form's schema.
type Field struct {
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Enum        []string `json:"enum"`
	OneOf       []Option `json:"oneOf"`
	Items       struct {
		Enum  []string `json:"enum"`
		AnyOf []Option `json:"anyOf"`
	} `json:"items"`
}

// Option is one of the values a field offers.
type Option struct {
	Const       string `json:"const"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Options returns the values the field offers. An array offers its items'
// options, and any other field offers its own.
func (f Field) Options() []Option {
	var options []Option
	if f.Type == "array" {
		options = append(options, f.Items.AnyOf...)
		for _, v := range f.Items.Enum {
			options = append(options, Option{Const: v})
		}
	} else {
		options = append(options, f.OneOf...)
	}
	for _, v := range f.Enum {
		options = append(options, Option{Const: v})
	}
	return options
}

// ParseForm reads a form's properties in the order the schema lists them.
func ParseForm(properties json.RawMessage) (keys []string, fields map[string]Field, err error) {
	dec := json.NewDecoder(bytes.NewReader(properties))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, nil, errors.New("a form without fields")
	}
	fields = map[string]Field{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key := tok.(string)
		if _, ok := fields[key]; ok {
			return nil, nil, fmt.Errorf("a form that repeats the field %q", key)
		}
		var f Field
		if err := dec.Decode(&f); err != nil {
			return nil, nil, err
		}
		keys = append(keys, key)
		fields[key] = f
	}
	return keys, fields, nil
}

// Notes returns notes for an item's Notes field: nil when no option has one.
func Notes(notes []string) []string {
	for _, note := range notes {
		if note != "" {
			return notes
		}
	}
	return nil
}
