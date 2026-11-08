package harness

import (
	"strings"
	"testing"
)

func TestDecodeChecksEvents(t *testing.T) {
	for name, tc := range map[string]struct {
		line string
		want string
	}{
		"idle with a final item":  {line: `{"event":{"state":"idle","final":["a"]}}`},
		"final without idle":      {line: `{"event":{"state":"working","final":["a"]}}`, want: "without the idle state"},
		"final without a state":   {line: `{"event":{"final":["a"]}}`, want: "without the idle state"},
		"empty final ID":          {line: `{"event":{"state":"idle","final":[""]}}`, want: "without an ID"},
		"state item from plugin":  {line: `{"event":{"items":[{"id":"a","kind":"state"}]}}`, want: "unknown kind"},
		"approval with a grant":   {line: `{"event":{"items":[{"id":"a","kind":"attention","choices":["Allow"],"grant":"Bash"}]}}`},
		"grant without choices":   {line: `{"event":{"items":[{"id":"a","kind":"attention","grant":"Bash"}]}}`, want: "grant"},
		"limit":                   {line: `{"event":{"limits":[{"minutes":300,"used":42,"at":1}]}}`},
		"limit without a window":  {line: `{"event":{"limits":[{"minutes":0,"used":42,"at":1}]}}`, want: "limit"},
		"limit with negative use": {line: `{"event":{"limits":[{"minutes":300,"used":-1,"at":1}]}}`, want: "limit"},
		"limit without a time":    {line: `{"event":{"limits":[{"minutes":300,"used":42}]}}`, want: "limit"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeRecord([]byte(tc.line))
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}
