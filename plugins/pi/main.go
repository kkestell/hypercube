// Command hypercube-harness-pi runs Pi as a Hypercube agent. Hypercube runs
// `launch`; inside Pi, pi.js reports the conversation and starts turns with
// waiting messages.
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

//go:embed pi.js
var source string

func main() {
	var err error
	if len(os.Args) == 2 && os.Args[1] == "launch" {
		err = launch(os.Stdin, os.Stdout)
	} else {
		err = fmt.Errorf("usage: hypercube-harness-pi launch")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-harness-pi:", err)
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
	provider, id, ok := strings.Cut(in.Model, "/")
	if !ok {
		return fmt.Errorf("model %q is not provider/id; see pi --list-models", in.Model)
	}
	extension, err := writeExtension(in.Hypercube, in.StateDir)
	if err != nil {
		return err
	}
	session := in.Session
	if session == "" {
		session = "hypercube-" + in.Agent
	}
	env := map[string]string{
		"HYPERCUBE_MODEL": in.Model,
		"HYPERCUBE_MCP":   jsonString(map[string]any{"command": in.Hypercube, "args": []string{"mcp"}, "env": in.Env, "exposure": "direct"}),
	}
	argv := []string{"pi", "--provider", provider, "--model", id, "-e", extension, "--session-id", session}
	if in.Effort != "" {
		argv = append(argv, "--thinking", in.Effort)
	}
	if in.Prompt != "" {
		argv = append(argv, "--", in.Prompt)
	}
	return json.NewEncoder(stdout).Encode(map[string]any{"argv": argv, "env": env})
}

// writeExtension writes pi.js for this hypercube executable. Running Pi
// processes may be reading it, so it is replaced only when it changes, through
// a rename.
func writeExtension(exe, stateDir string) (string, error) {
	dir := filepath.Join(stateDir, "pi-extension")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "index.js")
	text := []byte(strings.ReplaceAll(source, "HYPERCUBE_EXE", jsonString(exe)))
	if before, err := os.ReadFile(path); err == nil && bytes.Equal(before, text) {
		return path, nil
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
	return path, os.Rename(file.Name(), path)
}

func jsonString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
