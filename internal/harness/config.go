package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Spec picks a harness, model, and effort for an agent.
type Spec struct {
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Effort  string `json:"effort"`
}

// Profile is a named spec the orchestrator can give a worker. The
// description tells the orchestrator when to use it.
type Profile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Spec
}

type Config struct {
	Orchestrator Spec      `json:"orchestrator"`
	Profiles     []Profile `json:"profiles"`
}

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
	var c Config
	path := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if err := c.Orchestrator.check(); err != nil {
		return c, fmt.Errorf("%s: orchestrator: %w", path, err)
	}
	if len(c.Profiles) == 0 {
		return c, fmt.Errorf("%s: missing profiles", path)
	}
	seen := map[string]bool{}
	for i, p := range c.Profiles {
		switch {
		case p.Name == "":
			return c, fmt.Errorf("%s: profile %d: missing name", path, i+1)
		case seen[p.Name]:
			return c, fmt.Errorf("%s: duplicate profile %q", path, p.Name)
		case p.Description == "":
			return c, fmt.Errorf("%s: profile %q: missing description", path, p.Name)
		}
		if err := p.check(); err != nil {
			return c, fmt.Errorf("%s: profile %q: %w", path, p.Name, err)
		}
		seen[p.Name] = true
	}
	return c, nil
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

func (s Spec) check() error {
	switch {
	case s.Harness != "claude" && s.Harness != "codex" && s.Harness != "opencode" && s.Harness != "ox" && s.Harness != "pi":
		return fmt.Errorf("harness must be claude, codex, opencode, ox, or pi, not %q", s.Harness)
	case s.Model == "":
		return fmt.Errorf("missing model")
	case s.Effort == "":
		return fmt.Errorf("missing effort")
	}
	return nil
}
