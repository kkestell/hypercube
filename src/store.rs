//! A workspace's agents, message queue, and scheduled tasks in one JSON file. Every change
//! holds an exclusive lock, so hooks, the MCP server, and the host can update
//! the file concurrently.

use std::error::Error;
use std::fs::{self, File, OpenOptions, TryLockError};
use std::io::{ErrorKind, Read};
use std::os::unix::fs::MetadataExt;
use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime};

use serde::{Deserialize, Deserializer, Serialize};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

pub const ORCHESTRATOR: &str = "orchestrator";
pub const WORKER: &str = "worker";
pub const BUSY: &str = "busy";
pub const IDLE: &str = "idle";
/// The activity of an agent the host has given up running until the session
/// starts again.
pub const EXITED: &str = "exited";
/// The team every workspace has, whose orchestrator is named `orchestrator`.
pub const DEFAULT_TEAM: &str = "default";
/// The sender of the messages a scheduled task's firings queue for its
/// orchestrator.
pub const SCHEDULE: &str = "schedule";

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Agent {
    pub id: String,
    pub name: String,
    pub role: String,
    pub harness: String,
    pub model: String,
    pub effort: String,
    pub dir: String,
    pub branch: String,
    /// The name of the agent's team. An agent written without one is in the
    /// default team.
    #[serde(default = "default_team")]
    pub team: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub session_id: String,
    pub activity: String,
    /// The number of the agent's newest `hypercube wait`. An older one ends
    /// when it sees a newer number.
    #[serde(skip_serializing_if = "is_zero")]
    pub waiter: u64,
    /// Set while the agent waits for the user to answer a tool approval.
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub attention: bool,
}

fn default_team() -> String {
    DEFAULT_TEAM.into()
}

fn is_zero(n: &u64) -> bool {
    *n == 0
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Message {
    pub from: String,
    pub to: String,
    pub text: String,
}

/// A scheduled task. Its team, and its orchestrator's, has its name.
#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Task {
    pub name: String,
    /// Five cron fields; see `schedule::Schedule`.
    pub schedule: String,
    /// The text each firing sends the task's orchestrator.
    pub prompt: String,
    /// When the task last fired, in Unix seconds, or 0 if it never has.
    pub last_run: u64,
}

impl Task {
    /// How long ago the task last fired, or "never".
    pub fn ran(&self) -> String {
        match self.last_run {
            0 => "never".into(),
            secs => ago(SystemTime::UNIX_EPOCH + Duration::from_secs(secs)),
        }
    }
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct State {
    #[serde(deserialize_with = "null_as_default")]
    pub agents: Vec<Agent>,
    #[serde(deserialize_with = "null_as_default")]
    pub messages: Vec<Message>,
    /// The scheduled tasks, in the order they were created.
    #[serde(deserialize_with = "null_as_default")]
    pub tasks: Vec<Task>,
    /// Set for a workspace whose directory is not a Git checkout. Its workers
    /// work directly in existing directories, without branches or worktrees.
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub no_git: bool,
    /// The workspace's accent color, numbered in `colors::ACCENTS`.
    #[serde(skip_serializing_if = "is_zero")]
    pub accent: u64,
}

/// A `null` list, which an empty list may have been written as, loads empty.
fn null_as_default<'de, D, T>(deserializer: D) -> std::result::Result<T, D::Error>
where
    D: Deserializer<'de>,
    T: Deserialize<'de> + Default,
{
    Ok(Option::deserialize(deserializer)?.unwrap_or_default())
}

/// The directory that holds state files and worktrees.
pub fn dir() -> PathBuf {
    let base = match std::env::var_os("XDG_STATE_HOME").filter(|v| !v.is_empty()) {
        Some(base) => PathBuf::from(base),
        None => PathBuf::from(std::env::var_os("HOME").unwrap_or_default())
            .join(".local")
            .join("state"),
    };
    base.join("hypercube")
}

/// The state file of the named workspace.
pub fn path(workspace: &str) -> PathBuf {
    dir().join(format!("{workspace}.json"))
}

/// The workspace name of a state file path.
pub fn workspace(path: &Path) -> String {
    let name = path.file_name().unwrap_or_default().to_string_lossy();
    name.strip_suffix(".json").unwrap_or(&name).to_string()
}

/// A short random identifier.
pub fn new_id() -> Result<String> {
    let mut bytes = [0u8; 4];
    File::open("/dev/urandom")
        .and_then(|mut f| f.read_exact(&mut bytes))
        .map_err(|e| format!("read /dev/urandom: {e}"))?;
    Ok(bytes.iter().map(|b| format!("{b:02x}")).collect())
}

/// Read the state without locking. Writes go through `rename`, so a read
/// always sees a complete file. A missing file is an empty state.
pub fn load(path: &Path) -> Result<State> {
    let data = match fs::read(path) {
        Ok(data) => data,
        Err(e) if e.kind() == ErrorKind::NotFound => return Ok(State::default()),
        Err(e) => return Err(format!("read {}: {e}", path.display()).into()),
    };
    serde_json::from_slice(&data).map_err(|e| format!("parse {}: {e}", path.display()).into())
}

fn lock_path(path: &Path) -> PathBuf {
    path.with_extension("lock")
}

/// Open the state file's lock and hold it exclusively until the file is
/// dropped.
fn lock(path: &Path) -> Result<File> {
    let lock_path = lock_path(path);
    lock_file(&lock_path, true)?.ok_or_else(|| format!("lock {}", lock_path.display()).into())
}

/// Lock the file at `path` exclusively, creating it, until the file is
/// dropped. Waits for another holder if `wait`, or else returns `None`.
/// `delete` unlinks lock files while holding them, so a lock obtained on a
/// file that is no longer at the path is released and the current file
/// locked instead.
pub fn lock_file(path: &Path, wait: bool) -> Result<Option<File>> {
    if let Some(dir) = path.parent() {
        fs::create_dir_all(dir).map_err(|e| format!("create {}: {e}", dir.display()))?;
    }
    loop {
        let file = OpenOptions::new()
            .create(true)
            .truncate(false)
            .write(true)
            .open(path)
            .map_err(|e| format!("open {}: {e}", path.display()))?;
        let locked = if wait {
            file.lock().map_err(TryLockError::Error)
        } else {
            file.try_lock()
        };
        match locked {
            Ok(()) => {}
            Err(TryLockError::WouldBlock) => return Ok(None),
            Err(TryLockError::Error(e)) => {
                return Err(format!("lock {}: {e}", path.display()).into());
            }
        }
        if at_path(&file, path)? {
            return Ok(Some(file));
        }
    }
}

/// Whether `file` is the file at `path`.
fn at_path(file: &File, path: &Path) -> Result<bool> {
    let held = file
        .metadata()
        .map_err(|e| format!("stat {}: {e}", path.display()))?;
    match fs::metadata(path) {
        Ok(current) => Ok((current.dev(), current.ino()) == (held.dev(), held.ino())),
        Err(e) if e.kind() == ErrorKind::NotFound => Ok(false),
        Err(e) => Err(format!("stat {}: {e}", path.display()).into()),
    }
}

/// Remove the state file and its lock. Waits for any update in progress.
pub fn delete(path: &Path) -> Result<()> {
    let _lock = lock(path)?;
    for p in [path.to_path_buf(), lock_path(path)] {
        match fs::remove_file(&p) {
            Err(e) if e.kind() != ErrorKind::NotFound => {
                return Err(format!("remove {}: {e}", p.display()).into());
            }
            _ => {}
        }
    }
    Ok(())
}

/// Apply `f` to the state under an exclusive lock and write the result. If
/// `f` returns an error, or leaves an existing state as it was, nothing is
/// written, so the file's modification time is its last change.
pub fn update<T>(path: &Path, f: impl FnOnce(&mut State) -> Result<T>) -> Result<T> {
    let _lock = lock(path)?;
    let before = load(path)?;
    let mut state = before.clone();
    let value = f(&mut state)?;
    if state == before && path.exists() {
        return Ok(value);
    }
    let data = serde_json::to_string_pretty(&state)?;
    let mut tmp = path.as_os_str().to_owned();
    tmp.push(".tmp");
    let tmp = PathBuf::from(tmp);
    fs::write(&tmp, data).map_err(|e| format!("write {}: {e}", tmp.display()))?;
    fs::rename(&tmp, path).map_err(|e| format!("rename {}: {e}", tmp.display()))?;
    Ok(value)
}

/// A workspace with a state file.
pub struct Workspace {
    pub path: PathBuf,
    pub state: State,
}

impl Workspace {
    /// The workspace's checkout; see `State::root`.
    pub fn root(&self) -> Option<&Path> {
        self.state.root()
    }

    /// The text that names the workspace: its directory, or its state's name
    /// when the state has no orchestrator.
    pub fn name(&self) -> String {
        self.root()
            .map_or_else(|| workspace(&self.path), crate::workspace::short)
    }

    /// When the workspace was last active: when its state last changed, as
    /// agents start and end turns and messages are queued and delivered.
    pub fn active(&self) -> Result<SystemTime> {
        fs::metadata(&self.path)
            .and_then(|m| m.modified())
            .map_err(|e| format!("stat {}: {e}", self.path.display()).into())
    }
}

/// Every workspace with a state file.
pub fn workspaces() -> Result<Vec<Workspace>> {
    let entries = match fs::read_dir(dir()) {
        Ok(entries) => entries.collect::<std::io::Result<Vec<_>>>()?,
        Err(e) if e.kind() == ErrorKind::NotFound => Vec::new(),
        Err(e) => return Err(format!("read {}: {e}", dir().display()).into()),
    };
    let mut list = Vec::new();
    for entry in entries {
        let path = entry.path();
        if path.extension().is_none_or(|e| e != "json") {
            continue;
        }
        let state = load(&path)?;
        list.push(Workspace { path, state });
    }
    Ok(list)
}

/// How long before now `time` was.
pub fn ago(time: SystemTime) -> String {
    let d = SystemTime::now()
        .duration_since(time)
        .unwrap_or(Duration::ZERO)
        .as_secs();
    match d {
        0..60 => "just now".to_string(),
        60..3600 => format!("{}m ago", d / 60),
        3600..86400 => format!("{}h ago", d / 3600),
        _ => format!("{}d ago", d / 86400),
    }
}

impl State {
    /// The workspace's checkout: the default team's orchestrator's
    /// directory.
    pub fn root(&self) -> Option<&Path> {
        let o = self.team_orchestrator(DEFAULT_TEAM)?;
        Some(Path::new(&o.dir))
    }

    /// The name of every team, in the order of their orchestrators: the
    /// default team first, then the others in the order they were created.
    pub fn teams(&self) -> Vec<&str> {
        let mut teams: Vec<&str> = self
            .agents
            .iter()
            .filter(|a| a.role == ORCHESTRATOR)
            .map(|a| a.team.as_str())
            .collect();
        teams.sort_by_key(|&t| t != DEFAULT_TEAM);
        teams
    }

    /// The orchestrator of the named team.
    pub fn team_orchestrator(&self, team: &str) -> Option<&Agent> {
        self.agents
            .iter()
            .find(|a| a.role == ORCHESTRATOR && a.team == team)
    }

    /// The scheduled task named `name`.
    pub fn task(&self, name: &str) -> Option<&Task> {
        self.tasks.iter().find(|t| t.name == name)
    }

    /// Whether any agent of `team` is busy. A running task does not fire.
    pub fn running(&self, team: &str) -> bool {
        self.agents
            .iter()
            .any(|a| a.team == team && a.activity == BUSY)
    }

    /// Whether the orchestrator of `team` has exited and is not running
    /// again.
    pub fn failed(&self, team: &str) -> bool {
        self.team_orchestrator(team)
            .is_some_and(|o| o.activity == EXITED)
    }

    /// How many scheduled tasks there are, and how many of them are running.
    pub fn task_counts(&self) -> (usize, usize) {
        let running = self.tasks.iter().filter(|t| self.running(&t.name)).count();
        (self.tasks.len(), running)
    }

    /// The agent with the given ID or name.
    pub fn agent(&self, id_or_name: &str) -> Result<&Agent> {
        self.agents
            .iter()
            .find(|a| a.id == id_or_name || a.name == id_or_name)
            .ok_or_else(|| format!("no agent {id_or_name:?}").into())
    }

    pub fn agent_mut(&mut self, id_or_name: &str) -> Result<&mut Agent> {
        self.agents
            .iter_mut()
            .find(|a| a.id == id_or_name || a.name == id_or_name)
            .ok_or_else(|| format!("no agent {id_or_name:?}").into())
    }

    /// Remove and return the messages queued for an agent.
    pub fn take(&mut self, id: &str) -> Vec<Message> {
        let (taken, kept) = std::mem::take(&mut self.messages)
            .into_iter()
            .partition(|m| m.to == id);
        self.messages = kept;
        taken
    }

    /// Delete an agent and its queued messages.
    pub fn remove(&mut self, id: &str) {
        self.agents.retain(|a| a.id != id);
        self.take(id);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testutil::TempDir;
    use std::sync::mpsc;
    use std::time::Duration;

    fn agent(id: &str, name: &str) -> Agent {
        Agent {
            id: id.into(),
            name: name.into(),
            ..Agent::default()
        }
    }

    fn message(to: &str, text: &str) -> Message {
        Message {
            to: to.into(),
            text: text.into(),
            ..Message::default()
        }
    }

    #[test]
    fn concurrent_updates_all_land() {
        let dir = TempDir::new();
        let path = dir.path().join("w.json");
        let threads: Vec<_> = (0..30)
            .map(|i| {
                let path = path.clone();
                std::thread::spawn(move || {
                    update(&path, |s| {
                        s.messages.push(message("a", &i.to_string()));
                        Ok(())
                    })
                    .unwrap();
                })
            })
            .collect();
        for t in threads {
            t.join().unwrap();
        }
        assert_eq!(load(&path).unwrap().messages.len(), 30);
    }

    #[test]
    fn update_error_writes_nothing() {
        let dir = TempDir::new();
        let path = dir.path().join("w.json");
        update(&path, |s| {
            s.agents = vec![agent("a", "")];
            Ok(())
        })
        .unwrap();
        let err = update(&path, |s| -> Result<()> {
            s.agents.clear();
            Err("no".into())
        })
        .unwrap_err();
        assert_eq!(err.to_string(), "no");
        assert_eq!(load(&path).unwrap().agents.len(), 1);
    }

    #[test]
    fn update_returns_the_closures_value() {
        let dir = TempDir::new();
        let path = dir.path().join("w.json");
        let id = update(&path, |s| {
            s.agents.push(agent("a", "one"));
            Ok(s.agents[0].id.clone())
        })
        .unwrap();
        assert_eq!(id, "a");
    }

    #[test]
    fn update_leaves_no_temporary_file() {
        let dir = TempDir::new();
        let path = dir.path().join("w.json");
        update(&path, |_| Ok(())).unwrap();
        let mut names: Vec<_> = fs::read_dir(dir.path())
            .unwrap()
            .map(|e| e.unwrap().file_name().to_string_lossy().into_owned())
            .collect();
        names.sort();
        assert_eq!(names, ["w.json", "w.lock"]);
    }

    #[test]
    fn an_update_that_changes_nothing_writes_nothing() {
        let dir = TempDir::new();
        let path = dir.path().join("w.json");
        // A missing state is written even unchanged.
        update(&path, |_| Ok(())).unwrap();
        assert!(path.exists());
        fs::write(&path, "{\"agents\": []}").unwrap();
        update(&path, |s| {
            s.messages.clear();
            Ok(())
        })
        .unwrap();
        assert_eq!(fs::read_to_string(&path).unwrap(), "{\"agents\": []}");
        update(&path, |s| {
            s.accent = 2;
            Ok(())
        })
        .unwrap();
        assert_eq!(load(&path).unwrap().accent, 2);
    }

    #[test]
    fn load_of_a_missing_file_is_empty() {
        let dir = TempDir::new();
        assert_eq!(load(&dir.path().join("w.json")).unwrap(), State::default());
    }

    #[test]
    fn load_of_invalid_json_names_the_file() {
        let dir = TempDir::new();
        let path = dir.path().join("w.json");
        fs::write(&path, "{").unwrap();
        let err = load(&path).unwrap_err().to_string();
        assert!(err.contains("parse") && err.contains("w.json"), "{err}");
    }

    #[test]
    fn loads_a_state_written_with_a_target() {
        let dir = TempDir::new();
        let path = dir.path().join("w.json");
        fs::write(
            &path,
            r#"{
  "agents": [
    {
      "id": "a1b2c3d4",
      "name": "orchestrator",
      "role": "orchestrator",
      "harness": "claude",
      "model": "opus",
      "effort": "high",
      "dir": "/r",
      "branch": "main",
      "session_id": "s1",
      "target": "%3",
      "activity": "idle"
    }
  ],
  "messages": null
}"#,
        )
        .unwrap();
        let state = load(&path).unwrap();
        assert_eq!(state.agents.len(), 1);
        let a = &state.agents[0];
        assert_eq!((a.id.as_str(), a.session_id.as_str()), ("a1b2c3d4", "s1"));
        assert_eq!((a.branch.as_str(), a.activity.as_str()), ("main", IDLE));
        assert_eq!(a.team, DEFAULT_TEAM);
        assert!(state.messages.is_empty() && state.tasks.is_empty() && !state.no_git);

        // Rewriting drops the target and keeps the rest.
        update(&path, |s| {
            s.accent = 1;
            Ok(())
        })
        .unwrap();
        let text = fs::read_to_string(&path).unwrap();
        assert!(!text.contains("target"), "{text}");
        assert_eq!(load(&path).unwrap(), State { accent: 1, ..state });
    }

    #[test]
    fn serialized_form_keeps_the_field_names() {
        let state = State {
            agents: vec![agent("a", "one")],
            messages: vec![Message {
                from: "a".into(),
                to: "b".into(),
                text: "hi".into(),
            }],
            tasks: vec![Task {
                name: "nightly".into(),
                schedule: "0 3 * * *".into(),
                prompt: "check".into(),
                last_run: 5,
            }],
            no_git: true,
            accent: 3,
        };
        let value = serde_json::to_value(&state).unwrap();
        assert_eq!(value["agents"][0]["id"], "a");
        assert!(value["agents"][0].get("session_id").is_none());
        assert_eq!(value["messages"][0]["from"], "a");
        assert_eq!(value["tasks"][0]["schedule"], "0 3 * * *");
        assert_eq!(value["tasks"][0]["last_run"], 5);
        assert_eq!(value["no_git"], true);
        assert_eq!(value["accent"], 3);
        let value = serde_json::to_value(State::default()).unwrap();
        assert!(value.get("no_git").is_none() && value.get("accent").is_none());
    }

    #[test]
    fn take_and_remove() {
        let mut s = State {
            agents: vec![agent("a", "one"), agent("b", "two")],
            messages: vec![message("a", "1"), message("b", "2"), message("a", "3")],
            ..State::default()
        };
        let taken = s.take("a");
        assert_eq!(taken.len(), 2);
        assert_eq!(s.messages, [message("b", "2")]);
        s.messages.push(message("b", "4"));
        s.remove("b");
        assert_eq!(s.agents, [agent("a", "one")]);
        assert!(s.messages.is_empty());
        assert_eq!(s.agent("one").unwrap().id, "a");
        assert_eq!(s.agent("a").unwrap().name, "one");
        assert!(s.agent("missing").is_err());
        s.agent_mut("one").unwrap().activity = BUSY.into();
        assert_eq!(s.agents[0].activity, BUSY);
    }

    #[test]
    fn teams_follow_their_orchestrators_with_the_default_team_first() {
        let member = |id: &str, role: &str, team: &str| Agent {
            id: id.into(),
            role: role.into(),
            team: team.into(),
            dir: format!("/{id}"),
            ..Agent::default()
        };
        let s = State {
            agents: vec![
                member("d", ORCHESTRATOR, "docs"),
                member("w", WORKER, "docs"),
                member("o", ORCHESTRATOR, DEFAULT_TEAM),
                member("r", ORCHESTRATOR, "review"),
            ],
            ..State::default()
        };
        assert_eq!(s.teams(), [DEFAULT_TEAM, "docs", "review"]);
        assert_eq!(s.team_orchestrator("docs").unwrap().id, "d");
        assert_eq!(s.team_orchestrator(DEFAULT_TEAM).unwrap().id, "o");
        assert!(s.team_orchestrator("missing").is_none());
        assert_eq!(s.root(), Some(Path::new("/o")));
        assert!(State::default().teams().is_empty());
        assert_eq!(State::default().root(), None);
    }

    #[test]
    fn tasks_and_the_running_and_failed_teams() {
        let member = |id: &str, role: &str, team: &str, activity: &str| Agent {
            id: id.into(),
            role: role.into(),
            team: team.into(),
            activity: activity.into(),
            ..Agent::default()
        };
        let task = |name: &str| Task {
            name: name.into(),
            ..Task::default()
        };
        let mut s = State {
            agents: vec![
                member("n", ORCHESTRATOR, "nightly", IDLE),
                member("w", WORKER, "nightly", BUSY),
                member("r", ORCHESTRATOR, "report", EXITED),
                member("x", WORKER, "report", EXITED),
            ],
            tasks: vec![task("nightly"), task("report")],
            ..State::default()
        };
        assert_eq!(s.task("report"), Some(&s.tasks[1]));
        assert_eq!(s.task("missing"), None);
        assert!(s.running("nightly") && !s.failed("nightly"));
        // An exited worker is not busy.
        assert!(!s.running("report") && s.failed("report"));
        assert_eq!(s.task_counts(), (2, 1));
        s.agents[1].activity = IDLE.into();
        assert_eq!(s.task_counts(), (2, 0));
        assert_eq!(s.tasks[0].ran(), "never");
        let now = SystemTime::now().duration_since(SystemTime::UNIX_EPOCH);
        s.tasks[0].last_run = now.unwrap().as_secs() - 7200;
        assert_eq!(s.tasks[0].ran(), "2h ago");
    }

    #[test]
    fn delete_waits_for_update() {
        let dir = TempDir::new();
        let path = dir.path().join("w.json");
        update(&path, |s| {
            s.agents = vec![agent("a", "")];
            Ok(())
        })
        .unwrap();

        let (locked_tx, locked_rx) = mpsc::channel();
        let (release_tx, release_rx) = mpsc::channel::<()>();
        let updater = {
            let path = path.clone();
            std::thread::spawn(move || {
                update(&path, |_| {
                    locked_tx.send(()).unwrap();
                    release_rx.recv().unwrap();
                    Ok(())
                })
                .unwrap();
            })
        };
        locked_rx.recv().unwrap();
        let (deleted_tx, deleted_rx) = mpsc::channel();
        let deleter = {
            let path = path.clone();
            std::thread::spawn(move || {
                deleted_tx
                    .send(delete(&path).map_err(|e| e.to_string()))
                    .unwrap()
            })
        };
        assert!(
            deleted_rx.recv_timeout(Duration::from_millis(50)).is_err(),
            "delete returned while an update held the lock"
        );
        release_tx.send(()).unwrap();
        deleted_rx.recv().unwrap().unwrap();
        updater.join().unwrap();
        deleter.join().unwrap();
        assert!(!path.exists());
        assert!(!lock_path(&path).exists());
    }

    #[test]
    fn a_lock_waiting_during_delete_moves_to_the_replacement() {
        let dir = TempDir::new();
        let path = dir.path().join("w.json");
        let first = lock(&path).unwrap();

        let (locked_tx, locked_rx) = mpsc::channel();
        let (release_tx, release_rx) = mpsc::channel::<()>();
        let waiter = {
            let path = path.clone();
            std::thread::spawn(move || {
                update(&path, |_| {
                    locked_tx.send(()).unwrap();
                    release_rx.recv().unwrap();
                    Ok(())
                })
                .unwrap();
            })
        };
        // The waiter blocks on the lock file that delete unlinks.
        std::thread::sleep(Duration::from_millis(100));
        fs::remove_file(lock_path(&path)).unwrap();
        drop(first);
        locked_rx.recv().unwrap();

        let replacement = OpenOptions::new()
            .create(true)
            .truncate(false)
            .write(true)
            .open(lock_path(&path))
            .unwrap();
        assert!(
            matches!(replacement.try_lock(), Err(fs::TryLockError::WouldBlock)),
            "an update holds a lock file that is no longer at the path"
        );
        release_tx.send(()).unwrap();
        waiter.join().unwrap();
        replacement.lock().unwrap();
    }

    #[test]
    fn delete_of_a_missing_state_succeeds() {
        let dir = TempDir::new();
        delete(&dir.path().join("w.json")).unwrap();
    }

    #[test]
    fn workspace_names_and_paths() {
        let path = path("repo-1a2b3c");
        assert_eq!(path.file_name().unwrap(), "repo-1a2b3c.json");
        assert_eq!(path.parent().unwrap().file_name().unwrap(), "hypercube");
        assert_eq!(workspace(&path), "repo-1a2b3c");
    }

    #[test]
    fn ids_are_eight_hex_digits() {
        let (a, b) = (new_id().unwrap(), new_id().unwrap());
        assert_eq!(a.len(), 8);
        assert!(a.bytes().all(|c| c.is_ascii_hexdigit()));
        assert_ne!(a, b);
    }

    #[test]
    fn a_lock_on_an_unlinked_file_is_not_the_lock() {
        let dir = TempDir::new();
        let path = dir.path().join("w.host.lock");
        let lock = lock_file(&path, false).unwrap().unwrap();
        assert!(at_path(&lock, &path).unwrap());
        // A host that opened the file before delete unlinked it would lock
        // the old file.
        let old = File::open(&path).unwrap();
        fs::remove_file(&path).unwrap();
        assert!(!at_path(&old, &path).unwrap());
        let new = lock_file(&path, false).unwrap().unwrap();
        assert!(!at_path(&old, &path).unwrap());
        assert!(at_path(&new, &path).unwrap());
        drop(lock);
    }
}
