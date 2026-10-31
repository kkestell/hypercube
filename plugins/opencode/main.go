// Command hypercube-harness-opencode runs OpenCode 2.x as a Hypercube agent.
// Hypercube runs `launch`; inside OpenCode, opencode.js reports the
// conversation and starts turns with waiting messages.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed opencode.js
var source string

func main() {
	var err error
	if len(os.Args) == 2 && os.Args[1] == "launch" {
		err = launch(os.Stdin, os.Stdout)
	} else {
		err = fmt.Errorf("usage: hypercube-harness-opencode launch")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-harness-opencode:", err)
		os.Exit(1)
	}
}

type launchInput struct {
	Hypercube string            `json:"hypercube"`
	Env       map[string]string `json:"env"`
	Agent     string            `json:"agent"`
	Model     string            `json:"model"`
	Effort    string            `json:"effort"`
	Session   string            `json:"session"`
	Prompt    string            `json:"prompt"`
	StateDir  string            `json:"state_dir"`
}

func launch(stdin io.Reader, stdout io.Writer) error {
	var in launchInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return err
	}
	plugin, err := writePlugin(in.Hypercube, in.StateDir)
	if err != nil {
		return err
	}
	session := in.Session
	if session == "" {
		session = "ses_hypercube_" + in.Agent
	}
	env := map[string]string{
		"OPENCODE_CONFIG_CONTENT": jsonString(map[string]any{
			"model":   in.Model,
			"agents":  map[string]any{"build": map[string]any{"model": in.Model + "#" + in.Effort}},
			"mcp":     map[string]any{"servers": map[string]any{"hypercube": map[string]any{"type": "local", "command": []string{in.Hypercube, "mcp"}, "environment": in.Env, "codemode": false}}},
			"plugins": []string{plugin},
		}),
		"OPENCODE_CLI_CONFIG_CONTENT": `{"tabs":{"mode":"off"}}`,
		"HYPERCUBE_SESSION_ID":        session,
	}
	argv := []string{"opencode", "--standalone", "--auto", "--session", session}
	if in.Prompt != "" {
		argv = append(argv, "--prompt", in.Prompt)
	}
	return json.NewEncoder(stdout).Encode(map[string]any{"argv": argv, "env": env})
}

// writePlugin writes opencode.js for this hypercube executable. Running
// OpenCode processes may be reading it, so it is replaced only when it
// changes, through a rename.
func writePlugin(exe, stateDir string) (string, error) {
	dir := filepath.Join(stateDir, "opencode-plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "index.js")
	text := []byte(strings.ReplaceAll(source, "HYPERCUBE_EXE", jsonString(exe)))
	if before, err := os.ReadFile(path); err == nil && bytes.Equal(before, text) {
		return dir, nil
	}
	file, err := os.CreateTemp(dir, "index-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(text); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return dir, os.Rename(file.Name(), path)
}

func jsonString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
