//! Harness hook events. Claude Code's response format is the one the Pi
//! extension reads too: `hookSpecificOutput.additionalContext` carries text for
//! the model, and `decision` with `reason` blocks a stop.

use std::error::Error;
use std::io::{Read, Write};
use std::path::Path;

use serde::{Deserialize, Serialize};

use crate::config::{self, Profile};
use crate::store::{self, Agent, Message, State};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

#[derive(Default, Deserialize)]
#[serde(default)]
struct Input {
    session_id: String,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct Specific<'a> {
    hook_event_name: &'a str,
    additional_context: String,
}

#[derive(Default, Serialize)]
#[serde(rename_all = "camelCase")]
struct Response<'a> {
    #[serde(skip_serializing_if = "Option::is_none")]
    decision: Option<&'static str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    reason: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    hook_specific_output: Option<Specific<'a>>,
}

/// Handle one hook event for the agent named by `HYPERCUBE_AGENT_ID` and
/// `HYPERCUBE_STATE`. Processes outside a Hypercube agent do nothing.
pub fn run(event: &str, stdin: impl Read, stdout: impl Write) -> Result<()> {
    let id = std::env::var("HYPERCUBE_AGENT_ID").unwrap_or_default();
    let state = std::env::var("HYPERCUBE_STATE").unwrap_or_default();
    if id.is_empty() || state.is_empty() {
        return Ok(());
    }
    handle(
        event,
        &id,
        Path::new(&state),
        &config::path(),
        stdin,
        stdout,
    )
}

/// Handle one hook event for agent `id` with the state at `state_path`. The
/// orchestrator's SessionStart reads the profiles from `config_path`.
pub fn handle(
    event: &str,
    id: &str,
    state_path: &Path,
    config_path: &Path,
    stdin: impl Read,
    mut stdout: impl Write,
) -> Result<()> {
    let input = serde_json::Deserializer::from_reader(stdin)
        .into_iter::<Input>()
        .next()
        .ok_or("hook input is empty")?
        .map_err(|e| format!("hook input: {e}"))?;

    store::update(state_path, |state| {
        let agent = state.agent(id)?;
        let response = match event {
            "SessionStart" => {
                let profiles = if agent.role == store::ORCHESTRATOR {
                    config::load_from(config_path)?.profiles
                } else {
                    Vec::new()
                };
                context(event, facts(state, agent, &profiles))
            }
            "UserPromptSubmit" => {
                let messages = state.take(id);
                let text = format(state, &messages);
                let agent = state.agent_mut(id)?;
                // Claude Code saves no conversation until the first prompt, so a
                // session ID recorded earlier may not be resumable.
                if !input.session_id.is_empty() {
                    agent.session_id = input.session_id.clone();
                }
                agent.activity = store::BUSY.into();
                context(event, text)
            }
            "PostToolUse" => {
                let messages = state.take(id);
                context(event, format(state, &messages))
            }
            "Stop" => {
                let messages = state.take(id);
                let text = format(state, &messages);
                if text.is_empty() {
                    state.agent_mut(id)?.activity = store::IDLE.into();
                    None
                } else {
                    Some(Response {
                        decision: Some("block"),
                        reason: Some(text),
                        ..Response::default()
                    })
                }
            }
            _ => return Err(format!("unknown hook event {event:?}").into()),
        };
        // Writing the response inside the update keeps taken messages queued
        // if the harness cannot read them.
        if let Some(response) = response {
            let mut line = serde_json::to_string(&response)?;
            line.push('\n');
            stdout
                .write_all(line.as_bytes())
                .and_then(|()| stdout.flush())
                .map_err(|e| format!("write hook response: {e}"))?;
        }
        Ok(())
    })
}

fn context<'a>(event: &'a str, text: String) -> Option<Response<'a>> {
    if text.is_empty() {
        return None;
    }
    Some(Response {
        hook_specific_output: Some(Specific {
            hook_event_name: event,
            additional_context: text,
        }),
        ..Response::default()
    })
}

/// Render messages as the model sees them.
pub fn format(state: &State, messages: &[Message]) -> String {
    messages
        .iter()
        .map(|m| {
            let name = state.agent(&m.from).map_or(m.from.as_str(), |a| &a.name);
            format!("Message from {name} ({}):\n{}", m.from, m.text)
        })
        .collect::<Vec<_>>()
        .join("\n\n")
}

/// Describe the agent's place in the team. The orchestrator also gets the
/// configured worker profiles.
pub fn facts(state: &State, agent: &Agent, profiles: &[Profile]) -> String {
    let mut b = format!(
        "You are {} ({}), the {} in a Hypercube team of coding agents.\n",
        agent.name, agent.id, agent.role
    );
    b.push_str(&format!(
        "Your working directory is {}{}.\n\nTeam:\n",
        agent.dir,
        on_branch(&agent.branch)
    ));
    for t in &state.agents {
        b.push_str(&format!(
            "- {} ({}): {}, {}, {}{}\n",
            t.name,
            t.id,
            t.role,
            t.harness,
            t.dir,
            on_branch(&t.branch)
        ));
    }
    b.push_str(MESSAGE_TOOLS);
    if state.no_git {
        b.push_str(&format!(
            r#"- create_agent(name, profile, dir, message?) creates a worker that works directly in dir, {DIR_ARG} Its profile is one of the worker profiles below. The optional message is delivered on the worker's first turn. Orchestrator only.
- remove_agent(id) closes a worker's pane and removes it from the team. Its directory is left as it is. Orchestrator only.
"#
        ));
    } else {
        b.push_str(&format!(
            r#"- create_agent(name, profile, dir?, message?) creates a worker. Without dir, the worker gets its own branch and worktree from the orchestrator's HEAD, with submodules checked out and copies of the gitignored paths the checkout's .worktreeinclude lists. With dir, {DIR_ARG} the worker works directly in it, and Hypercube creates no branch or worktree. Its profile is one of the worker profiles below. The optional message is delivered on the worker's first turn. Orchestrator only.
- remove_agent(id) closes a worker's pane and removes it from the team. A worker with a worktree also has its worktree and branch removed, and Git refuses dirty worktrees and unmerged branches. A worker without one has its directory left as it is. Orchestrator only.
"#
        ));
    }
    let orchestrator = agent.role == store::ORCHESTRATOR;
    if orchestrator && state.no_git {
        b.push_str(
            r#"
Your working directory is not a Git checkout. Each worker works directly in the directory you give it, usually its own Git checkout. Tell each worker how to handle branches and commits in its directory. Workers given the same directory share one checkout. Remove a worker when its work is done. You decide how to divide the work, coordinate the workers, and judge when it is done. The user talks to you in this session.
"#,
        );
        write_teamwork(&mut b, profiles);
    } else if orchestrator {
        b.push_str(
            r#"
You work in the original checkout. Unless the user says otherwise, give a worker its own worktree, by omitting dir, when it will change files. It commits on its own branch in its worktree. Merge its branch into your branch from your checkout, then remove the worker when its work is merged. A worker that will not change files, such as one doing research or review, needs no worktree: pass dir "." to run it in your checkout, and remove it when it is done. When the user asks for changes made directly in a directory, pass that directory, and tell the worker how to handle branches and commits there. Workers given the same directory share one checkout. You decide how to divide the work, coordinate the workers, and judge when it is done. The user talks to you in this session.
"#,
        );
        write_teamwork(&mut b, profiles);
    } else if !agent.branch.is_empty() {
        b.push_str(
            "\nCommit your work on your branch. The orchestrator merges it from its checkout.",
        );
        b.push_str(REPORT_BACK);
    } else {
        b.push_str(
            "\nYou work directly in your working directory. Handle branches and commits as the orchestrator tells you.",
        );
        b.push_str(REPORT_BACK);
    }
    b
}

fn on_branch(branch: &str) -> String {
    if branch.is_empty() {
        String::new()
    } else {
        format!(" on branch {branch}")
    }
}

const DIR_ARG: &str = "an existing directory given as an absolute path, a path starting with ~/, or a path relative to the orchestrator's working directory,";

const REPORT_BACK: &str = " When you finish, or need something, send the orchestrator a short summary: what you did, the result, and anything unresolved. Leave details in your commits or files. A message you send is the only way other agents see your work's status.\n";

const MESSAGE_TOOLS: &str = r#"
Hypercube MCP tools:
- send_message(to, text) sends a message to an agent by ID or name. Messages arrive in your context as "Message from <name> (<id>):".
- list_agents() returns the team with each agent's activity.
"#;

/// The orchestrator's teamwork guidance and the configured worker
/// profiles.
fn write_teamwork(b: &mut String, profiles: &[Profile]) {
    b.push_str(
        r#"
Teamwork:
- Do small or tightly coupled tasks yourself. Create workers when the work splits into long, self-contained parts. A team uses several times the tokens of a single agent.
- Run independent work in parallel and coupled work in sequence. Give each file that several tasks touch to one worker.
- A worker's first message states the objective, the expected result, the files and sources to start from, and the boundaries: what to leave alone and when to stop and report.
- Choose the cheapest profile that fits the task. When a worker's result fails its checks, give the task to a stronger profile.
- Before you merge a change, run the build and tests. A non-trivial change also needs a review by a worker that did not write it. A trivial change needs none.
- Ask workers for conclusions, not transcripts, to keep your context for coordination.

Worker profiles. Pass the name of the one that fits the task to create_agent as profile:
"#,
    );
    for p in profiles {
        b.push_str(&format!(
            "- {} ({}, {}, {}): {}\n",
            p.name, p.spec.harness, p.spec.model, p.spec.effort, p.description
        ));
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testutil::TempDir;
    use std::path::PathBuf;

    const CONFIG: &str = r#"{
  "orchestrator": {"harness": "claude", "model": "opus", "effort": "high"},
  "profiles": [{"name": "sol", "description": "coding.", "harness": "pi", "model": "openai/gpt-6.1-sol", "effort": "medium"}]
}"#;

    struct Fixture {
        _dir: TempDir,
        state: PathBuf,
        config: PathBuf,
    }

    impl Fixture {
        fn new(agents: Vec<Agent>, messages: Vec<Message>, no_git: bool) -> Fixture {
            let dir = TempDir::new();
            let (state, config) = (dir.path().join("w.json"), dir.path().join("config.json"));
            std::fs::write(&config, CONFIG).unwrap();
            store::update(&state, |s| {
                *s = State {
                    agents,
                    messages,
                    no_git,
                };
                Ok(())
            })
            .unwrap();
            Fixture {
                _dir: dir,
                state,
                config,
            }
        }

        fn try_run(&self, id: &str, event: &str, stdin: &str, out: impl Write) -> Result<()> {
            handle(event, id, &self.state, &self.config, stdin.as_bytes(), out)
        }

        /// The response's JSON, or `Null` when the hook wrote nothing.
        fn run(&self, id: &str, event: &str) -> serde_json::Value {
            let mut out = Vec::new();
            self.try_run(id, event, r#"{"session_id":"s1"}"#, &mut out)
                .unwrap();
            if out.is_empty() {
                return serde_json::Value::Null;
            }
            assert!(out.ends_with(b"\n"));
            serde_json::from_slice(&out).unwrap()
        }

        fn load(&self) -> State {
            store::load(&self.state).unwrap()
        }

        fn queue(&self, from: &str, to: &str, text: &str) {
            store::update(&self.state, |s| {
                s.messages.push(Message {
                    from: from.into(),
                    to: to.into(),
                    text: text.into(),
                });
                Ok(())
            })
            .unwrap();
        }
    }

    fn agent(id: &str, name: &str, role: &str) -> Agent {
        Agent {
            id: id.into(),
            name: name.into(),
            role: role.into(),
            ..Agent::default()
        }
    }

    fn pair() -> Vec<Agent> {
        vec![
            agent("a", "orchestrator", store::ORCHESTRATOR),
            agent("b", "w1", store::WORKER),
        ]
    }

    fn context_of(response: &serde_json::Value) -> &str {
        response["hookSpecificOutput"]["additionalContext"]
            .as_str()
            .unwrap_or_else(|| panic!("no additional context in {response}"))
    }

    #[test]
    fn session_start_returns_facts_and_records_nothing() {
        let f = Fixture::new(pair(), vec![], false);
        let r = f.run("b", "SessionStart");
        assert_eq!(r["hookSpecificOutput"]["hookEventName"], "SessionStart");
        assert!(context_of(&r).contains("You are w1 (b)"));
        assert_eq!(f.load().agents[1].session_id, "");
        assert_eq!(f.load().agents[1].activity, "");
    }

    #[test]
    fn user_prompt_submit_delivers_records_the_session_and_marks_busy() {
        let f = Fixture::new(pair(), vec![], false);
        f.queue("a", "b", "hello");
        let r = f.run("b", "UserPromptSubmit");
        assert_eq!(r["hookSpecificOutput"]["hookEventName"], "UserPromptSubmit");
        assert_eq!(context_of(&r), "Message from orchestrator (a):\nhello");
        let s = f.load();
        assert_eq!(s.agents[1].activity, store::BUSY);
        assert_eq!(s.agents[1].session_id, "s1");
        assert!(s.messages.is_empty());

        // Without a queue the hook still marks the agent busy.
        let r = f.run("b", "UserPromptSubmit");
        assert!(r.is_null(), "{r}");
    }

    #[test]
    fn user_prompt_submit_without_a_session_id_keeps_the_recorded_one() {
        let mut agents = pair();
        agents[1].session_id = "old".into();
        let f = Fixture::new(agents, vec![], false);
        f.try_run("b", "UserPromptSubmit", "{}", Vec::new())
            .unwrap();
        assert_eq!(f.load().agents[1].session_id, "old");
    }

    #[test]
    fn stop_blocks_with_queued_messages_then_goes_idle() {
        let mut agents = pair();
        agents[1].activity = store::BUSY.into();
        let f = Fixture::new(agents, vec![], false);
        f.queue("a", "b", "more");
        let r = f.run("b", "Stop");
        assert_eq!(r["decision"], "block");
        assert_eq!(r["reason"], "Message from orchestrator (a):\nmore");
        assert!(r.get("hookSpecificOutput").is_none());
        let s = f.load();
        assert_eq!(
            s.agents[1].activity,
            store::BUSY,
            "a blocked Stop marks the agent"
        );
        assert!(s.messages.is_empty());

        let r = f.run("b", "Stop");
        assert!(r.is_null(), "{r}");
        assert_eq!(f.load().agents[1].activity, store::IDLE);
    }

    #[test]
    fn post_tool_use_delivers_only_the_agents_messages() {
        let mut agents = pair();
        agents[1].activity = store::BUSY.into();
        let f = Fixture::new(agents, vec![], false);
        f.queue("a", "b", "mid-turn");
        f.queue("a", "a", "other");
        let r = f.run("b", "PostToolUse");
        assert_eq!(r["hookSpecificOutput"]["hookEventName"], "PostToolUse");
        assert_eq!(context_of(&r), "Message from orchestrator (a):\nmid-turn");
        let s = f.load();
        assert_eq!(s.messages.len(), 1);
        assert_eq!(s.messages[0].to, "a");
        assert_eq!(s.agents[1].activity, store::BUSY);
        assert!(f.run("b", "PostToolUse").is_null());
    }

    #[test]
    fn messages_are_joined_in_order() {
        let f = Fixture::new(pair(), vec![], false);
        f.queue("a", "b", "one");
        f.queue("gone", "b", "two");
        let r = f.run("b", "PostToolUse");
        assert_eq!(
            context_of(&r),
            "Message from orchestrator (a):\none\n\nMessage from gone (gone):\ntwo"
        );
    }

    #[test]
    fn unknown_events_fail_and_change_nothing() {
        let f = Fixture::new(pair(), vec![], false);
        f.queue("a", "b", "hello");
        let err = f
            .try_run("b", "Nope", "{}", Vec::new())
            .unwrap_err()
            .to_string();
        assert_eq!(err, r#"unknown hook event "Nope""#);
        assert_eq!(f.load().messages.len(), 1);
    }

    #[test]
    fn unknown_agents_fail() {
        let f = Fixture::new(pair(), vec![], false);
        let err = f
            .try_run("zzz", "Stop", "{}", Vec::new())
            .unwrap_err()
            .to_string();
        assert_eq!(err, r#"no agent "zzz""#);
    }

    #[test]
    fn input_is_one_json_object() {
        let f = Fixture::new(pair(), vec![], false);
        let out = |stdin: &str| f.try_run("b", "Stop", stdin, Vec::new());
        out(r#"{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"ls"}}"#).unwrap();
        out("{}\n").unwrap();
        assert!(out("").unwrap_err().to_string().contains("empty"));
        assert!(out("{").unwrap_err().to_string().starts_with("hook input"));
        assert!(out("5").is_err());
    }

    struct FailingWriter;

    impl Write for FailingWriter {
        fn write(&mut self, _: &[u8]) -> std::io::Result<usize> {
            Err(std::io::Error::other("closed"))
        }

        fn flush(&mut self) -> std::io::Result<()> {
            Ok(())
        }
    }

    #[test]
    fn a_failed_response_keeps_the_messages_queued() {
        let f = Fixture::new(pair(), vec![], false);
        f.queue("a", "b", "hello");
        for event in ["PostToolUse", "UserPromptSubmit", "Stop"] {
            assert!(f.try_run("b", event, "{}", FailingWriter).is_err());
            let s = f.load();
            assert_eq!(s.messages.len(), 1, "{event}");
            assert_eq!(s.agents[1].activity, "", "{event}");
        }
    }

    #[test]
    fn orchestrator_facts_list_the_profiles() {
        let f = Fixture::new(
            vec![agent("a", "orchestrator", store::ORCHESTRATOR)],
            vec![],
            false,
        );
        let r = f.run("a", "SessionStart");
        assert!(
            context_of(&r).contains("\n- sol (pi, openai/gpt-6.1-sol, medium): coding.\n"),
            "{}",
            context_of(&r)
        );
    }

    #[test]
    fn orchestrator_session_start_needs_a_valid_config() {
        let f = Fixture::new(
            vec![agent("a", "orchestrator", store::ORCHESTRATOR)],
            vec![],
            false,
        );
        std::fs::write(&f.config, "{}").unwrap();
        assert!(f.try_run("a", "SessionStart", "{}", Vec::new()).is_err());
        // Workers never read it.
        let f = Fixture::new(pair(), vec![], false);
        std::fs::remove_file(&f.config).unwrap();
        f.run("b", "SessionStart");
    }

    #[test]
    fn no_git_facts() {
        let agents = vec![
            Agent {
                dir: "/o".into(),
                ..agent("a", "orchestrator", store::ORCHESTRATOR)
            },
            Agent {
                dir: "/w".into(),
                ..agent("b", "w1", store::WORKER)
            },
        ];
        let f = Fixture::new(agents, vec![], true);

        let r = f.run("a", "SessionStart");
        let got = context_of(&r);
        for want in [
            "create_agent(name, profile, dir, message?) creates a worker that works directly in dir,",
            "Tell each worker how to handle branches and commits in its directory.",
            "Its directory is left as it is.",
            "Worker profiles.",
        ] {
            assert!(
                got.contains(want),
                "orchestrator context lacks {want:?}:\n{got}"
            );
        }
        for unwanted in ["on branch", "Merge its branch", "worktree"] {
            assert!(
                !got.contains(unwanted),
                "orchestrator context has {unwanted:?}:\n{got}"
            );
        }

        let r = f.run("b", "SessionStart");
        let got = context_of(&r);
        for want in [
            "Your working directory is /w.\n",
            "Handle branches and commits as the orchestrator tells you",
        ] {
            assert!(got.contains(want), "worker context lacks {want:?}:\n{got}");
        }
        assert!(!got.contains("on branch"), "{got}");
    }

    #[test]
    fn git_facts() {
        let agents = vec![
            Agent {
                dir: "/r".into(),
                branch: "main".into(),
                harness: "claude".into(),
                ..agent("a", "orchestrator", store::ORCHESTRATOR)
            },
            Agent {
                dir: "/r/.hypercube/w1".into(),
                branch: "hypercube/w1".into(),
                harness: "pi".into(),
                ..agent("b", "w1", store::WORKER)
            },
            Agent {
                dir: "/r".into(),
                harness: "claude".into(),
                ..agent("c", "w2", store::WORKER)
            },
        ];
        let f = Fixture::new(agents, vec![], false);
        let cases = [
            (
                "a",
                vec![
                    "You are orchestrator (a), the orchestrator in a Hypercube team of coding agents.\n",
                    "Your working directory is /r on branch main.\n\nTeam:\n",
                    "- orchestrator (a): orchestrator, claude, /r on branch main\n",
                    "- w1 (b): worker, pi, /r/.hypercube/w1 on branch hypercube/w1\n",
                    "- w2 (c): worker, claude, /r\n",
                    "create_agent(name, profile, dir?,",
                    "Merge its branch into your branch",
                    r#"pass dir ".""#,
                    "- Run independent work in parallel and coupled work in sequence.",
                    "A non-trivial change also needs a review by a worker that did not write it.",
                ],
            ),
            (
                "b",
                vec![
                    "Your working directory is /r/.hypercube/w1 on branch hypercube/w1.",
                    "Commit your work on your branch. The orchestrator merges it from its checkout. When you finish, or need something, send the orchestrator a short summary:",
                ],
            ),
            (
                "c",
                vec![
                    "Your working directory is /r.\n",
                    "Handle branches and commits as the orchestrator tells you. When you finish,",
                ],
            ),
        ];
        for (id, wants) in cases {
            let r = f.run(id, "SessionStart");
            let got = context_of(&r);
            for want in wants {
                assert!(got.contains(want), "context of {id} lacks {want:?}:\n{got}");
            }
        }
        let r = f.run("b", "SessionStart");
        assert!(!context_of(&r).contains("Worker profiles"));
    }

    #[test]
    fn facts_text() {
        let state = State {
            agents: vec![
                Agent {
                    dir: "/r".into(),
                    branch: "main".into(),
                    harness: "claude".into(),
                    ..agent("a", "orchestrator", store::ORCHESTRATOR)
                },
                Agent {
                    dir: "/r/.hypercube/w1".into(),
                    branch: "hypercube/w1".into(),
                    harness: "pi".into(),
                    ..agent("b", "w1", store::WORKER)
                },
            ],
            ..State::default()
        };
        let want = r#"You are w1 (b), the worker in a Hypercube team of coding agents.
Your working directory is /r/.hypercube/w1 on branch hypercube/w1.

Team:
- orchestrator (a): orchestrator, claude, /r on branch main
- w1 (b): worker, pi, /r/.hypercube/w1 on branch hypercube/w1

Hypercube MCP tools:
- send_message(to, text) sends a message to an agent by ID or name. Messages arrive in your context as "Message from <name> (<id>):".
- list_agents() returns the team with each agent's activity.
- create_agent(name, profile, dir?, message?) creates a worker. Without dir, the worker gets its own branch and worktree from the orchestrator's HEAD, with submodules checked out and copies of the gitignored paths the checkout's .worktreeinclude lists. With dir, an existing directory given as an absolute path, a path starting with ~/, or a path relative to the orchestrator's working directory, the worker works directly in it, and Hypercube creates no branch or worktree. Its profile is one of the worker profiles below. The optional message is delivered on the worker's first turn. Orchestrator only.
- remove_agent(id) closes a worker's pane and removes it from the team. A worker with a worktree also has its worktree and branch removed, and Git refuses dirty worktrees and unmerged branches. A worker without one has its directory left as it is. Orchestrator only.

Commit your work on your branch. The orchestrator merges it from its checkout. When you finish, or need something, send the orchestrator a short summary: what you did, the result, and anything unresolved. Leave details in your commits or files. A message you send is the only way other agents see your work's status.
"#;
        assert_eq!(facts(&state, &state.agents[1], &[]), want);
    }
}
