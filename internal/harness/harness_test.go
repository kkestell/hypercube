package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestOpenCodeModelSelection(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is required to exercise the OpenCode plugin")
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, model := range []string{"openrouter/meta/muse-spark-1.3-contributor", "meta/muse-spark-1.3-contributor"} {
		t.Run(model, func(t *testing.T) {
			_, env, err := Command(store.Agent{Harness: "opencode", Model: model, Effort: "medium"}, "/tmp/test-state.json", "Check your Hypercube messages.")
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bun, "--eval", `
import assert from "node:assert/strict"
const { default: plugin } = await import(process.env.HYPERCUBE_TEST_PLUGIN)
const requested = process.env.HYPERCUBE_TEST_MODEL
assert.equal(JSON.parse(process.env.OPENCODE_CONFIG_CONTENT).model, requested)
const [providerID, ...rest] = requested.split("/")
const wanted = { providerID, id: rest.join("/") }
const fallback = { providerID: "opencode", id: "deepseek-v4.1-flash-free" }
let selected = fallback
const calls = []
let fail
const failure = new Promise((resolve) => { fail = resolve })
Bun.spawn = (argv) => {
  calls.push(argv[2])
  return { stdout: new Response("{}").body, exited: Promise.resolve(0) }
}
let prompt
const cleanup = await plugin.setup({
  session: {
    get: async ({ sessionID }) => ({ model: selected, parentID: sessionID === "child" ? "parent" : undefined }),
    hook: async (event, callback) => { if (event === "prompt") prompt = callback },
  },
  tool: { hook: async () => {} },
  event: { subscribe: async function* () { yield await failure } },
})
const send = (sessionID) => prompt({ sessionID, prompt: { text: "hello" } })
try {
  // Native subagents keep their own models.
  await send("child")
  assert.deepEqual(calls, [])

  // An unavailable default must not silently run the prompt on a recent model.
  await assert.rejects(send("top"), (err) => {
    assert.match(err.message, /OpenCode selected/)
    assert.ok(err.message.includes(requested), err.message)
    assert.ok(err.message.includes("opencode/deepseek-v4.1-flash-free"), err.message)
    return true
  })
  assert.deepEqual(calls, [], "a rejected model must not consume queued messages")
  fail({ type: "session.execution.failed", data: { sessionID: "top" } })
  await new Promise((resolve) => setTimeout(resolve, 0))
  assert.deepEqual(calls, [], "a rejected startup must not deliver messages through Stop")

  // Correcting the selection retries SessionStart and delivers the first message.
  selected = wanted
  await send("top")
  assert.deepEqual(calls, ["SessionStart", "UserPromptSubmit"])

  // Later manual model changes remain possible within the running session.
  selected = fallback
  await send("top")
  assert.deepEqual(calls, ["SessionStart", "UserPromptSubmit", "UserPromptSubmit"])
} finally {
  cleanup()
}
`)
			cmd.Env = os.Environ()
			for key, value := range env {
				cmd.Env = append(cmd.Env, key+"="+value)
			}
			cmd.Env = append(cmd.Env,
				"HYPERCUBE_TEST_MODEL="+model,
				"HYPERCUBE_TEST_PLUGIN="+filepath.Join(store.Dir(), "opencode-plugin", "index.js"))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("OpenCode model selection: %v\n%s", err, out)
			}
		})
	}
}

func TestOxCommand(t *testing.T) {
	argv, env, err := Command(store.Agent{ID: "a1", Harness: "ox", Model: "openrouter:x/y", Effort: "high", SessionID: "s1"}, "/tmp/w.json", "Check your Hypercube messages.")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(argv, " "); got != "ox --resume s1 -- Check your Hypercube messages." {
		t.Errorf("argv = %q", got)
	}
	var settings struct {
		Model, Effort, Mode string
		MCPServers          map[string]struct {
			Args []string
			Env  map[string]string
		} `json:"mcpServers"`
		Hooks map[string][]struct {
			Hooks []struct{ Type, Command string }
		}
	}
	if err := json.Unmarshal([]byte(env["OX_SETTINGS"]), &settings); err != nil {
		t.Fatal(err)
	}
	server := settings.MCPServers["hypercube"]
	if settings.Model != "openrouter:x/y" || settings.Effort != "high" || settings.Mode != "auto" ||
		strings.Join(server.Args, " ") != "mcp" || server.Env["HYPERCUBE_AGENT_ID"] != "a1" || server.Env["OX_SETTINGS"] != "" {
		t.Errorf("OX_SETTINGS = %s", env["OX_SETTINGS"])
	}
	for _, event := range events {
		if hooks := settings.Hooks[event]; len(hooks) != 1 || !strings.HasSuffix(hooks[0].Hooks[0].Command, "' hook "+event) {
			t.Errorf("%s hooks = %+v", event, hooks)
		}
	}
}

func TestCodexTrustOutsideGit(t *testing.T) {
	dir := t.TempDir()
	argv, _, err := Command(store.Agent{ID: "a1", Harness: "codex", Model: "m", Effort: "high", Dir: dir}, "/tmp/w.json", "")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("projects={%q={trust_level=\"trusted\"}}", dir)
	if !slices.Contains(argv, want) {
		t.Errorf("argv lacks %s: %q", want, argv)
	}
}
