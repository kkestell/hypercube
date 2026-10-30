package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigProfiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "hypercube/config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	valid := `{"orchestrator":{"harness":"claude","model":"opus","effort":"high"},"profiles":[{"name":"fast","description":"Small jobs","harness":"opencode","model":"p/m","effort":"low"}]}`
	for _, source := range []string{valid, strings.Replace(valid, `"opencode"`, `"pi"`, 1), strings.Replace(valid, `"Small jobs"`, `""`, 1), strings.Replace(valid, `"opus"`, `""`, 1), `{`} {
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := LoadConfig()
		if source == valid {
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Profile("fast"); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Profile("missing"); err == nil {
				t.Fatal("missing profile accepted")
			}
		} else if err == nil {
			t.Fatalf("invalid config accepted: %s", source)
		}
	}
}
