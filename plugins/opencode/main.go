// Command hypercube-harness-opencode runs OpenCode 2.x as a Hypercube agent.
// Hypercube runs `launch`; inside OpenCode, opencode.js reports the
// conversation and starts turns with waiting messages.
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

func launch(stdin io.Reader, stdout io.Writer) error {
	var in harness.LaunchInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return err
	}
	pluginPath, err := writePlugin(in.Hypercube, in.StateDir)
	if err != nil {
		return err
	}
	session := in.Session
	if session == "" {
		session = "ses_hypercube_" + in.Agent
	}
	env := map[string]string{
		"OPENCODE_CONFIG_CONTENT": plugin.JSONString(map[string]any{
			"model":   in.Model,
			"agents":  map[string]any{"build": map[string]any{"model": in.Model + "#" + in.Effort}},
			"mcp":     map[string]any{"servers": map[string]any{"hypercube": map[string]any{"type": "local", "command": []string{in.Hypercube, "mcp"}, "environment": in.Env, "codemode": false}}},
			"plugins": []string{pluginPath},
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

// writePlugin writes opencode.js for this hypercube executable and returns the
// directory OpenCode loads it from.
func writePlugin(exe, stateDir string) (string, error) {
	text := []byte(strings.ReplaceAll(source, "HYPERCUBE_EXE", plugin.JSONString(exe)))
	path, err := plugin.WriteFile(filepath.Join(stateDir, "opencode-plugin"), "index.js", text)
	if err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}
