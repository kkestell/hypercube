package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSaveConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hypercube")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(dir))
	cfg := Config{DefaultSelection: &Settings{Harness: "codex", Model: "sol"}, ChildGuidance: "Use sol for reviews.",
		DefaultWorktree: true, Theme: "dark", TerminalFontSize: 16, Notifications: Notifications{Replies: true}}
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
		{Theme: "blue", TerminalFontSize: cfg.TerminalFontSize},
		{Theme: cfg.Theme, TerminalFontSize: 15},
		{Theme: cfg.Theme, TerminalFontSize: cfg.TerminalFontSize, DefaultSelection: &Settings{Harness: "codex"}},
	} {
		if err := SaveConfig(invalid); err == nil {
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

func TestLoadConfigDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hypercube")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(dir))
	defaults := Config{Theme: "system", TerminalFontSize: 13, Notifications: DefaultNotifications}
	// A fresh installation has no file.
	got, err := LoadConfig()
	if err != nil || !reflect.DeepEqual(got, defaults) {
		t.Fatalf("missing config = %+v, %v", got, err)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = LoadConfig()
	if err != nil || !reflect.DeepEqual(got, defaults) {
		t.Fatalf("empty config = %+v, %v", got, err)
	}
}

func TestSaveConfigWriteFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// A regular file where the config directory belongs makes saving fail.
	if err := os.WriteFile(filepath.Join(dir, "hypercube"), []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(Config{Theme: "system", TerminalFontSize: 13}); err == nil {
		t.Fatal("save succeeded without a config directory")
	}
}

func TestUpgradeConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hypercube")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(dir))
	path := filepath.Join(dir, "config.json")
	if err := UpgradeConfig(); err != nil {
		t.Fatalf("upgrading without a file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("upgrade created a config: %v", err)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lead := `{"name":"lead","description":" Hard work. ","harness":"claude","model":"opus","effort":"high"}`
	worker := `{"name":"worker","description":"Small jobs.","harness":"opencode","model":"p/m/x","effort":"low"}`
	guidance := "- Harness claude, model opus, effort high: Hard work.\n- Harness opencode, model p/m/x, effort low: Small jobs."
	for name, tc := range map[string]struct {
		source string
		want   Config
	}{
		"implicit default": {`{"theme":"dark","profiles":[` + lead + `,` + worker + `]}`,
			Config{DefaultSelection: &Settings{Harness: "claude", Model: "opus", Effort: "high"}, ChildGuidance: guidance, Theme: "dark", TerminalFontSize: 13, Notifications: DefaultNotifications}},
		"explicit default": {`{"default_profile":"worker","default_worktree":true,"terminal_font_size":16,"notifications":{"requests":false,"replies":true,"macos":false,"sound":false},"profiles":[` + lead + `,` + worker + `]}`,
			Config{DefaultSelection: &Settings{Harness: "opencode", Model: "p/m/x", Effort: "low"}, ChildGuidance: guidance, DefaultWorktree: true, Theme: "system", TerminalFontSize: 16, Notifications: Notifications{Replies: true}}},
		"explicit guidance": {`{"child_model_guidance":"","profiles":[` + lead + `]}`,
			Config{DefaultSelection: &Settings{Harness: "claude", Model: "opus", Effort: "high"}, Theme: "system", TerminalFontSize: 13, Notifications: DefaultNotifications}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.source), 0o644); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := UpgradeConfig(); err != nil {
					t.Fatal(err)
				}
				got, err := LoadConfig()
				if err != nil || !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("upgraded config = %+v, %v", got, err)
				}
			}
			data, err := os.ReadFile(path)
			if err != nil || strings.Contains(string(data), "profile") {
				t.Fatalf("upgraded file = %s, %v", data, err)
			}
		})
	}
	// A malformed file stays as it is.
	if err := os.WriteFile(path, []byte(`{"profiles":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpgradeConfig(); err == nil {
		t.Fatal("upgraded a malformed config")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != `{"profiles":` {
		t.Fatalf("malformed config changed: %s, %v", data, err)
	}
}

func TestUpgradeConfigNotifications(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hypercube")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(dir))
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	lead := `{"name":"lead","harness":"claude","model":"opus","effort":"high"}`
	for name, tc := range map[string]struct {
		source string
		want   Notifications // after the upgrade
		kept   bool          // the file stays byte for byte
	}{
		"renamed": {source: `{"notifications":{"requests":true,"replies":false,"macos":false,"sound":true}}`,
			want: Notifications{Requests: true, Sound: true}},
		"both names": {source: `{"theme":"dark","notifications":{"macos":false,"desktop":true}}`, kept: true},
		"new name":   {source: `{"theme":"dark","notifications":{"desktop":false}}`, kept: true},
		"none":       {source: `{"theme":"dark"}`, kept: true},
		"with profiles": {source: `{"notifications":{"requests":false,"replies":true,"macos":false,"sound":true},"profiles":[` + lead + `]}`,
			want: Notifications{Replies: true, Sound: true}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.source), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := UpgradeConfig(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if tc.kept {
				if string(data) != tc.source {
					t.Fatalf("config changed: %s", data)
				}
				return
			}
			if strings.Contains(string(data), "macos") || !strings.Contains(string(data), `"desktop"`) {
				t.Fatalf("upgraded file = %s", data)
			}
			got, err := LoadConfig()
			if err != nil || got.Notifications != tc.want {
				t.Fatalf("notifications = %+v, %v", got.Notifications, err)
			}
			if name == "with profiles" && (got.DefaultSelection == nil || strings.Contains(string(data), "profiles")) {
				t.Fatalf("profiles not upgraded in the same write: %s", data)
			}
		})
	}
}
