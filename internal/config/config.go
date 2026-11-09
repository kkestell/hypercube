// Package config reads and writes config.json, the user's preferences, in
// $XDG_CONFIG_HOME/hypercube or ~/.config/hypercube.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

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

// DefaultNotifications enables every alert.
var DefaultNotifications = Notifications{Requests: true, Replies: true, Desktop: true, Sound: true}

var (
	themes            = []string{"system", "light", "dark"}
	terminalFontSizes = []int{12, 13, 14, 16, 18}
)

// defaults supplies preferences omitted from config.json.
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
