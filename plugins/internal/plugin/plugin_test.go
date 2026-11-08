package plugin

import (
	"encoding/json"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestMCPCommand(t *testing.T) {
	for _, tc := range []struct {
		name    string
		start   string
		command string
		args    string
	}{
		{"bridge", `{"mcp":"/bin/hc-mcp","hypercube":"/bin/hc"}`, "/bin/hc-mcp", `[]`},
		{"saved start", `{"hypercube":"/bin/hc"}`, "/bin/hc", `["mcp"]`},
		{"missing", `{}`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var a harness.StartArgs
			if err := json.Unmarshal([]byte(tc.start), &a); err != nil {
				t.Fatal(err)
			}
			command, args, err := MCPCommand(a)
			if tc.command == "" {
				if err == nil {
					t.Fatal("a start without an MCP server succeeded")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			if command != tc.command || string(encoded) != tc.args {
				t.Fatalf("MCP command = %q %s, want %q %s", command, encoded, tc.command, tc.args)
			}
		})
	}
}
