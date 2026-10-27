//! The stdio MCP server that gives agents the Hypercube tools. It speaks
//! line-delimited JSON-RPC. Tools change the state file and the worktrees
//! themselves and ask the host, through `control`, to launch, close, and nudge
//! panes.

use std::error::Error;
use std::io::{BufRead, Write};
use std::path::{Path, PathBuf};

use serde::Serialize;
use serde_json::{Map, Value, json};

use crate::config::{self, Profile};
use crate::control::{self, NUDGE, Request};
use crate::store::{self, Agent, Message, State};
use crate::workspace;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

const DIR_ARG: &str = "an existing directory given as an absolute path, a path starting with ~/, or a path relative to the orchestrator's directory, the worker works directly in it";

const PROFILE_ARG: &str = "The profile sets the worker's harness, model, and effort. An optional message is queued for the worker before it starts.";

/// The MCP server of one agent.
pub struct Server {
    agent: String,
    state: PathBuf,
    config: PathBuf,
}

/// Answer MCP requests on `stdin` until it closes, as the agent named by
/// `HYPERCUBE_AGENT_ID` in the workspace whose state file is
/// `HYPERCUBE_STATE`.
pub fn serve(stdin: impl BufRead, stdout: impl Write) -> Result<()> {
    let agent = std::env::var("HYPERCUBE_AGENT_ID").unwrap_or_default();
    let state = std::env::var("HYPERCUBE_STATE").unwrap_or_default();
    if agent.is_empty() || state.is_empty() {
        return Err("HYPERCUBE_AGENT_ID and HYPERCUBE_STATE must be set".into());
    }
    Server::new(agent, state.into(), config::path()).serve(stdin, stdout)
}

fn schema(required: &[&str], properties: &[&str]) -> Value {
    let properties: Map<String, Value> = properties
        .iter()
        .map(|name| (name.to_string(), json!({"type": "string"})))
        .collect();
    json!({"type": "object", "properties": properties, "required": required})
}

fn tool(name: &str, description: String, input_schema: Value) -> Value {
    json!({"name": name, "description": description, "inputSchema": input_schema})
}

/// The tools for a workspace's mode and the configured profiles. Outside
/// Git, workers cannot have worktrees, so `create_agent` requires `dir`.
pub fn tools(no_git: bool, profiles: &[Profile]) -> Vec<Value> {
    let create_properties = ["name", "profile", "dir", "message"];
    let (mut create, remove) = if no_git {
        (
            tool(
                "create_agent",
                format!("Orchestrator only. Create a worker and launch it. Given dir, {DIR_ARG}. {PROFILE_ARG}"),
                schema(&["name", "profile", "dir"], &create_properties),
            ),
            tool(
                "remove_agent",
                "Orchestrator only. Close a worker's pane and remove it from the team. Its directory is left as it is.".into(),
                schema(&["id"], &["id"]),
            ),
        )
    } else {
        (
            tool(
                "create_agent",
                format!("Orchestrator only. Create a worker and launch it. Without dir, the worker gets its own branch and worktree from HEAD. With dir, {DIR_ARG}, and Hypercube creates no branch or worktree. {PROFILE_ARG}"),
                schema(&["name", "profile"], &create_properties),
            ),
            tool(
                "remove_agent",
                "Orchestrator only. Close a worker's pane and remove it from the team. A worker with a worktree also has its worktree and branch removed, and Git refuses dirty worktrees and unmerged branches. A worker without one has its directory left as it is.".into(),
                schema(&["id"], &["id"]),
            ),
        )
    };
    if !profiles.is_empty() {
        let names: Vec<&str> = profiles.iter().map(|p| p.name.as_str()).collect();
        create["inputSchema"]["properties"]["profile"] = json!({"type": "string", "enum": names});
    }
    vec![
        tool(
            "send_message",
            "Send a message to another agent by ID or name.".into(),
            schema(&["to", "text"], &["to", "text"]),
        ),
        tool(
            "list_agents",
            "List the team: id, name, role, harness, activity, directory, and branch.".into(),
            schema(&[], &[]),
        ),
        create,
        remove,
    ]
}

impl Server {
    /// A server for agent `agent` of the workspace whose state file is
    /// `state`, reading the profiles from the configuration file `config`.
    pub fn new(agent: String, state: PathBuf, config: PathBuf) -> Server {
        Server {
            agent,
            state,
            config,
        }
    }

    /// Answer MCP requests on `stdin` until it closes.
    pub fn serve(&self, stdin: impl BufRead, mut stdout: impl Write) -> Result<()> {
        for line in stdin.lines() {
            let Ok(message) = serde_json::from_str::<Value>(&line?) else {
                continue;
            };
            // A message without an ID is a notification and gets no reply.
            let Some(id) = message.get("id") else {
                continue;
            };
            let params = &message["params"];
            let result = match message["method"].as_str().unwrap_or("") {
                "initialize" => json!({
                    "protocolVersion": params["protocolVersion"].as_str().unwrap_or(""),
                    "capabilities": {"tools": {}},
                    "serverInfo": {"name": "hypercube", "version": "0.1.0"},
                }),
                "tools/list" => {
                    // An unreadable state or configuration fails the tool calls
                    // that need it, so it needs no separate error here.
                    let state = store::load(&self.state).unwrap_or_default();
                    let config = config::load_from(&self.config).unwrap_or_default();
                    json!({"tools": tools(state.no_git, &config.profiles)})
                }
                "tools/call" => {
                    let name = params["name"].as_str().unwrap_or("");
                    let no_arguments = Map::new();
                    let arguments = params["arguments"].as_object().unwrap_or(&no_arguments);
                    let (text, is_error) = match self.call(name, arguments) {
                        Ok(text) => (text, false),
                        Err(e) => (e.to_string(), true),
                    };
                    json!({"content": [{"type": "text", "text": text}], "isError": is_error})
                }
                _ => json!({}),
            };
            writeln!(
                stdout,
                "{}",
                json!({"jsonrpc": "2.0", "id": id, "result": result})
            )?;
            stdout.flush()?;
        }
        Ok(())
    }

    /// Run a tool. A string argument that is missing or not a string is
    /// empty.
    pub fn call(&self, name: &str, arguments: &Map<String, Value>) -> Result<String> {
        let arg = |key: &str| arguments.get(key).and_then(Value::as_str).unwrap_or("");
        match name {
            "send_message" => self.send_message(arg("to"), arg("text")),
            "list_agents" => self.list_agents(),
            "create_agent" => {
                self.create_agent(arg("name"), arg("profile"), arg("dir"), arg("message"))
            }
            "remove_agent" => self.remove_agent(arg("id")),
            _ => Err(format!("unknown tool {name:?}").into()),
        }
    }

    fn socket(&self) -> Result<PathBuf> {
        control::socket_path(&self.state)
    }

    /// Ask the host to do something and fail if it cannot.
    fn ask_host(&self, request: Request) -> Result<()> {
        control::request(&self.socket()?, &request)?.into_result()
    }

    fn list_agents(&self) -> Result<String> {
        #[derive(Serialize)]
        struct Entry<'a> {
            id: &'a str,
            name: &'a str,
            role: &'a str,
            harness: &'a str,
            activity: &'a str,
            directory: &'a str,
            branch: &'a str,
        }
        let state = store::load(&self.state)?;
        let team: Vec<Entry> = state
            .agents
            .iter()
            .map(|a| Entry {
                id: &a.id,
                name: &a.name,
                role: &a.role,
                harness: &a.harness,
                activity: &a.activity,
                directory: &a.dir,
                branch: &a.branch,
            })
            .collect();
        Ok(serde_json::to_string_pretty(&team)?)
    }

    fn send_message(&self, to: &str, text: &str) -> Result<String> {
        let (recipient, idle) = store::update(&self.state, |s| {
            let a = s.agent(to)?;
            if a.id == self.agent {
                return Err("cannot send a message to yourself".into());
            }
            let recipient = a.clone();
            let idle = a.activity == store::IDLE;
            s.messages.push(Message {
                from: self.agent.clone(),
                to: recipient.id.clone(),
                text: text.into(),
            });
            Ok((recipient, idle))
        })?;
        if idle {
            self.ask_host(Request::Nudge {
                agent: recipient.id.clone(),
            })
            .map_err(|e| format!("message queued, but waking {} failed: {e}", recipient.name))?;
        }
        Ok(format!(
            "Message queued for {} ({}).",
            recipient.name, recipient.id
        ))
    }

    fn create_agent(&self, name: &str, profile: &str, dir: &str, message: &str) -> Result<String> {
        if !valid_name(name) {
            return Err("name must be lowercase letters, digits, and hyphens".into());
        }
        let config = config::load_from(&self.config)?;
        let spec = &config.profile(profile)?.spec;
        let mut agent = Agent {
            id: store::new_id()?,
            name: name.into(),
            role: store::WORKER.into(),
            harness: spec.harness.clone(),
            model: spec.model.clone(),
            effort: spec.effort.clone(),
            activity: store::BUSY.into(),
            ..Agent::default()
        };
        let root = store::update(&self.state, |s| {
            let root = PathBuf::from(&orchestrator(s, &self.agent)?.dir);
            if s.agent(name).is_ok() {
                return Err(format!("an agent named {name} already exists").into());
            }
            if !dir.is_empty() {
                agent.dir = path_str(&workspace::resolve(&root, dir)?)?;
            } else if s.no_git {
                return Err("dir is required: this workspace is not a Git checkout, so workers cannot have worktrees".into());
            } else {
                agent.dir = path_str(&workspace::worktree_dir(&root, name))?;
                agent.branch = workspace::worker_branch(name);
            }
            s.agents.push(agent.clone());
            if !message.is_empty() {
                s.messages.push(Message {
                    from: self.agent.clone(),
                    to: agent.id.clone(),
                    text: message.into(),
                });
            }
            Ok(root)
        })?;

        if !agent.branch.is_empty()
            && let Err(e) = workspace::add_worktree(&root, Path::new(&agent.dir), &agent.branch)
        {
            self.forget(&agent.id);
            return Err(e);
        }
        let launch = Request::Launch {
            agent: agent.id.clone(),
            prompt: NUDGE.into(),
        };
        if let Err(e) = self.ask_host(launch) {
            let mut message = e.to_string();
            if !agent.branch.is_empty()
                && let Err(e) =
                    workspace::remove_worktree(&root, Path::new(&agent.dir), &agent.branch)
            {
                message.push_str(&format!("; removing the worktree failed: {e}"));
            }
            self.forget(&agent.id);
            return Err(message.into());
        }
        Ok(if agent.branch.is_empty() {
            format!("Created {} ({}) in {}.", agent.name, agent.id, agent.dir)
        } else {
            format!(
                "Created {} ({}) on branch {} in {}.",
                agent.name, agent.id, agent.branch, agent.dir
            )
        })
    }

    /// Delete an agent and its queued messages after a failed creation.
    fn forget(&self, id: &str) {
        let _ = store::update(&self.state, |s| {
            s.remove(id);
            Ok(())
        });
    }

    fn remove_agent(&self, id: &str) -> Result<String> {
        let state = store::load(&self.state)?;
        let root = PathBuf::from(&orchestrator(&state, &self.agent)?.dir);
        let agent = state.agent(id)?;
        if agent.role != store::WORKER {
            return Err("only workers can be removed".into());
        }
        let dir = Path::new(&agent.dir);
        if !agent.branch.is_empty() {
            workspace::removable(&root, dir, &agent.branch)?;
        }
        self.ask_host(Request::Close {
            agent: agent.id.clone(),
        })
        .map_err(|e| format!("closing the pane of {}: {e}", agent.name))?;
        if !agent.branch.is_empty() {
            workspace::remove_worktree(&root, dir, &agent.branch)?;
        }
        self.forget(&agent.id);
        Ok(format!("Removed {} ({}).", agent.name, agent.id))
    }
}

/// Lowercase letters, digits, and hyphens, starting with a letter or digit.
fn valid_name(name: &str) -> bool {
    let mut chars = name.chars();
    chars
        .next()
        .is_some_and(|c| c.is_ascii_lowercase() || c.is_ascii_digit())
        && chars.all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
}

fn orchestrator<'a>(state: &'a State, id: &str) -> Result<&'a Agent> {
    let agent = state.agent(id)?;
    if agent.role != store::ORCHESTRATOR {
        return Err("only the orchestrator can do this".into());
    }
    Ok(agent)
}

fn path_str(path: &Path) -> Result<String> {
    path.to_str()
        .map(str::to_string)
        .ok_or_else(|| format!("{} is not valid UTF-8", path.display()).into())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::control::{Reply, testing};
    use crate::testutil::TempDir;
    use std::fs;
    use std::process::Command;
    use std::sync::{Arc, Mutex};

    type Seen = Arc<Mutex<Vec<Request>>>;

    fn git(dir: &Path, args: &[&str]) {
        let out = Command::new("git")
            .args(["-c", "user.name=t", "-c", "user.email=t@t"])
            .args(args)
            .current_dir(dir)
            .output()
            .unwrap();
        assert!(
            out.status.success(),
            "git {args:?}: {}",
            String::from_utf8_lossy(&out.stderr)
        );
    }

    /// A checkout with one commit.
    fn repo() -> TempDir {
        let dir = TempDir::new();
        git(dir.path(), &["init", "-q", "-b", "main"]);
        git(dir.path(), &["commit", "-q", "--allow-empty", "-m", "init"]);
        dir
    }

    fn has_branch(root: &Path, branch: &str) -> bool {
        let reference = format!("refs/heads/{branch}");
        Command::new("git")
            .args(["rev-parse", "--verify", "--quiet", &reference])
            .current_dir(root)
            .status()
            .unwrap()
            .success()
    }

    /// A workspace whose orchestrator `a` works in `orchestrator_dir`, and
    /// whose worker `w1`, if there is a directory for it, works directly in
    /// that directory.
    struct Team {
        _dir: TempDir,
        server: Server,
        state: PathBuf,
    }

    fn team(no_git: bool, orchestrator_dir: &Path, worker_dir: Option<&Path>) -> Team {
        let dir = TempDir::new();
        let config = dir.path().join("config.json");
        fs::write(
            &config,
            r#"{
  "orchestrator": {"harness": "claude", "model": "opus", "effort": "high"},
  "profiles": [{"name": "sol", "description": "coding.", "harness": "pi", "model": "gpt-6.1-sol", "effort": "medium"}]
}"#,
        )
        .unwrap();
        let state = dir.path().join("w.json");
        store::update(&state, |s| {
            s.no_git = no_git;
            s.agents.push(Agent {
                id: "a".into(),
                name: "orchestrator".into(),
                role: store::ORCHESTRATOR.into(),
                dir: orchestrator_dir.to_str().unwrap().into(),
                ..Agent::default()
            });
            if let Some(worker_dir) = worker_dir {
                s.agents.push(Agent {
                    id: "b".into(),
                    name: "w1".into(),
                    role: store::WORKER.into(),
                    dir: worker_dir.to_str().unwrap().into(),
                    ..Agent::default()
                });
            }
            Ok(())
        })
        .unwrap();
        Team {
            server: Server::new("a".into(), state.clone(), config),
            state,
            _dir: dir,
        }
    }

    impl Team {
        /// Start a host that answers every request with `reply`.
        fn host(&self, reply: impl Fn(&Request) -> Reply + Send + 'static) -> Seen {
            testing::serve(&control::socket_path(&self.state).unwrap(), reply)
        }

        fn state(&self) -> State {
            store::load(&self.state).unwrap()
        }

        fn call(&self, name: &str, args: &[(&str, &str)]) -> Result<String> {
            let arguments: Map<String, Value> = args
                .iter()
                .map(|(k, v)| (k.to_string(), json!(v)))
                .collect();
            self.server.call(name, &arguments)
        }

        fn error(&self, name: &str, args: &[(&str, &str)]) -> String {
            self.call(name, args).unwrap_err().to_string()
        }

        fn as_agent(&self, id: &str) -> Server {
            Server::new(id.into(), self.state.clone(), self.server.config.clone())
        }
    }

    fn profiles() -> Vec<Profile> {
        ["sol", "haiku"]
            .iter()
            .map(|name| Profile {
                name: name.to_string(),
                ..Profile::default()
            })
            .collect()
    }

    fn find<'a>(tools: &'a [Value], name: &str) -> &'a Value {
        tools.iter().find(|t| t["name"] == name).unwrap()
    }

    #[test]
    fn schemas_in_a_git_workspace() {
        let tools = tools(false, &profiles());
        let names: Vec<_> = tools.iter().map(|t| t["name"].as_str().unwrap()).collect();
        assert_eq!(
            names,
            [
                "send_message",
                "list_agents",
                "create_agent",
                "remove_agent"
            ]
        );
        let create = &find(&tools, "create_agent")["inputSchema"];
        assert_eq!(create["required"], json!(["name", "profile"]));
        assert_eq!(
            create["properties"]["profile"],
            json!({"type": "string", "enum": ["sol", "haiku"]})
        );
        for property in ["name", "dir", "message"] {
            assert_eq!(create["properties"][property], json!({"type": "string"}));
        }
        let description = find(&tools, "create_agent")["description"]
            .as_str()
            .unwrap();
        assert!(description.contains("branch and worktree from HEAD"));
        assert!(description.contains("path starting with ~/"));
        let remove = find(&tools, "remove_agent");
        assert_eq!(remove["inputSchema"]["required"], json!(["id"]));
        assert!(remove["description"].as_str().unwrap().contains("worktree"));
        assert_eq!(
            find(&tools, "send_message")["inputSchema"]["required"],
            json!(["to", "text"])
        );
        let list = &find(&tools, "list_agents")["inputSchema"];
        assert_eq!(list["required"], json!([]));
        assert_eq!(list["properties"], json!({}));
    }

    #[test]
    fn schemas_outside_git_require_dir_and_leave_out_worktrees() {
        let tools = tools(true, &profiles());
        let create = &find(&tools, "create_agent")["inputSchema"];
        assert_eq!(create["required"], json!(["name", "profile", "dir"]));
        assert_eq!(
            create["properties"]["profile"]["enum"],
            json!(["sol", "haiku"])
        );
        for t in &tools {
            let description = t["description"].as_str().unwrap();
            assert!(!description.contains("worktree"), "{description}");
        }
    }

    #[test]
    fn without_profiles_the_profile_is_free_text() {
        let tools = tools(false, &[]);
        let create = &find(&tools, "create_agent")["inputSchema"];
        assert_eq!(create["properties"]["profile"], json!({"type": "string"}));
    }

    #[test]
    fn create_agent_validates_before_changing_anything() {
        let repo = repo();
        let team = team(false, repo.path(), Some(repo.path()));
        let no_git = self::team(true, repo.path(), None);
        let check = |team: &Team, args: &[(&str, &str)], expected: &str| {
            let err = team.error("create_agent", args);
            assert!(err.contains(expected), "{args:?}: {err}");
        };
        for name in ["", "Bad", "-x", "a_b"] {
            check(
                &team,
                &[("name", name), ("profile", "sol")],
                "lowercase letters",
            );
        }
        check(
            &team,
            &[("name", "x"), ("profile", "nope")],
            "no profile named",
        );
        check(&team, &[("name", "x")], "no profile named");
        check(
            &team,
            &[("name", "w1"), ("profile", "sol")],
            "already exists",
        );
        check(
            &team,
            &[("name", "x"), ("profile", "sol"), ("dir", "missing")],
            "missing",
        );
        check(
            &no_git,
            &[("name", "x"), ("profile", "sol")],
            "dir is required",
        );
        assert_eq!(team.state().agents.len(), 2);
        assert_eq!(no_git.state().agents.len(), 1);
        assert!(!repo.path().join(".hypercube").exists());

        let worker = team.as_agent("b");
        let args: Map<String, Value> = [("name", "x"), ("profile", "sol")]
            .iter()
            .map(|(k, v)| (k.to_string(), json!(v)))
            .collect();
        let err = worker.call("create_agent", &args).unwrap_err().to_string();
        assert_eq!(err, "only the orchestrator can do this");
        let outsider = team.as_agent("nobody").call("create_agent", &args);
        assert!(outsider.unwrap_err().to_string().contains("no agent"));
    }

    #[test]
    fn create_agent_launches_a_worktree_worker() {
        let repo = repo();
        let team = team(false, repo.path(), None);
        let seen = team.host(|_| Reply::ok());
        let text = team
            .call(
                "create_agent",
                &[("name", "w1"), ("profile", "sol"), ("message", "do it")],
            )
            .unwrap();

        let state = team.state();
        let worker = state.agent("w1").unwrap();
        let worktree = repo.path().join(".hypercube").join("w1");
        assert_eq!(worker.dir, worktree.to_str().unwrap());
        assert_eq!(worker.branch, "hypercube/w1");
        assert_eq!(
            (worker.role.as_str(), worker.activity.as_str()),
            (store::WORKER, store::BUSY)
        );
        assert_eq!(
            (
                worker.harness.as_str(),
                worker.model.as_str(),
                worker.effort.as_str()
            ),
            ("pi", "gpt-6.1-sol", "medium")
        );
        assert_eq!(
            state.messages,
            [Message {
                from: "a".into(),
                to: worker.id.clone(),
                text: "do it".into()
            }]
        );
        assert_eq!(
            text,
            format!(
                "Created w1 ({}) on branch hypercube/w1 in {}.",
                worker.id,
                worktree.display()
            )
        );
        assert!(worktree.is_dir() && has_branch(repo.path(), "hypercube/w1"));
        assert_eq!(
            *seen.lock().unwrap(),
            [Request::Launch {
                agent: worker.id.clone(),
                prompt: NUDGE.into()
            }]
        );
    }

    #[test]
    fn create_agent_in_a_directory_makes_no_worktree() {
        let repo = repo();
        let other = TempDir::new();
        let team = team(false, repo.path(), None);
        let seen = team.host(|_| Reply::ok());
        let text = team
            .call(
                "create_agent",
                &[("name", "here"), ("profile", "sol"), ("dir", ".")],
            )
            .unwrap();
        let other_dir = other.path().to_str().unwrap();
        team.call(
            "create_agent",
            &[("name", "there"), ("profile", "sol"), ("dir", other_dir)],
        )
        .unwrap();

        let state = team.state();
        let here = state.agent("here").unwrap();
        assert_eq!(
            (here.dir.as_str(), here.branch.as_str()),
            (repo.path().to_str().unwrap(), "")
        );
        assert_eq!(text, format!("Created here ({}) in {}.", here.id, here.dir));
        assert_eq!(state.agent("there").unwrap().dir, other_dir);
        assert!(!repo.path().join(".hypercube").exists());
        assert!(state.messages.is_empty());
        assert_eq!(seen.lock().unwrap().len(), 2);
    }

    #[test]
    fn create_agent_rolls_back_when_the_host_refuses_the_launch() {
        let repo = repo();
        let team = team(false, repo.path(), None);
        let seen = team.host(|_| Reply::error("no room for another pane"));
        let err = team.error(
            "create_agent",
            &[("name", "w1"), ("profile", "sol"), ("message", "do it")],
        );
        assert_eq!(err, "no room for another pane");

        let state = team.state();
        assert_eq!(state.agents.len(), 1);
        assert!(state.messages.is_empty());
        assert!(!repo.path().join(".hypercube").join("w1").exists());
        assert!(!has_branch(repo.path(), "hypercube/w1"));
        assert_eq!(seen.lock().unwrap().len(), 1);
    }

    #[test]
    fn create_agent_rolls_back_when_no_host_is_running() {
        let repo = repo();
        let team = team(false, repo.path(), None);
        let err = team.error("create_agent", &[("name", "w1"), ("profile", "sol")]);
        assert!(err.contains("is hypercube running"), "{err}");
        assert_eq!(team.state().agents.len(), 1);
        assert!(!repo.path().join(".hypercube").join("w1").exists());
        assert!(!has_branch(repo.path(), "hypercube/w1"));
    }

    #[test]
    fn create_agent_in_a_directory_rolls_back_the_record_only() {
        let repo = repo();
        let other = TempDir::new();
        let team = team(false, repo.path(), None);
        team.host(|_| Reply::error("no"));
        let dir = other.path().to_str().unwrap();
        team.error(
            "create_agent",
            &[
                ("name", "w1"),
                ("profile", "sol"),
                ("dir", dir),
                ("message", "hi"),
            ],
        );
        let state = team.state();
        assert_eq!(state.agents.len(), 1);
        assert!(state.messages.is_empty());
        assert!(other.path().is_dir());
    }

    #[test]
    fn create_agent_that_cannot_make_the_worktree_keeps_an_existing_branch() {
        let repo = repo();
        git(repo.path(), &["branch", "hypercube/w1"]);
        let team = team(false, repo.path(), None);
        let seen = team.host(|_| Reply::ok());
        team.error("create_agent", &[("name", "w1"), ("profile", "sol")]);
        assert_eq!(team.state().agents.len(), 1);
        assert!(has_branch(repo.path(), "hypercube/w1"));
        assert!(seen.lock().unwrap().is_empty());
    }

    #[test]
    fn send_message_nudges_an_idle_recipient_only() {
        let team = team(false, Path::new("/o"), Some(Path::new("/w")));
        store::update(&team.state, |s| {
            s.agent_mut("w1")?.activity = store::IDLE.into();
            s.agents.push(Agent {
                id: "c".into(),
                name: "busy".into(),
                role: store::WORKER.into(),
                activity: store::BUSY.into(),
                ..Agent::default()
            });
            Ok(())
        })
        .unwrap();
        let seen = team.host(|_| Reply::ok());
        assert_eq!(
            team.call("send_message", &[("to", "w1"), ("text", "hi")])
                .unwrap(),
            "Message queued for w1 (b)."
        );
        assert_eq!(
            team.call("send_message", &[("to", "c"), ("text", "yo")])
                .unwrap(),
            "Message queued for busy (c)."
        );
        assert_eq!(
            *seen.lock().unwrap(),
            [Request::Nudge { agent: "b".into() }]
        );
        let messages = team.state().messages;
        let queued: Vec<_> = messages
            .iter()
            .map(|m| (&m.from[..], &m.to[..], &m.text[..]))
            .collect();
        assert_eq!(queued, [("a", "b", "hi"), ("a", "c", "yo")]);
    }

    #[test]
    fn send_message_validates_the_recipient() {
        let team = team(false, Path::new("/o"), Some(Path::new("/w")));
        assert_eq!(
            team.error("send_message", &[("to", "orchestrator"), ("text", "hi")]),
            "cannot send a message to yourself"
        );
        assert_eq!(
            team.error("send_message", &[("to", "a"), ("text", "hi")]),
            "cannot send a message to yourself"
        );
        assert!(
            team.error("send_message", &[("to", "ghost"), ("text", "hi")])
                .contains("no agent")
        );
        assert!(team.state().messages.is_empty());
    }

    #[test]
    fn send_message_reports_a_failed_nudge_and_keeps_the_message() {
        let team = team(false, Path::new("/o"), Some(Path::new("/w")));
        store::update(&team.state, |s| {
            s.agent_mut("w1")?.activity = store::IDLE.into();
            Ok(())
        })
        .unwrap();
        team.host(|_| Reply::error("no pane"));
        let err = team.error("send_message", &[("to", "w1"), ("text", "hi")]);
        assert_eq!(err, "message queued, but waking w1 failed: no pane");
        assert_eq!(team.state().messages.len(), 1);
    }

    #[test]
    fn list_agents_describes_the_team() {
        let team = team(false, Path::new("/o"), Some(Path::new("/w")));
        let text = team.call("list_agents", &[]).unwrap();
        let list: Value = serde_json::from_str(&text).unwrap();
        assert_eq!(list.as_array().unwrap().len(), 2);
        assert_eq!(
            list[1],
            json!({"id": "b", "name": "w1", "role": "worker", "harness": "", "activity": "", "directory": "/w", "branch": ""})
        );
        assert!(
            text.starts_with(
                "[\n  {\n    \"id\": \"a\",\n    \"name\": \"orchestrator\",\n    \"role\""
            ),
            "{text}"
        );
    }

    #[test]
    fn remove_agent_leaves_a_worker_directory_alone() {
        for no_git in [true, false] {
            let repo = repo();
            let worker_dir = TempDir::new();
            fs::write(worker_dir.path().join("keep"), "").unwrap();
            let team = team(no_git, repo.path(), Some(worker_dir.path()));
            store::update(&team.state, |s| {
                s.messages.push(Message {
                    to: "b".into(),
                    ..Message::default()
                });
                Ok(())
            })
            .unwrap();
            let seen = team.host(|_| Reply::ok());
            assert_eq!(
                team.call("remove_agent", &[("id", "w1")]).unwrap(),
                "Removed w1 (b)."
            );
            let state = team.state();
            assert_eq!(state.agents.len(), 1, "no_git {no_git}");
            assert!(state.messages.is_empty());
            assert!(worker_dir.path().join("keep").exists());
            assert_eq!(
                *seen.lock().unwrap(),
                [Request::Close { agent: "b".into() }]
            );
        }
    }

    #[test]
    fn remove_agent_removes_the_worktree_and_branch() {
        let repo = repo();
        let team = team(false, repo.path(), None);
        let seen = team.host(|_| Reply::ok());
        team.call("create_agent", &[("name", "w1"), ("profile", "sol")])
            .unwrap();
        let id = team.state().agent("w1").unwrap().id.clone();
        team.call("remove_agent", &[("id", &id)]).unwrap();
        assert_eq!(team.state().agents.len(), 1);
        assert!(!repo.path().join(".hypercube").join("w1").exists());
        assert!(!has_branch(repo.path(), "hypercube/w1"));
        assert_eq!(seen.lock().unwrap()[1], Request::Close { agent: id });
    }

    #[test]
    fn remove_agent_refuses_unmerged_work_before_closing_anything() {
        let repo = repo();
        let team = team(false, repo.path(), None);
        let seen = team.host(|_| Reply::ok());
        team.call("create_agent", &[("name", "w1"), ("profile", "sol")])
            .unwrap();
        let worktree = repo.path().join(".hypercube").join("w1");

        fs::write(worktree.join("new"), "x").unwrap();
        let err = team.error("remove_agent", &[("id", "w1")]);
        assert!(err.contains("uncommitted changes"), "{err}");

        git(&worktree, &["add", "new"]);
        git(&worktree, &["commit", "-q", "-m", "work"]);
        let err = team.error("remove_agent", &[("id", "w1")]);
        assert!(err.contains("not merged"), "{err}");

        assert_eq!(team.state().agents.len(), 2);
        assert!(worktree.is_dir() && has_branch(repo.path(), "hypercube/w1"));
        assert_eq!(seen.lock().unwrap().len(), 1, "only the launch");
    }

    #[test]
    fn remove_agent_stops_when_the_host_cannot_close_the_pane() {
        let repo = repo();
        let team = team(false, repo.path(), Some(repo.path()));
        team.host(|_| Reply::error("no such pane"));
        let err = team.error("remove_agent", &[("id", "w1")]);
        assert_eq!(err, "closing the pane of w1: no such pane");
        assert_eq!(team.state().agents.len(), 2);
    }

    #[test]
    fn remove_agent_validates_caller_and_target() {
        let team = team(false, Path::new("/o"), Some(Path::new("/w")));
        assert_eq!(
            team.error("remove_agent", &[("id", "a")]),
            "only workers can be removed"
        );
        assert!(
            team.error("remove_agent", &[("id", "ghost")])
                .contains("no agent")
        );
        assert!(team.error("remove_agent", &[]).contains("no agent"));
        let worker = team.as_agent("b");
        let args: Map<String, Value> = [("id".to_string(), json!("b"))].into_iter().collect();
        assert_eq!(
            worker.call("remove_agent", &args).unwrap_err().to_string(),
            "only the orchestrator can do this"
        );
        assert_eq!(team.state().agents.len(), 2);
    }

    #[test]
    fn unknown_tools_are_errors() {
        let team = team(false, Path::new("/o"), None);
        assert_eq!(team.error("explode", &[]), "unknown tool \"explode\"");
    }

    /// The responses to `lines`, one per line.
    fn exchange(team: &Team, lines: &[&str]) -> Vec<Value> {
        let input = lines.join("\n") + "\n";
        let mut output = Vec::new();
        team.server.serve(input.as_bytes(), &mut output).unwrap();
        String::from_utf8(output)
            .unwrap()
            .lines()
            .map(|l| serde_json::from_str(l).unwrap())
            .collect()
    }

    #[test]
    fn the_server_answers_json_rpc_lines() {
        let team = team(false, Path::new("/o"), Some(Path::new("/w")));
        let replies = exchange(
            &team,
            &[
                r#"{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}"#,
                r#"{"jsonrpc":"2.0","method":"notifications/initialized"}"#,
                "not json",
                r#"{"jsonrpc":"2.0","id":"x","method":"tools/list"}"#,
                r#"{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_agents","arguments":{}}}"#,
                r#"{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"send_message","arguments":{"to":"a","text":"hi"}}}"#,
                r#"{"jsonrpc":"2.0","id":5,"method":"ping"}"#,
            ],
        );
        assert_eq!(replies.len(), 5);
        assert_eq!(
            replies[0],
            json!({"jsonrpc": "2.0", "id": 1, "result": {
                "protocolVersion": "2025-06-18",
                "capabilities": {"tools": {}},
                "serverInfo": {"name": "hypercube", "version": "0.1.0"},
            }})
        );
        assert_eq!(replies[1]["id"], "x");
        let listed = replies[1]["result"]["tools"].as_array().unwrap();
        assert_eq!(listed.len(), 4);
        assert_eq!(
            find(listed, "create_agent")["inputSchema"]["properties"]["profile"]["enum"],
            json!(["sol"])
        );
        let result = &replies[2]["result"];
        assert_eq!(result["isError"], false);
        assert_eq!(result["content"][0]["type"], "text");
        assert!(
            result["content"][0]["text"]
                .as_str()
                .unwrap()
                .contains("\"w1\"")
        );
        assert_eq!(replies[3]["result"]["isError"], true);
        assert_eq!(
            replies[3]["result"]["content"][0]["text"],
            "cannot send a message to yourself"
        );
        assert_eq!(replies[4], json!({"jsonrpc": "2.0", "id": 5, "result": {}}));
    }

    #[test]
    fn tools_list_follows_the_workspace_mode() {
        let team = team(true, Path::new("/o"), None);
        let replies = exchange(
            &team,
            &[r#"{"jsonrpc":"2.0","id":1,"method":"tools/list"}"#],
        );
        let listed = replies[0]["result"]["tools"].as_array().unwrap();
        assert_eq!(
            find(listed, "create_agent")["inputSchema"]["required"],
            json!(["name", "profile", "dir"])
        );
    }

    #[test]
    fn tools_list_survives_unreadable_configuration() {
        let team = team(false, Path::new("/o"), None);
        fs::write(&team.server.config, "{").unwrap();
        let replies = exchange(
            &team,
            &[r#"{"jsonrpc":"2.0","id":1,"method":"tools/list"}"#],
        );
        let listed = replies[0]["result"]["tools"].as_array().unwrap();
        assert_eq!(
            find(listed, "create_agent")["inputSchema"]["properties"]["profile"],
            json!({"type": "string"})
        );
    }

    #[test]
    fn names_are_lowercase_letters_digits_and_hyphens() {
        for name in ["a", "w1", "0", "fix-bug-2", "9-lives"] {
            assert!(valid_name(name), "{name}");
        }
        for name in ["", "-a", "A", "a_b", "a b", "a.b", "é", "a\n"] {
            assert!(!valid_name(name), "{name:?}");
        }
    }
}
