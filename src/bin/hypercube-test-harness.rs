//! The stand-in for `claude` and `pi` that the integration tests
//! (tests/integration.rs) put first on `PATH`. It behaves like a harness
//! in a pane: it runs the hypercube hooks, serves the hypercube MCP server,
//! and treats a typed line as a prompt. Everything it observes is appended to
//! `$HYPERCUBE_TEST_DIR/<agent id>/events.jsonl`, and the test drives it with
//! lines written to `$HYPERCUBE_TEST_DIR/<agent id>/fifo`:
//!
//! - `<tag> <tool> <json arguments>` calls an MCP tool and records the result.
//! - `hold-hup` makes the stub, once hung up, wait for the file `release` in
//!   its directory before it exits.
use std::collections::BTreeMap;
use std::io::{BufRead, BufReader, Write};
use std::path::{Path, PathBuf};
use std::process::{Child, ChildStdin, ChildStdout, Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, SystemTime};

use serde_json::{Value, json};

static WINCH: AtomicBool = AtomicBool::new(false);
static HUP: AtomicBool = AtomicBool::new(false);

extern "C" fn on_signal(signal: libc::c_int) {
    let flag = if signal == libc::SIGWINCH {
        &WINCH
    } else {
        &HUP
    };
    flag.store(true, Ordering::Relaxed);
}

fn handle(signal: libc::c_int) {
    unsafe {
        let mut action: libc::sigaction = std::mem::zeroed();
        action.sa_sigaction = on_signal as extern "C" fn(libc::c_int) as libc::sighandler_t;
        libc::sigemptyset(&mut action.sa_mask);
        libc::sigaction(signal, &action, std::ptr::null_mut());
    }
}

fn millis() -> u64 {
    SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .map_or(0, |d| d.as_millis() as u64)
}

/// The `hypercube` executable next to this one, which is reached through a
/// symlink named after the harness.
fn hypercube() -> PathBuf {
    std::fs::canonicalize(std::env::current_exe().unwrap())
        .unwrap()
        .with_file_name("hypercube")
}

fn size() -> Value {
    let mut ws: libc::winsize = unsafe { std::mem::zeroed() };
    unsafe { libc::ioctl(0, libc::TIOCGWINSZ, &mut ws) };
    json!([ws.ws_col, ws.ws_row])
}

/// The MCP server child and its pipes.
struct Mcp {
    _child: Child,
    stdin: ChildStdin,
    stdout: BufReader<ChildStdout>,
    next: u64,
}

impl Mcp {
    fn start(program: &str, args: &[String], env: &BTreeMap<String, String>) -> Mcp {
        let mut child = Command::new(program)
            .args(args)
            .envs(env)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::null())
            .spawn()
            .expect("start the MCP server");
        let stdin = child.stdin.take().unwrap();
        let stdout = BufReader::new(child.stdout.take().unwrap());
        let mut mcp = Mcp {
            _child: child,
            stdin,
            stdout,
            next: 0,
        };
        let params = json!({"protocolVersion": "2025-03-26", "capabilities": {},
            "clientInfo": {"name": "hypercube-test-harness", "version": "0"}});
        mcp.request("initialize", params);
        mcp
    }

    fn request(&mut self, method: &str, params: Value) -> Value {
        self.next += 1;
        let line = json!({"jsonrpc": "2.0", "id": self.next, "method": method, "params": params});
        writeln!(self.stdin, "{line}").expect("write to the MCP server");
        let mut reply = String::new();
        self.stdout
            .read_line(&mut reply)
            .expect("read from the MCP server");
        serde_json::from_str::<Value>(&reply).expect("MCP reply")["result"].clone()
    }
}

struct Stub {
    harness: String,
    dir: PathBuf,
    events: std::fs::File,
    session: String,
    /// The hook command of each event: the `--settings` entry for Claude Code.
    hooks: BTreeMap<String, String>,
    /// The `--settings` status line command, for Claude Code.
    status_line: Option<String>,
    mcp: Mcp,
    hold_hup: bool,
    typed: Vec<u8>,
}

impl Stub {
    fn record(&mut self, event: &str, fields: Value) {
        let mut line = json!({"event": event, "ms": millis()});
        for (key, value) in fields.as_object().into_iter().flatten() {
            line[key] = value.clone();
        }
        // One write, so a reader never sees half a line.
        self.events
            .write_all(format!("{line}\n").as_bytes())
            .unwrap();
    }

    /// Run the hook for `event` the way the harness does and record what it
    /// read and wrote. Returns its output.
    fn hook(&mut self, event: &str, extra: Value) -> String {
        let mut input = json!({"session_id": self.session, "hook_event_name": event});
        for (key, value) in extra.as_object().into_iter().flatten() {
            input[key] = value.clone();
        }
        let input = input.to_string();
        let mut command = if self.harness == "pi" {
            // Pi's extension runs `hypercube hook` from the executable it embeds.
            let mut c = Command::new(hypercube());
            c.args(["hook", event]);
            c
        } else {
            let command = self
                .hooks
                .get(event)
                .unwrap_or_else(|| panic!("--settings has no {event} hook"));
            let mut c = Command::new("sh");
            c.args(["-c", command]);
            c
        };
        let mut child = command
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::null())
            .spawn()
            .expect("run the hook");
        child
            .stdin
            .take()
            .unwrap()
            .write_all(input.as_bytes())
            .unwrap();
        let out = child.wait_with_output().unwrap();
        let output = String::from_utf8_lossy(&out.stdout).into_owned();
        let status = out.status.code();
        self.record(
            "hook",
            json!({"name": event, "input": input, "output": output, "status": status}),
        );
        output
    }

    /// Run the status line command the way Claude Code does, with `input` on
    /// its stdin, and record what it wrote.
    fn status_line(&mut self, input: &str) {
        let command = self
            .status_line
            .clone()
            .expect("--settings has a statusLine");
        let mut child = Command::new("sh")
            .args(["-c", &command])
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::null())
            .spawn()
            .expect("run the status line command");
        child
            .stdin
            .take()
            .unwrap()
            .write_all(input.as_bytes())
            .unwrap();
        let out = child.wait_with_output().unwrap();
        let output = String::from_utf8_lossy(&out.stdout).into_owned();
        self.record(
            "statusline",
            json!({"command": command, "output": output, "status": out.status.code()}),
        );
    }

    /// Submit a prompt and run the turn it starts.
    fn prompt(&mut self, text: &str) {
        self.record("prompt", json!({"text": text}));
        self.hook("UserPromptSubmit", json!({"prompt": text}));
        // A blocked stop continues the turn.
        for _ in 0..5 {
            let output = self.hook("Stop", json!({"stop_hook_active": false}));
            if !output.contains("\"decision\":\"block\"") {
                break;
            }
        }
    }

    fn command(&mut self, line: &str) {
        if line == "hold-hup" {
            self.hold_hup = true;
            self.record("hold-hup", json!({}));
            return;
        }
        if let Some(input) = line.strip_prefix("statusline ") {
            self.status_line(input);
            return;
        }
        let mut parts = line.splitn(3, ' ');
        let (tag, tool, args) = (
            parts.next().unwrap_or(""),
            parts.next().unwrap_or(""),
            parts.next().unwrap_or("{}"),
        );
        let args: Value = serde_json::from_str(args).unwrap_or_else(|_| json!({}));
        let result = self
            .mcp
            .request("tools/call", json!({"name": tool, "arguments": args}));
        let text = result["content"][0]["text"].as_str().unwrap_or("");
        let is_error = result["isError"].as_bool().unwrap_or(true);
        self.record(
            "tool",
            json!({"tag": tag, "name": tool, "text": text, "is_error": is_error}),
        );
        self.hook("PostToolUse", json!({"tool_name": tool}));
    }

    /// Record typed bytes and submit the line each Enter ends.
    fn input(&mut self, data: &[u8]) {
        self.record("input", json!({"data": String::from_utf8_lossy(data)}));
        for &b in data {
            if b == b'\r' {
                let text = String::from_utf8_lossy(&std::mem::take(&mut self.typed)).into_owned();
                self.prompt(&text);
            } else {
                self.typed.push(b);
            }
        }
    }

    fn hung_up(&mut self) -> ! {
        self.record("hup", json!({}));
        if self.hold_hup {
            let release = self.dir.join("release");
            let end = millis() + 1500;
            while !release.exists() && millis() < end {
                std::thread::sleep(Duration::from_millis(10));
            }
        }
        std::process::exit(129)
    }
}

fn raw_mode() {
    unsafe {
        let mut t = std::mem::zeroed();
        if libc::tcgetattr(0, &mut t) == 0 {
            libc::cfmakeraw(&mut t);
            libc::tcsetattr(0, libc::TCSANOW, &t);
        }
    }
}

fn open_fifo(path: &Path) -> i32 {
    let c = std::ffi::CString::new(path.to_str().unwrap()).unwrap();
    unsafe {
        libc::unlink(c.as_ptr());
        assert_eq!(libc::mkfifo(c.as_ptr(), 0o600), 0, "mkfifo");
        // Opened for writing as well, so it never reports end of file.
        libc::open(c.as_ptr(), libc::O_RDWR | libc::O_NONBLOCK)
    }
}

/// The value after `flag` in `args`.
fn flag_value(args: &[String], flag: &str) -> Option<String> {
    let i = args.iter().position(|a| a == flag)?;
    args.get(i + 1).cloned()
}

/// The prompt: the argument that is neither a flag nor a flag's value.
fn prompt_arg(args: &[String]) -> Option<String> {
    const WITH_VALUE: [&str; 8] = [
        "--mcp-config",
        "--settings",
        "--model",
        "--effort",
        "--resume",
        "--extension",
        "--thinking",
        "--session-id",
    ];
    let mut i = 0;
    let mut prompt = None;
    while i < args.len() {
        if WITH_VALUE.contains(&args[i].as_str()) {
            i += 1;
        } else if !args[i].starts_with("--") {
            prompt = Some(args[i].clone());
        }
        i += 1;
    }
    prompt
}

fn read_fd(fd: i32, buf: &mut [u8]) -> usize {
    let n = unsafe { libc::read(fd, buf.as_mut_ptr().cast(), buf.len()) };
    if n < 0 { 0 } else { n as usize }
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let harness = Path::new(&args[0])
        .file_name()
        .unwrap()
        .to_string_lossy()
        .into_owned();
    let args = &args[1..];
    let agent = std::env::var("HYPERCUBE_AGENT_ID").expect("HYPERCUBE_AGENT_ID");
    let dir = PathBuf::from(std::env::var("HYPERCUBE_TEST_DIR").expect("HYPERCUBE_TEST_DIR"))
        .join(&agent);
    std::fs::create_dir_all(&dir).unwrap();

    raw_mode();
    handle(libc::SIGWINCH);
    handle(libc::SIGHUP);

    let events = std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(dir.join("events.jsonl"))
        .unwrap();
    let session = flag_value(args, "--resume")
        .or_else(|| flag_value(args, "--session-id"))
        .unwrap_or_else(|| format!("session-{}-{}", std::process::id(), millis()));

    // Claude Code's hooks and MCP server come from its flags, Pi's from the
    // extension, which the stub does not run: it starts the server that
    // the extension embeds and calls the same hook commands.
    let mut hooks = BTreeMap::new();
    let mut status_line = None;
    let (program, mcp_args, mcp_env) = match flag_value(args, "--mcp-config") {
        Some(config) => {
            let config: Value = serde_json::from_str(&config).expect("--mcp-config");
            let server = &config["mcpServers"]["hypercube"];
            let settings: Value =
                serde_json::from_str(&flag_value(args, "--settings").expect("--settings")).unwrap();
            for (event, entry) in settings["hooks"].as_object().unwrap() {
                let command = entry[0]["hooks"][0]["command"].as_str().unwrap();
                hooks.insert(event.clone(), command.to_string());
            }
            status_line = settings["statusLine"]["command"].as_str().map(String::from);
            let strings = |v: &Value| -> Vec<String> {
                v.as_array()
                    .into_iter()
                    .flatten()
                    .map(|s| s.as_str().unwrap().to_string())
                    .collect()
            };
            let env = server["env"]
                .as_object()
                .into_iter()
                .flatten()
                .map(|(k, v)| (k.clone(), v.as_str().unwrap().to_string()))
                .collect();
            (
                server["command"].as_str().unwrap().to_string(),
                strings(&server["args"]),
                env,
            )
        }
        None => (
            hypercube().to_string_lossy().into_owned(),
            vec!["mcp".to_string()],
            BTreeMap::new(),
        ),
    };
    let mut stub = Stub {
        harness,
        dir: dir.clone(),
        events,
        session,
        hooks,
        status_line,
        mcp: Mcp::start(&program, &mcp_args, &mcp_env),
        hold_hup: false,
        typed: Vec::new(),
    };
    let env: BTreeMap<String, String> = std::env::vars().collect();
    let cwd = std::env::current_dir().unwrap();
    stub.record(
        "launch",
        json!({"pid": std::process::id(), "harness": stub.harness, "argv": args, "env": env,
            "cwd": cwd, "size": size(), "session": stub.session}),
    );

    stub.hook("SessionStart", json!({}));
    let fifo = open_fifo(&dir.join("fifo"));
    stub.record("ready", json!({}));
    if let Some(prompt) = prompt_arg(args) {
        stub.prompt(&prompt);
    }

    let mut pending = Vec::new();
    loop {
        if HUP.load(Ordering::Relaxed) {
            stub.hung_up();
        }
        if WINCH.swap(false, Ordering::Relaxed) {
            stub.record("size", json!({"size": size()}));
        }
        let mut fds = [
            libc::pollfd {
                fd: 0,
                events: libc::POLLIN,
                revents: 0,
            },
            libc::pollfd {
                fd: fifo,
                events: libc::POLLIN,
                revents: 0,
            },
        ];
        if unsafe { libc::poll(fds.as_mut_ptr(), 2, 50) } <= 0 {
            continue;
        }
        let mut buf = [0u8; 4096];
        if fds[0].revents & libc::POLLIN != 0 {
            let n = read_fd(0, &mut buf);
            if n > 0 {
                stub.input(&buf[..n]);
            }
        }
        if fds[1].revents & libc::POLLIN != 0 {
            let n = read_fd(fifo, &mut buf);
            pending.extend_from_slice(&buf[..n]);
            while let Some(i) = pending.iter().position(|&b| b == b'\n') {
                let line: Vec<u8> = pending.drain(..=i).collect();
                stub.command(String::from_utf8_lossy(&line[..i]).trim());
            }
        }
        // End of input: the host closed the pty.
        if fds[0].revents & libc::POLLHUP != 0 {
            std::process::exit(0);
        }
    }
}
