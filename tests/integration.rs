//! Integration tests of the whole product. Each test runs `hypercube` in a pty
//! whose other side feeds a Ghostty terminal standing in for the user's
//! terminal, with isolated state and configuration, a temporary repository,
//! and `claude` and `opencode` stand-ins (src/bin/hypercube-test-harness.rs) first on
//! `PATH`. The client there starts the workspace's host in the background.
//! The stand-ins record what they see in an event log and take tool calls on
//! a fifo.
#[path = "support/terminal.rs"]
mod terminal;

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
use libghostty_vt::Terminal;
use serde_json::{Value, json};
use terminal::{contains, host_replies, winsize};

const HYPERCUBE: &str = env!("CARGO_BIN_EXE_hypercube");
const HARNESS: &str = env!("CARGO_BIN_EXE_hypercube-test-harness");
const NUDGE: &str = "Check your Hypercube messages.";
/// The user's terminal. A pane's program gets the rows below its label.
const COLS: u16 = 120;
const ROWS: u16 = 40;
/// How long a wait for the product lasts, with many tests sharing the machine.
const TIMEOUT: Duration = Duration::from_secs(30);

const CONFIG: &str = r#"{
  "orchestrator": {"harness": "claude", "model": "haiku", "effort": "low"},
  "profiles": [
    {"name": "fast", "description": "Claude Code", "harness": "claude",
     "model": "haiku", "effort": "low"},
    {"name": "deepseek", "description": "OpenCode", "harness": "opencode",
     "model": "stub/opencode", "effort": "medium"}
  ]
}"#;

/// Read the user's side of the pty into `output` until `stop` is set or the
/// pty closes, answering the client's startup queries.
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

/// The `hypercube` client in the user's terminal.
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
        terminal::screen(&self.terminal)
    }

    /// Everything the client wrote to the terminal, as text.
    fn text(&self) -> String {
        String::from_utf8_lossy(&self.output.lock().unwrap()).into_owned()
    }

    /// The client's exit code, once it has exited.
    fn exited(&mut self) -> Option<i32> {
        self.child
            .try_wait()
            .unwrap()
            .map(|s| s.code().unwrap_or(-1))
    }

    /// Close the user's terminal without signalling the client, and wait for
    /// it to exit on its own.
    fn close_terminal(mut self) -> i32 {
        assert!(
            self.child.try_wait().unwrap().is_none(),
            "client exited before its terminal closed"
        );
        self.stop.store(true, Ordering::Relaxed);
        if let Some(reader) = self.reader.take() {
            reader.join().unwrap();
        }
        drop(self.master);
        let deadline = Instant::now() + TIMEOUT;
        loop {
            if let Some(status) = self.child.try_wait().unwrap() {
                return status.code().unwrap_or(-1);
            }
            if Instant::now() >= deadline {
                let _ = self.child.kill();
                let _ = self.child.wait();
                panic!("client did not exit after its terminal closed");
            }
            std::thread::sleep(Duration::from_millis(20));
        }
    }

    /// End the client, gently first, and release the pty.
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
/// with every process the test started, hosts included.
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
        for name in ["claude", "opencode"] {
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

    /// Run `hypercube` with `input` on its standard input.
    fn run_with_input(&self, args: &[&str], input: &str) -> Output {
        let mut child = self
            .cli(args)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .unwrap();
        child
            .stdin
            .take()
            .unwrap()
            .write_all(input.as_bytes())
            .unwrap();
        child.wait_with_output().unwrap()
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
        self.session = Some(self.client());
        self.wait("the orchestrator to start", |fx| {
            (fx.agent_events("orchestrator", "ready").len() > launches).then_some(())
        });
    }

    /// Run `hypercube` in a terminal of its own, attaching to the running
    /// host, and wait until it shows the orchestrator's pane.
    fn attach(&mut self) {
        assert!(self.session.is_none());
        self.session = Some(self.client());
        let dir = self.state().agent("orchestrator").unwrap().dir.clone();
        self.wait_screen(&format!(" {dir}"));
    }

    /// `hypercube` in a new terminal.
    fn client(&self) -> Session {
        let ws = winsize(COLS, ROWS);
        let pty = Pty::spawn(&[HYPERCUBE], &self.repo, &self.env(), ws).unwrap();
        let output = Arc::new(Mutex::new(Vec::new()));
        let stop = Arc::new(AtomicBool::new(false));
        // The clone is close-on-exec, so no other test's processes inherit it.
        let reader = read_terminal(
            pty.master.try_clone().unwrap(),
            output.clone(),
            stop.clone(),
        );
        Session {
            child: pty.child,
            master: pty.master,
            output,
            stop,
            reader: Some(reader),
            terminal: Terminal::new(COLS, ROWS).unwrap(),
            fed: 0,
        }
    }

    /// Make `fx.repo` a new repository named `name`, and return the old one.
    fn new_repo(&mut self, name: &str) -> PathBuf {
        let old = std::mem::replace(&mut self.repo, self.dir.join(name));
        fs::create_dir(&self.repo).unwrap();
        self.git(&self.repo, &["init", "-q", "-b", "main"]);
        self.git(&self.repo, &["commit", "-q", "--allow-empty", "-m", "init"]);
        old
    }

    /// Open the workspace overlay and select the row naming `dir`.
    fn select_workspace(&mut self, dir: &Path) {
        self.type_text("\x02s");
        self.wait_screen("Workspaces");
        let dir = dir.to_str().unwrap().to_string();
        self.wait_screen(&dir);
        let screen = self.screen();
        let lines: Vec<&str> = screen.lines().collect();
        let row = |text: &str| lines.iter().position(|l| l.contains(text)).unwrap();
        let current = lines.iter().position(|l| l.starts_with(" * ")).unwrap();
        let target = row(&dir);
        let key = if target > current { "j" } else { "k" };
        self.type_text(&key.repeat(target.abs_diff(current)));
    }

    /// The process ID the workspace's host wrote into its lock.
    fn host_pid(&self) -> i32 {
        let lock = host::lock_path(&self.state_path());
        fs::read_to_string(lock).unwrap().trim().parse().unwrap()
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

    /// Wait for the host to end and the client to exit successfully, and
    /// release the client's terminal.
    fn wait_host_exit(&mut self) {
        let code = self.wait("the client to exit", |fx| fx.session().exited());
        assert_eq!(code, 0);
        let text = self.session().text();
        assert!(text.contains("hypercube: stopped"), "{text}");
        self.wait("the host to exit", |fx| {
            (!host::running(&fx.state_path()).unwrap()).then_some(())
        });
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

    /// The state directory's files other than the OpenCode plugin.
    fn state_files(&self) -> Vec<String> {
        let Ok(entries) = fs::read_dir(self.dir.join("state/hypercube")) else {
            return Vec::new();
        };
        entries
            .flatten()
            .map(|e| e.file_name().to_string_lossy().into_owned())
            .filter(|n| n != "opencode-plugin")
            .collect()
    }
}

impl Drop for Fixture {
    fn drop(&mut self) {
        if let Some(session) = self.session.take() {
            session.end();
        }
        // Every host the test started, gently first.
        let hosts: Vec<i32> = fs::read_dir(self.dir.join("state/hypercube"))
            .into_iter()
            .flatten()
            .flatten()
            .map(|e| e.path())
            .filter(|p| p.to_string_lossy().ends_with(".host.lock"))
            .filter(|p| host::running(&p.with_extension("").with_extension("json")).unwrap())
            .filter_map(|p| fs::read_to_string(p).ok()?.trim().parse().ok())
            .collect();
        for &pid in &hosts {
            unsafe { libc::kill(pid, libc::SIGTERM) };
        }
        let end = Instant::now() + Duration::from_secs(10);
        while hosts.iter().any(|&p| alive(p)) && Instant::now() < end {
            std::thread::sleep(Duration::from_millis(20));
        }
        for &pid in &hosts {
            unsafe { libc::kill(pid, libc::SIGKILL) };
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
    assert_eq!(flag_value(argv, "--permission-mode"), Some("auto"));
    assert!(!argv.iter().any(|a| a == "--resume"));
    assert_eq!(launch["harness"], "claude");
    assert_eq!(launch["cwd"], fx.repo.to_str().unwrap());
    assert_eq!(launch["env"]["HYPERCUBE_AGENT_ID"], o.id.as_str());
    assert_eq!(
        launch["env"]["HYPERCUBE_STATE"],
        fx.state_path().to_str().unwrap()
    );
    assert_eq!(launch["env"]["TERM"], "xterm-256color");
    // The pane takes half the columns, and every row but the header, title,
    // and bar.
    assert_eq!(fx.size("orchestrator"), [59, u64::from(ROWS) - 3]);

    let events = fx.agent_events("orchestrator", "hook");
    let facts = &hook_contexts(&events, "SessionStart")[0];
    assert!(
        facts.contains(&format!("You are orchestrator ({})", o.id)),
        "{facts}"
    );
    assert!(
        facts.contains("deepseek (opencode, stub/opencode, medium): OpenCode"),
        "{facts}"
    );

    // The header names the workspace, at the left.
    let dir = fx.state().agent("orchestrator").unwrap().dir.clone();
    fx.wait_screen(&format!(" {dir}"));
    let screen = fx.screen();
    let lines: Vec<&str> = screen.lines().collect();
    assert!(lines[0].starts_with(&format!(" {dir}")), "{screen}");
    // The title turns down into the divider right of the pane.
    assert!(lines[1].trim_end().ends_with("━┓"), "{screen}");
    assert_eq!(lines[2].trim_end().chars().nth(59), Some('┃'), "{screen}");
    // The bar lists the workspace, the only one running.
    assert_eq!(
        lines[usize::from(ROWS) - 1].trim_end(),
        " 0 repo",
        "{screen}"
    );
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
    assert_eq!(launch["size"], json!([60, 37]));
    fx.wait_size("orchestrator", [59, 37]);

    // The message queued at creation arrived with the first prompt.
    fx.wait_idle("w1");
    let events = fx.agent_events("w1", "hook");
    let delivered = format!("Message from orchestrator ({oid}):\ndo the thing");
    assert!(
        hook_contexts(&events, "UserPromptSubmit")[0].contains(&delivered),
        "{events:?}"
    );
    assert!(fx.state().messages.is_empty());

    // An OpenCode worker joins as the third pane, and the panes share the
    // right side.
    let (text, is_error) = fx.call(
        "orchestrator",
        "create_agent",
        json!({"name": "w2", "profile": "deepseek"}),
    );
    assert!(!is_error, "{text}");
    fx.wait_event("w2", "ready", 1);
    let launch = fx.agent_events("w2", "launch").remove(0);
    let argv = args_of(&launch);
    assert_eq!(launch["harness"], "opencode");
    let w2 = fx.agent_id("w2").unwrap();
    let session = format!("ses_hypercube_{w2}");
    assert_eq!(
        argv,
        [
            "--standalone",
            "--auto",
            "--session",
            session.as_str(),
            "--prompt",
            NUDGE
        ]
    );
    assert_eq!(launch["env"]["HYPERCUBE_SESSION_ID"], session.as_str());
    let config: Value =
        serde_json::from_str(launch["env"]["OPENCODE_CONFIG_CONTENT"].as_str().unwrap()).unwrap();
    let plugin = fx.dir.join("state/hypercube/opencode-plugin");
    assert_eq!(config["plugins"], json!([plugin.to_str().unwrap()]));
    assert!(plugin.join("index.js").exists());
    assert_eq!(config["model"], "stub/opencode");
    assert_eq!(config["agents"]["build"]["model"], "stub/opencode#medium");
    assert_eq!(launch["size"], json!([60, 18]));
    fx.wait_size("w1", [60, 18]);
    fx.wait_size("orchestrator", [59, 37]);

    fx.wait_screen("⎇ w2");
    let screen = fx.screen();
    let lines: Vec<&str> = screen.lines().collect();
    // Each worker's label row is below the header, and the second's is
    // below the first's pane.
    assert!(lines[1].contains(" w1 "), "{screen}");
    assert!(lines[1].contains("⎇ w1"), "{screen}");
    assert!(lines[20].contains(" w2 "), "{screen}");
    assert!(lines[20].contains("⎇ w2"), "{screen}");
}

#[test]
fn send_message_wakes_an_idle_agent_without_typing() {
    let mut fx = Fixture::new();
    fx.start();
    let oid = fx.agent_id("orchestrator").unwrap();
    let w1 = fx.create("w1", "fast", "");
    let w2 = fx.create("w2", "deepseek", "");

    for (name, id) in [("w1", w1), ("w2", w2)] {
        let text = format!("hello {name}");
        let (reply, is_error) = fx.call(
            "orchestrator",
            "send_message",
            json!({"to": name, "text": text}),
        );
        assert!(!is_error, "{reply}");
        assert_eq!(reply, format!("Message queued for {name} ({id})."));

        // The agent's waiter took the message and woke its harness.
        let wakes = fx.wait_event(name, "wake", 1);
        assert_eq!(
            wakes[0]["text"],
            format!("Message from orchestrator ({oid}):\n{text}\n")
        );
        fx.wait_idle(name);
        assert!(fx.agent_events(name, "input").is_empty());
        assert_eq!(fx.agent_events(name, "prompt").len(), 1);
        // Claude Code continues its session; OpenCode's plugin starts a turn.
        let submits = hook_contexts(&fx.agent_events(name, "hook"), "UserPromptSubmit").len();
        assert_eq!(submits, if name == "w2" { 2 } else { 1 });
    }
    assert!(fx.state().messages.is_empty());

    // An agent cannot message itself.
    let (text, is_error) = fx.call("w1", "send_message", json!({"to": "w1", "text": "x"}));
    assert!(is_error, "{text}");
}

#[test]
fn a_woken_agent_keeps_the_prompt_being_typed() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    fx.wait_idle("orchestrator");

    // The orchestrator's pane has the focus.
    fx.type_text("half a prom");
    fx.wait("the typing to arrive", |fx| {
        let inputs = fx.agent_events("orchestrator", "input");
        let typed: String = inputs.iter().map(|e| e["data"].as_str().unwrap()).collect();
        (typed == "half a prom").then_some(())
    });
    let (text, is_error) = fx.call(
        "w1",
        "send_message",
        json!({"to": "orchestrator", "text": "done"}),
    );
    assert!(!is_error, "{text}");
    fx.wait_event("orchestrator", "wake", 1);
    fx.wait_idle("orchestrator");
    assert!(fx.agent_events("orchestrator", "prompt").is_empty());

    fx.type_text("pt\r");
    let prompts = fx.wait_event("orchestrator", "prompt", 1);
    assert_eq!(prompts[0]["text"], "half a prompt");
}

#[test]
fn the_orchestrator_title_tracks_checkout_branch_changes() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    fx.wait_screen("⎇ main");
    fx.git(&fx.repo, &["checkout", "-b", "feature/title"]);
    fx.wait_screen("⎇ feature/title");
    fx.wait_no_screen("⎇ main");
    fx.wait_screen("⎇ w1");
    fx.git(&fx.repo, &["checkout", "--detach"]);
    fx.wait_no_screen("⎇ feature/title");
    fx.wait_screen("⎇ w1");
    fx.git(&fx.repo, &["checkout", "main"]);
    fx.wait_screen("⎇ main");
}

#[test]
fn title_attention_refreshes_even_when_the_workspace_dot_does_not_change() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    fx.wait_idle("orchestrator");
    fx.wait_idle("w1");
    store::update(&fx.state_path(), |s| {
        for agent in &mut s.agents {
            agent.attention = true;
        }
        Ok(())
    })
    .unwrap();
    fx.wait_screen("orchestrator ●");
    fx.wait_screen("w1 ●");
    fx.wait_screen("0 repo ●");

    // The bar still has a dot, but the orchestrator's title must redraw.
    store::update(&fx.state_path(), |s| {
        s.agents
            .iter_mut()
            .find(|a| a.name == "orchestrator")
            .unwrap()
            .attention = false;
        Ok(())
    })
    .unwrap();
    fx.wait_no_screen("orchestrator ●");
    fx.wait_screen("w1 ●");
    fx.wait_screen("0 repo ●");

    store::update(&fx.state_path(), |s| {
        for agent in &mut s.agents {
            agent.attention = false;
        }
        Ok(())
    })
    .unwrap();
    fx.wait_no_screen("w1 ●");
    fx.wait_no_screen("0 repo ●");
}

#[test]
fn remove_agent_closes_the_pane() {
    let mut fx = Fixture::new();
    fx.start();
    let w1 = fx.create("w1", "fast", "");
    let pid = fx.pid("w1");
    assert!(alive(pid));
    fx.wait_screen("⎇ w1");
    fx.wait_size("orchestrator", [59, 37]);

    let (text, is_error) = fx.call("orchestrator", "remove_agent", json!({"id": w1}));
    assert!(!is_error, "{text}");
    assert_eq!(text, format!("Removed w1 ({w1})."));

    fx.wait_gone(&[pid]);
    fx.wait_no_screen("⎇ w1");
    // The orchestrator keeps its half of the terminal.
    assert_eq!(fx.size("orchestrator"), [59, 37]);
    assert_eq!(fx.state().agents.len(), 1);
    assert!(!fx.worktree("w1").exists());
    assert_eq!(fx.git(&fx.repo, &["branch", "--list", "hypercube/w1"]), "");
    assert!(host::running(&fx.state_path()).unwrap());
    // The closed agent is not relaunched.
    std::thread::sleep(Duration::from_millis(500));
    assert_eq!(
        fx.events_of(&w1)
            .iter()
            .filter(|e| e["event"] == "launch")
            .count(),
        1
    );
    assert_eq!(fx.pids().len(), 2);
}

#[test]
fn start_runs_a_host_in_the_background_until_a_terminal_attaches() {
    let mut fx = Fixture::new();
    let repo = fx.repo.to_str().unwrap().to_string();
    let running = |fx: &Fixture| String::from_utf8(fx.run(&["running"]).stdout).unwrap();
    assert_eq!(running(&fx), "");
    fx.start();
    assert_eq!(running(&fx), format!("{repo}\n"));
    fx.stop();
    assert_eq!(running(&fx), "");

    // `start` returns once the host listens, and the agents wait for a
    // terminal to attach.
    let out = fx.run(&["start", &repo]);
    assert!(out.status.success(), "{out:?}");
    assert!(host::running(&fx.state_path()).unwrap());
    assert_eq!(running(&fx), format!("{repo}\n"));
    assert_eq!(fx.agent_events("orchestrator", "launch").len(), 1);
    // A running host is left alone.
    let host = fx.host_pid();
    let out = fx.run(&["start", &repo]);
    assert!(out.status.success(), "{out:?}");
    assert_eq!(fx.host_pid(), host);

    fx.start();
    assert_eq!(fx.host_pid(), host);
    assert_eq!(fx.agent_events("orchestrator", "launch").len(), 2);
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
    assert!(alive(fx.host_pid()));

    // The host ends the client, which says why.
    fx.stop();
    assert!(fx.session.is_none());
    fx.wait_gone(&pids);
    // The agents that stop hung up are not relaunched.
    assert_eq!(fx.pids(), pids);
    assert_eq!(
        fx.status().unwrap()[..3],
        [repo, "stopped".into(), "3".into()]
    );
    assert!(!host::running(&fx.state_path()).unwrap());
    assert!(!control::socket_path(&fx.state_path()).unwrap().exists());
    // The agents stay for the next start.
    assert_eq!(fx.state().agents.len(), 3);

    let out = fx.run(&["stop"]);
    assert!(!out.status.success());
    assert!(String::from_utf8_lossy(&out.stderr).contains("not running"));
}

#[test]
fn a_directory_that_became_a_checkout_has_git_on_restart() {
    let mut fx = Fixture::new();
    fx.start();
    fx.stop();
    // As if the workspace had first started before `git init`.
    store::update(&fx.state_path(), |s| {
        s.no_git = true;
        Ok(())
    })
    .unwrap();
    fx.start();
    assert!(!fx.state().no_git);
    fx.stop();
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
        flag_value(args_of(&launch), "--session"),
        Some(sessions[2].as_str())
    );
    assert_eq!(flag_value(args_of(&launch), "--prompt"), Some(NUDGE));

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
fn a_moved_checkout_can_be_stopped_and_deleted_by_its_old_path() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    let old = fx.repo.clone();
    let moved = fx.dir.join("moved");
    fs::rename(&old, &moved).unwrap();
    fx.repo = moved;
    let old = old.to_str().unwrap();
    let out = fx.run(&["stop", old]);
    assert!(out.status.success(), "{out:?}");
    fx.wait_host_exit();
    let out = fx.run(&["delete", old]);
    assert!(out.status.success(), "{out:?}");
    assert_eq!(fx.state_files(), Vec::<String>::new());
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
    assert!(host::running(&fx.state_path()).unwrap());
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
    fx.wait_screen("old · failed");
    fx.wait_screen("old failed to launch");
    assert!(host::running(&fx.state_path()).unwrap());
    assert_eq!(fx.status().unwrap()[1], "running");

    let (text, is_error) = fx.call("orchestrator", "list_agents", json!({}));
    assert!(!is_error && text.contains("\"old\""), "{text}");
    let (text, is_error) = fx.call("orchestrator", "remove_agent", json!({"id": "old00001"}));
    assert!(!is_error, "{text}");
    assert_eq!(text, "Removed old (old00001).");
    fx.wait_no_screen("old · failed");
    assert_eq!(fx.size("orchestrator"), [59, u64::from(ROWS) - 3]);
    assert_eq!(fx.state().agents.len(), 1);
    assert!(host::running(&fx.state_path()).unwrap());
}

#[test]
fn an_exited_agent_is_relaunched_in_its_pane() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    fx.wait_screen("⎇ w1");
    let session = fx.state().agent("w1").unwrap().session_id.clone();
    let prompts = fx.agent_events("w1", "prompt").len();

    fx.command("w1", "exit 3");
    let launches = fx.wait_event("w1", "ready", 2);
    assert_eq!(launches.len(), 2);
    let launches = fx.agent_events("w1", "launch");
    let again = &launches[1];
    // It resumes its session in the same place, and picks up its messages.
    assert_eq!(
        flag_value(args_of(again), "--resume"),
        Some(session.as_str())
    );
    assert_eq!(args_of(again).last().unwrap(), NUDGE);
    assert_eq!(again["size"], launches[0]["size"]);
    assert_ne!(again["pid"], launches[0]["pid"]);
    // The host marked the worker busy before its harness started, so the
    // hooks of the new process come after.
    assert_eq!(again["activity"], "busy");
    let prompt = fx.wait_event("w1", "prompt", prompts + 1);
    assert_eq!(prompt[prompts]["text"], NUDGE);
    fx.wait_idle("w1");
    fx.wait_size("orchestrator", [59, 37]);
    let screen = fx.screen();
    assert!(screen.contains("⎇ w1"), "{screen}");
    assert!(!screen.contains("exited"), "{screen}");
    assert_eq!(fx.state().agent("w1").unwrap().session_id, session);

    // The orchestrator, which has no nudge, comes back idle.
    fx.command("orchestrator", "exit 0");
    fx.wait_event("orchestrator", "ready", 2);
    let launch = fx.agent_events("orchestrator", "launch").remove(1);
    assert_ne!(args_of(&launch).last().unwrap(), NUDGE);
    assert_eq!(launch["activity"], "idle");
    fx.wait_idle("orchestrator");
    assert_eq!(fx.agent_events("w1", "launch").len(), 2);
}

#[test]
fn an_exited_agents_waiter_ends_and_its_replacement_is_woken() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    let old: Vec<i32> = fx
        .agent_events("w1", "waiter")
        .iter()
        .map(|e| e["pid"].as_i64().unwrap() as i32)
        .collect();
    fx.command("w1", "exit 1");
    fx.wait_event("w1", "ready", 2);
    fx.wait_idle("w1");
    fx.wait_gone(&old);

    let (text, is_error) = fx.call(
        "orchestrator",
        "send_message",
        json!({"to": "w1", "text": "hello"}),
    );
    assert!(!is_error, "{text}");
    let wakes = fx.wait_event("w1", "wake", 1);
    assert!(wakes[0]["text"].as_str().unwrap().ends_with("hello\n"));
    fx.wait_idle("w1");
    assert_eq!(fx.agent_events("w1", "wake").len(), 1);
    assert!(fx.state().messages.is_empty());
}

#[test]
fn an_agent_that_keeps_exiting_is_left_exited() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    for n in 2..=3 {
        fx.command("w1", "exit 1");
        fx.wait_event("w1", "ready", n);
    }
    // The third quick exit in a row is not relaunched.
    fx.command("w1", "exit 1");
    // The team sees that it has exited.
    fx.wait("w1 to be marked exited", |fx| {
        (fx.state().agent("w1").unwrap().activity == store::EXITED).then_some(())
    });
    assert_eq!(fx.agent_events("w1", "launch").len(), 3);
    let (text, is_error) = fx.call("orchestrator", "list_agents", json!({}));
    assert!(!is_error && text.contains("\"exited\""), "{text}");
    let (text, is_error) = fx.call(
        "orchestrator",
        "send_message",
        json!({"to": "w1", "text": "hi"}),
    );
    assert!(!is_error && text.contains("w1 is not running"), "{text}");

    // The session goes on with the other agents.
    assert!(host::running(&fx.state_path()).unwrap());
    fx.stop();
}

#[test]
fn a_relaunched_pane_keeps_the_focus() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    fx.type_text("\x02o");
    fx.command("w1", "exit 1");
    fx.wait_event("w1", "ready", 2);
    let prompts = fx.wait_event("w1", "prompt", 2).len();
    fx.wait_idle("w1");
    fx.type_text("zz\r");
    let prompts = fx.wait_event("w1", "prompt", prompts + 1);
    assert_eq!(prompts.last().unwrap()["text"], "zz");
    assert_eq!(fx.agent_events("orchestrator", "prompt").len(), 0);
}

#[test]
fn the_host_runs_on_when_every_agent_is_left_exited() {
    let mut fx = Fixture::new();
    fx.start();
    for n in 2..=3 {
        fx.command("orchestrator", "exit 1");
        fx.wait_event("orchestrator", "ready", n);
    }
    let pid = fx.pid("orchestrator");
    fx.command("orchestrator", "exit 1");
    fx.wait_gone(&[pid]);
    fx.wait_screen("exited 1");
    std::thread::sleep(Duration::from_millis(500));
    assert_eq!(fx.agent_events("orchestrator", "launch").len(), 3);
    assert!(host::running(&fx.state_path()).unwrap());
    assert_eq!(fx.session().exited(), None);
    fx.stop();
}

#[test]
fn a_relaunch_that_fails_shows_why_and_is_not_retried() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    store::update(&fx.state_path(), |s| {
        s.agent_mut("w1")?.harness = "codex".into();
        Ok(())
    })
    .unwrap();

    fx.command("w1", "exit 1");
    fx.wait_screen("w1 failed to launch");
    std::thread::sleep(Duration::from_millis(500));
    assert_eq!(fx.agent_events("w1", "launch").len(), 1);
    assert!(host::running(&fx.state_path()).unwrap());
    fx.stop();
}

#[test]
fn ctrl_b_z_zooms_the_focused_pane() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    fx.wait_size("orchestrator", [59, 37]);
    let (w1_size, w1_resizes) = (fx.size("w1"), fx.agent_events("w1", "size").len());
    let whole = [u64::from(COLS), u64::from(ROWS) - 3];

    // The orchestrator fills the pane area, and w1 is hidden at its size.
    fx.type_text("\x02z");
    fx.wait_size("orchestrator", whole);
    fx.wait_screen("zoomed");
    fx.wait_no_screen("⎇ w1");
    fx.type_text("hi\r");
    fx.wait_event("orchestrator", "prompt", 1);
    fx.wait_idle("orchestrator");

    // The same key restores the layout.
    fx.type_text("\x02z");
    fx.wait_size("orchestrator", [59, 37]);
    fx.wait_screen("⎇ w1");
    fx.wait_no_screen("zoomed");

    // A change of focus restores it too, at once.
    fx.type_text("\x02o\x02z");
    // Both panes keep their label row while zoomed.
    fx.wait_size("w1", whole);
    fx.wait_screen("zoomed");
    let start = Instant::now();
    fx.type_text("\x02o");
    fx.wait_size("w1", w1_size.clone().try_into().unwrap());
    fx.wait_no_screen("zoomed");
    assert!(start.elapsed() < Duration::from_secs(5));

    // As does adding or removing a pane.
    fx.type_text("\x02z");
    fx.wait_size("orchestrator", whole);
    let w2 = fx.create("w2", "fast", "");
    fx.wait_no_screen("zoomed");
    fx.wait_size("orchestrator", [59, 37]);
    fx.type_text("\x02z");
    fx.wait_size("orchestrator", whole);
    let (text, is_error) = fx.call("orchestrator", "remove_agent", json!({"id": w2}));
    assert!(!is_error, "{text}");
    fx.wait_no_screen("zoomed");
    fx.wait_size("orchestrator", [59, 37]);

    // w1 was resized only to be zoomed and restored, and the stand-ins
    // never received the key.
    assert_eq!(fx.size("w1"), w1_size);
    assert!(fx.agent_events("w1", "size").len() > w1_resizes);
    let typed: String = ["orchestrator", "w1"]
        .iter()
        .flat_map(|n| fx.agent_events(n, "input"))
        .map(|e| e["data"].as_str().unwrap().to_string())
        .collect();
    assert_eq!(typed, "hi\r");
}

#[test]
fn stop_all_stops_running_workspaces_after_confirmation() {
    let mut fx = Fixture::new();
    let out = fx.run_with_input(&["stop", "--all"], "y\n");
    assert!(out.status.success(), "{out:?}");
    assert_eq!(
        String::from_utf8_lossy(&out.stdout),
        "No workspaces to stop.\n"
    );

    // The first workspace is stopped, and the second runs.
    fx.start();
    fx.create("w1", "fast", "");
    fx.stop();
    let first = fx.repo.clone();
    fx.repo = fx.dir.join("other");
    fs::create_dir(&fx.repo).unwrap();
    fx.git(&fx.repo, &["init", "-q", "-b", "main"]);
    fx.git(&fx.repo, &["commit", "-q", "--allow-empty", "-m", "init"]);
    fx.start();
    let pids = fx.pids();

    let out = fx.run_with_input(&["stop", "--all"], "n\n");
    assert!(!out.status.success());
    let stdout = String::from_utf8_lossy(&out.stdout).into_owned();
    assert_eq!(
        stdout,
        format!("{}\nStop all of these? [y/N] ", fx.repo.display())
    );
    assert!(String::from_utf8_lossy(&out.stderr).contains("stop cancelled"));
    assert!(host::running(&fx.state_path()).unwrap());

    let out = fx.run_with_input(&["stop", "--all"], "y\n");
    assert!(out.status.success(), "{out:?}");
    let stdout = String::from_utf8_lossy(&out.stdout).into_owned();
    assert_eq!(
        stdout,
        format!("{}\nStop all of these? [y/N] ", fx.repo.display())
    );
    fx.wait_host_exit();
    fx.wait_gone(&pids);
    assert!(fx.status().is_some());
    assert_eq!(
        fx.run_with_input(&["stop", "--all"], "y\n").stdout,
        b"No workspaces to stop.\n"
    );
    // The stopped workspace keeps its state.
    let name = workspace::name(&first);
    let state = store::load(&fx.dir.join(format!("state/hypercube/{name}.json"))).unwrap();
    assert_eq!(state.agents.len(), 2);
}

#[test]
fn delete_all_reports_refusals_and_deletes_the_rest() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    fx.stop();
    let first = fx.repo.clone();
    let dirty = fx.worktree("w1").join("scratch.txt");
    fs::write(&dirty, "work in progress").unwrap();

    fx.repo = fx.dir.join("other");
    fs::create_dir(&fx.repo).unwrap();
    fx.git(&fx.repo, &["init", "-q", "-b", "main"]);
    fx.git(&fx.repo, &["commit", "-q", "--allow-empty", "-m", "init"]);
    fx.start();
    fx.create("w2", "deepseek", "");
    let pids = fx.pids();
    let second_state = fx.state_path();

    let out = fx.run_with_input(&["delete", "--all"], "no\n");
    assert!(!out.status.success());
    assert!(String::from_utf8_lossy(&out.stderr).contains("delete cancelled"));
    assert!(host::running(&second_state).unwrap());

    // The first workspace refuses, and the second is deleted, ending its
    // session.
    let out = fx.run_with_input(&["delete", "--all"], "y\n");
    assert!(!out.status.success());
    let stdout = String::from_utf8_lossy(&out.stdout).into_owned();
    assert!(stdout.contains(first.to_str().unwrap()), "{stdout}");
    assert!(stdout.ends_with("Delete all of these? [y/N] "), "{stdout}");
    let stderr = String::from_utf8_lossy(&out.stderr).into_owned();
    assert!(
        stderr.contains(&format!(
            "delete failed for:\n{}: not deleting the workspace",
            first.display()
        )),
        "{stderr}"
    );
    assert!(!stderr.contains("other"), "{stderr}");
    fx.wait_host_exit();
    fx.wait_gone(&pids);
    assert!(!second_state.exists());
    assert!(!fx.dir.join("other/.hypercube/w2").exists());
    assert!(dirty.exists());
    let name = workspace::name(&first);
    assert!(
        fx.state_files().iter().all(|f| f.starts_with(&name)),
        "{:?}",
        fx.state_files()
    );

    fs::remove_file(&dirty).unwrap();
    let out = fx.run_with_input(&["delete", "--all"], "yes\n");
    assert!(out.status.success(), "{out:?}");
    assert_eq!(fx.state_files(), Vec::<String>::new());
    assert!(!first.join(".hypercube/w1").exists());
    assert_eq!(
        fx.run_with_input(&["delete", "--all"], "y\n").stdout,
        b"No workspaces to delete.\n"
    );
}

#[test]
fn closing_the_terminal_leaves_the_team_running() {
    let mut fx = Fixture::new();
    fx.start();
    fx.create("w1", "fast", "");
    let pids = fx.pids();
    let host = fx.host_pid();
    // Closing the client's terminal ends the client, and only it.
    assert_eq!(fx.session.take().unwrap().close_terminal(), 0);
    assert!(alive(host));
    assert!(pids.iter().all(|&p| alive(p)));
    assert_eq!(fx.status().unwrap()[1], "running");

    // Running hypercube again shows the same team, without relaunching it.
    fx.attach();
    fx.wait_screen("⎇ w1");
    assert_eq!(fx.pids(), pids);
    assert_eq!(fx.host_pid(), host);
    fx.type_text("hi\r");
    let prompts = fx.wait_event("orchestrator", "prompt", 1);
    assert_eq!(prompts[0]["text"], "hi");
    fx.stop();
    fx.wait_gone(&pids);
}

#[test]
fn a_second_terminal_takes_the_attachment() {
    let mut fx = Fixture::new();
    fx.start();
    let mut first = fx.session.take().unwrap();
    fx.attach();
    let code = fx.wait("the first client to exit", |_| first.exited());
    assert_eq!(code, 0);
    assert!(first.text().contains("attached in another terminal"));
    first.end();
    // The second has the input.
    fx.type_text("hi\r");
    fx.wait_event("orchestrator", "prompt", 1);
    fx.stop();
}

#[test]
fn the_overlay_switches_between_workspaces() {
    let mut fx = Fixture::new();
    // The other workspace has run before, and is stopped.
    fx.start();
    fx.stop();
    let other = fx.repo.clone();
    let other_state = fx.state_path();
    fx.new_repo("first");
    fx.start();
    fx.create("w1", "fast", "");
    fx.wait_screen("⎇ w1");
    let first = fx.repo.clone();
    let pids = vec![fx.pid("orchestrator"), fx.pid("w1")];

    // The overlay lists both, and Enter on the stopped one starts it.
    fx.select_workspace(&other);
    let screen = fx.screen();
    assert!(
        screen.contains("running") && screen.contains("stopped"),
        "{screen}"
    );
    fx.type_text("\r");
    fx.repo = other.clone();
    fx.wait_event("orchestrator", "ready", 2);
    fx.wait_no_screen("Workspaces");
    fx.wait_screen(other.to_str().unwrap());
    fx.wait_no_screen("⎇ w1");
    assert!(host::running(&other_state).unwrap());

    // Switching back shows the first team as it was.
    fx.select_workspace(&first);
    fx.type_text("\r");
    fx.repo = first.clone();
    fx.wait_screen("⎇ w1");
    fx.wait_no_screen("Workspaces");
    assert_eq!(fx.agent_events("orchestrator", "launch").len(), 1);
    assert_eq!(fx.agent_events("w1", "launch").len(), 1);
    assert!(pids.iter().all(|&p| alive(p)));
    assert!(host::running(&other_state).unwrap());

    // The bar numbers the running workspaces in the order their hosts
    // started, and Ctrl-b and a number switches to one.
    fx.wait_screen(" 0 first | 1 repo");
    fx.type_text("\x021");
    fx.wait_no_screen("⎇ w1");
    fx.type_text("\x020");
    fx.wait_screen(" 0 first | 1 repo");
    fx.wait_screen("⎇ w1");
    // Ctrl-b ( and ) step along the bar, wrapping around.
    fx.type_text("\x02(");
    fx.wait_screen(" 0 first | 1 repo");
    fx.wait_no_screen("⎇ w1");
    fx.type_text("\x02)");
    fx.wait_screen(" 0 first | 1 repo");
    fx.wait_screen("⎇ w1");

    // x stops the other workspace. While its teardown is held, this host
    // keeps serving.
    fx.repo = other.clone();
    fx.command("orchestrator", "hold-hup");
    fx.wait_event("orchestrator", "hold-hup", 1);
    fx.repo = first.clone();
    fx.select_workspace(&other);
    fx.type_text("x");
    fx.repo = other.clone();
    fx.wait_event("orchestrator", "hup", 1);
    fx.repo = first.clone();
    fx.type_text("\x1b");
    fx.wait_no_screen("Workspaces");
    fx.type_text("hi\r");
    fx.wait_event("orchestrator", "prompt", 1);
    assert!(host::running(&other_state).unwrap());
    fx.wait("the other workspace to stop", |_| {
        (!host::running(&other_state).unwrap()).then_some(())
    });
    fx.type_text("\x02s");
    fx.wait_screen("Workspaces");
    fx.wait_screen("stopped  1 agent");
    // Esc closes the overlay, and x on this workspace ends the client.
    fx.type_text("\x1b");
    fx.wait_no_screen("Workspaces");
    fx.select_workspace(&first);
    fx.type_text("x");
    fx.wait_host_exit();
    fx.wait_gone(&pids);
}

#[test]
fn ctrl_b_c_switches_to_a_new_workspace_at_a_typed_path() {
    let mut fx = Fixture::new();
    fx.start();
    fx.wait_screen(" 0 repo");

    // The bar prompts for a path, and Esc closes the prompt.
    fx.type_text("\x02c");
    fx.wait_screen(" workspace path:");
    fx.type_text("x\x1b");
    fx.wait_screen(" 0 repo");

    // A path that isn't a directory shows an error in the bar.
    fx.type_text("\x02c/nonexistent-hypercube\r");
    fx.wait_screen(" /nonexistent-hypercube: No such file or directory");

    // A path relative to this workspace, inside another checkout, starts
    // that checkout's workspace and switches to it.
    let first = fx.new_repo("second");
    fs::create_dir(fx.repo.join("sub")).unwrap();
    fx.type_text("\x02c../second/sub\x7fb\r");
    fx.wait_event("orchestrator", "ready", 1);
    fx.wait_screen(" 0 repo | 1 second");
    let second = fx.repo.clone();
    fx.wait_screen(second.to_str().unwrap());
    assert!(host::running(&fx.state_path()).unwrap());
    fx.repo = first;
    assert!(host::running(&fx.state_path()).unwrap());
}

#[test]
fn a_host_of_another_protocol_is_refused() {
    let fx = Fixture::new();
    let state = fx.state_path();
    let _lock = host::try_lock(&host::lock_path(&state)).unwrap().unwrap();
    let listener =
        std::os::unix::net::UnixListener::bind(control::socket_path(&state).unwrap()).unwrap();
    let error = "the host of the repo runs another version of hypercube; \
                 stop it with `hypercube stop` and start it again";
    let server = std::thread::spawn(move || {
        let (stream, _) = listener.accept().unwrap();
        let request = control::read_request(&stream).unwrap();
        assert!(matches!(
            request,
            control::Request::Attach { protocol: 1, .. }
        ));
        control::write_reply(&stream, &control::Reply::error(error)).unwrap();
    });
    let mut session = fx.client();
    let code = loop {
        if let Some(code) = session.exited() {
            break code;
        }
        std::thread::sleep(Duration::from_millis(20));
    };
    server.join().unwrap();
    assert_eq!(code, 1);
    let text = session.text();
    assert!(text.contains("stop it with `hypercube stop`"), "{text}");
    // The terminal was given back.
    assert!(text.contains("\x1b[?1049l"), "{text}");
    session.end();
}

/// Whether the terminal of the client's pty is in canonical mode.
fn canonical(session: &Session) -> bool {
    let mut termios = std::mem::MaybeUninit::<libc::termios>::uninit();
    assert_eq!(
        unsafe { libc::tcgetattr(session.master.as_raw_fd(), termios.as_mut_ptr()) },
        0
    );
    unsafe { termios.assume_init() }.c_lflag & libc::ICANON != 0
}

#[test]
fn signals_give_the_terminal_back_and_leave_the_host_running() {
    let mut fx = Fixture::new();
    fx.start();
    let host = fx.host_pid();
    for signal in [libc::SIGHUP, libc::SIGINT, libc::SIGTERM] {
        if fx.session.is_none() {
            fx.attach();
        }
        assert!(!canonical(fx.session()));
        let pid = fx.session().child.id() as i32;
        unsafe { libc::kill(pid, signal) };
        let code = fx.wait("the client to exit", |fx| fx.session().exited());
        assert_eq!(code, 0, "signal {signal}");
        let session = fx.session.take().unwrap();
        assert!(canonical(&session), "signal {signal}");
        assert!(session.text().ends_with("\x1b[?1049l"), "signal {signal}");
        session.end();
        assert!(alive(host));
        assert!(host::running(&fx.state_path()).unwrap());
    }
}

#[test]
fn keys_typed_with_the_overlay_key_go_to_the_overlay() {
    let mut fx = Fixture::new();
    fx.start();
    fx.wait_idle("orchestrator");
    // Ctrl-b ? opens the list of keys, and any key closes it.
    fx.type_text("\x02?");
    fx.wait_screen("Ctrl-b o");
    fx.type_text("x");
    fx.wait_no_screen("Ctrl-b o");
    // Ctrl-b a gives the workspace the next color, and keeps it.
    fx.type_text("\x02a\x02a");
    fx.wait("the second color", |fx| {
        (fx.state().accent == 2).then_some(())
    });
    // Ctrl-b s and d arrive in one read, and detach.
    fx.type_text("\x02sd\r");
    let code = fx.wait("the client to detach", |fx| fx.session().exited());
    assert_eq!(code, 0);
    fx.session.take().unwrap().end();
    std::thread::sleep(Duration::from_millis(300));
    assert!(fx.agent_events("orchestrator", "input").is_empty());
    assert!(fx.agent_events("orchestrator", "prompt").is_empty());
    assert!(host::running(&fx.state_path()).unwrap());

    // Ctrl-b d detaches too.
    fx.attach();
    fx.type_text("\x02d");
    let code = fx.wait("the client to detach", |fx| fx.session().exited());
    assert_eq!(code, 0);
    fx.session.take().unwrap().end();
    assert!(fx.agent_events("orchestrator", "input").is_empty());
    assert!(host::running(&fx.state_path()).unwrap());
}

/// Wait up to five seconds for the client to exit, and return its code.
fn exit_code(session: &mut Session) -> i32 {
    let end = Instant::now() + Duration::from_secs(5);
    loop {
        if let Some(code) = session.exited() {
            return code;
        }
        assert!(Instant::now() < end, "the client did not exit");
        std::thread::sleep(Duration::from_millis(20));
    }
}

#[test]
fn a_signal_ends_an_attach_the_host_does_not_answer() {
    let fx = Fixture::new();
    let state = fx.state_path();
    let _lock = host::try_lock(&host::lock_path(&state)).unwrap().unwrap();
    let listener =
        std::os::unix::net::UnixListener::bind(control::socket_path(&state).unwrap()).unwrap();
    let (got, release) = (
        Arc::new(AtomicBool::new(false)),
        Arc::new(AtomicBool::new(false)),
    );
    let (g, r) = (got.clone(), release.clone());
    let server = std::thread::spawn(move || {
        let (stream, _) = listener.accept().unwrap();
        control::read_request(&stream).unwrap();
        g.store(true, Ordering::Relaxed);
        // The reply never comes.
        while !r.load(Ordering::Relaxed) {
            std::thread::sleep(Duration::from_millis(20));
        }
    });
    let mut session = fx.client();
    while !got.load(Ordering::Relaxed) {
        std::thread::sleep(Duration::from_millis(20));
    }
    assert!(!canonical(&session));
    unsafe { libc::kill(session.child.id() as i32, libc::SIGHUP) };
    assert_eq!(exit_code(&mut session), 0);
    assert!(canonical(&session));
    assert!(!session.text().contains("cancelled"), "{}", session.text());
    release.store(true, Ordering::Relaxed);
    server.join().unwrap();
    session.end();
}

#[test]
fn a_signal_ends_a_client_whose_terminal_takes_no_output() {
    let mut fx = Fixture::new();
    fx.start();
    let host = fx.host_pid();
    let session = fx.session();
    // The terminal stops reading, and a large repaint fills it.
    session.stop.store(true, Ordering::Relaxed);
    session.reader.take().unwrap().join().unwrap();
    let ws = Winsize {
        ws_col: 500,
        ws_row: 151,
        ws_xpixel: 0,
        ws_ypixel: 0,
    };
    unsafe { libc::ioctl(session.master.as_raw_fd(), libc::TIOCSWINSZ, &ws) };
    std::thread::sleep(Duration::from_millis(500));
    assert_eq!(session.exited(), None);
    unsafe { libc::kill(session.child.id() as i32, libc::SIGHUP) };
    assert_eq!(exit_code(session), 0);
    assert!(canonical(session));
    fx.session.take().unwrap().end();
    assert!(alive(host));
}
