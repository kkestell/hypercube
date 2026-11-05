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
	cfg := Config{DefaultProfile: "worker", Profiles: []Profile{
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
		{DefaultProfile: "missing", Profiles: cfg.Profiles},
		{Profiles: []Profile{cfg.Profiles[0], cfg.Profiles[0]}},
		{Profiles: []Profile{{Name: "unfinished"}}},
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

func TestSaveConfigWriteFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// A regular file where the config directory belongs makes saving fail.
	if err := os.WriteFile(filepath.Join(dir, "hypercube"), []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Profiles: []Profile{{Name: "one", Description: "Jobs", Harness: "codex", Model: "sol", Effort: "high"}}}
	if err := SaveConfig(cfg); err == nil {
		t.Fatal("save succeeded without a config directory")
	}
}
