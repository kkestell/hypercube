//! The launch command of each harness, Claude Code and OpenCode.

use std::collections::BTreeMap;
use std::error::Error;
use std::path::{Path, PathBuf};

use serde_json::{Value, json};

use crate::store::{Agent, ORCHESTRATOR};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// The hook events Claude Code runs `hypercube hook` for. The OpenCode plugin
/// calls all but PermissionRequest.
const EVENTS: [&str; 5] = [
    "SessionStart",
    "UserPromptSubmit",
    "PostToolUse",
    "Stop",
    "PermissionRequest",
];

/// The events after which Claude Code starts `hypercube wait` in the
/// background: the start of a session and the end of each turn. The waiter's
/// exit status 2 wakes the session with the messages it wrote.
const WAIT_EVENTS: [&str; 2] = ["SessionStart", "Stop"];

/// How long Claude Code lets a waiter run: 20 days.
const WAIT_TIMEOUT_SECONDS: u64 = 20 * 24 * 60 * 60;

const OPENCODE_PLUGIN: &str = include_str!("harness/opencode.js");

/// What to run for an agent: the harness's argv, the environment the host adds
/// to its own, and the directory to run it in.
#[derive(Clone, Debug, PartialEq)]
pub struct Launch {
    pub argv: Vec<String>,
    pub env: BTreeMap<String, String>,
    pub cwd: PathBuf,
}

/// The command that launches an agent's harness. `exe` is the hypercube
/// executable that the harness runs as its MCP server and hook command. A
/// non-empty `prompt` starts the first turn. An OpenCode launch writes its
/// plugin next to the state file.
pub fn command(exe: &Path, agent: &Agent, state_path: &Path, prompt: &str) -> Result<Launch> {
    let exe_str = utf8(exe)?;
    let mut env = BTreeMap::from([
        ("HYPERCUBE_AGENT_ID".to_string(), agent.id.clone()),
        ("HYPERCUBE_STATE".to_string(), utf8(state_path)?.to_string()),
    ]);
    let mut argv: Vec<String> = match agent.harness.as_str() {
        "claude" => {
            let mcp = json!({"mcpServers": {"hypercube": {
                "command": exe_str,
                "args": ["mcp"],
                "env": env,
            }}});
            let hooks: BTreeMap<_, _> = EVENTS
                .iter()
                .map(|e| {
                    let command = format!("{} hook {e}", shell_quote(exe_str));
                    let mut hooks = vec![json!({"type": "command", "command": command})];
                    if WAIT_EVENTS.contains(e) {
                        hooks.push(json!({
                            "type": "command",
                            "command": format!("{} wait", shell_quote(exe_str)),
                            "asyncRewake": true,
                            "timeout": WAIT_TIMEOUT_SECONDS,
                        }));
                    }
                    (*e, json!([{"hooks": hooks}]))
                })
                .collect();
            // The orchestrator's status line also shows the usage limits.
            let mut statusline = format!("{} statusline", shell_quote(exe_str));
            if agent.role == ORCHESTRATOR {
                statusline.push_str(" --limits");
            }
            let settings = json!({
                "hooks": hooks,
                "statusLine": {"type": "command", "command": statusline},
            });
            let mut argv = strings(["claude", "--mcp-config"]);
            argv.extend([
                mcp.to_string(),
                "--settings".into(),
                settings.to_string(),
                "--model".into(),
                agent.model.clone(),
                "--effort".into(),
                agent.effort.clone(),
                "--permission-mode".into(),
                "auto".into(),
            ]);
            if !agent.session_id.is_empty() {
                argv.extend(["--resume".into(), agent.session_id.clone()]);
            }
            argv
        }
        "opencode" => {
            let plugin = write_plugin(exe_str, state_path)?;
            // OpenCode creates a session with a missing ID on its first
            // prompt, so the plugin knows the session from the start.
            let session = if agent.session_id.is_empty() {
                format!("ses_hypercube_{}", agent.id)
            } else {
                agent.session_id.clone()
            };
            let config = json!({
                "model": agent.model,
                "agents": {"build": {"model": format!("{}#{}", agent.model, agent.effort)}},
                "mcp": {"servers": {"hypercube": {
                    "type": "local",
                    "command": [exe_str, "mcp"],
                    "environment": env,
                    "codemode": false,
                }}},
                "plugins": [utf8(&plugin)?],
            });
            env.insert("OPENCODE_CONFIG_CONTENT".into(), config.to_string());
            env.insert("HYPERCUBE_SESSION_ID".into(), session.clone());
            let mut argv = strings(["opencode", "--standalone", "--auto", "--session"]);
            argv.push(session);
            if !prompt.is_empty() {
                argv.push("--prompt".into());
            }
            argv
        }
        other => {
            return Err(format!(
                "agent {:?} has harness {other:?}, which is not supported; use claude or opencode",
                agent.name
            )
            .into());
        }
    };
    if !prompt.is_empty() {
        argv.push(prompt.to_string());
    }
    Ok(Launch {
        argv,
        env,
        cwd: PathBuf::from(&agent.dir),
    })
}

fn strings<const N: usize>(items: [&str; N]) -> Vec<String> {
    items.map(String::from).to_vec()
}

fn utf8(path: &Path) -> Result<&str> {
    path.to_str()
        .ok_or_else(|| format!("{} is not valid UTF-8", path.display()).into())
}

fn shell_quote(s: &str) -> String {
    format!("'{}'", s.replace('\'', r"'\''"))
}

/// Write the OpenCode plugin, with the executable path embedded, to the state
/// directory and return its directory. The file is renamed into place, so a harness
/// starting meanwhile never reads a half-written copy, and a current copy is
/// left alone.
fn write_plugin(exe: &str, state_path: &Path) -> Result<PathBuf> {
    let dir = state_path
        .parent()
        .unwrap_or(Path::new(""))
        .join("opencode-plugin");
    std::fs::create_dir_all(&dir).map_err(|e| format!("create {}: {e}", dir.display()))?;
    let path = dir.join("index.js");
    let source = OPENCODE_PLUGIN.replace("HYPERCUBE_EXE", &Value::from(exe).to_string());
    if std::fs::read_to_string(&path).ok().as_deref() != Some(&source) {
        let temp = dir.join(format!("index.js.{}.tmp", std::process::id()));
        std::fs::write(&temp, source).map_err(|e| format!("write {}: {e}", temp.display()))?;
        std::fs::rename(&temp, &path).map_err(|e| format!("rename {}: {e}", temp.display()))?;
    }
    Ok(dir)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testutil::TempDir;
    use std::os::unix::fs::PermissionsExt;
    use std::process::Command;

    const EXE: &str = "/opt/hyper cube/hypercube";
    const NUDGE: &str = "Check your Hypercube messages.";

    fn agent(harness: &str, session_id: &str) -> Agent {
        Agent {
            id: "a1".into(),
            name: "w1".into(),
            role: "worker".into(),
            harness: harness.into(),
            model: "m".into(),
            effort: "high".into(),
            dir: "/r/.hypercube/w1".into(),
            session_id: session_id.into(),
            ..Agent::default()
        }
    }

    fn launch(state: &Path, agent: &Agent, prompt: &str) -> Launch {
        command(Path::new(EXE), agent, state, prompt).unwrap()
    }

    fn json_arg(argv: &[String], flag: &str) -> Value {
        let i = argv.iter().position(|a| a == flag).unwrap();
        serde_json::from_str(&argv[i + 1]).unwrap()
    }

    #[test]
    fn claude_argv_and_environment() {
        let state = Path::new("/tmp/w.json");
        let mut l = launch(state, &agent("claude", ""), "");
        assert_eq!(l.argv.len(), 11);
        let (mcp, settings) = (
            json_arg(&l.argv, "--mcp-config"),
            json_arg(&l.argv, "--settings"),
        );
        l.argv[2] = "<mcp>".into();
        l.argv[4] = "<settings>".into();
        assert_eq!(
            l.argv,
            [
                "claude",
                "--mcp-config",
                "<mcp>",
                "--settings",
                "<settings>",
                "--model",
                "m",
                "--effort",
                "high",
                "--permission-mode",
                "auto",
            ]
        );
        assert_eq!(
            l.env,
            BTreeMap::from([
                ("HYPERCUBE_AGENT_ID".to_string(), "a1".to_string()),
                ("HYPERCUBE_STATE".to_string(), "/tmp/w.json".to_string()),
            ])
        );
        assert_eq!(l.cwd, Path::new("/r/.hypercube/w1"));

        assert_eq!(
            mcp,
            json!({"mcpServers": {"hypercube": {
                "command": EXE,
                "args": ["mcp"],
                "env": {"HYPERCUBE_AGENT_ID": "a1", "HYPERCUBE_STATE": "/tmp/w.json"},
            }}})
        );
        let hook =
            |event: &str| json!({"type": "command", "command": format!("'{EXE}' hook {event}")});
        let wait = json!({"type": "command", "command": format!("'{EXE}' wait"),
            "asyncRewake": true, "timeout": 1_728_000});
        assert_eq!(
            settings,
            json!({
                "hooks": {
                    "SessionStart": [{"hooks": [hook("SessionStart"), wait]}],
                    "UserPromptSubmit": [{"hooks": [hook("UserPromptSubmit")]}],
                    "PostToolUse": [{"hooks": [hook("PostToolUse")]}],
                    "Stop": [{"hooks": [hook("Stop"), wait]}],
                    "PermissionRequest": [{"hooks": [hook("PermissionRequest")]}],
                },
                "statusLine": {"type": "command", "command": format!("'{EXE}' statusline")},
            })
        );
    }

    #[test]
    fn the_orchestrator_status_line_shows_the_limits() {
        let mut a = agent("claude", "");
        a.role = ORCHESTRATOR.into();
        let settings = json_arg(&launch(Path::new("/tmp/w.json"), &a, "").argv, "--settings");
        assert_eq!(
            settings["statusLine"]["command"],
            format!("'{EXE}' statusline --limits")
        );
    }

    #[test]
    fn claude_resume_and_prompt() {
        let state = Path::new("/tmp/w.json");
        let argv = |session_id: &str, prompt: &str| {
            launch(state, &agent("claude", session_id), prompt).argv
        };
        let (mode, auto) = ("--permission-mode", "auto");
        assert_eq!(
            argv("s1", NUDGE)[9..],
            [mode, auto, "--resume", "s1", NUDGE]
        );
        assert_eq!(argv("s1", "")[9..], [mode, auto, "--resume", "s1"]);
        assert_eq!(argv("", NUDGE)[9..], [mode, auto, NUDGE]);
        assert_eq!(argv("", "")[9..], [mode, auto]);
    }

    #[test]
    fn hook_commands_quote_the_executable() {
        let state = Path::new("/tmp/w.json");
        let l = command(
            Path::new("/it's/hypercube"),
            &agent("claude", ""),
            state,
            "",
        )
        .unwrap();
        let settings = json_arg(&l.argv, "--settings");
        assert_eq!(
            settings["hooks"]["Stop"][0]["hooks"][0]["command"],
            r"'/it'\''s/hypercube' hook Stop"
        );
    }

    #[test]
    fn opencode_argv_environment_and_plugin() {
        let dir = TempDir::new();
        let state = dir.path().join("w.json");
        let plugin = dir.path().join("opencode-plugin");
        let base = ["opencode", "--standalone", "--auto", "--session"];

        let l = launch(&state, &agent("opencode", ""), "");
        assert_eq!(l.argv, [&base[..], &["ses_hypercube_a1"]].concat());
        assert_eq!(l.env["HYPERCUBE_AGENT_ID"], "a1");
        assert_eq!(l.env["HYPERCUBE_STATE"], state.to_str().unwrap());
        assert_eq!(l.env["HYPERCUBE_SESSION_ID"], "ses_hypercube_a1");
        assert_eq!(l.env.len(), 4);
        assert_eq!(l.cwd, Path::new("/r/.hypercube/w1"));
        let config: Value = serde_json::from_str(&l.env["OPENCODE_CONFIG_CONTENT"]).unwrap();
        assert_eq!(
            config,
            json!({
                "model": "m",
                "agents": {"build": {"model": "m#high"}},
                "mcp": {"servers": {"hypercube": {
                    "type": "local",
                    "command": [EXE, "mcp"],
                    "environment": {"HYPERCUBE_AGENT_ID": "a1", "HYPERCUBE_STATE": state.to_str().unwrap()},
                    "codemode": false,
                }}},
                "plugins": [plugin.to_str().unwrap()],
            })
        );

        let l = launch(&state, &agent("opencode", ""), NUDGE);
        assert_eq!(
            l.argv,
            [&base[..], &["ses_hypercube_a1", "--prompt", NUDGE]].concat()
        );

        let l = launch(&state, &agent("opencode", "s1"), "");
        assert_eq!(l.argv, [&base[..], &["s1"]].concat());
        assert_eq!(l.env["HYPERCUBE_SESSION_ID"], "s1");

        let source = std::fs::read_to_string(plugin.join("index.js")).unwrap();
        assert!(!source.contains("HYPERCUBE_EXE"), "unreplaced placeholder");
        assert!(
            source.contains(&format!("const exe = {:?}\n", EXE)),
            "{source}"
        );
    }

    #[test]
    fn opencode_plugin_is_written_once_and_updated_when_the_executable_changes() {
        let dir = TempDir::new();
        let state = dir.path().join("w.json");
        let a = agent("opencode", "");
        let plugin = dir.path().join("opencode-plugin/index.js");
        launch(&state, &a, "");
        let first = std::fs::metadata(&plugin).unwrap().modified().unwrap();
        launch(&state, &a, "");
        assert_eq!(
            std::fs::metadata(&plugin).unwrap().modified().unwrap(),
            first
        );
        let names: Vec<_> = std::fs::read_dir(plugin.parent().unwrap())
            .unwrap()
            .map(|e| e.unwrap().file_name())
            .collect();
        assert_eq!(names, ["index.js"], "a temporary file remains");

        command(Path::new("/other/hypercube"), &a, &state, "").unwrap();
        let source = std::fs::read_to_string(&plugin).unwrap();
        assert!(
            source.contains("const exe = \"/other/hypercube\"\n"),
            "{source}"
        );
    }

    #[test]
    fn unsupported_harnesses_fail_to_launch() {
        let state = Path::new("/tmp/w.json");
        for harness in ["codex", "pi", "ox", "gpt", ""] {
            let err = command(Path::new(EXE), &agent(harness, ""), state, "")
                .unwrap_err()
                .to_string();
            assert_eq!(
                err,
                format!(
                    "agent \"w1\" has harness \"{harness}\", which is not supported; use claude or opencode"
                )
            );
        }
    }

    #[test]
    fn shell_quoting() {
        assert_eq!(shell_quote("a b"), "'a b'");
        assert_eq!(shell_quote("it's"), r"'it'\''s'");
    }

    /// Runs the plugin under Node against a fake hypercube executable and a
    /// stand-in for OpenCode's plugin context.
    #[test]
    fn opencode_plugin_maps_events_to_hooks() {
        if Command::new("node").arg("--version").output().is_err() {
            eprintln!("skipped: node is required to exercise the OpenCode plugin");
            return;
        }
        let dir = TempDir::new();
        let fake = dir.path().join("fake hypercube");
        std::fs::write(
            &fake,
            r#"#!/bin/sh
# The first waiter writes a message; later ones wait until they are killed.
if [ "$1" = wait ]; then
  echo $$ >> "$HYPERCUBE_TEST_LOG.waits"
  if [ "$(wc -l < "$HYPERCUBE_TEST_LOG.waits")" -eq 1 ]; then
    printf 'Message from o (o1):\nhi\n' >&2
    exit 2
  fi
  exec sleep 30
fi
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
"#,
        )
        .unwrap();
        std::fs::set_permissions(&fake, std::fs::Permissions::from_mode(0o755)).unwrap();
        let state = dir.path().join("state.json");
        let mut a = agent("opencode", "s1");
        a.model = "p/m".into();
        let l = command(&fake, &a, &state, "").unwrap();

        let out = Command::new("node")
            .args(["--input-type=module", "--eval", OPENCODE_PLUGIN_TEST])
            .envs(&l.env)
            .env("HYPERCUBE_TEST_LOG", dir.path().join("hooks.log"))
            .env(
                "HYPERCUBE_TEST_PLUGIN",
                dir.path().join("opencode-plugin/index.js"),
            )
            .output()
            .unwrap();
        assert!(
            out.status.success(),
            "OpenCode plugin:\n{}{}",
            String::from_utf8_lossy(&out.stdout),
            String::from_utf8_lossy(&out.stderr)
        );
    }

    const OPENCODE_PLUGIN_TEST: &str = r##"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { pathToFileURL } from "node:url"
const { default: plugin } = await import(pathToFileURL(process.env.HYPERCUBE_TEST_PLUGIN))

// A stand-in for OpenCode's plugin context. s1 is the launched session, s2
// is on another model, and child is a subagent's session.
const model = { providerID: "p", id: "m" }
const sessions = { s1: { model }, s2: { model: { providerID: "p", id: "other" } }, child: { model, parentID: "s1" } }
const hooks = new Map()
const prompts = []
const events = []
let push
const ctx = {
  session: {
    get: async ({ sessionID }) => {
      if (!sessions[sessionID]) throw new Error(`no session ${sessionID}`)
      return sessions[sessionID]
    },
    hook: async (name, callback) => { hooks.set(`session.${name}`, callback) },
    prompt: async (input) => { prompts.push(input) },
  },
  tool: { hook: async (name, callback) => { hooks.set(`tool.${name}`, callback) } },
  event: {
    subscribe: async function* ({ signal }) {
      while (!signal.aborted) {
        if (events.length) yield events.shift()
        else await new Promise((resolve) => { push = resolve; signal.addEventListener("abort", resolve) })
      }
    },
  },
}
const emit = (type, sessionID) => { events.push({ type, data: { sessionID } }); push?.() }
const until = async (what, f) => {
  for (let i = 0; i < 500; i++) {
    if (f()) return
    await new Promise((resolve) => setTimeout(resolve, 10))
  }
  assert.fail(`timed out waiting for ${what}`)
}
const lines = (path) => {
  try { return readFileSync(path, "utf8").trim().split("\n") } catch { return [] }
}
const calls = () => lines(process.env.HYPERCUBE_TEST_LOG).map((line) => {
  const space = line.indexOf(" ")
  return [line.slice(0, space), JSON.parse(line.slice(space + 1)).session_id]
})
const waits = () => lines(`${process.env.HYPERCUBE_TEST_LOG}.waits`).map(Number)
const alive = (pid) => {
  try { process.kill(pid, 0); return true } catch { return false }
}

// The launched session exists, so its waiter starts, and its messages start
// a turn there.
const cleanup = await plugin.setup(ctx)
await until("the waiter's messages", () => prompts.length === 1)
assert.deepEqual(prompts, [{ sessionID: "s1", text: "Message from o (o1):\nhi" }])

// The first prompt of a session runs SessionStart, and every prompt runs
// UserPromptSubmit. Their context is added to the prompt.
const prompt = async (sessionID, text) => {
  const event = { sessionID, prompt: { text } }
  await hooks.get("session.prompt")(event)
  return event.prompt.text
}
assert.equal(await prompt("s1", "hello"), "hello\n\nSessionStart context\n\nUserPromptSubmit context")
assert.equal(await prompt("s1", "again"), "again\n\nUserPromptSubmit context")
assert.deepEqual(calls(), [["SessionStart", "s1"], ["UserPromptSubmit", "s1"], ["UserPromptSubmit", "s1"]])

// A session on another model is refused before any hook runs.
await assert.rejects(prompt("s2", "x"), /OpenCode selected p\/other instead of requested p\/m/)

// A subagent's prompts and tools never consume queued messages.
assert.equal(await prompt("child", "sub"), "sub")
const tool = async (event) => {
  await hooks.get("tool.execute.after")(event)
  return event.result
}
const result = { output: 1, content: "original" }
assert.equal(await tool({ status: "completed", sessionID: "child", result }), result)
assert.equal(await tool({ status: "error", sessionID: "s1", error: {} }), undefined)
assert.equal(calls().length, 3)

// A completed tool's result gets PostToolUse context.
assert.deepEqual(await tool({ status: "completed", sessionID: "s1", result }), {
  output: 1,
  content: [{ type: "text", text: "original" }, { type: "text", text: "PostToolUse context" }],
})
assert.equal(result.content, "original")

// The end of a turn runs Stop. A block reason starts a turn with it;
// otherwise a waiter starts. Sessions without a prompt here are ignored.
emit("session.execution.succeeded", "child")
emit("session.execution.succeeded", "s1")
await until("the blocked stop's turn", () => prompts.length === 2)
assert.deepEqual(prompts[1], { sessionID: "s1", text: "queued" })
assert.equal(waits().length, 1, "a blocked stop continues the turn, so no waiter starts")
emit("session.execution.interrupted", "s1")
await until("a waiter after the stop", () => waits().length === 2)
assert.equal(prompts.length, 2)
assert.deepEqual(calls().slice(3), [["PostToolUse", "s1"], ["Stop", "s1"], ["Stop", "s1"]])

// Cleanup ends the waiter.
const waiter = waits()[1]
assert(alive(waiter))
cleanup()
await until("the waiter to end", () => !alive(waiter))
"##;
}
