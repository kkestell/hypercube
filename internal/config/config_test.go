package config

import (
	"bytes"
	"encoding/json"
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
		DefaultWorktree: true, ReviewSelection: &harness.Settings{Harness: "opencode", Model: "provider/model"},
		UtilitySelection: &harness.Settings{Harness: "claude", Model: "haiku", Effort: "low"},
		ReviewPrompt:     "Review #{number} at {url}.", Theme: "dark", TerminalFontSize: 16, Notifications: Notifications{Replies: true},
		Models: []ModelEntry{
			{Harness: "codex", Model: "sol", Title: "Sol", Efforts: []string{"low", "high"}, DefaultEffort: "high"},
			{Harness: "opencode", Model: "provider/model", Title: "Model", Efforts: []string{}},
		}}
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
	prompt := cfg.ReviewPrompt
	for _, invalid := range []Config{
		{},
		{Theme: "blue", TerminalFontSize: cfg.TerminalFontSize, ReviewPrompt: prompt},
		{Theme: cfg.Theme, TerminalFontSize: 15, ReviewPrompt: prompt},
		{Theme: cfg.Theme, TerminalFontSize: cfg.TerminalFontSize, ReviewPrompt: prompt, DefaultSelection: &harness.Settings{Harness: "codex"}},
		{Theme: cfg.Theme, TerminalFontSize: cfg.TerminalFontSize, ReviewPrompt: prompt, ReviewSelection: &harness.Settings{Harness: "codex"}},
		{Theme: cfg.Theme, TerminalFontSize: cfg.TerminalFontSize, ReviewPrompt: prompt, UtilitySelection: &harness.Settings{Harness: "claude"}},
		{Theme: cfg.Theme, TerminalFontSize: cfg.TerminalFontSize, ReviewPrompt: " \n "},
		{Theme: cfg.Theme, TerminalFontSize: cfg.TerminalFontSize, ReviewPrompt: prompt, Models: []ModelEntry{cfg.Models[1], cfg.Models[1]}},
		{Theme: cfg.Theme, TerminalFontSize: cfg.TerminalFontSize, ReviewPrompt: prompt, Models: []ModelEntry{{Harness: "codex", Model: "sol"}}},
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
	defaults := Config{ReviewPrompt: DefaultReviewPrompt, Theme: "system", TerminalFontSize: 13, Notifications: DefaultNotifications, Models: []ModelEntry{}}
	// A fresh installation has no file.
	got, err := Load()
	if err != nil || !reflect.DeepEqual(got, defaults) {
		t.Fatalf("missing config = %+v, %v", got, err)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{`{}`, `{"models":null}`} {
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err = Load()
		if err != nil || !reflect.DeepEqual(got, defaults) {
			t.Fatalf("config %s = %+v, %v", source, got, err)
		}
	}
}

// TestExampleConfig keeps docs/examples/config.json valid and in the form Save
// writes.
func TestExampleConfig(t *testing.T) {
	data, err := os.ReadFile("../../docs/examples/config.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaults()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	saved, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(append(saved, '\n')) != string(data) {
		t.Fatalf("example differs from the saved format:\n%s", saved)
	}
}

func TestModelEntryCheck(t *testing.T) {
	for _, test := range []struct {
		name  string
		entry ModelEntry
		valid bool
	}{
		{"empty harness", ModelEntry{Model: "m", Title: "M"}, false},
		{"empty model", ModelEntry{Harness: "h", Title: "M"}, false},
		{"empty title", ModelEntry{Harness: "h", Model: "m"}, false},
		{"blank title", ModelEntry{Harness: "h", Model: "m", Title: " "}, false},
		{"blank level", ModelEntry{Harness: "h", Model: "m", Title: "M", Efforts: []string{" "}}, false},
		{"repeated level", ModelEntry{Harness: "h", Model: "m", Title: "M", Efforts: []string{"low", " low "}}, false},
		{"missing default", ModelEntry{Harness: "h", Model: "m", Title: "M", Efforts: []string{"low", "high"}}, false},
		{"unlisted default", ModelEntry{Harness: "h", Model: "m", Title: "M", Efforts: []string{"low", "high"}, DefaultEffort: "max"}, false},
		{"no levels with default", ModelEntry{Harness: "h", Model: "m", Title: "M", DefaultEffort: "high"}, false},
		{"one level", ModelEntry{Harness: " h ", Model: " m ", Title: " M ", Efforts: []string{" low "}}, true},
		{"no levels", ModelEntry{Harness: "h", Model: "m", Title: "M"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.entry.Check()
			if (err == nil) != test.valid {
				t.Fatalf("Check() = %v", err)
			}
			if test.valid && (test.entry.Harness != "h" || test.entry.Model != "m" || test.entry.Title != "M" || test.entry.Efforts == nil) {
				t.Fatalf("not normalized: %+v", test.entry)
			}
			if test.name == "one level" && test.entry.DefaultEffort != "low" {
				t.Fatalf("default = %q", test.entry.DefaultEffort)
			}
		})
	}
}

func TestSaveWriteFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// A regular file where the config directory belongs makes saving fail.
	if err := os.WriteFile(filepath.Join(dir, "hypercube"), []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(Config{ReviewPrompt: DefaultReviewPrompt, Theme: "system", TerminalFontSize: 13}); err == nil {
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
				ChildGuidance: "Use the small model for simple tasks.", DefaultWorktree: true, ReviewPrompt: DefaultReviewPrompt,
				Theme: "dark", TerminalFontSize: 16,
				Notifications: Notifications{Replies: true}, Models: []ModelEntry{}},
		},
		"partial": {
			source: `{"notifications":{"desktop":false}}`,
			want: Config{ReviewPrompt: DefaultReviewPrompt, Theme: "system", TerminalFontSize: 13,
				Notifications: Notifications{Requests: true, Replies: true, Sound: true}, Models: []ModelEntry{}},
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
