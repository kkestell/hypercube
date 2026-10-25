package harness

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Spec picks a harness, model, and effort for an agent.
type Spec struct {
	Harness string `yaml:"harness"`
	Model   string `yaml:"model"`
	Effort  string `yaml:"effort"`
}

type Config struct {
	Orchestrator Spec `yaml:"orchestrator"`
	Worker       Spec `yaml:"worker"`
}

func configPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "hypercube", "config.yaml")
}

// LoadConfig reads config.yaml and checks that every value is set.
func LoadConfig() (Config, error) {
	var c Config
	path := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	for role, s := range map[string]Spec{"orchestrator": c.Orchestrator, "worker": c.Worker} {
		if err := s.check(); err != nil {
			return c, fmt.Errorf("%s: %s: %w", path, role, err)
		}
	}
	return c, nil
}

func (s Spec) check() error {
	switch {
	case s.Harness != "claude" && s.Harness != "codex" && s.Harness != "opencode":
		return fmt.Errorf("harness must be claude, codex, or opencode, not %q", s.Harness)
	case s.Model == "":
		return fmt.Errorf("missing model")
	case s.Effort == "":
		return fmt.Errorf("missing effort")
	}
	return nil
}
