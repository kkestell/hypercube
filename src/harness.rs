//! The launch command of each harness, Claude Code and Pi.

use std::collections::BTreeMap;
use std::error::Error;
use std::path::{Path, PathBuf};

use serde_json::{Value, json};

use crate::store::Agent;
use crate::workspace;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// The hook events every harness reports.
pub const EVENTS: [&str; 4] = ["SessionStart", "UserPromptSubmit", "PostToolUse", "Stop"];

const PI_EXTENSION: &str = include_str!("harness/pi.js");

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
/// non-empty `prompt` starts the first turn. A Pi launch writes its extension
/// next to the state file.
pub fn command(exe: &Path, agent: &Agent, state_path: &Path, prompt: &str) -> Result<Launch> {
    let exe_str = utf8(exe)?;
    let env = BTreeMap::from([
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
                    (
                        *e,
                        json!([{"hooks": [{"type": "command", "command": command}]}]),
                    )
                })
                .collect();
            let settings = json!({"hooks": hooks});
            let mut argv = strings(["claude", "--mcp-config"]);
            argv.extend([
                mcp.to_string(),
                "--settings".into(),
                settings.to_string(),
                "--model".into(),
                agent.model.clone(),
                "--effort".into(),
                agent.effort.clone(),
                "--dangerously-skip-permissions".into(),
            ]);
            if !agent.session_id.is_empty() {
                argv.extend(["--resume".into(), agent.session_id.clone()]);
            }
            argv
        }
        "pi" => {
            let extension = write_pi_extension(exe_str, state_path)?;
            let mut argv = strings(["pi", "--extension"]);
            argv.extend([
                utf8(&extension)?.to_string(),
                "--model".into(),
                agent.model.clone(),
                "--thinking".into(),
                agent.effort.clone(),
                "--approve".into(),
            ]);
            if !agent.session_id.is_empty() {
                argv.extend(["--session-id".into(), agent.session_id.clone()]);
            }
            argv
        }
        other => {
            return Err(format!(
                "agent {:?} has harness {other:?}, which is not supported; use claude or pi",
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

/// The text of an agent's pane label: name, harness, model, effort, and
/// directory.
pub fn label(agent: &Agent) -> String {
    format!(
        "{} · {} {} {} · {}",
        agent.name,
        agent.harness,
        agent.model,
        agent.effort,
        workspace::short(Path::new(&agent.dir))
    )
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

/// Write the Pi extension, with the executable path embedded, to the state
/// directory and return its path. The file is renamed into place, so a harness
/// starting meanwhile never reads a half-written copy, and a current copy is
/// left alone.
fn write_pi_extension(exe: &str, state_path: &Path) -> Result<PathBuf> {
    let dir = state_path
        .parent()
        .unwrap_or(Path::new(""))
        .join("pi-extension");
    std::fs::create_dir_all(&dir).map_err(|e| format!("create {}: {e}", dir.display()))?;
    let path = dir.join("index.js");
    let source = PI_EXTENSION.replace("HYPERCUBE_EXE", &Value::from(exe).to_string());
    if std::fs::read_to_string(&path).ok().as_deref() != Some(&source) {
        let temp = dir.join(format!("index.js.{}.tmp", std::process::id()));
        std::fs::write(&temp, source).map_err(|e| format!("write {}: {e}", temp.display()))?;
        std::fs::rename(&temp, &path).map_err(|e| format!("rename {}: {e}", temp.display()))?;
    }
    Ok(path)
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
        assert_eq!(l.argv.len(), 10);
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
                "--dangerously-skip-permissions",
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
        let hook = |event: &str| {
            json!([{"hooks": [{"type": "command",
                "command": format!("'{EXE}' hook {event}")}]}])
        };
        assert_eq!(
            settings,
            json!({
                "hooks": {
                    "SessionStart": hook("SessionStart"),
                    "UserPromptSubmit": hook("UserPromptSubmit"),
                    "PostToolUse": hook("PostToolUse"),
                    "Stop": hook("Stop"),
                },
            })
        );
    }

    #[test]
    fn claude_resume_and_prompt() {
        let state = Path::new("/tmp/w.json");
        let argv = |session_id: &str, prompt: &str| {
            launch(state, &agent("claude", session_id), prompt).argv
        };
        let skip = "--dangerously-skip-permissions";
        assert_eq!(argv("s1", NUDGE)[9..], [skip, "--resume", "s1", NUDGE]);
        assert_eq!(argv("s1", "")[9..], [skip, "--resume", "s1"]);
        assert_eq!(argv("", NUDGE)[9..], [skip, NUDGE]);
        assert_eq!(argv("", "")[9..], [skip]);
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
    fn pi_argv_environment_and_extension() {
        let dir = TempDir::new();
        let state = dir.path().join("w.json");
        let extension = dir.path().join("pi-extension/index.js");
        let base = [
            "pi",
            "--extension",
            extension.to_str().unwrap(),
            "--model",
            "m",
            "--thinking",
            "high",
            "--approve",
        ];

        let l = launch(&state, &agent("pi", ""), "");
        assert_eq!(l.argv, base);
        assert_eq!(l.env["HYPERCUBE_AGENT_ID"], "a1");
        assert_eq!(l.env["HYPERCUBE_STATE"], state.to_str().unwrap());
        assert_eq!(l.env.len(), 2);
        assert_eq!(l.cwd, Path::new("/r/.hypercube/w1"));

        let l = launch(&state, &agent("pi", ""), NUDGE);
        assert_eq!(l.argv, [&base[..], &[NUDGE]].concat());

        let l = launch(&state, &agent("pi", "s1"), NUDGE);
        assert_eq!(l.argv, [&base[..], &["--session-id", "s1", NUDGE]].concat());

        let l = launch(&state, &agent("pi", "s1"), "");
        assert_eq!(l.argv, [&base[..], &["--session-id", "s1"]].concat());

        let source = std::fs::read_to_string(&extension).unwrap();
        assert!(!source.contains("HYPERCUBE_EXE"), "unreplaced placeholder");
        assert!(
            source.contains(&format!("const exe = {:?}\n", EXE)),
            "{source}"
        );
    }

    #[test]
    fn pi_extension_is_written_once_and_updated_when_the_executable_changes() {
        let dir = TempDir::new();
        let state = dir.path().join("w.json");
        let a = agent("pi", "");
        let extension = dir.path().join("pi-extension/index.js");
        launch(&state, &a, "");
        let first = std::fs::metadata(&extension).unwrap().modified().unwrap();
        launch(&state, &a, "");
        assert_eq!(
            std::fs::metadata(&extension).unwrap().modified().unwrap(),
            first
        );
        let names: Vec<_> = std::fs::read_dir(extension.parent().unwrap())
            .unwrap()
            .map(|e| e.unwrap().file_name())
            .collect();
        assert_eq!(names, ["index.js"], "a temporary file remains");

        command(Path::new("/other/hypercube"), &a, &state, "").unwrap();
        let source = std::fs::read_to_string(&extension).unwrap();
        assert!(
            source.contains("const exe = \"/other/hypercube\"\n"),
            "{source}"
        );
    }

    #[test]
    fn unsupported_harnesses_fail_to_launch() {
        let state = Path::new("/tmp/w.json");
        for harness in ["codex", "opencode", "ox", "gpt", ""] {
            let err = command(Path::new(EXE), &agent(harness, ""), state, "")
                .unwrap_err()
                .to_string();
            assert_eq!(
                err,
                format!(
                    "agent \"w1\" has harness \"{harness}\", which is not supported; use claude or pi"
                )
            );
        }
    }

    #[test]
    fn labels() {
        let mut a = agent("claude", "");
        a.name = "w1".into();
        a.model = "opus".into();
        a.dir = "/r/w1".into();
        assert_eq!(label(&a), "w1 · claude opus high · /r/w1");
    }

    #[test]
    fn shell_quoting() {
        assert_eq!(shell_quote("a b"), "'a b'");
        assert_eq!(shell_quote("it's"), r"'it'\''s'");
    }

    /// Runs the extension under Node against a fake hypercube executable.
    #[test]
    fn pi_extension_maps_events_to_hooks() {
        if Command::new("node").arg("--version").output().is_err() {
            eprintln!("skipped: node is required to exercise the Pi extension");
            return;
        }
        let dir = TempDir::new();
        let fake = dir.path().join("fake hypercube");
        std::fs::write(
            &fake,
            r#"#!/bin/sh
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
        command(&fake, &agent("pi", ""), &state, "").unwrap();
        let extension = dir.path().join("pi-extension/index.js");

        let out = Command::new("node")
            .args(["--input-type=module", "--eval", PI_EXTENSION_TEST])
            .env("HYPERCUBE_AGENT_ID", "a1")
            .env("HYPERCUBE_STATE", &state)
            .env("HYPERCUBE_TEST_LOG", dir.path().join("hooks.log"))
            .env("HYPERCUBE_TEST_EXTENSION", &extension)
            .env("HYPERCUBE_TEST_EXE", &fake)
            .output()
            .unwrap();
        assert!(
            out.status.success(),
            "Pi extension:\n{}{}",
            String::from_utf8_lossy(&out.stdout),
            String::from_utf8_lossy(&out.stderr)
        );
    }

    const PI_EXTENSION_TEST: &str = r##"
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
"##;
}
