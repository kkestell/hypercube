package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Profile names a harness, model, and effort for an agent, and its
// description says when to use it.
type Profile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Harness     string `json:"harness"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
}

// Config is config.json: the agent profiles and the user's preferences.
// DefaultWorktree starts new sessions in Git workspaces in a worktree.
type Config struct {
	DefaultProfile   string        `json:"default_profile"`
	DefaultWorktree  bool          `json:"default_worktree"`
	Theme            string        `json:"theme"`
	TerminalFontSize int           `json:"terminal_font_size"`
	Notifications    Notifications `json:"notifications"`
	Profiles         []Profile     `json:"profiles"`
}

// Notifications chooses which alerts reach the user and how. Requests and
// Replies choose the alerts for sessions the user is not viewing; MacOS sends
// them to Notification Center while the window is not focused, with a sound
// when Sound is set.
type Notifications struct {
	Requests bool `json:"requests"`
	Replies  bool `json:"replies"`
	MacOS    bool `json:"macos"`
	Sound    bool `json:"sound"`
}

// DefaultNotifications sends every alert, as Hypercube did before the
// preferences existed.
var DefaultNotifications = Notifications{Requests: true, Replies: true, MacOS: true, Sound: true}

var (
	themes            = []string{"system", "light", "dark"}
	terminalFontSizes = []int{12, 13, 14, 16, 18}
)

func configPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "hypercube", "config.json")
}

// LoadConfig reads config.json and checks that every value is set.
func LoadConfig() (Config, error) {
	// A file written before a preference existed takes its default.
	c := Config{Theme: "system", TerminalFontSize: 13, Notifications: DefaultNotifications}
	path := configPath()
	data, err := os.ReadFile(path)
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

// Validate checks the configuration and resolves an omitted default to the
// first profile, as files written before default_profile did.
func (c *Config) Validate() error {
	if !slices.Contains(themes, c.Theme) {
		return fmt.Errorf("theme %q is not one of %v", c.Theme, themes)
	}
	if !slices.Contains(terminalFontSizes, c.TerminalFontSize) {
		return fmt.Errorf("terminal font size %d is not one of %v", c.TerminalFontSize, terminalFontSizes)
	}
	if len(c.Profiles) == 0 {
		return fmt.Errorf("at least one profile is required")
	}
	seen := map[string]bool{}
	for i, p := range c.Profiles {
		switch {
		case strings.TrimSpace(p.Name) == "":
			return fmt.Errorf("profile %d: missing name", i+1)
		case seen[p.Name]:
			return fmt.Errorf("duplicate profile %q", p.Name)
		case strings.TrimSpace(p.Description) == "":
			return fmt.Errorf("profile %q: missing description", p.Name)
		}
		if err := p.check(); err != nil {
			return fmt.Errorf("profile %q: %w", p.Name, err)
		}
		seen[p.Name] = true
	}
	if c.DefaultProfile == "" {
		c.DefaultProfile = c.Profiles[0].Name
	}
	if !seen[c.DefaultProfile] {
		return fmt.Errorf("default profile %q does not exist", c.DefaultProfile)
	}
	return nil
}

// SaveConfig validates and replaces the config as a complete document. Readers
// keep using LoadConfig; direct file edits while the app runs are unsupported.
func SaveConfig(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := configPath()
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

// Profile returns the profile with the given name.
func (c Config) Profile(name string) (Profile, error) {
	for _, p := range c.Profiles {
		if p.Name == name {
			return p, nil
		}
	}
	return Profile{}, fmt.Errorf("no profile named %q", name)
}

func (p Profile) check() error {
	switch {
	case strings.TrimSpace(p.Harness) == "":
		return fmt.Errorf("missing harness")
	case strings.TrimSpace(p.Model) == "":
		return fmt.Errorf("missing model")
	case strings.TrimSpace(p.Effort) == "":
		return fmt.Errorf("missing effort")
	}
	return nil
}
