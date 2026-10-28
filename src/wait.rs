//! `hypercube wait`, which wakes an idle agent when messages arrive for it.
//! Claude Code runs it as a background hook that wakes the session when it
//! exits with status 2, and the Pi extension runs it and starts a turn the same
//! way. Either way the harness, not the pane's input, starts the turn, so a
//! prompt the user is typing stays in the harness's editor.

use std::error::Error;
use std::io::{Read, Write};
use std::path::Path;
use std::time::Duration;

use crate::hook;
use crate::store;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// How often a waiter reads the state.
const POLL: Duration = Duration::from_millis(250);

/// What a waiter found in the state.
#[derive(Debug, PartialEq)]
enum Found {
    /// The agent is idle with messages queued.
    Ready,
    /// The agent is gone, or a newer waiter replaced this one.
    Replaced,
    /// The agent is busy or has no messages.
    Nothing,
}

/// Wait for the agent named by `HYPERCUBE_AGENT_ID` and `HYPERCUBE_STATE`,
/// and return whether it wrote the agent's messages to `out`. Processes
/// outside a Hypercube agent return at once. A harness may write hook input to
/// `stdin`; it is read and ignored.
pub fn run(mut stdin: impl Read, out: impl Write) -> Result<bool> {
    let id = std::env::var("HYPERCUBE_AGENT_ID").unwrap_or_default();
    let state = std::env::var("HYPERCUBE_STATE").unwrap_or_default();
    if id.is_empty() || state.is_empty() {
        return Ok(false);
    }
    let _ = stdin.read_to_end(&mut Vec::new());
    wait(&id, Path::new(&state), out)
}

/// Become agent `id`'s newest waiter, then wait until the agent is idle with
/// messages queued. Take the messages, mark the agent busy, write the
/// messages to `out`, and return true. Return false, without taking anything,
/// once the agent is gone, a newer waiter has started, or the process that
/// started this one has exited.
pub fn wait(id: &str, state_path: &Path, mut out: impl Write) -> Result<bool> {
    let parent = std::os::unix::process::parent_id();
    let number = store::update(state_path, |s| {
        let agent = s.agent_mut(id)?;
        agent.waiter += 1;
        Ok(agent.waiter)
    })?;
    loop {
        if std::os::unix::process::parent_id() != parent {
            return Ok(false);
        }
        // Reading without the lock keeps an idle agent's waiter from
        // contending with the hooks and tools of busy ones.
        let state = store::load(state_path)?;
        let found = match check(&state, id, number) {
            Found::Ready => deliver(id, state_path, number, &mut out)?,
            found => found,
        };
        match found {
            Found::Ready => return Ok(true),
            Found::Replaced => return Ok(false),
            Found::Nothing => std::thread::sleep(POLL),
        }
    }
}

/// What waiter `number` of agent `id` would find in `state`.
fn check(state: &store::State, id: &str, number: u64) -> Found {
    match state.agent(id) {
        Ok(agent) if agent.waiter == number => {
            if agent.activity == store::IDLE && state.messages.iter().any(|m| m.to == id) {
                Found::Ready
            } else {
                Found::Nothing
            }
        }
        _ => Found::Replaced,
    }
}

/// Check again under the lock, and if the agent is still ready, take its
/// messages, mark it busy, and write them. The messages are written inside the
/// update, so they stay queued if they cannot be.
fn deliver(id: &str, state_path: &Path, number: u64, out: &mut impl Write) -> Result<Found> {
    store::update(state_path, |s| {
        let found = check(s, id, number);
        if found != Found::Ready {
            return Ok(found);
        }
        let messages = s.take(id);
        let text = hook::format(s, &messages);
        s.agent_mut(id)?.activity = store::BUSY.into();
        out.write_all(format!("{text}\n").as_bytes())
            .and_then(|()| out.flush())
            .map_err(|e| format!("write messages: {e}"))?;
        Ok(Found::Ready)
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::store::{Agent, Message, State};
    use crate::testutil::TempDir;
    use std::path::PathBuf;
    use std::thread;

    fn fixture(activity: &str, messages: Vec<Message>) -> (TempDir, PathBuf) {
        let dir = TempDir::new();
        let state = dir.path().join("w.json");
        let agent = |id: &str, name: &str| Agent {
            id: id.into(),
            name: name.into(),
            activity: activity.into(),
            ..Agent::default()
        };
        store::update(&state, |s| {
            *s = State {
                agents: vec![agent("o1", "orchestrator"), agent("w1", "w1")],
                messages,
                no_git: false,
            };
            Ok(())
        })
        .unwrap();
        (dir, state)
    }

    fn message(from: &str, to: &str, text: &str) -> Message {
        Message {
            from: from.into(),
            to: to.into(),
            text: text.into(),
        }
    }

    fn update(state: &Path, f: impl FnOnce(&mut State)) {
        store::update(state, |s| {
            f(s);
            Ok(())
        })
        .unwrap();
    }

    /// Start a waiter for `id` and return it once it has its number.
    fn start(id: &str, state: &Path) -> thread::JoinHandle<(bool, String)> {
        let before = store::load(state).unwrap().agent(id).unwrap().waiter;
        let (agent, path) = (id.to_string(), state.to_path_buf());
        let waiter = thread::spawn(move || {
            let mut out = Vec::new();
            let delivered = wait(&agent, &path, &mut out).unwrap();
            (delivered, String::from_utf8(out).unwrap())
        });
        while store::load(state).unwrap().agent(id).unwrap().waiter == before {
            thread::sleep(Duration::from_millis(5));
        }
        waiter
    }

    #[test]
    fn an_idle_agent_with_messages_gets_them_and_turns_busy() {
        let (_dir, state) = fixture(
            store::IDLE,
            vec![message("o1", "w1", "one"), message("w1", "o1", "kept")],
        );
        let (delivered, out) = start("w1", &state).join().unwrap();
        assert!(delivered);
        assert_eq!(out, "Message from orchestrator (o1):\none\n");
        let s = store::load(&state).unwrap();
        assert_eq!(s.agent("w1").unwrap().activity, store::BUSY);
        assert_eq!(s.messages, [message("w1", "o1", "kept")]);
    }

    #[test]
    fn a_busy_agent_is_woken_once_idle_with_messages() {
        let (_dir, state) = fixture(store::BUSY, vec![message("o1", "w1", "early")]);
        let waiter = start("w1", &state);
        thread::sleep(POLL * 2);
        assert!(!waiter.is_finished());
        // Idle with nothing queued: still waiting.
        update(&state, |s| {
            s.messages.clear();
            s.agent_mut("w1").unwrap().activity = store::IDLE.into();
        });
        thread::sleep(POLL * 2);
        assert!(!waiter.is_finished());
        update(&state, |s| s.messages.push(message("o1", "w1", "late")));
        let (delivered, out) = waiter.join().unwrap();
        assert!(delivered);
        assert_eq!(out, "Message from orchestrator (o1):\nlate\n");
    }

    #[test]
    fn a_newer_waiter_replaces_an_older_one() {
        let (_dir, state) = fixture(store::BUSY, Vec::new());
        let older = start("w1", &state);
        let newer = start("w1", &state);
        assert_eq!(older.join().unwrap(), (false, String::new()));
        assert_eq!(store::load(&state).unwrap().agent("w1").unwrap().waiter, 2);
        update(&state, |s| {
            s.messages.push(message("o1", "w1", "hi"));
            s.agent_mut("w1").unwrap().activity = store::IDLE.into();
        });
        assert!(newer.join().unwrap().0);
    }

    #[test]
    fn a_removed_agent_ends_its_waiter() {
        let (_dir, state) = fixture(store::IDLE, Vec::new());
        let waiter = start("w1", &state);
        update(&state, |s| s.remove("w1"));
        assert_eq!(waiter.join().unwrap(), (false, String::new()));
    }

    #[test]
    fn an_unknown_agent_fails() {
        let (_dir, state) = fixture(store::IDLE, Vec::new());
        assert!(wait("nobody", &state, Vec::new()).is_err());
    }

    struct Broken;

    impl Write for Broken {
        fn write(&mut self, _: &[u8]) -> std::io::Result<usize> {
            Err(std::io::ErrorKind::BrokenPipe.into())
        }
        fn flush(&mut self) -> std::io::Result<()> {
            Ok(())
        }
    }

    #[test]
    fn messages_that_cannot_be_written_stay_queued() {
        let (_dir, state) = fixture(store::IDLE, vec![message("o1", "w1", "hi")]);
        assert!(wait("w1", &state, Broken).is_err());
        let s = store::load(&state).unwrap();
        assert_eq!(s.messages.len(), 1);
        assert_eq!(s.agent("w1").unwrap().activity, store::IDLE);
    }
}
