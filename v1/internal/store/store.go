// Package store holds a workspace's agents and message queue in one JSON file.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	Orchestrator = "orchestrator"
	Worker       = "worker"
	Busy         = "busy"
	Idle         = "idle"
)

type Agent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Harness   string `json:"harness"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	Dir       string `json:"dir"`
	Branch    string `json:"branch"`
	SessionID string `json:"session_id,omitempty"`
	Target    string `json:"target,omitempty"`
	Activity  string `json:"activity"`
}

type Message struct {
	From string `json:"from"`
	To   string `json:"to"`
	Text string `json:"text"`
}

type State struct {
	Agents   []Agent   `json:"agents"`
	Messages []Message `json:"messages"`
	// NoGit is set for a workspace whose directory is not a Git checkout.
	// Its workers work directly in existing directories, without branches
	// or worktrees.
	NoGit bool `json:"no_git,omitempty"`
}

// Dir is the directory that holds state files and worktrees.
func Dir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "hypercube")
}

// Path returns the state file for the named workspace.
func Path(workspace string) string {
	return filepath.Join(Dir(), workspace+".json")
}

// Workspace returns the workspace name of a state file path.
func Workspace(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".json")
}

// NewID returns a short random identifier.
func NewID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Load reads the state without locking. Writes go through rename, so a read
// always sees a complete file.
func Load(path string) (State, error) {
	var s State
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(data, &s)
}

// Delete removes the state file and its lock. It waits for any update in
// progress.
func Delete(path string) error {
	lock, err := lockFile(path)
	if err != nil {
		return err
	}
	defer lock.Close()
	for _, p := range []string{path, strings.TrimSuffix(path, ".json") + ".lock"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// lockFile opens the state file's lock and holds it exclusively until the
// file is closed.
func lockFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(strings.TrimSuffix(path, ".json")+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		lock.Close()
		return nil, err
	}
	return lock, nil
}

// Update applies fn to the state under an exclusive lock and writes the
// result. If fn returns an error, nothing is written.
func Update(path string, fn func(*State) error) error {
	lock, err := lockFile(path)
	if err != nil {
		return err
	}
	defer lock.Close()
	s, err := Load(path)
	if err != nil {
		return err
	}
	if err := fn(&s); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Agent returns the agent with the given ID or name.
func (s *State) Agent(idOrName string) (*Agent, error) {
	for i := range s.Agents {
		if s.Agents[i].ID == idOrName || s.Agents[i].Name == idOrName {
			return &s.Agents[i], nil
		}
	}
	return nil, fmt.Errorf("no agent %q", idOrName)
}

// Take removes and returns the messages queued for an agent.
func (s *State) Take(id string) []Message {
	var taken, kept []Message
	for _, m := range s.Messages {
		if m.To == id {
			taken = append(taken, m)
		} else {
			kept = append(kept, m)
		}
	}
	s.Messages = kept
	return taken
}

// Remove deletes an agent and its queued messages.
func (s *State) Remove(id string) {
	var agents []Agent
	for _, a := range s.Agents {
		if a.ID != id {
			agents = append(agents, a)
		}
	}
	s.Agents = agents
	s.Take(id)
}
