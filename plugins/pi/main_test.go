package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLaunch(t *testing.T) {
	dir := t.TempDir()
	run := func(input string) ([]string, map[string]string, error) {
		t.Helper()
		var out bytes.Buffer
		if err := launch(strings.NewReader(input), &out); err != nil {
			return nil, nil, err
		}
		var o struct {
			Argv []string
			Env  map[string]string
		}
		if err := json.Unmarshal(out.Bytes(), &o); err != nil {
			t.Fatal(err)
		}
		return o.Argv, o.Env, nil
	}
	extension := filepath.Join(dir, "pi-extension", "index.js")
	argv, env, err := run(`{"hypercube":"/bin/hc","env":{"HYPERCUBE_AGENT_ID":"a1"},"agent":"a1","model":"openrouter/~x/m","effort":"high","prompt":"-go","state_dir":"` + dir + `"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pi", "--provider", "openrouter", "--model", "~x/m", "-e", extension, "--session-id", "hypercube-a1", "--thinking", "high", "--", "-go"}
	if !slices.Equal(argv, want) {
		t.Fatal(argv)
	}
	if env["HYPERCUBE_MODEL"] != "openrouter/~x/m" || env["HYPERCUBE_MCP"] != `{"args":["mcp"],"command":"/bin/hc","env":{"HYPERCUBE_AGENT_ID":"a1"},"exposure":"direct"}` {
		t.Error(env)
	}
	text, err := os.ReadFile(extension)
	if err != nil || !strings.Contains(string(text), `const exe = "/bin/hc"`) {
		t.Fatal(err, string(text))
	}
	argv, _, err = run(`{"hypercube":"/bin/hc","agent":"a1","model":"p/m","conversation":"saved","state_dir":"` + dir + `"}`)
	if want := []string{"pi", "--provider", "p", "--model", "m", "-e", extension, "--session-id", "saved"}; err != nil || !slices.Equal(argv, want) {
		t.Fatal(err, argv)
	}
	if _, _, err := run(`{"hypercube":"/bin/hc","agent":"a1","model":"m","state_dir":"` + dir + `"}`); err == nil {
		t.Fatal("accepted a model without a provider")
	}
}
