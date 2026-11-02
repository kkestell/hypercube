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

const (
	RoleLead    = "lead"
	RoleGeneral = "general"
)

// Profile is a named spec. Its role says whether it is a lead or a child; its
// description tells the user or a top-level session when to use it.
type Profile struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Description string `json:"description"`
	Spec
}

type Config struct {
	Profiles []Profile `json:"profiles"`
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
	seen := map[string]bool{}
	var leads, children int
	for i, p := range c.Profiles {
		switch {
		case p.Name == "":
			return c, fmt.Errorf("%s: profile %d: missing name", path, i+1)
		case seen[p.Name]:
			return c, fmt.Errorf("%s: duplicate profile %q", path, p.Name)
		case p.Description == "":
			return c, fmt.Errorf("%s: profile %q: missing description", path, p.Name)
		}
		switch p.Role {
		case RoleLead:
			leads++
		case RoleGeneral:
			children++
		default:
			return c, fmt.Errorf("%s: profile %q: invalid role %q (want lead or general)", path, p.Name, p.Role)
		}
		if err := p.check(); err != nil {
			return c, fmt.Errorf("%s: profile %q: %w", path, p.Name, err)
		}
		seen[p.Name] = true
	}
	if leads == 0 {
		return c, fmt.Errorf("%s: at least one lead profile is required", path)
	}
	if children == 0 {
		return c, fmt.Errorf("%s: at least one child profile is required", path)
	}
	return c, nil
}

// Leads returns lead profiles in file order.
func (c Config) Leads() []Profile {
	var profiles []Profile
	for _, p := range c.Profiles {
		if p.Role == RoleLead {
			profiles = append(profiles, p)
		}
	}
	return profiles
}

// ChildProfiles returns child profiles in file order.
func (c Config) ChildProfiles() []Profile {
	var profiles []Profile
	for _, p := range c.Profiles {
		if p.Role != RoleLead {
			profiles = append(profiles, p)
		}
	}
	return profiles
}

// Lead returns the named lead profile, or the first lead when name is empty.
func (c Config) Lead(name string) (Profile, error) {
	for _, p := range c.Profiles {
		if p.Role == RoleLead && (name == "" || p.Name == name) {
			return p, nil
		}
	}
	return Profile{}, fmt.Errorf("no lead profile named %q", name)
}

// ChildProfile returns the child profile with the given name.
func (c Config) ChildProfile(name string) (Profile, error) {
	for _, p := range c.Profiles {
		if p.Role != RoleLead && p.Name == name {
			return p, nil
		}
	}
	return Profile{}, fmt.Errorf("no child profile named %q", name)
}

func (s Spec) check() error {
	switch {
	case s.Harness == "":
		return fmt.Errorf("missing harness")
	case s.Model == "":
		return fmt.Errorf("missing model")
	case s.Effort == "":
		return fmt.Errorf("missing effort")
	}
	return nil
}
