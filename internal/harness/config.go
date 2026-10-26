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

type Config struct {
	Orchestrator Spec `json:"orchestrator"`
	Worker       Spec `json:"worker"`
	// Models describes the models the orchestrator can give workers. It is
	// free text, shown to the orchestrator as written.
	Models string `json:"models"`
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
	for role, s := range map[string]Spec{"orchestrator": c.Orchestrator, "worker": c.Worker} {
		if err := s.check(); err != nil {
			return c, fmt.Errorf("%s: %s: %w", path, role, err)
		}
	}
	return c, nil
}

func (s Spec) check() error {
	switch {
	case s.Harness != "claude" && s.Harness != "codex" && s.Harness != "opencode" && s.Harness != "ox":
		return fmt.Errorf("harness must be claude, codex, opencode, or ox, not %q", s.Harness)
	case s.Model == "":
		return fmt.Errorf("missing model")
	case s.Effort == "":
		return fmt.Errorf("missing effort")
	}
	return nil
}
