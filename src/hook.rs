//! Harness hook events. Claude Code's response format is the one the OpenCode
//! plugin reads too: `hookSpecificOutput.additionalContext` carries text for
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
pub(crate) struct Input {
    session_id: String,
    pub(crate) agent_id: Option<String>,
    error: String,
    error_details: String,
    last_assistant_message: String,
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

    // Settings hooks also run in Claude Code's native subagents. Their
    // context and activity are not the Hypercube agent's, but an approval
    // still needs the user's attention in the same pane.
    if input.agent_id.is_some() && event != "PermissionRequest" {
        return Ok(());
    }

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
            "PermissionRequest" => {
                state.agent_mut(id)?.attention = true;
                None
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
            "StopFailure" => {
                if agent.role == store::WORKER
                    && let Some(orchestrator) = state.team_orchestrator(&agent.team)
                {
                    state.messages.push(Message {
                        from: id.into(),
                        to: orchestrator.id.clone(),
                        text: failure(&input),
                    });
                }
                state.agent_mut(id)?.activity = store::IDLE.into();
                None
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

fn failure(input: &Input) -> String {
    let detail = [
        &input.error_details,
        &input.last_assistant_message,
        &input.error,
    ]
    .into_iter()
    .map(|s| s.trim())
    .find(|s| !s.is_empty())
    .unwrap_or("");
    if detail.is_empty() {
        return "My turn failed.".into();
    }
    let mut chars = detail.chars();
    let mut text: String = chars.by_ref().take(2000).collect();
    if chars.next().is_some() {
        text.push('…');
    }
    format!("My turn failed: {text}")
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

/// Render messages as the model sees them. A sender that is no agent, such
/// as the schedule, is named alone.
pub fn format(state: &State, messages: &[Message]) -> String {
    messages
        .iter()
        .map(|m| match state.agent(&m.from) {
            Ok(a) => format!("Message from {} ({}):\n{}", a.name, m.from, m.text),
            Err(_) => format!("Message from {}:\n{}", m.from, m.text),
        })
        .collect::<Vec<_>>()
        .join("\n\n")
}

/// Describe the agent's place in its team. An orchestrator also gets the
/// other teams' orchestrators and the configured worker profiles, the default
/// team's orchestrator the task tools and the scheduled tasks, and a task's
/// orchestrator its task.
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
    for t in state.agents.iter().filter(|t| t.team == agent.team) {
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
    let task = state.task(&agent.team).filter(|_| orchestrator);
    if orchestrator && agent.team == store::DEFAULT_TEAM {
        b.push_str(TASK_TOOLS);
    }
    if let Some(task) = task {
        write_task(&mut b, state, task);
        let place = if state.no_git {
            "directory"
        } else {
            "checkout"
        };
        write_other_teams(&mut b, state, agent, place);
        write_teamwork(&mut b, profiles);
    } else if orchestrator && state.no_git {
        b.push_str(
            r#"
Your working directory is not a Git checkout. Each worker works directly in the directory you give it, usually its own Git checkout. Tell each worker how to handle branches and commits in its directory. Workers given the same directory share one checkout. Remove a worker when its work is done. You decide how to divide the work, coordinate the workers, and judge when it is done. The user talks to you in this session.
"#,
        );
        write_other_teams(&mut b, state, agent, "directory");
        write_tasks(&mut b, state, agent);
        write_teamwork(&mut b, profiles);
    } else if orchestrator {
        b.push_str(
            r#"
You work in the original checkout. Unless the user says otherwise, give a worker its own worktree, by omitting dir, when it will change files. It commits on its own branch in its worktree. Merge its branch into your branch from your checkout, then remove the worker when its work is merged. A worker that will not change files, such as one doing research or review, needs no worktree: pass dir "." to run it in your checkout, and remove it when it is done. When the user asks for changes made directly in a directory, pass that directory, and tell the worker how to handle branches and commits there. Workers given the same directory share one checkout. You decide how to divide the work, coordinate the workers, and judge when it is done. The user talks to you in this session.
"#,
        );
        write_other_teams(&mut b, state, agent, "checkout");
        write_tasks(&mut b, state, agent);
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

/// The other teams' orchestrators, which work in the same `place` as
/// `agent`, if the workspace has other teams.
fn write_other_teams(b: &mut String, state: &State, agent: &Agent, place: &str) {
    let others: Vec<String> = state
        .agents
        .iter()
        .filter(|a| a.role == store::ORCHESTRATOR && a.team != agent.team)
        .map(|a| format!("{} ({})", a.name, a.id))
        .collect();
    if !others.is_empty() {
        b.push_str(&format!(
            "Other teams' orchestrators work in the same {place}: {}.\n",
            others.join(", ")
        ));
    }
}

/// The scheduled tasks and their schedules, for the default team's
/// orchestrator, if there are any.
fn write_tasks(b: &mut String, state: &State, agent: &Agent) {
    if agent.team != store::DEFAULT_TEAM || state.tasks.is_empty() {
        return;
    }
    b.push_str("\nScheduled tasks:\n");
    for t in &state.tasks {
        b.push_str(&format!("- {}: {}\n", t.name, t.schedule));
    }
}

/// The guidance of the orchestrator of `task`, which works in the checkout
/// it shares with the default team's orchestrator.
fn write_task(b: &mut String, state: &State, task: &store::Task) {
    let lead = state.team_orchestrator(store::DEFAULT_TEAM).map_or_else(
        || "orchestrator".to_string(),
        |o| format!("{} ({})", o.name, o.id),
    );
    b.push_str(&format!(
        "\nYou run the scheduled task {} on the schedule {}, in local time. Each time it fires, its prompt arrives in this same session as a message from schedule. A firing is skipped while you or one of your workers is busy, or while the last one is still queued.\n",
        task.name, task.schedule
    ));
    if state.no_git {
        b.push_str(&format!(
            "You work in the workspace's directory, which you share with the default team's orchestrator, {lead}. Do not change files in it. Do read-only work yourself, and give work that changes files to workers in the directories it belongs in, telling each how to handle branches and commits there. Report what changed to {lead} with send_message. Remove a worker when its work is done.\n"
        ));
    } else {
        b.push_str(&format!(
            "You work in the original checkout, which you share with the default team's orchestrator, {lead}. Do not merge, commit, or change files in it. Do read-only work yourself. Give work that changes files to workers with their own worktrees, by omitting dir, and send their branches to {lead} with send_message; it integrates them. Remove a worker once its branch is merged.\n"
        ));
    }
    b.push_str(&format!(
        "Reach {lead} with send_message. The user sees this session only when they view your team.\n"
    ));
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

const TASK_TOOLS: &str = r#"- create_task(name, schedule, prompt, profile) creates a scheduled task and its team, both named name, whose orchestrator works in your checkout with the profile's harness, model, and effort and keeps one session. Each time the schedule fires while the task's team is idle, the prompt arrives in that session as a message from schedule. The schedule is five cron fields in local time: minute hour day-of-month month day-of-week, each *, a number, a-b, */s, a-b/s, or a comma list of these. Use the profile the user names, and otherwise the worker profile below that suits the task.
- update_task(name, schedule?, prompt?) changes a task's schedule or prompt.
- list_tasks() returns each task's schedule, prompt, state, last run, orchestrator's harness, model, and effort, and agents.
- delete_task(name) deletes a task, its team, and its orchestrator. A task whose team has workers stays until they are removed.
To run a task now, send a message to its orchestrator, which has the task's name. remove_agent also removes the workers of a task's team.
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
  "profiles": [{"name": "sol", "description": "coding.", "harness": "opencode", "model": "openai/gpt-6.1-sol", "effort": "medium"}]
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
                    ..State::default()
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
    fn permission_request_sets_the_attention_flag() {
        for input in [
            r#"{"session_id":"s1","tool_name":"Bash"}"#,
            r#"{"session_id":"s1","tool_name":"Bash","agent_id":"native-subagent"}"#,
        ] {
            let f = Fixture::new(pair(), Vec::new(), false);
            f.queue("a", "b", "hi");
            let mut out = Vec::new();
            f.try_run("b", "PermissionRequest", input, &mut out)
                .unwrap();
            // A subagent's approval needs attention too. No output leaves the
            // decision to the user, and messages stay queued.
            assert!(out.is_empty());
            let s = f.load();
            assert!(s.agents[1].attention && !s.agents[0].attention);
            assert_eq!(s.messages.len(), 1);
        }
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
    fn stop_failure_notifies_the_workers_team_and_keeps_its_messages() {
        let mut agents = pair();
        agents[1].team = "docs".into();
        agents[1].activity = store::BUSY.into();
        agents.push(Agent {
            team: "docs".into(),
            ..agent("c", "docs", store::ORCHESTRATOR)
        });
        let f = Fixture::new(agents, vec![], false);
        f.queue("c", "b", "more");
        let mut out = Vec::new();
        f.try_run(
            "b",
            "StopFailure",
            r#"{"error":"billing_error","error_details":"  This request would exceed your available credits.\n"}"#,
            &mut out,
        )
        .unwrap();
        assert!(out.is_empty());
        let s = f.load();
        assert_eq!(s.agents[1].activity, store::IDLE);
        assert_eq!(s.messages.len(), 2);
        assert_eq!(s.messages[0].to, "b");
        assert_eq!(s.messages[0].text, "more");
        assert_eq!(s.messages[1].from, "b");
        assert_eq!(s.messages[1].to, "c");
        assert_eq!(
            s.messages[1].text,
            "My turn failed: This request would exceed your available credits."
        );
        // The notice uses the same queue and sender formatting as any message.
        let r = f.run("c", "PostToolUse");
        assert_eq!(
            context_of(&r),
            "Message from w1 (b):\nMy turn failed: This request would exceed your available credits."
        );
        assert_eq!(f.load().messages.len(), 1);
    }

    #[test]
    fn stop_failure_uses_the_available_error_text() {
        let f = Fixture::new(pair(), vec![], false);
        for (input, want) in [
            (r#"{"error":"rate_limit"}"#, "My turn failed: rate_limit"),
            (
                r#"{"error":"rate_limit","error_details":" ","last_assistant_message":"Request rejected (429)"}"#,
                "My turn failed: Request rejected (429)",
            ),
            ("{}", "My turn failed."),
        ] {
            f.try_run("b", "StopFailure", input, Vec::new()).unwrap();
            assert_eq!(f.load().messages.last().unwrap().text, want);
        }
    }

    #[test]
    fn stop_failure_limits_error_text_at_a_character_boundary() {
        let f = Fixture::new(pair(), vec![], false);
        for n in [2000, 2001] {
            let detail = "é".repeat(n);
            let input = serde_json::json!({"error_details": detail}).to_string();
            f.try_run("b", "StopFailure", &input, Vec::new()).unwrap();
            let suffix = if n > 2000 { "…" } else { "" };
            assert_eq!(
                f.load().messages.last().unwrap().text,
                format!("My turn failed: {}{suffix}", "é".repeat(2000))
            );
        }
    }

    #[test]
    fn an_orchestrators_failure_only_marks_it_idle() {
        let mut agents = pair();
        agents[0].activity = store::BUSY.into();
        let f = Fixture::new(agents, vec![], false);
        f.queue("b", "a", "hello");
        assert!(f.run("a", "StopFailure").is_null());
        let s = f.load();
        assert_eq!(s.agents[0].activity, store::IDLE);
        assert_eq!(s.messages.len(), 1);
        assert_eq!(s.messages[0].text, "hello");
    }

    #[test]
    fn a_workers_failure_without_an_orchestrator_still_marks_it_idle() {
        let f = Fixture::new(vec![agent("b", "w1", store::WORKER)], vec![], false);
        assert!(f.run("b", "StopFailure").is_null());
        let s = f.load();
        assert_eq!(s.agents[0].activity, store::IDLE);
        assert!(s.messages.is_empty());
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
    fn native_subagent_hooks_leave_the_main_conversations_state_untouched() {
        let mut agents = pair();
        agents[1].activity = store::BUSY.into();
        agents[1].session_id = "main".into();
        let f = Fixture::new(agents, vec![], false);
        f.queue("a", "b", "for the main conversation");
        let before = f.load();
        for event in [
            "SessionStart",
            "UserPromptSubmit",
            "PostToolUse",
            "Stop",
            "StopFailure",
        ] {
            let mut out = Vec::new();
            f.try_run(
                "b",
                event,
                r#"{"session_id":"s1","agent_id":"native-subagent","agent_type":"Explore","error":"server_error"}"#,
                &mut out,
            )
            .unwrap();
            assert!(out.is_empty(), "{event}");
            assert_eq!(f.load(), before, "{event}");
        }
        let r = f.run("b", "PostToolUse");
        assert_eq!(
            context_of(&r),
            "Message from orchestrator (a):\nfor the main conversation"
        );
    }

    #[test]
    fn an_agent_type_alone_does_not_identify_a_native_subagent() {
        let f = Fixture::new(pair(), vec![], false);
        f.queue("a", "b", "main conversation using --agent");
        let mut out = Vec::new();
        f.try_run(
            "b",
            "PostToolUse",
            r#"{"session_id":"s1","agent_type":"code-reviewer"}"#,
            &mut out,
        )
        .unwrap();
        let r = serde_json::from_slice(&out).unwrap();
        assert_eq!(
            context_of(&r),
            "Message from orchestrator (a):\nmain conversation using --agent"
        );
        assert!(f.load().messages.is_empty());
    }

    #[test]
    fn messages_are_joined_in_order() {
        let f = Fixture::new(pair(), vec![], false);
        f.queue("a", "b", "one");
        f.queue("gone", "b", "two");
        let r = f.run("b", "PostToolUse");
        assert_eq!(
            context_of(&r),
            "Message from orchestrator (a):\none\n\nMessage from gone:\ntwo"
        );
    }

    #[test]
    fn a_firing_is_a_message_from_schedule() {
        let f = Fixture::new(pair(), vec![], false);
        f.queue(store::SCHEDULE, "a", "check the build");
        let r = f.run("a", "UserPromptSubmit");
        assert_eq!(context_of(&r), "Message from schedule:\ncheck the build");
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
            context_of(&r).contains("\n- sol (opencode, openai/gpt-6.1-sol, medium): coding.\n"),
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
                harness: "opencode".into(),
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
                    "- w1 (b): worker, opencode, /r/.hypercube/w1 on branch hypercube/w1\n",
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
                    harness: "opencode".into(),
                    ..agent("b", "w1", store::WORKER)
                },
            ],
            ..State::default()
        };
        let want = r#"You are w1 (b), the worker in a Hypercube team of coding agents.
Your working directory is /r/.hypercube/w1 on branch hypercube/w1.

Team:
- orchestrator (a): orchestrator, claude, /r on branch main
- w1 (b): worker, opencode, /r/.hypercube/w1 on branch hypercube/w1

Hypercube MCP tools:
- send_message(to, text) sends a message to an agent by ID or name. Messages arrive in your context as "Message from <name> (<id>):".
- list_agents() returns the team with each agent's activity.
- create_agent(name, profile, dir?, message?) creates a worker. Without dir, the worker gets its own branch and worktree from the orchestrator's HEAD, with submodules checked out and copies of the gitignored paths the checkout's .worktreeinclude lists. With dir, an existing directory given as an absolute path, a path starting with ~/, or a path relative to the orchestrator's working directory, the worker works directly in it, and Hypercube creates no branch or worktree. Its profile is one of the worker profiles below. The optional message is delivered on the worker's first turn. Orchestrator only.
- remove_agent(id) closes a worker's pane and removes it from the team. A worker with a worktree also has its worktree and branch removed, and Git refuses dirty worktrees and unmerged branches. A worker without one has its directory left as it is. Orchestrator only.

Commit your work on your branch. The orchestrator merges it from its checkout. When you finish, or need something, send the orchestrator a short summary: what you did, the result, and anything unresolved. Leave details in your commits or files. A message you send is the only way other agents see your work's status.
"#;
        assert_eq!(facts(&state, &state.agents[1], &[]), want);
    }

    #[test]
    fn facts_list_the_agents_team_and_the_other_teams_orchestrators() {
        let member = |id: &str, name: &str, role: &str, team: &str| Agent {
            team: team.into(),
            ..agent(id, name, role)
        };
        let mut state = State {
            agents: vec![
                member(
                    "a",
                    "orchestrator",
                    store::ORCHESTRATOR,
                    store::DEFAULT_TEAM,
                ),
                member("b", "w1", store::WORKER, store::DEFAULT_TEAM),
                member("d", "docs", store::ORCHESTRATOR, "docs"),
                member("e", "writer", store::WORKER, "docs"),
            ],
            ..State::default()
        };
        let team = |text: &str| -> Vec<String> {
            let (_, rest) = text.split_once("Team:\n").unwrap();
            let (list, _) = rest.split_once("\n\n").unwrap();
            list.lines()
                .map(|l| l.split(' ').nth(1).unwrap().into())
                .collect()
        };
        let writer = facts(&state, &state.agents[3], &[]);
        assert_eq!(team(&writer), ["docs", "writer"]);
        assert!(!writer.contains("Other teams"), "{writer}");
        let docs = facts(&state, &state.agents[2], &[]);
        assert_eq!(team(&docs), ["docs", "writer"]);
        assert!(
            docs.contains("in this session.\nOther teams' orchestrators work in the same checkout: orchestrator (a).\n\nTeamwork:"),
            "{docs}"
        );
        let orchestrator = facts(&state, &state.agents[0], &[]);
        assert_eq!(team(&orchestrator), ["orchestrator", "w1"]);
        assert!(orchestrator.contains("work in the same checkout: docs (d).\n"));
        state.no_git = true;
        let docs = facts(&state, &state.agents[2], &[]);
        assert!(docs.contains("work in the same directory: orchestrator (a).\n"));
        // With one team, there are no others to name.
        state.agents.truncate(2);
        assert!(!facts(&state, &state.agents[0], &[]).contains("Other teams"));
    }

    #[test]
    fn task_facts() {
        let member = |id: &str, name: &str, role: &str, team: &str| Agent {
            team: team.into(),
            dir: "/r".into(),
            branch: "main".into(),
            ..agent(id, name, role)
        };
        let mut state = State {
            agents: vec![
                member(
                    "a",
                    "orchestrator",
                    store::ORCHESTRATOR,
                    store::DEFAULT_TEAM,
                ),
                member("n", "nightly", store::ORCHESTRATOR, "nightly"),
                member("w", "helper", store::WORKER, "nightly"),
                member("d", "docs", store::ORCHESTRATOR, "docs"),
            ],
            tasks: vec![store::Task {
                name: "nightly".into(),
                schedule: "0 3 * * *".into(),
                prompt: "check".into(),
                last_run: 0,
            }],
            ..State::default()
        };
        let lead = facts(&state, &state.agents[0], &[]);
        for want in [
            "- create_task(name, schedule, prompt, profile) creates a scheduled task",
            "- delete_task(name) deletes a task",
            "\nScheduled tasks:\n- nightly: 0 3 * * *\n\nTeamwork:",
        ] {
            assert!(lead.contains(want), "lacks {want:?}:\n{lead}");
        }
        let nightly = facts(&state, &state.agents[1], &[]);
        for want in [
            "You run the scheduled task nightly on the schedule 0 3 * * *, in local time.",
            "its prompt arrives in this same session as a message from schedule.",
            "Do not merge, commit, or change files in it.",
            "send their branches to orchestrator (a) with send_message",
            "The user sees this session only when they view your team.",
            "Worker profiles.",
        ] {
            assert!(nightly.contains(want), "lacks {want:?}:\n{nightly}");
        }
        for unwanted in ["create_task", "Scheduled tasks:", "Merge its branch"] {
            assert!(!nightly.contains(unwanted), "has {unwanted:?}:\n{nightly}");
        }
        // Neither another team's orchestrator nor a task's worker gets them.
        for i in [2, 3] {
            let text = facts(&state, &state.agents[i], &[]);
            for unwanted in ["create_task", "scheduled task", "Scheduled tasks:"] {
                assert!(!text.contains(unwanted), "has {unwanted:?}:\n{text}");
            }
        }
        state.no_git = true;
        let nightly = facts(&state, &state.agents[1], &[]);
        assert!(nightly.contains("Do not change files in it."), "{nightly}");
        assert!(!nightly.contains("worktree"), "{nightly}");
        state.tasks.clear();
        assert!(!facts(&state, &state.agents[0], &[]).contains("Scheduled tasks:"));
    }
}
