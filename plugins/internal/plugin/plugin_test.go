package plugin

import "testing"

func TestMCPTool(t *testing.T) {
	for name, want := range map[string][2]string{
		"mcp__hypercube__send_message": {"hypercube", "send_message"},
		"mcp__srv__a__b":               {"srv", "a__b"},
		"Bash":                         {"", "Bash"},
		"mcp__":                        {"", "mcp__"},
		"mcp__srv":                     {"", "mcp__srv"},
	} {
		if server, tool := MCPTool(name); server != want[0] || tool != want[1] {
			t.Errorf("MCPTool(%q) = %q, %q", name, server, tool)
		}
	}
}
