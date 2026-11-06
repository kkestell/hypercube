package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hypercube")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(dir))
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{DefaultProfile: "worker", DefaultWorktree: true, Theme: "dark", TerminalFontSize: 16, Notifications: Notifications{Replies: true}, Profiles: []Profile{
		{Name: "lead", Description: "Large jobs", Harness: "claude", Model: "opus", Effort: "high"},
		{Name: "worker", Description: "Small jobs", Harness: "codex", Model: "sol", Effort: "low"},
	}}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig()
	if err != nil || !reflect.DeepEqual(got, cfg) {
		t.Fatalf("saved config = %+v, %v", got, err)
	}
	path := filepath.Join(dir, "config.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []Config{
		{},
		{DefaultProfile: "missing", Theme: cfg.Theme, TerminalFontSize: cfg.TerminalFontSize, Profiles: cfg.Profiles},
		{Theme: cfg.Theme, TerminalFontSize: cfg.TerminalFontSize, Profiles: []Profile{cfg.Profiles[0], cfg.Profiles[0]}},
		{Theme: cfg.Theme, TerminalFontSize: cfg.TerminalFontSize, Profiles: []Profile{{Name: "unfinished"}}},
		{Theme: "blue", TerminalFontSize: cfg.TerminalFontSize, Profiles: cfg.Profiles},
		{Theme: cfg.Theme, TerminalFontSize: 15, Profiles: cfg.Profiles},
	} {
		if err := SaveConfig(invalid); err == nil {
			t.Fatalf("saved invalid config: %+v", invalid)
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) {
			t.Fatalf("invalid save changed file: %s, %v", after, err)
		}
	}
	// A replacement of the profiles also persists the new explicit default.
	cfg.Profiles = cfg.Profiles[:1]
	cfg.Profiles[0].Model = "sonnet"
	cfg.DefaultProfile = "lead"
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got, err = LoadConfig()
	if err != nil || !reflect.DeepEqual(got, cfg) {
		t.Fatalf("updated config = %+v, %v", got, err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("temporary config left behind: %v, %v", files, err)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hypercube")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(dir))
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A file from before the preferences and default_profile existed.
	profile := `{"name":"one","description":"Jobs","harness":"codex","model":"sol","effort":"high"}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"profiles":[`+profile+`]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig()
	want := Config{DefaultProfile: "one", Theme: "system", TerminalFontSize: 13,
		Notifications: Notifications{Requests: true, Replies: true, MacOS: true, Sound: true},
		Profiles:      []Profile{{Name: "one", Description: "Jobs", Harness: "codex", Model: "sol", Effort: "high"}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded config = %+v, %v", got, err)
	}
}

func TestSaveConfigWriteFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// A regular file where the config directory belongs makes saving fail.
	if err := os.WriteFile(filepath.Join(dir, "hypercube"), []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Theme: "system", TerminalFontSize: 13, Profiles: []Profile{{Name: "one", Description: "Jobs", Harness: "codex", Model: "sol", Effort: "high"}}}
	if err := SaveConfig(cfg); err == nil {
		t.Fatal("save succeeded without a config directory")
	}
}
