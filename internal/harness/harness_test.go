package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestPiCommand(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	extension := filepath.Join(store.Dir(), "pi-extension", "index.js")
	base := []string{"pi", "--extension", extension, "--model", "m", "--thinking", "high", "--approve"}
	for _, resume := range []bool{false, true} {
		t.Run(fmt.Sprint(resume), func(t *testing.T) {
			a := store.Agent{ID: "a1", Harness: "pi", Model: "m", Effort: "high"}
			prompt := ""
			want := slices.Clone(base)
			if resume {
				a.SessionID = "s1"
				prompt = "Check your Hypercube messages."
				want = append(want, "--session-id", "s1", prompt)
			}
			argv, env, err := Command(a, "/tmp/w.json", prompt)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(argv, want) {
				t.Errorf("argv = %q, want %q", argv, want)
			}
			if env["HYPERCUBE_AGENT_ID"] != "a1" || env["HYPERCUBE_STATE"] != "/tmp/w.json" {
				t.Errorf("env = %v", env)
			}
			data, err := os.ReadFile(extension)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "HYPERCUBE_EXE") {
				t.Error("extension contains unreplaced executable placeholder")
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "const exe = "+strconv.Quote(exe)) {
				t.Error("extension does not contain the executable path")
			}
		})
	}
}

func TestPiExtension(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to exercise the Pi extension")
	}
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	t.Setenv("HYPERCUBE_AGENT_ID", "a1")
	t.Setenv("HYPERCUBE_STATE", filepath.Join(dir, "state.json"))
	t.Setenv("TMUX", "/tmp/pi-test,1,0")
	t.Setenv("HYPERCUBE_TEST_LOG", filepath.Join(dir, "hooks.log"))
	fake := filepath.Join(dir, "fake hypercube")
	if err := os.WriteFile(fake, []byte(`#!/bin/sh
input=$(cat)
printf '%s %s\n' "$2" "$input" >> "$HYPERCUBE_TEST_LOG"
if [ "$2" = Stop ]; then
  if [ ! -f "$HYPERCUBE_TEST_LOG.stopped" ]; then
    touch "$HYPERCUBE_TEST_LOG.stopped"
    printf '%s' '{"decision":"block","reason":"queued"}'
  fi
else
  printf '{"hookSpecificOutput":{"additionalContext":"%s context"}}' "$2"
fi
`), 0o755); err != nil {
		t.Fatal(err)
	}
	extension, err := writePiExtension(fake)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--input-type=module", "--eval", `
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { pathToFileURL } from "node:url"
const { default: extension } = await import(pathToFileURL(process.env.HYPERCUBE_TEST_EXTENSION))
const handlers = new Map()
const sent = []
const servers = []
extension({
  on: (event, handler) => { handlers.set(event, handler) },
  registerMcpServer: (...args) => { servers.push(args) },
  sendUserMessage: (text) => { sent.push(text) },
})
assert.deepEqual(servers, [["hypercube", {
  command: process.env.HYPERCUBE_TEST_EXE,
  args: ["mcp"],
  env: {
    HYPERCUBE_AGENT_ID: "a1",
    HYPERCUBE_STATE: process.env.HYPERCUBE_STATE,
    TMUX: process.env.TMUX,
  },
  exposure: "direct",
}]])
const ctx = { sessionManager: { getSessionId: () => "s1" } }
const call = (name, event = {}, context = ctx) => handlers.get(name)(event, context)
const calls = () => readFileSync(process.env.HYPERCUBE_TEST_LOG, "utf8").trim().split("\n").map((line) => {
  const space = line.indexOf(" ")
  return [line.slice(0, space), JSON.parse(line.slice(space + 1))]
})
assert.deepEqual(await call("before_agent_start"), {
  message: { customType: "hypercube", content: "SessionStart context\n\nUserPromptSubmit context", display: true },
})
assert.deepEqual(calls(), [["SessionStart", { session_id: "s1" }], ["UserPromptSubmit", { session_id: "s1" }]])
assert.deepEqual(await call("before_agent_start"), {
  message: { customType: "hypercube", content: "UserPromptSubmit context", display: true },
})
assert.deepEqual(calls().map(([event]) => event), ["SessionStart", "UserPromptSubmit", "UserPromptSubmit"])
const content = [{ type: "text", text: "original" }]
assert.equal(await call("tool_result", { content, parentToolCallId: "parent" }), undefined)
assert.equal(calls().length, 3, "nested tools must not consume queued messages")
assert.deepEqual(await call("tool_result", { content }), {
  content: [...content, { type: "text", text: "PostToolUse context" }],
})
assert.deepEqual(content, [{ type: "text", text: "original" }])
await call("agent_settled")
assert.deepEqual(sent, ["queued"])
await call("agent_settled")
assert.deepEqual(sent, ["queued"], "empty Stop output must not start another run")
assert.deepEqual(calls(), ["SessionStart", "UserPromptSubmit", "UserPromptSubmit", "PostToolUse", "Stop", "Stop"].map((event) => [event, { session_id: "s1" }]))

// Switching sessions runs SessionStart again, while returning to one does not.
await call("before_agent_start", {}, { sessionManager: { getSessionId: () => "s2" } })
assert.deepEqual(calls().slice(-2), [["SessionStart", { session_id: "s2" }], ["UserPromptSubmit", { session_id: "s2" }]])
await call("before_agent_start")
assert.deepEqual(calls().at(-1), ["UserPromptSubmit", { session_id: "s1" }])
assert.equal(calls().length, 9)
`)
	cmd.Env = append(os.Environ(), "HYPERCUBE_TEST_EXTENSION="+extension, "HYPERCUBE_TEST_EXE="+fake)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Pi extension: %v\n%s", err, out)
	}
}

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

func TestLoadConfigProfiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "hypercube"), 0o755)
	const orch = `"orchestrator": {"harness": "claude", "model": "opus", "effort": "high"}`
	const sol = `{"name": "sol", "description": "coding.", "harness": "codex", "model": "gpt-6.1-sol", "effort": "high"}`
	for profiles, ok := range map[string]bool{
		`[` + sol + `]`:             true,
		`[]`:                        false,
		`[` + sol + `,` + sol + `]`: false,
		`[{"name": "sol", "harness": "codex", "model": "gpt-6.1-sol", "effort": "high"}]`:                         false,
		`[{"name": "sol", "description": "coding.", "harness": "gpt", "model": "gpt-6.1-sol", "effort": "high"}]`: false,
	} {
		os.WriteFile(filepath.Join(dir, "hypercube", "config.json"), []byte(`{`+orch+`, "profiles": `+profiles+`}`), 0o644)
		c, err := LoadConfig()
		if (err == nil) != ok {
			t.Errorf("profiles %s: err = %v", profiles, err)
		}
		if ok {
			if p, err := c.Profile("sol"); err != nil || p.Model != "gpt-6.1-sol" {
				t.Errorf("Profile(sol) = %+v, %v", p, err)
			}
		}
	}
}

func TestLoadConfigPi(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "hypercube"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{"orchestrator":{"harness":"pi","model":"openai/gpt-6.1-sol","effort":"high"},"profiles":[{"name":"pi","description":"coding","harness":"pi","model":"openrouter/~deepseek/deepseek-flash-latest","effort":"medium"}]}`
	if err := os.WriteFile(filepath.Join(dir, "hypercube", "config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Orchestrator.Harness != "pi" || len(c.Profiles) != 1 || c.Profiles[0].Harness != "pi" {
		t.Errorf("config = %+v", c)
	}
	if err := (Spec{Harness: "unknown"}).check(); err == nil || !strings.Contains(err.Error(), "ox, or pi") {
		t.Errorf("unknown harness error = %v", err)
	}
}
