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
	worker := `{"name":"fast","role":"work","description":"Small jobs","harness":"two","model":"p/m","effort":"low"}`
	valid := `{"profiles":[` + first + `,` + worker + `,` + second + `]}`
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
	if got := c.Workers(); len(got) != 1 || got[0].Name != "fast" {
		t.Fatalf("workers = %+v", got)
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
	if p, err := c.Worker("fast"); err != nil || p.Name != "fast" {
		t.Errorf("Worker(fast) = %+v, %v", p, err)
	}
	for _, name := range []string{"first", "missing", ""} {
		if _, err := c.Worker(name); err == nil {
			t.Errorf("Worker(%q) accepted", name)
		}
	}
	for _, role := range []string{RolePlan, RoleWork, RoleReview} {
		if _, err := load(`{"profiles":[` + first + `,` + strings.Replace(worker, `"work"`, `"`+role+`"`, 1) + `]}`); err != nil {
			t.Errorf("one lead and a %s worker: %v", role, err)
		}
	}
	for name, source := range map[string]string{
		"missing role":           strings.Replace(valid, `"role":"work",`, "", 1),
		"unknown role":           strings.Replace(valid, `"work"`, `"other"`, 1),
		"no lead":                `{"profiles":[` + worker + `]}`,
		"no worker":              `{"profiles":[` + first + `]}`,
		"duplicate across roles": strings.Replace(valid, `"name":"fast"`, `"name":"first"`, 1),
		"old lead object":        `{"lead":{"harness":"one","model":"opus","effort":"high"},"profiles":[` + worker + `]}`,
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
	// All worker roles retain their file order, even with leads between them.
	c = Config{Profiles: []Profile{{Name: "plan", Role: RolePlan}, {Name: "lead", Role: RoleLead}, {Name: "work", Role: RoleWork}, {Name: "review", Role: RoleReview}}}
	var names []string
	for _, p := range c.Workers() {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"plan", "work", "review"}) {
		t.Errorf("worker order = %v", names)
	}
}
