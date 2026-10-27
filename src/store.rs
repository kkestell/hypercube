//! A workspace's agents and message queue in one JSON file. Every change
//! holds an exclusive lock, so hooks, the MCP server, and the host can update
//! the file concurrently.

use std::error::Error;
use std::fs::{self, File, OpenOptions};
use std::io::{ErrorKind, Read};
use std::path::{Path, PathBuf};

use serde::{Deserialize, Deserializer, Serialize};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

pub const ORCHESTRATOR: &str = "orchestrator";
pub const WORKER: &str = "worker";
pub const BUSY: &str = "busy";
pub const IDLE: &str = "idle";

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
    #[serde(skip_serializing_if = "String::is_empty")]
    pub session_id: String,
    pub activity: String,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Message {
    pub from: String,
    pub to: String,
    pub text: String,
}

#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct State {
    #[serde(deserialize_with = "null_as_default")]
    pub agents: Vec<Agent>,
    #[serde(deserialize_with = "null_as_default")]
    pub messages: Vec<Message>,
    /// Set for a workspace whose directory is not a Git checkout. Its workers
    /// work directly in existing directories, without branches or worktrees.
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub no_git: bool,
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
    if let Some(dir) = path.parent() {
        fs::create_dir_all(dir).map_err(|e| format!("create {}: {e}", dir.display()))?;
    }
    let lock_path = lock_path(path);
    let lock = OpenOptions::new()
        .create(true)
        .truncate(false)
        .write(true)
        .open(&lock_path)
        .map_err(|e| format!("open {}: {e}", lock_path.display()))?;
    lock.lock()
        .map_err(|e| format!("lock {}: {e}", lock_path.display()))?;
    Ok(lock)
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
/// `f` returns an error, nothing is written.
pub fn update<T>(path: &Path, f: impl FnOnce(&mut State) -> Result<T>) -> Result<T> {
    let _lock = lock(path)?;
    let mut state = load(path)?;
    let value = f(&mut state)?;
    let data = serde_json::to_string_pretty(&state)?;
    let mut tmp = path.as_os_str().to_owned();
    tmp.push(".tmp");
    let tmp = PathBuf::from(tmp);
    fs::write(&tmp, data).map_err(|e| format!("write {}: {e}", tmp.display()))?;
    fs::rename(&tmp, path).map_err(|e| format!("rename {}: {e}", tmp.display()))?;
    Ok(value)
}

impl State {
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
        assert!(state.messages.is_empty() && !state.no_git);

        // Rewriting drops the target and keeps the rest.
        update(&path, |_| Ok(())).unwrap();
        let text = fs::read_to_string(&path).unwrap();
        assert!(!text.contains("target"), "{text}");
        assert_eq!(load(&path).unwrap(), state);
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
            no_git: true,
        };
        let value = serde_json::to_value(&state).unwrap();
        assert_eq!(value["agents"][0]["id"], "a");
        assert!(value["agents"][0].get("session_id").is_none());
        assert_eq!(value["messages"][0]["from"], "a");
        assert_eq!(value["no_git"], true);
        let value = serde_json::to_value(State::default()).unwrap();
        assert!(value.get("no_git").is_none());
    }

    #[test]
    fn take_and_remove() {
        let mut s = State {
            agents: vec![agent("a", "one"), agent("b", "two")],
            messages: vec![message("a", "1"), message("b", "2"), message("a", "3")],
            no_git: false,
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
}
