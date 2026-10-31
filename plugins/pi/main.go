// Command hypercube-harness-pi runs Pi as a Hypercube agent. Hypercube runs
// `launch`; inside Pi, pi.js reports the conversation and starts turns with
// waiting messages.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
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

func launch(stdin io.Reader, stdout io.Writer) error {
	var in harness.LaunchInput
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
		"HYPERCUBE_MCP":   plugin.JSONString(map[string]any{"command": in.Hypercube, "args": []string{"mcp"}, "env": in.Env, "exposure": "direct"}),
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

// writeExtension writes pi.js for this hypercube executable.
func writeExtension(exe, stateDir string) (string, error) {
	text := []byte(strings.ReplaceAll(source, "HYPERCUBE_EXE", plugin.JSONString(exe)))
	return plugin.WriteFile(filepath.Join(stateDir, "pi-extension"), "index.js", text)
}
