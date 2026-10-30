package harness

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestCommandsPreserveNativeHarnessIntegration(t *testing.T) {
	for _, kind := range []string{"claude", "opencode"} {
		t.Run(kind, func(t *testing.T) {
			a := store.Agent{ID: "w1", Name: "worker", Role: store.Worker, Harness: kind, Model: "p/m", Effort: "high", SessionID: "saved"}
			dir := t.TempDir()
			args, env, err := command("/tmp/with a ' quote/hypercube", a, filepath.Join(dir, "w.json"), "first prompt")
			if err != nil {
				t.Fatal(err)
			}
			if args[0] != kind || args[len(args)-1] != "first prompt" || env["HYPERCUBE_AGENT_ID"] != "w1" {
				t.Fatalf("launch = %q, %v", args, env)
			}
			if kind == "claude" {
				if !slices.Contains(args, "--resume") || !slices.Contains(args, "saved") || !slices.Contains(args, "auto") {
					t.Fatal(args)
				}
				index := slices.Index(args, "--settings")
				var settings struct {
					Hooks map[string][]struct {
						Hooks []map[string]any `json:"hooks"`
					} `json:"hooks"`
					StatusLine map[string]string `json:"statusLine"`
				}
				if err := json.Unmarshal([]byte(args[index+1]), &settings); err != nil {
					t.Fatal(err)
				}
				for _, event := range events {
					hooks := settings.Hooks[event][0].Hooks
					if !strings.Contains(hooks[0]["command"].(string), "hook "+event) {
						t.Fatal(hooks)
					}
					if event == "SessionStart" || event == "Stop" || event == "StopFailure" {
						if len(hooks) != 2 || hooks[1]["asyncRewake"] != true || hooks[1]["timeout"] != float64(1728000) {
							t.Fatal(hooks)
						}
					}
				}
				if strings.Contains(settings.StatusLine["command"], "--limits") {
					t.Fatal("worker got limits")
				}
			} else {
				if !slices.Contains(args, "--standalone") || !slices.Contains(args, "--auto") || env["HYPERCUBE_SESSION_ID"] != "saved" {
					t.Fatal(args, env)
				}
				var config map[string]any
				if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &config); err != nil {
					t.Fatal(err)
				}
				if config["model"] != "p/m" || env["OPENCODE_CLI_CONFIG_CONTENT"] != `{"tabs":{"mode":"off"}}` {
					t.Fatal(config)
				}
				source, err := os.ReadFile(filepath.Join(dir, "opencode-plugin/index.js"))
				if err != nil || strings.Contains(string(source), "HYPERCUBE_EXE") {
					t.Fatalf("plugin = %s, %v", source, err)
				}
			}
		})
	}
}

func TestConfigProfiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "hypercube/config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	valid := `{"orchestrator":{"harness":"claude","model":"opus","effort":"high"},"profiles":[{"name":"fast","description":"Small jobs","harness":"opencode","model":"p/m","effort":"low"}]}`
	for _, source := range []string{valid, strings.Replace(valid, `"opencode"`, `"pi"`, 1), strings.Replace(valid, `"Small jobs"`, `""`, 1), strings.Replace(valid, `"opus"`, `""`, 1), `{`} {
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := LoadConfig()
		if source == valid {
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Profile("fast"); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Profile("missing"); err == nil {
				t.Fatal("missing profile accepted")
			}
		} else if err == nil {
			t.Fatalf("invalid config accepted: %s", source)
		}
	}
}

func TestOpenCodePlugin(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun is required for the plugin test")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake hypercube")
	source, err := os.ReadFile("testdata/fake-hypercube.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fake, source, 0o755); err != nil {
		t.Fatal(err)
	}
	a := store.Agent{ID: "w1", Harness: "opencode", Model: "p/m", Effort: "high", SessionID: "s1"}
	_, env, err := command(fake, a, filepath.Join(dir, "state.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bun", "testdata/opencode-plugin.mjs")
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Env = append(cmd.Env, "HYPERCUBE_TEST_LOG="+filepath.Join(dir, "hooks.log"), "HYPERCUBE_TEST_PLUGIN="+filepath.Join(dir, "opencode-plugin/index.js"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin: %v\n%s", err, output)
	}
}
