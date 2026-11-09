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
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

// Config is config.json: the user's preferences. DefaultSelection, when set,
// starts new sessions with that harness, model, and effort. ChildGuidance
// tells agents when a child session should use other settings than theirs.
// DefaultWorktree starts new sessions in Git workspaces in a worktree.
// ReviewSelection, when set, starts pull request reviews with that harness,
// model, and effort; ReviewPrompt is their first prompt, with {number} and
// {url} standing for the pull request's number and link. Models lists the model
// picker's entries in the order they were added.
type Config struct {
	DefaultSelection *harness.Settings `json:"default_selection,omitempty"`
	ChildGuidance    string            `json:"child_model_guidance"`
	DefaultWorktree  bool              `json:"default_worktree"`
	ReviewSelection  *harness.Settings `json:"review_selection,omitempty"`
	ReviewPrompt     string            `json:"review_prompt"`
	Theme            string            `json:"theme"`
	TerminalFontSize int               `json:"terminal_font_size"`
	Notifications    Notifications     `json:"notifications"`
	Models           []ModelEntry      `json:"models"`
}

// ModelEntry is a model the user lists for the picker: a harness's native
// model ID, its title, its effort levels from lowest to highest, and the effort
// a session starts with. Sessions keep their own settings, not a reference to
// an entry.
type ModelEntry struct {
	Harness       string   `json:"harness"`
	Model         string   `json:"model"`
	Title         string   `json:"title"`
	Efforts       []string `json:"efforts"`
	DefaultEffort string   `json:"default_effort"`
}

// Check trims fields and validates the entry without asking a harness.
func (e *ModelEntry) Check() error {
	e.Harness = strings.TrimSpace(e.Harness)
	e.Model = strings.TrimSpace(e.Model)
	e.Title = strings.TrimSpace(e.Title)
	e.DefaultEffort = strings.TrimSpace(e.DefaultEffort)
	if e.Harness == "" || e.Model == "" || e.Title == "" {
		return errors.New("harness, model, and title are required")
	}
	seen := make(map[string]bool)
	if e.Efforts == nil {
		e.Efforts = []string{}
	}
	for i, level := range e.Efforts {
		level = strings.TrimSpace(level)
		if level == "" {
			return errors.New("effort levels must not be empty")
		}
		if seen[level] {
			return fmt.Errorf("effort level %q repeats", level)
		}
		seen[level], e.Efforts[i] = true, level
	}
	if len(e.Efforts) == 1 {
		e.DefaultEffort = e.Efforts[0]
	}
	if len(e.Efforts) == 0 {
		if e.DefaultEffort != "" {
			return errors.New("a model without effort levels cannot have a default effort")
		}
	} else if !seen[e.DefaultEffort] {
		return errors.New("default effort must be one of the effort levels")
	}
	return nil
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

// DefaultReviewPrompt is the review prompt until the user changes it.
const DefaultReviewPrompt = "Review pull request #{number} ({url}). Its head commit is checked out in your working directory. " +
	"Read its description, discussion, and diff with gh, and report what you find. Post nothing to GitHub."

// DefaultNotifications enables every alert.
var DefaultNotifications = Notifications{Requests: true, Replies: true, Desktop: true, Sound: true}

var (
	themes            = []string{"system", "light", "dark"}
	terminalFontSizes = []int{12, 13, 14, 16, 18}
)

// defaults supplies preferences omitted from config.json.
func defaults() Config {
	return Config{ReviewPrompt: DefaultReviewPrompt, Theme: "system", TerminalFontSize: 13, Notifications: DefaultNotifications, Models: []ModelEntry{}}
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

// Validate checks the configuration's values and trims model entries and the
// review prompt.
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
	if c.ReviewSelection != nil {
		if err := c.ReviewSelection.Check(); err != nil {
			return fmt.Errorf("review selection: %w", err)
		}
	}
	c.ReviewPrompt = strings.TrimSpace(c.ReviewPrompt)
	if c.ReviewPrompt == "" {
		return errors.New("review prompt is required")
	}
	if c.Models == nil {
		c.Models = []ModelEntry{}
	}
	seen := make(map[[2]string]bool)
	for i := range c.Models {
		e := &c.Models[i]
		if err := e.Check(); err != nil {
			return fmt.Errorf("model %s %q: %w", e.Harness, e.Model, err)
		}
		key := [2]string{e.Harness, e.Model}
		if seen[key] {
			return fmt.Errorf("%s lists model %q twice", e.Harness, e.Model)
		}
		seen[key] = true
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
