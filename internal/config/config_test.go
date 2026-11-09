package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestSave(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hypercube")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(dir))
	cfg := Config{DefaultSelection: &harness.Settings{Harness: "codex", Model: "sol"}, ChildGuidance: "Use sol for reviews.",
		DefaultWorktree: true, Theme: "dark", TerminalFontSize: 16, Notifications: Notifications{Replies: true}}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
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
		{Theme: "blue", TerminalFontSize: cfg.TerminalFontSize},
		{Theme: cfg.Theme, TerminalFontSize: 15},
		{Theme: cfg.Theme, TerminalFontSize: cfg.TerminalFontSize, DefaultSelection: &harness.Settings{Harness: "codex"}},
	} {
		if err := Save(invalid); err == nil {
			t.Fatalf("saved invalid config: %+v", invalid)
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) {
			t.Fatalf("invalid save changed file: %s, %v", after, err)
		}
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("temporary config left behind: %v, %v", files, err)
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hypercube")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(dir))
	defaults := Config{Theme: "system", TerminalFontSize: 13, Notifications: DefaultNotifications}
	// A fresh installation has no file.
	got, err := Load()
	if err != nil || !reflect.DeepEqual(got, defaults) {
		t.Fatalf("missing config = %+v, %v", got, err)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = Load()
	if err != nil || !reflect.DeepEqual(got, defaults) {
		t.Fatalf("empty config = %+v, %v", got, err)
	}
}

func TestSaveWriteFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// A regular file where the config directory belongs makes saving fail.
	if err := os.WriteFile(filepath.Join(dir, "hypercube"), []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(Config{Theme: "system", TerminalFontSize: 13}); err == nil {
		t.Fatal("save succeeded without a config directory")
	}
}

func TestLoadPreservesConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hypercube")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(dir))
	path := filepath.Join(dir, "config.json")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		source string
		want   Config
		bad    bool
	}{
		"current": {
			source: `{
  "default_selection": {"harness":"opencode","model":"provider/model","effort":"high"},
  "child_model_guidance": "Use the small model for simple tasks.",
  "default_worktree": true, "theme": "dark", "terminal_font_size": 16,
  "notifications": {"requests":false,"replies":true,"desktop":false,"sound":false},
  "custom": {"keep":"untouched"}
}`,
			want: Config{DefaultSelection: &harness.Settings{Harness: "opencode", Model: "provider/model", Effort: "high"},
				ChildGuidance: "Use the small model for simple tasks.", DefaultWorktree: true, Theme: "dark", TerminalFontSize: 16,
				Notifications: Notifications{Replies: true}},
		},
		"partial": {
			source: `{"notifications":{"desktop":false}}`,
			want: Config{Theme: "system", TerminalFontSize: 13,
				Notifications: Notifications{Requests: true, Replies: true, Sound: true}},
		},
		"malformed": {source: `{"theme":`, bad: true},
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.source), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := Load()
			if tc.bad {
				if err == nil {
					t.Error("loaded malformed config")
				}
			} else if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("loaded config = %+v, %v; want %+v", got, err, tc.want)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != tc.source {
				t.Fatalf("loading changed config: %s, %v", data, err)
			}
		})
	}
}
