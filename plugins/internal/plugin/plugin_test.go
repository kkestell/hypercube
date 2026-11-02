package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

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

func TestErrorsNumberTheItems(t *testing.T) {
	var errs Errors
	first, second := errs.Item("one"), errs.Item("two")
	if first.ID != "error-1" || second.ID != "error-2" {
		t.Errorf("IDs = %q, %q", first.ID, second.ID)
	}
	if first.Kind != harness.KindError || first.Text != "one" || !first.Done {
		t.Errorf("item = %+v", first)
	}
}

func TestWriteFileReplacesOnlyOnChange(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	path, err := WriteFile(dir, "index.md", []byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteFile(dir, "index.md", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if after, err := os.Stat(path); err != nil || !os.SameFile(before, after) {
		t.Error("an unchanged file was replaced")
	}
	if _, err := WriteFile(dir, "index.md", []byte("two")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "two" {
		t.Errorf("file = %q, %v", got, err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Errorf("directory holds %v, %v", entries, err)
	}
}
