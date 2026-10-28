//! Integration tests of the whole product. Each test runs `hypercube` in a pty
//! whose other side feeds a Ghostty terminal standing in for the user's
//! terminal, with isolated state and configuration, a temporary repository,
//! and `claude` and `pi` stand-ins (src/bin/hypercube-test-harness.rs) first on
//! `PATH`. The stand-ins record what they see in an event log and take tool
//! calls on a fifo.
use std::fs;
use std::io::Write as _;
use std::os::fd::{AsRawFd as _, OwnedFd};
use std::os::unix::fs::OpenOptionsExt as _;
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Output, Stdio};
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;
use std::time::{Duration, Instant};

use hypercube::mux::{Winsize, write_all};
use hypercube::pty::Pty;
use hypercube::store::{self, Agent, State};
use hypercube::{control, host, workspace};
use libghostty_vt::render::{CellIterator, RowIterator};
use libghostty_vt::screen::CellWide;
use libghostty_vt::{RenderState, Terminal};
use serde_json::{Value, json};

const HYPERCUBE: &str = env!("CARGO_BIN_EXE_hypercube");
const HARNESS: &str = env!("CARGO_BIN_EXE_hypercube-test-harness");
const NUDGE: &str = "Check your Hypercube messages.";
/// The user's terminal. A pane's program gets the rows below its label and
/// above the status bar.
const COLS: u16 = 120;
const ROWS: u16 = 40;
/// How long a wait for the product lasts, with many tests sharing the machine.
const TIMEOUT: Duration = Duration::from_secs(30);

const CONFIG: &str = r#"{
  "orchestrator": {"harness": "claude", "model": "haiku", "effort": "low"},
  "profiles": [
    {"name": "fast", "description": "Claude Code", "harness": "claude",
     "model": "haiku", "effort": "low"},
    {"name": "deepseek", "description": "Pi", "harness": "pi",
     "model": "stub/pi", "effort": "medium"}
  ]
}"#;

fn contains(haystack: &[u8], needle: &[u8]) -> bool {
    haystack.windows(needle.len()).any(|w| w == needle)
}

/// What the stand-in user's terminal answers to the runtime's startup queries.
fn host_replies() -> Vec<u8> {
    let mut s = "\x1b]10;rgb:d0d0/d1d1/d2d2\x1b\\\x1b]11;rgb:1010/1111/1212\x1b\\".to_owned();
    for i in 0..16 {
        s.push_str(&format!(
            "\x1b]4;{i};rgb:{:02x}/{:02x}/{:02x}\x1b\\",
            i * 16 + 1,
            255 - i * 15,
            i * 7 + 3
        ));
    }
    s.push_str("\x1b[?2027;1$y\x1b[?62;22c");
    s.into_bytes()
}

/// Read the user's side of the pty into `output` until `stop` is set or the
/// pty closes, answering the runtime's startup queries.
fn read_terminal(
    fd: OwnedFd,
    output: Arc<Mutex<Vec<u8>>>,
    stop: Arc<AtomicBool>,
) -> JoinHandle<()> {
    std::thread::spawn(move || {
        let fd = fd.as_raw_fd();
        let mut buf = [0u8; 65536];
        let mut answered = false;
        while !stop.load(Ordering::Relaxed) {
            let mut p = libc::pollfd {
                fd,
                events: libc::POLLIN,
                revents: 0,
            };
            if unsafe { libc::poll(&mut p, 1, 50) } <= 0 {
                continue;
            }
            let n = unsafe { libc::read(fd, buf.as_mut_ptr().cast(), buf.len()) };
            if n < 0 {
                match std::io::Error::last_os_error().raw_os_error() {
                    Some(libc::EAGAIN | libc::EINTR) => continue,
                    _ => return,
                }
            }
            if n == 0 {
                return;
            }
            let mut out = output.lock().unwrap();
            out.extend_from_slice(&buf[..n as usize]);
            if !answered && contains(&out, b"\x1b[c") {
                write_all(fd, &host_replies());
                answered = true;
            }
        }
    })
}

/// The `hypercube` process in the user's terminal.
struct Session {
    child: Child,
    master: OwnedFd,
    output: Arc<Mutex<Vec<u8>>>,
    stop: Arc<AtomicBool>,
    reader: Option<JoinHandle<()>>,
    terminal: Terminal<'static, 'static>,
    fed: usize,
}

impl Session {
    fn screen(&mut self) -> String {
        let data = {
            let output = self.output.lock().unwrap();
            output[self.fed..].to_vec()
        };
        self.fed += data.len();
        self.terminal.vt_write(&data);
        let mut state = RenderState::new().unwrap();
        let snapshot = state.update(&self.terminal).unwrap();
        let mut rows = RowIterator::new().unwrap();
        let mut rows = rows.update(&snapshot).unwrap();
        let mut cells = CellIterator::new().unwrap();
        let mut text = String::new();
        while let Some(row) = rows.next() {
            let mut line = String::new();
            let mut iter = cells.update(row).unwrap();
            while let Some(cell) = iter.next() {
                let raw = cell.raw_cell().unwrap();
                if raw.wide().unwrap() == CellWide::SpacerTail {
                    continue;
                }
                if raw.has_text().unwrap() {
                    let mut grapheme = String::new();
                    cell.graphemes_utf8(&mut grapheme).unwrap();
                    line.push_str(&grapheme);
                } else {
                    line.push(' ');
                }
            }
            text.push_str(line.trim_end_matches(' '));
            text.push('\n');
        }
        text.trim_end_matches('\n').to_owned()
    }

    /// The host's exit code, once it has exited.
    fn exited(&mut self) -> Option<i32> {
        self.child
            .try_wait()
            .unwrap()
            .map(|s| s.code().unwrap_or(-1))
    }

    /// End the host, gently first, and release the pty.
    fn end(mut self) {
        if self.child.try_wait().ok().flatten().is_none() {
            unsafe { libc::kill(self.child.id() as i32, libc::SIGTERM) };
            let end = Instant::now() + Duration::from_secs(10);
            while self.child.try_wait().ok().flatten().is_none() && Instant::now() < end {
                std::thread::sleep(Duration::from_millis(20));
            }
            if self.child.try_wait().ok().flatten().is_none() {
                let _ = self.child.kill();
                let _ = self.child.wait();
            }
        }
        self.stop.store(true, Ordering::Relaxed);
        if let Some(reader) = self.reader.take() {
            let _ = reader.join();
        }
    }
}

/// A child process that is killed if the test ends first.
struct Guard(Option<Child>);

impl Guard {
    fn finish(mut self) -> Output {
        self.0.take().unwrap().wait_with_output().unwrap()
    }
}

impl Drop for Guard {
    fn drop(&mut self) {
        if let Some(child) = &mut self.0 {
            let _ = child.kill();
            let _ = child.wait();
        }
    }
}

/// An isolated environment for one test: a repository, state, configuration,
/// and the harness stand-ins, in a temporary directory removed on drop along
/// with every process the test started.
struct Fixture {
    dir: PathBuf,
    repo: PathBuf,
    session: Option<Session>,
}

impl Fixture {
    fn new() -> Fixture {
        static NEXT: AtomicU32 = AtomicU32::new(0);
        // A short path keeps the control socket within macOS's limit.
        let dir = PathBuf::from("/tmp").join(format!(
            "hc-it-{}-{}",
            std::process::id(),
            NEXT.fetch_add(1, Ordering::Relaxed)
        ));
        fs::create_dir_all(&dir).unwrap();
        let dir = dir.canonicalize().unwrap();
        let fx = Fixture {
            repo: dir.join("repo"),
            dir,
            session: None,
        };
        for sub in ["bin", "home", "records", "repo", "config/hypercube"] {
            fs::create_dir_all(fx.dir.join(sub)).unwrap();
        }
        for name in ["claude", "pi"] {
            std::os::unix::fs::symlink(HARNESS, fx.dir.join("bin").join(name)).unwrap();
        }
        fs::write(fx.dir.join("config/hypercube/config.json"), CONFIG).unwrap();
        fx.git(&fx.repo, &["init", "-q", "-b", "main"]);
        fx.git(&fx.repo, &["commit", "-q", "--allow-empty", "-m", "init"]);
        fx
    }

    fn git(&self, dir: &Path, args: &[&str]) -> String {
        let out = Command::new("git")
            .args(["-c", "user.name=t", "-c", "user.email=t@t"])
            .args(args)
            .current_dir(dir)
            .env("HOME", self.dir.join("home"))
            .output()
            .unwrap();
        assert!(
            out.status.success(),
            "git {args:?}: {}",
            String::from_utf8_lossy(&out.stderr)
        );
        String::from_utf8_lossy(&out.stdout).trim().to_string()
    }

    fn env(&self) -> Vec<(String, String)> {
        let path = format!(
            "{}:{}",
            self.dir.join("bin").display(),
            std::env::var("PATH").unwrap_or_default()
        );
        let dir = |sub: &str| self.dir.join(sub).to_string_lossy().into_owned();
        vec![
            ("HOME".into(), dir("home")),
            ("XDG_STATE_HOME".into(), dir("state")),
            ("XDG_CONFIG_HOME".into(), dir("config")),
            ("HYPERCUBE_TEST_DIR".into(), dir("records")),
            ("PATH".into(), path),
        ]
    }

    fn state_path(&self) -> PathBuf {
        let name = workspace::name(&self.repo);
        self.dir
            .join("state/hypercube")
            .join(format!("{name}.json"))
    }

    fn state(&self) -> State {
        store::load(&self.state_path()).unwrap()
    }

    fn worktree(&self, name: &str) -> PathBuf {
        self.repo.join(".hypercube").join(name)
    }

    /// A `hypercube` command in the repository with only the test's
    /// environment.
    fn cli(&self, args: &[&str]) -> Command {
        let mut c = Command::new(HYPERCUBE);
        c.args(args)
            .current_dir(&self.repo)
            .env_clear()
            .envs(self.env());
        c
    }

    fn run(&self, args: &[&str]) -> Output {
        self.cli(args).output().unwrap()
    }

    /// The row of `hypercube status` for the repository: workspace, session,
    /// and number of agents.
    fn status(&self) -> Option<Vec<String>> {
        let out = self.run(&["status"]);
        assert!(out.status.success(), "{out:?}");
        let text = String::from_utf8_lossy(&out.stdout).into_owned();
        let repo = self.repo.to_str().unwrap();
        text.lines()
            .find(|l| l.starts_with(repo))
            .map(|l| l.split_whitespace().map(String::from).collect())
    }

    /// Run `hypercube` in a terminal and wait for the orchestrator's stand-in.
    /// Every start after the first waits for its latest launch.
    fn start(&mut self) {
        assert!(self.session.is_none());
        let launches = self.agent_events("orchestrator", "launch").len();
        let ws = Winsize {
            ws_col: COLS,
            ws_row: ROWS,
            ws_xpixel: COLS * 8,
            ws_ypixel: ROWS * 16,
        };
        let pty = Pty::spawn(&[HYPERCUBE], &self.repo, &self.env(), ws).unwrap();
        let output = Arc::new(Mutex::new(Vec::new()));
        let stop = Arc::new(AtomicBool::new(false));
        // The clone is close-on-exec, so no other test's processes inherit it.
        let reader = read_terminal(
            pty.master.try_clone().unwrap(),
            output.clone(),
            stop.clone(),
        );
        self.session = Some(Session {
            child: pty.child,
            master: pty.master,
            output,
            stop,
            reader: Some(reader),
            terminal: Terminal::new(COLS, ROWS).unwrap(),
            fed: 0,
        });
        self.wait("the orchestrator to start", |fx| {
            (fx.agent_events("orchestrator", "ready").len() > launches).then_some(())
        });
    }

    fn session(&mut self) -> &mut Session {
        self.session.as_mut().expect("hypercube is running")
    }

    fn screen(&mut self) -> String {
        self.session().screen()
    }

    /// Type into the user's terminal, which goes to the focused pane.
    fn type_text(&mut self, text: &str) {
        let fd = self.session().master.as_raw_fd();
        write_all(fd, text.as_bytes());
    }

    /// Run `hypercube stop` and wait until the host has ended.
    fn stop(&mut self) {
        let out = self.run(&["stop"]);
        assert!(out.status.success(), "{out:?}");
        self.wait_host_exit();
    }

    /// Wait for the host to exit successfully and release its terminal.
    fn wait_host_exit(&mut self) {
        let code = self.wait("the host to exit", |fx| fx.session().exited());
        assert_eq!(code, 0);
        self.session.take().unwrap().end();
    }

    /// Poll `f` until it returns a value. A timeout fails the test with the
    /// screen and every stand-in's events.
    fn wait<T>(&mut self, what: &str, mut f: impl FnMut(&mut Fixture) -> Option<T>) -> T {
        let end = Instant::now() + TIMEOUT;
        loop {
            if let Some(value) = f(self) {
                return value;
            }
            if Instant::now() > end {
                panic!("timed out waiting for {what}\n{}", self.dump());
            }
            std::thread::sleep(Duration::from_millis(20));
        }
    }

    fn dump(&mut self) -> String {
        let mut text = String::new();
        if self.session.is_some() {
            text.push_str(&self.screen());
            text.push('\n');
        }
        for agent in self.state().agents {
            text.push_str(&format!("{} ({}):\n", agent.name, agent.id));
            for mut e in self.events_of(&agent.id) {
                e.as_object_mut().unwrap().remove("env");
                text.push_str(&format!("  {e}\n"));
            }
        }
        text
    }

    fn wait_screen(&mut self, text: &str) {
        self.wait(&format!("{text:?} on the screen"), |fx| {
            fx.screen().contains(text).then_some(())
        });
    }

    fn wait_no_screen(&mut self, text: &str) {
        self.wait(&format!("{text:?} to leave the screen"), |fx| {
            (!fx.screen().contains(text)).then_some(())
        });
    }

    fn agent_id(&self, name: &str) -> Option<String> {
        self.state().agent(name).ok().map(|a| a.id.clone())
    }

    fn events_of(&self, id: &str) -> Vec<Value> {
        let path = self.dir.join("records").join(id).join("events.jsonl");
        let text = fs::read_to_string(path).unwrap_or_default();
        // A line still being written has no newline yet.
        let complete = text.rfind('\n').map_or("", |i| &text[..i]);
        complete
            .lines()
            .map(|l| serde_json::from_str(l).unwrap())
            .collect()
    }

    /// The events of the kind `kind` that the agent's stand-in recorded.
    fn agent_events(&self, name: &str, kind: &str) -> Vec<Value> {
        let Some(id) = self.agent_id(name) else {
            return Vec::new();
        };
        let events = self.events_of(&id);
        events.into_iter().filter(|e| e["event"] == kind).collect()
    }

    fn wait_event(&mut self, name: &str, kind: &str, count: usize) -> Vec<Value> {
        self.wait(&format!("{count} {kind} events of {name}"), |fx| {
            let events = fx.agent_events(name, kind);
            (events.len() >= count).then_some(events)
        })
    }

    /// The pids of every stand-in that was started.
    fn pids(&self) -> Vec<i32> {
        let Ok(entries) = fs::read_dir(self.dir.join("records")) else {
            return Vec::new();
        };
        entries
            .flatten()
            .flat_map(|e| self.events_of(&e.file_name().to_string_lossy()))
            .filter(|e| e["event"] == "launch")
            .map(|e| e["pid"].as_i64().unwrap() as i32)
            .collect()
    }

    /// The pid of the agent's latest stand-in.
    fn pid(&self, name: &str) -> i32 {
        let launches = self.agent_events(name, "launch");
        launches.last().unwrap()["pid"].as_i64().unwrap() as i32
    }

    fn wait_gone(&mut self, pids: &[i32]) {
        self.wait("the stand-ins to end", |_| {
            pids.iter().all(|&p| !alive(p)).then_some(())
        });
    }

    fn wait_idle(&mut self, name: &str) {
        self.wait(&format!("{name} to be idle"), |fx| {
            let state = fx.state();
            let agent = state.agent(name).ok()?;
            (agent.activity == store::IDLE).then_some(())
        });
    }

    /// Have the agent's stand-in call an MCP tool. Returns the result text
    /// and whether it is an error.
    fn call(&mut self, name: &str, tool: &str, arguments: Value) -> (String, bool) {
        static TAGS: AtomicU32 = AtomicU32::new(0);
        let tag = format!("t{}", TAGS.fetch_add(1, Ordering::Relaxed));
        self.command(name, &format!("{tag} {tool} {arguments}"));
        let event = self.wait(&format!("{tool} to return"), |fx| {
            let events = fx.agent_events(name, "tool");
            events.into_iter().find(|e| e["tag"] == tag.as_str())
        });
        (
            event["text"].as_str().unwrap().to_string(),
            event["is_error"].as_bool().unwrap(),
        )
    }

    /// Write a line to the agent's stand-in's command fifo.
    fn command(&self, name: &str, line: &str) {
        let id = self.agent_id(name).unwrap();
        let path = self.dir.join("records").join(id).join("fifo");
        // Opening without a reader fails at once instead of blocking, so a
        // stand-in that has exited fails the test.
        let end = Instant::now() + Duration::from_secs(2);
        let mut fifo = loop {
            match fs::OpenOptions::new()
                .write(true)
                .custom_flags(libc::O_NONBLOCK)
                .open(&path)
            {
                Ok(fifo) => break fifo,
                Err(e) if e.raw_os_error() == Some(libc::ENXIO) && Instant::now() < end => {
                    std::thread::sleep(Duration::from_millis(20));
                }
                Err(e) => panic!("{name}'s stand-in is not reading {}: {e}", path.display()),
            }
        };
        fifo.write_all(format!("{line}\n").as_bytes()).unwrap();
    }

    /// Create a worker and wait until its stand-in has started and finished
    /// its first turn.
    fn create(&mut self, name: &str, profile: &str, message: &str) -> String {
        let (text, is_error) = self.call(
            "orchestrator",
            "create_agent",
            json!({"name": name, "profile": profile, "message": message}),
        );
        assert!(!is_error, "{text}");
        self.wait_event(name, "ready", 1);
        self.wait_idle(name);
        self.agent_id(name).unwrap()
    }

    /// The size of the agent's pane as its program last saw it.
    fn size(&self, name: &str) -> Vec<u64> {
        let id = self.agent_id(name).unwrap();
        let events = self.events_of(&id);
        let event = events.iter().rev().find(|e| !e["size"].is_null()).unwrap();
        event["size"]
            .as_array()
            .unwrap()
            .iter()
            .map(|v| v.as_u64().unwrap())
            .collect()
    }

    fn wait_size(&mut self, name: &str, size: [u64; 2]) {
        self.wait(&format!("{name} to be {size:?}"), |fx| {
            (fx.size(name) == size).then_some(())
        });
    }

    /// The state directory's files other than the Pi extension.
    fn state_files(&self) -> Vec<String> {
        let Ok(entries) = fs::read_dir(self.dir.join("state/hypercube")) else {
            return Vec::new();
        };
        entries
            .flatten()
            .map(|e| e.file_name().to_string_lossy().into_owned())
            .filter(|n| n != "pi-extension")
            .collect()
    }
}

impl Drop for Fixture {
    fn drop(&mut self) {
        if let Some(session) = self.session.take() {
            session.end();
        }
        // Stand-ins that outlived the host.
        for pid in self.pids() {
            if alive(pid) {
                unsafe { libc::kill(-pid, libc::SIGKILL) };
            }
        }
        let _ = fs::remove_dir_all(&self.dir);
    }
}

fn alive(pid: i32) -> bool {
    unsafe { libc::kill(pid, 0) == 0 }
}

/// The value after `flag` in the stand-in's arguments.
fn flag_value<'a>(argv: &'a [Value], flag: &str) -> Option<&'a str> {
    let i = argv.iter().position(|a| a == flag)?;
    argv.get(i + 1)?.as_str()
}

fn args_of(launch: &Value) -> &[Value] {
    launch["argv"].as_array().unwrap()
}

/// The context each of the stand-in's `name` hook runs gave the model.
fn hook_contexts(events: &[Value], name: &str) -> Vec<String> {
    events
        .iter()
        .filter(|e| e["name"] == name)
        .map(|e| {
            let output: Value =
                serde_json::from_str(e["output"].as_str().unwrap_or("")).unwrap_or(Value::Null);
            let text = &output["hookSpecificOutput"]["additionalContext"];
            text.as_str().unwrap_or("").to_string()
        })
        .collect()
}

#[test]
fn first_start_lays_out_one_pane() {
    let mut fx = Fixture::new();
    fx.start();

    let state = fx.state();
    assert!(!state.no_git);
    let [o] = state.agents.as_slice() else {
        panic!("{:?}", state.agents)
    };
    assert_eq!(
        (
            o.name.as_str(),
            o.role.as_str(),
            o.harness.as_str(),
            o.model.as_str(),
            o.effort.as_str(),
            o.branch.as_str(),
            o.activity.as_str(),
        ),
        (
            "orchestrator",
            "orchestrator",
            "claude",
            "haiku",
            "low",
            "main",
            "idle"
        )
    );
    assert_eq!(Path::new(&o.dir), fx.repo);

    let launches = fx.agent_events("orchestrator", "launch");
    let [launch] = launches.as_slice() else {
        panic!("{launches:?}")
    };
    let argv = args_of(launch);
    assert_eq!(argv[0], "--mcp-config");
    assert_eq!(flag_value(argv, "--model"), Some("haiku"));
    assert_eq!(flag_value(argv, "--effort"), Some("low"));
    assert!(argv.iter().any(|a| a == "--dangerously-skip-permissions"));
    assert!(!argv.iter().any(|a| a == "--resume"));
    assert_eq!(launch["harness"], "claude");
    assert_eq!(launch["cwd"], fx.repo.to_str().unwrap());
    assert_eq!(launch["env"]["HYPERCUBE_AGENT_ID"], o.id.as_str());
    assert_eq!(
        launch["env"]["HYPERCUBE_STATE"],
        fx.state_path().to_str().unwrap()
    );
    assert_eq!(launch["env"]["TERM"], "xterm-256color");
    // The pane takes the whole terminal but its label row and the status bar.
    assert_eq!(
        fx.size("orchestrator"),
        [u64::from(COLS), u64::from(ROWS) - 2]
    );

    let events = fx.agent_events("orchestrator", "hook");
    let facts = &hook_contexts(&events, "SessionStart")[0];
    assert!(
        facts.contains(&format!("You are orchestrator ({})", o.id)),
        "{facts}"
    );
    assert!(
        facts.contains("deepseek (pi, stub/pi, medium): Pi"),
        "{facts}"
    );

    let repo = fx.repo.to_str().unwrap().to_string();
    fx.wait_screen("orchestrator · claude haiku low");
    let screen = fx.screen();
    let lines: Vec<&str> = screen.lines().collect();
    assert!(
        lines[0].contains("orchestrator · claude haiku low"),
        "{screen}"
    );
    assert!(lines.last().unwrap().contains(&repo), "{screen}");
    assert!(!screen.contains('┃'), "{screen}");
}

#[test]
fn create_agent_adds_panes_and_worktrees() {
    let mut fx = Fixture::new();
    fx.start();
    let oid = fx.agent_id("orchestrator").unwrap();

    let (text, is_error) = fx.call(
        "orchestrator",
        "create_agent",
        json!({"name": "w1", "profile": "fast", "message": "do the thing"}),
    );
    assert!(!is_error, "{text}");
    assert!(
        text.contains("Created w1") && text.contains("hypercube/w1"),
        "{text}"
    );
    fx.wait_event("w1", "ready", 1);
    let w1 = fx.agent_id("w1").unwrap();

    // The worktree is on the worker's branch.
    let worktree = fx.worktree("w1");
    assert_eq!(
        fx.git(&worktree, &["branch", "--show-current"]),
        "hypercube/w1"
    );
    let state = fx.state();
    let agent = state.agent("w1").unwrap();
    assert_eq!(
        (
            agent.role.as_str(),
            agent.harness.as_str(),
            Path::new(&agent.dir)
        ),
        ("worker", "claude", worktree.as_path())
    );

    // Claude Code launched in the worktree with the nudge as its prompt.
    let launch = fx.agent_events("w1", "launch").remove(0);
    let argv = args_of(&launch);
    assert_eq!(argv.last().unwrap(), NUDGE);
    assert_eq!(flag_value(argv, "--model"), Some("haiku"));
    assert_eq!(launch["cwd"], worktree.to_str().unwrap());
    assert_eq!(launch["env"]["HYPERCUBE_AGENT_ID"], w1.as_str());
    assert_eq!(launch["size"], json!([71, 38]));
    fx.wait_size("orchestrator", [48, 38]);

    // The message queued at creation arrived with the first prompt.
    fx.wait_idle("w1");
    let events = fx.agent_events("w1", "hook");
    let delivered = format!("Message from orchestrator ({oid}):\ndo the thing");
    assert!(
        hook_contexts(&events, "UserPromptSubmit")[0].contains(&delivered),
        "{events:?}"
    );
    assert!(fx.state().messages.is_empty());

    // A Pi worker joins as the third pane, and the panes share the right side.
    let (text, is_error) = fx.call(
        "orchestrator",
        "create_agent",
        json!({"name": "w2", "profile": "deepseek"}),
    );
    assert!(!is_error, "{text}");
    fx.wait_event("w2", "ready", 1);
    let launch = fx.agent_events("w2", "launch").remove(0);
    let argv = args_of(&launch);
    assert_eq!(launch["harness"], "pi");
    let extension = fx.dir.join("state/hypercube/pi-extension/index.js");
    assert_eq!(argv[0], "--extension");
    assert_eq!(argv[1], extension.to_str().unwrap());
    assert!(extension.exists());
    assert_eq!(flag_value(argv, "--model"), Some("stub/pi"));
    assert_eq!(flag_value(argv, "--thinking"), Some("medium"));
    assert!(argv.iter().any(|a| a == "--approve"));
    assert_eq!(argv.last().unwrap(), NUDGE);
    assert_eq!(launch["size"], json!([71, 19]));
    fx.wait_size("w1", [71, 18]);
    fx.wait_size("orchestrator", [48, 38]);

    fx.wait_screen("w2 · pi stub/pi medium");
    let screen = fx.screen();
    let lines: Vec<&str> = screen.lines().collect();
    assert!(
        lines[0].contains("orchestrator · claude haiku low"),
        "{screen}"
    );
    assert!(lines[0].contains("w1 · claude haiku low"), "{screen}");
    assert!(lines[19].contains("w2 · pi stub/pi medium"), "{screen}");
}

#[test]
fn send_message_nudges_an_idle_agent() {
    let mut fx = Fixture::new();
    fx.start();
    let oid = fx.agent_id("orchestrator").unwrap();
    let w1 = fx.create("w1", "fast", "");
    assert_eq!(fx.agent_events("w1", "prompt").len(), 1);

    let (text, is_error) = fx.call(
        "orchestrator",
        "send_message",
        json!({"to": "w1", "text": "hello worker"}),
    );
    assert!(!is_error, "{text}");
    assert_eq!(text, format!("Message queued for w1 ({w1})."));

    // The host typed the nudge and pressed Enter 200 ms later.
    let prompts = fx.wait_event("w1", "prompt", 2);
    assert_eq!(prompts[1]["text"], NUDGE);
    let inputs = fx.agent_events("w1", "input");
    let typed: String = inputs.iter().map(|e| e["data"].as_str().unwrap()).collect();
    assert_eq!(typed, format!("{NUDGE}\r"));

    // The nudge's turn delivered the message.
    let events = fx.wait("the nudge's turn to end", |fx| {
        let events = fx.agent_events("w1", "hook");
        let stops = events.iter().filter(|e| e["name"] == "Stop").count();
        (stops >= 2).then_some(events)
    });
    let delivered = format!("Message from orchestrator ({oid}):\nhello worker");
    assert!(
        hook_contexts(&events, "UserPromptSubmit")[1].contains(&delivered),
        "{events:?}"
    );
    fx.wait_idle("w1");
    assert!(fx.state().messages.is_empty());

    // An agent cannot message itself.
    let (text, is_error) = fx.call("w1", "send_message", json!({"to": "w1", "text": "x"}));
    assert!(is_error, "{text}");
}

#[test]
fn remove_agent_closes_the_pane() {
    let mut fx = Fixture::new();
    fx.start();
    let w1 = fx.create("w1", "fast", "");
    let pid = fx.pid("w1");
    assert!(alive(pid));
    fx.wait_screen("w1 · claude haiku low");
    fx.wait_size("orchestrator", [48, 38]);

    let (text, is_error) = fx.call("orchestrator", "remove_agent", json!({"id": w1}));
    assert!(!is_error, "{text}");
    assert_eq!(text, format!("Removed w1 ({w1})."));

    fx.wait_gone(&[pid]);
    fx.wait_no_screen("w1 · claude haiku low");
    // The orchestrator has the terminal to itself again.
    fx.wait_size("orchestrator", [u64::from(COLS), u64::from(ROWS) - 2]);
    assert_eq!(fx.state().agents.len(), 1);
    assert!(!fx.worktree("w1").exists());
    assert_eq!(fx.git(&fx.repo, &["branch", "--list", "hypercube/w1"]), "");
    assert!(host::running(&fx.state_path()).unwrap().is_some());
}

#[test]
fn status_shows_running_and_stop_ends_every_stand_in() {
    let mut fx = Fixture::new();
    assert_eq!(fx.status(), None);
    fx.start();
    fx.create("w1", "fast", "");
    fx.create("w2", "deepseek", "");
    let pids = fx.pids();
    assert_eq!(pids.len(), 3);

    let repo = fx.repo.to_str().unwrap().to_string();
    assert_eq!(
        fx.status().unwrap()[..3],
        [repo.clone(), "running".into(), "3".into()]
    );
    let terminal = host::running(&fx.state_path()).unwrap().unwrap();
    assert!(terminal.starts_with("/dev/"), "{terminal}");

    // A second hypercube reports where the first runs.
    let out = Command::new(HYPERCUBE)
        .current_dir(&fx.repo)
        .env_clear()
        .envs(fx.env())
        .stdin(Stdio::null())
        .output()
        .unwrap();
    assert!(!out.status.success());
    let stderr = String::from_utf8_lossy(&out.stderr);
    assert!(
        stderr.contains("hypercube is already running for"),
        "{stderr}"
    );

    fx.stop();
    fx.wait_gone(&pids);
    assert_eq!(
        fx.status().unwrap()[..3],
        [repo, "stopped".into(), "3".into()]
    );
    assert!(host::running(&fx.state_path()).unwrap().is_none());
    assert!(!control::socket_path(&fx.state_path()).unwrap().exists());
    // The agents stay for the next start.
    assert_eq!(fx.state().agents.len(), 3);

    let out = fx.run(&["stop"]);
    assert!(!out.status.success());
    assert!(String::from_utf8_lossy(&out.stderr).contains("not running"));
}

#[test]
fn restart_resumes_saved_sessions() {
    let mut fx = Fixture::new();
    fx.start();
    // A prompt typed in the focused pane starts the orchestrator's session.
    fx.type_text("hello\r");
    fx.wait_event("orchestrator", "prompt", 1);
    fx.wait_idle("orchestrator");
    fx.create("w1", "fast", "");
    fx.create("w2", "deepseek", "");

    let names = ["orchestrator", "w1", "w2"];
    let sessions: Vec<String> = names
        .iter()
        .map(|n| {
            let launch = fx.agent_events(n, "launch").remove(0);
            launch["session"].as_str().unwrap().to_string()
        })
        .collect();
    let state = fx.state();
    for (name, session) in names.iter().zip(&sessions) {
        assert_eq!(&state.agent(name).unwrap().session_id, session, "{name}");
    }
    fx.stop();

    fx.start();
    for name in names {
        fx.wait_event(name, "ready", 2);
    }
    let resumed = |fx: &Fixture, name: &str| fx.agent_events(name, "launch").remove(1);
    let launch = resumed(&fx, "orchestrator");
    assert_eq!(
        flag_value(args_of(&launch), "--resume"),
        Some(sessions[0].as_str())
    );
    assert_ne!(args_of(&launch).last().unwrap(), NUDGE);
    assert_eq!(launch["session"], sessions[0].as_str());
    let launch = resumed(&fx, "w1");
    assert_eq!(
        flag_value(args_of(&launch), "--resume"),
        Some(sessions[1].as_str())
    );
    assert_eq!(args_of(&launch).last().unwrap(), NUDGE);
    let launch = resumed(&fx, "w2");
    assert_eq!(
        flag_value(args_of(&launch), "--session-id"),
        Some(sessions[2].as_str())
    );
    assert_eq!(args_of(&launch).last().unwrap(), NUDGE);

    // The workers' turns on restart end idle, with their sessions unchanged.
    fx.wait_idle("w1");
    fx.wait_idle("w2");
    let state = fx.state();
    for (name, session) in names.iter().zip(&sessions) {
        assert_eq!(&state.agent(name).unwrap().session_id, session, "{name}");
    }
    assert_eq!(state.agents.len(), 3);
}

#[test]
fn delete_refuses_a_dirty_worktree_then_succeeds() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    let pids = fx.pids();
    let worktree = fx.worktree("w1");
    fs::write(worktree.join("scratch.txt"), "work in progress").unwrap();

    let out = fx.run(&["delete"]);
    assert!(!out.status.success());
    let stderr = String::from_utf8_lossy(&out.stderr);
    assert!(stderr.contains("not deleting the workspace"), "{stderr}");
    assert!(stderr.contains("w1"), "{stderr}");
    // Nothing changed, and the session still runs.
    assert!(host::running(&fx.state_path()).unwrap().is_some());
    assert!(worktree.join("scratch.txt").exists());
    assert_eq!(fx.state().agents.len(), 2);
    assert!(pids.iter().all(|&p| alive(p)));

    fs::remove_file(worktree.join("scratch.txt")).unwrap();
    // A worker created after delete checked the workers, while the session
    // ends, is removed too. The hung-up w1 holds the session open until the
    // worker exists.
    fx.command("w1", "hold-hup");
    fx.wait_event("w1", "hold-hup", 1);
    let delete = Guard(Some(
        fx.cli(&["delete"])
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .unwrap(),
    ));
    fx.wait_event("w1", "hup", 1);
    let late = fx.worktree("late");
    store::update(&fx.state_path(), |s| {
        s.agents.push(Agent {
            id: "late0001".into(),
            name: "late".into(),
            role: store::WORKER.into(),
            harness: "claude".into(),
            model: "haiku".into(),
            effort: "low".into(),
            dir: late.to_str().unwrap().into(),
            branch: "hypercube/late".into(),
            activity: store::BUSY.into(),
            ..Agent::default()
        });
        Ok(())
    })
    .unwrap();
    workspace::add_worktree(&fx.repo, &late, "hypercube/late").unwrap();
    let id = fx.agent_id("w1").unwrap();
    fs::write(fx.dir.join("records").join(id).join("release"), "").unwrap();

    let out = delete.finish();
    assert!(
        out.status.success(),
        "{}",
        String::from_utf8_lossy(&out.stderr)
    );
    fx.wait_host_exit();
    fx.wait_gone(&pids);
    assert_eq!(fx.state_files(), Vec::<String>::new());
    assert!(!worktree.exists());
    assert!(!late.exists());
    assert_eq!(
        fx.git(&fx.repo, &["branch", "--format=%(refname:short)"]),
        "main"
    );
    assert_eq!(
        fx.git(&fx.repo, &["worktree", "list", "--porcelain"])
            .matches("worktree ")
            .count(),
        1
    );
    assert_eq!(fx.status(), None);
}

#[test]
fn unsupported_harness_does_not_stop_startup() {
    let mut fx = Fixture::new();
    fx.start();
    fx.stop();
    store::update(&fx.state_path(), |s| {
        s.agents.push(Agent {
            id: "old00001".into(),
            name: "old".into(),
            role: store::WORKER.into(),
            harness: "codex".into(),
            model: "x".into(),
            effort: "high".into(),
            dir: fx.repo.to_str().unwrap().into(),
            activity: store::IDLE.into(),
            ..Agent::default()
        });
        Ok(())
    })
    .unwrap();

    fx.start();
    // The worker has a pane that shows why it did not start.
    fx.wait_screen("old · codex x high");
    fx.wait_screen("old failed to launch");
    assert!(host::running(&fx.state_path()).unwrap().is_some());
    assert_eq!(fx.status().unwrap()[1], "running");

    let (text, is_error) = fx.call("orchestrator", "list_agents", json!({}));
    assert!(!is_error && text.contains("\"old\""), "{text}");
    let (text, is_error) = fx.call("orchestrator", "remove_agent", json!({"id": "old00001"}));
    assert!(!is_error, "{text}");
    assert_eq!(text, "Removed old (old00001).");
    fx.wait_no_screen("old · codex x high");
    fx.wait_size("orchestrator", [u64::from(COLS), u64::from(ROWS) - 2]);
    assert_eq!(fx.state().agents.len(), 1);
    assert!(host::running(&fx.state_path()).unwrap().is_some());
}
