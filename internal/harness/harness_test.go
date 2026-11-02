package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "hypercube/config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	first := `{"name":"first","description":"First lead","harness":"one","model":"opus","effort":"high"}`
	second := `{"name":"second","description":"Second lead","harness":"one","model":"sol","effort":"high"}`
	child := `{"name":"fast","description":"Small jobs","harness":"two","model":"p/m","effort":"low"}`
	valid := `{"profiles":[` + first + `,` + child + `,` + second + `]}`
	load := func(source string) (Config, error) {
		t.Helper()
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		return LoadConfig()
	}
	c, err := load(valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "fast", "second"} {
		if p, err := c.Profile(name); err != nil || p.Name != name {
			t.Errorf("Profile(%q) = %+v, %v", name, p, err)
		}
	}
	for _, name := range []string{"", "missing", "First"} {
		if _, err := c.Profile(name); err == nil || err.Error() != fmt.Sprintf("no profile named %q", name) {
			t.Errorf("Profile(%q) error = %v", name, err)
		}
	}
	if _, err := load(`{"profiles":[` + child + `]}`); err != nil {
		t.Errorf("single profile: %v", err)
	}
	if _, err := load(strings.Replace(valid, `"name":"first"`, `"name":"first","role":"lead"`, 1)); err != nil {
		t.Errorf("legacy role field: %v", err)
	}
	for name, source := range map[string]string{
		"duplicate name":      strings.Replace(valid, `"name":"fast"`, `"name":"first"`, 1),
		"empty profiles":      `{"profiles":[]}`,
		"missing name":        strings.Replace(valid, `"name":"fast"`, `"name":""`, 1),
		"missing description": strings.Replace(valid, `"Small jobs"`, `""`, 1),
		"missing harness":     strings.Replace(valid, `"two"`, `""`, 1),
		"missing model":       strings.Replace(valid, `"opus"`, `""`, 1),
		"missing effort":      strings.Replace(valid, `"low"`, `""`, 1),
		"missing profiles":    `{}`,
		"broken JSON":         `{`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := load(source); err == nil {
				t.Fatalf("invalid config accepted: %s", source)
			}
		})
	}
}
