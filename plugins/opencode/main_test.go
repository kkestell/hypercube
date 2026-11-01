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
	run := func(input string) ([]string, map[string]string) {
		t.Helper()
		var out bytes.Buffer
		if err := launch(strings.NewReader(input), &out); err != nil {
			t.Fatal(err)
		}
		var o struct {
			Argv []string
			Env  map[string]string
		}
		if err := json.Unmarshal(out.Bytes(), &o); err != nil {
			t.Fatal(err)
		}
		return o.Argv, o.Env
	}
	argv, env := run(`{"hypercube":"/bin/hc","agent":"a1","model":"p/m","effort":"high","prompt":"go","state_dir":"` + dir + `"}`)
	if !slices.Equal(argv, []string{"opencode", "--standalone", "--auto", "--session", "ses_hypercube_a1", "--prompt", "go"}) {
		t.Fatal(argv)
	}
	if config := env["OPENCODE_CONFIG_CONTENT"]; !strings.Contains(config, `"model":"p/m#high"`) || !strings.Contains(config, `"command":["/bin/hc","mcp"]`) {
		t.Error(config)
	}
	plugin, err := os.ReadFile(filepath.Join(dir, "opencode-plugin", "index.js"))
	if err != nil || !strings.Contains(string(plugin), `const exe = "/bin/hc"`) {
		t.Fatal(err, string(plugin))
	}
	if argv, env := run(`{"hypercube":"/bin/hc","agent":"a1","conversation":"ses_saved","state_dir":"` + dir + `"}`); !slices.Equal(argv, []string{"opencode", "--standalone", "--auto", "--session", "ses_saved"}) || env["HYPERCUBE_SESSION_ID"] != "ses_saved" {
		t.Fatal(argv, env)
	}
}
