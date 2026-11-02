package harness

import (
	"os"
	"path/filepath"
	"slices"
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
	first := `{"name":"first","role":"lead","description":"First lead","harness":"one","model":"opus","effort":"high"}`
	second := `{"name":"second","role":"lead","description":"Second lead","harness":"one","model":"sol","effort":"high"}`
	child := `{"name":"fast","role":"general","description":"Small jobs","harness":"two","model":"p/m","effort":"low"}`
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
	if got := c.Leads(); len(got) != 2 || got[0].Name != "first" || got[1].Name != "second" {
		t.Fatalf("leads = %+v", got)
	}
	if got := c.ChildProfiles(); len(got) != 1 || got[0].Name != "fast" {
		t.Fatalf("child profiles = %+v", got)
	}
	for name, want := range map[string]string{"": "first", "first": "first", "second": "second"} {
		if p, err := c.Lead(name); err != nil || p.Name != want {
			t.Errorf("Lead(%q) = %+v, %v", name, p, err)
		}
	}
	for _, name := range []string{"fast", "missing"} {
		if _, err := c.Lead(name); err == nil {
			t.Errorf("Lead(%q) accepted", name)
		}
	}
	if p, err := c.ChildProfile("fast"); err != nil || p.Name != "fast" {
		t.Errorf("ChildProfile(fast) = %+v, %v", p, err)
	}
	for _, name := range []string{"first", "missing", ""} {
		if _, err := c.ChildProfile(name); err == nil {
			t.Errorf("ChildProfile(%q) accepted", name)
		}
	}
	if _, err := load(`{"profiles":[` + first + `,` + child + `]}`); err != nil {
		t.Errorf("one lead and one general child profile: %v", err)
	}
	for name, source := range map[string]string{
		"missing role":           strings.Replace(valid, `"role":"general",`, "", 1),
		"unknown role":           strings.Replace(valid, `"general"`, `"other"`, 1),
		"old plan role":          strings.Replace(valid, `"general"`, `"plan"`, 1),
		"old work role":          strings.Replace(valid, `"general"`, `"work"`, 1),
		"old review role":        strings.Replace(valid, `"general"`, `"review"`, 1),
		"no lead":                `{"profiles":[` + child + `]}`,
		"no child":               `{"profiles":[` + first + `]}`,
		"duplicate across roles": strings.Replace(valid, `"name":"fast"`, `"name":"first"`, 1),
		"old lead object":        `{"lead":{"harness":"one","model":"opus","effort":"high"},"profiles":[` + child + `]}`,
		"missing name":           strings.Replace(valid, `"name":"fast"`, `"name":""`, 1),
		"missing description":    strings.Replace(valid, `"Small jobs"`, `""`, 1),
		"missing harness":        strings.Replace(valid, `"two"`, `""`, 1),
		"missing model":          strings.Replace(valid, `"opus"`, `""`, 1),
		"missing effort":         strings.Replace(valid, `"low"`, `""`, 1),
		"missing profiles":       `{}`,
		"broken JSON":            `{`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := load(source); err == nil {
				t.Fatalf("invalid config accepted: %s", source)
			}
		})
	}
	// Children keep their file order, even with leads between them, and leads
	// are not children.
	c = Config{Profiles: []Profile{{Name: "alpha", Role: RoleGeneral}, {Name: "lead", Role: RoleLead}, {Name: "beta", Role: RoleGeneral}, {Name: "gamma", Role: RoleGeneral}}}
	if got := c.Leads(); len(got) != 1 || got[0].Name != "lead" {
		t.Errorf("leads = %+v", got)
	}
	var names []string
	for _, p := range c.ChildProfiles() {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"alpha", "beta", "gamma"}) {
		t.Errorf("child order = %v", names)
	}
}
