// Package config reads and writes config.json, the user's preferences, in
// $XDG_CONFIG_HOME/hypercube or ~/.config/hypercube. Upgrade converts files
// written by earlier versions before anything reads them.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

// Config is config.json: the user's preferences. DefaultSelection, when set,
// starts new sessions with that harness, model, and effort. ChildGuidance
// tells agents when a child session should use other settings than theirs.
// DefaultWorktree starts new sessions in Git workspaces in a worktree.
type Config struct {
	DefaultSelection *harness.Settings `json:"default_selection,omitempty"`
	ChildGuidance    string            `json:"child_model_guidance"`
	DefaultWorktree  bool              `json:"default_worktree"`
	Theme            string            `json:"theme"`
	TerminalFontSize int               `json:"terminal_font_size"`
	Notifications    Notifications     `json:"notifications"`
}

// Notifications chooses which alerts reach the user and how. Requests and
// Replies choose the alerts for sessions the user is not viewing; Desktop sends
// them to the desktop's notifications while the window is not focused, with a
// sound when Sound is set.
type Notifications struct {
	Requests bool `json:"requests"`
	Replies  bool `json:"replies"`
	Desktop  bool `json:"desktop"`
	Sound    bool `json:"sound"`
}

// DefaultNotifications sends every alert, as Hypercube did before the
// preferences existed.
var DefaultNotifications = Notifications{Requests: true, Replies: true, Desktop: true, Sound: true}

var (
	themes            = []string{"system", "light", "dark"}
	terminalFontSizes = []int{12, 13, 14, 16, 18}
)

// defaults is the configuration with every preference at its default. A file
// written before a preference existed takes that default.
func defaults() Config {
	return Config{Theme: "system", TerminalFontSize: 13, Notifications: DefaultNotifications}
}

func path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "hypercube", "config.json")
}

// Load reads config.json. Without a file, every preference takes its
// default.
func Load() (Config, error) {
	c := defaults()
	path := path()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Validate checks the configuration's values.
func (c *Config) Validate() error {
	if !slices.Contains(themes, c.Theme) {
		return fmt.Errorf("theme %q is not one of %v", c.Theme, themes)
	}
	if !slices.Contains(terminalFontSizes, c.TerminalFontSize) {
		return fmt.Errorf("terminal font size %d is not one of %v", c.TerminalFontSize, terminalFontSizes)
	}
	if c.DefaultSelection != nil {
		if err := c.DefaultSelection.Check(); err != nil {
			return fmt.Errorf("default selection: %w", err)
		}
	}
	return nil
}

// Save validates and replaces the config as a complete document. Readers
// keep using Load; direct file edits while the app runs are unsupported.
func Save(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return write(data)
}

func write(data []byte) error {
	path := path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	defer os.Remove(f.Name())
	_, err = f.Write(append(data, '\n'))
	closeErr := f.Close()
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("closing config: %w", closeErr)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}
	return nil
}

// legacyProfile is an agent profile of earlier versions: named settings and
// when to use them.
type legacyProfile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	harness.Settings
}

// upgrade changes the decoded config.json of an earlier version and reports
// whether it changed anything.
type upgrade func(doc map[string]json.RawMessage) (bool, error)

// Upgrade converts a config.json written by earlier versions before
// anything reads it. A file that no upgrade changes is left as it is; otherwise
// it is decoded and validated as Load will, and the typed value is
// written, which keeps the file's field order.
func Upgrade() error {
	path := path()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	changed := false
	for _, up := range []upgrade{upgradeProfiles, upgradeNotifications} {
		did, err := up(doc)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		changed = changed || did
	}
	if !changed {
		return nil
	}
	converted, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	c := defaults()
	if err := json.Unmarshal(converted, &c); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return write(out)
}

// upgradeProfiles converts the agent profiles of earlier versions. The default
// profile becomes the default selection, and the profiles' descriptions become
// the child guidance, unless the file already has guidance.
func upgradeProfiles(doc map[string]json.RawMessage) (bool, error) {
	_, hasProfiles := doc["profiles"]
	_, hasDefault := doc["default_profile"]
	if !hasProfiles && !hasDefault {
		return false, nil
	}
	var legacy struct {
		DefaultProfile string          `json:"default_profile"`
		Profiles       []legacyProfile `json:"profiles"`
	}
	if hasDefault {
		if err := json.Unmarshal(doc["default_profile"], &legacy.DefaultProfile); err != nil {
			return false, err
		}
	}
	if hasProfiles {
		if err := json.Unmarshal(doc["profiles"], &legacy.Profiles); err != nil {
			return false, err
		}
	}
	delete(doc, "profiles")
	delete(doc, "default_profile")
	// Earlier versions used the first profile when no default was named.
	name := legacy.DefaultProfile
	if name == "" && len(legacy.Profiles) > 0 {
		name = legacy.Profiles[0].Name
	}
	for _, p := range legacy.Profiles {
		if p.Name == name {
			var err error
			if doc["default_selection"], err = json.Marshal(p.Settings); err != nil {
				return false, err
			}
		}
	}
	if _, ok := doc["child_model_guidance"]; !ok {
		var b strings.Builder
		for _, p := range legacy.Profiles {
			fmt.Fprintf(&b, "- Harness %s, model %s, effort %s: %s\n", p.Harness, p.Model, p.Effort, strings.TrimSpace(p.Description))
		}
		var err error
		if doc["child_model_guidance"], err = json.Marshal(strings.TrimSuffix(b.String(), "\n")); err != nil {
			return false, err
		}
	}
	return true, nil
}

// upgradeNotifications renames notifications.macos to notifications.desktop,
// keeping the user's choice, when the file has only the old name.
func upgradeNotifications(doc map[string]json.RawMessage) (bool, error) {
	if doc["notifications"] == nil {
		return false, nil
	}
	var notifications map[string]json.RawMessage
	if err := json.Unmarshal(doc["notifications"], &notifications); err != nil {
		return false, err
	}
	old, hasOld := notifications["macos"]
	if _, hasNew := notifications["desktop"]; !hasOld || hasNew {
		return false, nil
	}
	notifications["desktop"] = old
	delete(notifications, "macos")
	var err error
	doc["notifications"], err = json.Marshal(notifications)
	return err == nil, err
}
