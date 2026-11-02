package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

// Codex reads these user inputs through links, while conversations, databases,
// caches, and logs belong to Hypercube's separate Codex home. Native file-auth
// refresh resolves credential links before replacing their targets.
var sharedInputs = []string{
	"config.toml", "auth.json", ".credentials.json", "AGENTS.md", "skills",
	"agents", "rules", "hooks", "plugins", "managed_config.toml",
}

func sourceHome() (string, error) {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return filepath.Abs(home)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

func prepareStorage(a harness.StartArgs) (string, error) {
	if a.StateDir == "" {
		return "", fmt.Errorf("Codex requires an agent state directory")
	}
	source, err := sourceHome()
	if err != nil {
		return "", err
	}
	home, err := filepath.Abs(filepath.Join(a.StateDir, "codex"))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return "", err
	}
	for _, name := range sharedInputs {
		target := filepath.Join(source, name)
		if _, err := os.Lstat(target); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return "", err
		}
		link := filepath.Join(home, name)
		if err := os.Symlink(target, link); err != nil {
			// Several agents can launch against the same private home at once.
			if os.IsExist(err) {
				if existing, readErr := os.Readlink(link); readErr == nil && existing == target {
					continue
				}
			}
			return "", fmt.Errorf("share Codex input %s: %w", name, err)
		}
	}
	if a.Conversation != "" {
		if err := importConversation(source, home, a.Conversation); err != nil {
			return "", err
		}
	}
	return home, nil
}

func rollout(home, id string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*.jsonl"))
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		if strings.HasSuffix(filepath.Base(path), "-"+id+".jsonl") {
			return path, nil
		}
	}
	return "", nil
}

// Import only the requested native rollout at the launch boundary, after its
// former process has ended. Codex resumes its original ID from this private copy.
func importConversation(source, home, id string) error {
	if path, err := rollout(home, id); err != nil || path != "" {
		return err
	}
	path, err := rollout(source, id)
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("Codex could not resume thread %s: no rollout found", id)
	}
	relative, err := filepath.Rel(source, path)
	if err != nil {
		return err
	}
	destination := filepath.Join(home, relative)
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(destination)
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	return nil
}
