//! The session host: the `hypercube` process in the user's terminal. It holds
//! the workspace's host lock, runs every agent in a pane, draws the panes with
//! their labels and the status bar, and answers requests on the control
//! socket until the session ends.

use std::error::Error;
use std::fs::{self, File, OpenOptions, TryLockError};
use std::io::{ErrorKind, Read, Write};
use std::os::fd::{AsRawFd, RawFd};
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicI32, Ordering};
use std::time::{Duration, Instant};

use crate::control::{self, NUDGE, Reply, Request};
use crate::harness;
use crate::mux::{self, Chrome, Key, Mux, Pane, PaneId, PaneView, Rect, Winsize, now_ms};
use crate::pty::Pty;
use crate::runtime::{Event, Runtime};
use crate::store::{self, Agent};
use crate::{config, workspace};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// How long after typing the nudge the host presses Enter.
const NUDGE_ENTER_MS: i64 = 200;
/// How long a control connection has to send its request.
const REQUEST_TIMEOUT_MS: i64 = 5000;
/// The longest request the host reads.
const MAX_REQUEST: usize = 1 << 20;
/// How long `stop` waits for the host to end.
const STOP_TIMEOUT: Duration = Duration::from_secs(10);

/// The blue of the rules.
const RULE: &str = "\x1b[0;38;2;0;135;255m";
/// Black on light blue, for labels and the status bar.
const LABEL: &str = "\x1b[0;30;48;2;135;215;255m";

/// The host lock of the workspace whose state file is `state_path`. The host
/// holds it for its whole life, and writes the terminal it runs in into it.
pub fn lock_path(state_path: &Path) -> PathBuf {
    state_path.with_extension("host.lock")
}

/// The terminal of the workspace's host, empty if not yet known, or `None` if
/// no host is running.
pub fn running(state_path: &Path) -> Result<Option<String>> {
    let path = lock_path(state_path);
    let mut file = match File::open(&path) {
        Ok(file) => file,
        Err(e) if e.kind() == ErrorKind::NotFound => return Ok(None),
        Err(e) => return Err(format!("open {}: {e}", path.display()).into()),
    };
    match file.try_lock_shared() {
        Ok(()) => Ok(None),
        Err(TryLockError::WouldBlock) => {
            let mut terminal = String::new();
            file.read_to_string(&mut terminal)
                .map_err(|e| format!("read {}: {e}", path.display()))?;
            Ok(Some(terminal.trim().to_string()))
        }
        Err(TryLockError::Error(e)) => Err(format!("lock {}: {e}", path.display()).into()),
    }
}

/// Ask the workspace's host to end the session, and wait until it has.
pub fn stop(state_path: &Path) -> Result<()> {
    control::request(&control::socket_path(state_path)?, &Request::Stop)?.into_result()?;
    let start = Instant::now();
    while running(state_path)?.is_some() {
        if start.elapsed() > STOP_TIMEOUT {
            return Err("the host did not end the session".into());
        }
        std::thread::sleep(Duration::from_millis(50));
    }
    Ok(())
}

/// Take the host lock without waiting, or `None` if another host holds it.
fn try_lock(path: &Path) -> Result<Option<File>> {
    if let Some(dir) = path.parent() {
        fs::create_dir_all(dir).map_err(|e| format!("create {}: {e}", dir.display()))?;
    }
    let file = OpenOptions::new()
        .create(true)
        .truncate(false)
        .write(true)
        .open(path)
        .map_err(|e| format!("open {}: {e}", path.display()))?;
    match file.try_lock() {
        Ok(()) => Ok(Some(file)),
        Err(TryLockError::WouldBlock) => Ok(None),
        Err(TryLockError::Error(e)) => Err(format!("lock {}: {e}", path.display()).into()),
    }
}

/// The name of the terminal on standard input, or the process if it has none.
fn terminal_name() -> String {
    let name = unsafe { libc::ttyname(0) };
    if name.is_null() {
        return format!("process {}", std::process::id());
    }
    unsafe { std::ffi::CStr::from_ptr(name) }
        .to_string_lossy()
        .into_owned()
}

/// Where each pane goes. The first pane, the orchestrator's, takes the left
/// 40% of the columns, or all of them when it is alone; a one-column rule
/// follows; the other panes split the remaining columns with equal heights.
/// Each pane has a label row above it, and the bottom row is the status bar.
/// `None` if a pane would get no row or column.
pub fn layout(size: Winsize, panes: &[PaneId]) -> Option<Vec<(PaneId, Rect)>> {
    let cols = u32::from(size.ws_col);
    let rows = u32::from(size.ws_row.saturating_sub(1));
    let Some((&first, rest)) = panes.split_first() else {
        return Some(Vec::new());
    };
    let left = if rest.is_empty() { cols } else { cols * 2 / 5 };
    let rect = |col: u32, top: u32, cols: u32, bottom: u32| Rect {
        col: col as u16,
        row: top as u16 + 1,
        cols: cols as u16,
        rows: bottom.saturating_sub(top + 1) as u16,
    };
    let mut layout = vec![(first, rect(0, 0, left, rows))];
    let n = rest.len() as u32;
    for (i, &id) in (0..).zip(rest) {
        let (top, bottom) = (rows * i / n, rows * (i + 1) / n);
        let width = cols.saturating_sub(left + 1);
        layout.push((id, rect(left + 1, top, width, bottom)));
    }
    layout
        .iter()
        .all(|(_, r)| r.cols > 0 && r.rows > 0)
        .then_some(layout)
}

/// The labels above the panes, the rule between the first pane and the
/// others, and the status bar.
pub struct TeamChrome {
    /// The status bar's text.
    pub status: String,
}

impl Chrome for TeamChrome {
    fn draw(&mut self, out: &mut Vec<u8>, panes: &[PaneView<'_>], size: Winsize) {
        let mut s = String::new();
        // Panes are hidden while the terminal is too small for them.
        let shown: Vec<&PaneView> = panes.iter().filter(|p| p.rect.rows > 0).collect();
        for p in &shown {
            draw_label(&mut s, p);
        }
        if let [first, others @ ..] = shown.as_slice()
            && !others.is_empty()
        {
            s.push_str(RULE);
            for row in 0..size.ws_row.saturating_sub(1) {
                let c = if row == 0 {
                    '┳'
                } else if others.iter().any(|p| p.rect.row == row + 1) {
                    '┣'
                } else {
                    '┃'
                };
                s.push_str(&format!("\x1b[{};{}H{c}", row + 1, first.rect.cols + 1));
            }
        }
        if size.ws_row > 0 {
            let width = usize::from(size.ws_col);
            let text = fit(&format!(" {}", self.status), width);
            s.push_str(&format!("\x1b[{};1H{LABEL}{text:<width$}", size.ws_row));
        }
        s.push_str("\x1b[0m");
        out.extend_from_slice(s.as_bytes());
    }
}

/// A pane's label row: a rule with the label near its left end, bold when the
/// pane has the focus, and the exit status once its program has exited.
fn draw_label(s: &mut String, p: &PaneView) {
    let width = usize::from(p.rect.cols);
    let lead = width.min(2);
    let text = match p.status {
        Some(status) => format!(" {} · exited {status} ", p.label),
        None => format!(" {} ", p.label),
    };
    let text = fit(&text, width - lead);
    let rest = width - lead - text.chars().count();
    let bold = if p.focused { "\x1b[1m" } else { "" };
    s.push_str(&format!(
        "\x1b[{};{}H{RULE}{}{LABEL}{bold}{text}{RULE}{}",
        p.rect.row,
        p.rect.col + 1,
        "━".repeat(lead),
        "━".repeat(rest),
    ));
}

/// The first `width` characters of `text`.
fn fit(text: &str, width: usize) -> String {
    text.chars().take(width).collect()
}

static SIGNAL_PIPE: AtomicI32 = AtomicI32::new(-1);

extern "C" fn on_signal(_: libc::c_int) {
    let fd = SIGNAL_PIPE.load(Ordering::Relaxed);
    if fd < 0 {
        return;
    }
    unsafe {
        let saved = *libc::__error();
        let byte = 1u8;
        libc::write(fd, (&raw const byte).cast(), 1);
        *libc::__error() = saved;
    }
}

/// Catch SIGHUP, SIGINT, and SIGTERM, and return a pipe that becomes readable
/// when one arrives. Programs started later run with the default handlers.
fn catch_signals() -> Result<RawFd> {
    let mut fds = [0; 2];
    if unsafe { libc::pipe(fds.as_mut_ptr()) } != 0 {
        return Err(format!("pipe: {}", std::io::Error::last_os_error()).into());
    }
    for fd in fds {
        unsafe {
            libc::fcntl(
                fd,
                libc::F_SETFL,
                libc::fcntl(fd, libc::F_GETFL) | libc::O_NONBLOCK,
            );
            libc::fcntl(fd, libc::F_SETFD, libc::FD_CLOEXEC);
        }
    }
    SIGNAL_PIPE.store(fds[1], Ordering::Relaxed);
    for signal in [libc::SIGHUP, libc::SIGINT, libc::SIGTERM] {
        unsafe {
            let mut action: libc::sigaction = std::mem::zeroed();
            action.sa_sigaction = on_signal as extern "C" fn(libc::c_int) as libc::sighandler_t;
            libc::sigemptyset(&mut action.sa_mask);
            libc::sigaction(signal, &action, std::ptr::null_mut());
        }
    }
    Ok(fds[0])
}

/// The control socket's listener. Dropping it removes the socket file.
struct Socket {
    listener: UnixListener,
    path: PathBuf,
}

impl Socket {
    /// Listen on the workspace's control socket, replacing a file left by a
    /// host that ended without removing it. The caller holds the host lock.
    fn listen(state_path: &Path) -> Result<Socket> {
        let path = control::socket_path(state_path)?;
        match fs::remove_file(&path) {
            Err(e) if e.kind() != ErrorKind::NotFound => {
                return Err(format!("remove {}: {e}", path.display()).into());
            }
            _ => {}
        }
        let listener =
            UnixListener::bind(&path).map_err(|e| format!("listen on {}: {e}", path.display()))?;
        listener.set_nonblocking(true)?;
        Ok(Socket { listener, path })
    }
}

impl Drop for Socket {
    fn drop(&mut self) {
        let _ = fs::remove_file(&self.path);
    }
}

/// A control connection whose request has not fully arrived.
struct Client {
    stream: UnixStream,
    request: Vec<u8>,
    since: i64,
}

struct Host {
    exe: PathBuf,
    state_path: PathBuf,
    mux: Mux,
    runtime: Runtime,
    socket: Socket,
    signals: RawFd,
    /// Each agent's ID and pane, in launch order.
    panes: Vec<(String, PaneId)>,
    clients: Vec<Client>,
    /// Nudged panes and when to press Enter in each.
    enters: Vec<(i64, PaneId)>,
}

/// Host the workspace whose checkout is `root` in this terminal until the
/// session ends: when it is stopped, when every agent has exited, or when the
/// terminal closes.
pub fn run(root: &Path) -> Result<()> {
    let state_path = store::path(&workspace::name(root));
    let Some(mut lock) = try_lock(&lock_path(&state_path))? else {
        let terminal = running(&state_path)?
            .filter(|t| !t.is_empty())
            .unwrap_or_else(|| "another terminal".into());
        return Err(format!(
            "hypercube is already running for {} in {terminal}",
            workspace::short(root)
        )
        .into());
    };
    if unsafe { libc::isatty(0) == 0 || libc::isatty(1) == 0 } {
        return Err("hypercube must run in a terminal".into());
    }
    lock.set_len(0)?;
    lock.write_all(terminal_name().as_bytes())?;

    let agents = prepare(root, &state_path)?;
    let socket = Socket::listen(&state_path)?;
    let signals = catch_signals()?;
    let exe = std::env::current_exe().map_err(|e| format!("finding hypercube: {e}"))?;
    let chrome = TeamChrome {
        status: workspace::short(root),
    };
    let mut mux = Mux::new(Winsize::default(), Box::new(chrome));
    let runtime = Runtime::new(&mut mux, 0, 1)?;
    let mut host = Host {
        exe,
        state_path,
        mux,
        runtime,
        socket,
        signals,
        panes: Vec::new(),
        clients: Vec::new(),
        enters: Vec::new(),
    };
    let result = host.start(&agents).and_then(|()| host.serve());
    host.end();
    result
}

/// Ready the state for a session: create the orchestrator on the first run,
/// set its branch, and mark it idle and the workers busy. Returns the agents,
/// the orchestrator first.
fn prepare(root: &Path, state_path: &Path) -> Result<Vec<Agent>> {
    let config = config::load()?;
    let branch = workspace::branch(root);
    let dir = root
        .to_str()
        .ok_or_else(|| format!("{} is not valid UTF-8", root.display()))?;
    store::update(state_path, |s| {
        if s.agents.is_empty() {
            s.no_git = branch.is_err();
            let o = &config.orchestrator;
            s.agents.push(Agent {
                id: store::new_id()?,
                name: "orchestrator".into(),
                role: store::ORCHESTRATOR.into(),
                harness: o.harness.clone(),
                model: o.model.clone(),
                effort: o.effort.clone(),
                dir: dir.into(),
                ..Agent::default()
            });
        }
        for a in &mut s.agents {
            if a.role == store::ORCHESTRATOR {
                a.branch = branch.as_ref().cloned().unwrap_or_default();
                a.activity = store::IDLE.into();
            } else {
                a.activity = store::BUSY.into();
            }
        }
        let mut agents = s.agents.clone();
        agents.sort_by_key(|a| a.role != store::ORCHESTRATOR);
        Ok(agents)
    })
}

impl Host {
    /// Launch every agent, resuming saved sessions. Workers start a turn to
    /// pick up messages queued while they were down.
    fn start(&mut self, agents: &[Agent]) -> Result<()> {
        for a in agents {
            let prompt = if a.role == store::ORCHESTRATOR {
                ""
            } else {
                NUDGE
            };
            self.launch(&a.id, prompt)
                .map_err(|e| format!("launching {}: {e}", a.name))?;
        }
        Ok(())
    }

    /// Run the session until it is stopped, every pane's program has exited,
    /// or the terminal closes.
    fn serve(&mut self) -> Result<()> {
        loop {
            let listener = self.socket.listener.as_raw_fd();
            let mut fds = vec![listener, self.signals];
            fds.extend(self.clients.iter().map(|c| c.stream.as_raw_fd()));
            let deadline = self
                .enters
                .iter()
                .map(|&(at, _)| at)
                .chain(self.clients.iter().map(|c| c.since + REQUEST_TIMEOUT_MS))
                .min();
            for event in self.runtime.wait(&mut self.mux, &fds, deadline)? {
                match event {
                    Event::Ready(fd) if fd == listener => self.accept(),
                    Event::Ready(fd) if fd == self.signals => return Ok(()),
                    Event::Ready(fd) => {
                        if self.read_client(fd) {
                            return Ok(());
                        }
                    }
                    Event::PaneExited(..) => {
                        if self.mux.panes().iter().all(|p| p.status().is_some()) {
                            return Ok(());
                        }
                    }
                    Event::Resized => self.lay_out()?,
                    Event::InputClosed => return Ok(()),
                    Event::Deadline => {}
                }
            }
            let now = now_ms();
            let (due, later) = self.enters.iter().partition(|&&(at, _)| at <= now);
            self.enters = later;
            for (_, pane) in due {
                self.mux.send_key(pane, Key::Enter)?;
            }
            self.clients.retain(|c| now - c.since < REQUEST_TIMEOUT_MS);
        }
    }

    /// Hang up every pane's program, give them `HANGUP_GRACE_MS` to exit
    /// before they are killed, and restore the host terminal.
    fn end(&mut self) {
        let ids: Vec<PaneId> = self.mux.panes().iter().map(Pane::id).collect();
        for id in ids {
            self.mux.remove_pane(id);
        }
        self.panes.clear();
        let end = now_ms() + mux::HANGUP_GRACE_MS + 1000;
        while self.mux.reaping() && now_ms() < end {
            let deadline = Some(now_ms() + 20);
            if self.runtime.wait(&mut self.mux, &[], deadline).is_err() {
                break;
            }
        }
        self.runtime.restore();
    }

    fn accept(&mut self) {
        while let Ok((stream, _)) = self.socket.listener.accept() {
            if stream.set_nonblocking(true).is_ok() {
                self.clients.push(Client {
                    stream,
                    request: Vec::new(),
                    since: now_ms(),
                });
            }
        }
    }

    /// Read what a connection has sent, and once its request has arrived,
    /// carry it out and reply. Returns whether the request was to stop.
    fn read_client(&mut self, fd: RawFd) -> bool {
        let Some(i) = self.clients.iter().position(|c| c.stream.as_raw_fd() == fd) else {
            return false;
        };
        let client = &mut self.clients[i];
        let mut buf = [0u8; 4096];
        let ended = loop {
            match (&client.stream).read(&mut buf) {
                Ok(0) => break true,
                Ok(n) => client.request.extend_from_slice(&buf[..n]),
                Err(e) if e.kind() == ErrorKind::WouldBlock => break false,
                Err(e) if e.kind() == ErrorKind::Interrupted => {}
                Err(_) => break true,
            }
            if client.request.len() > MAX_REQUEST {
                break true;
            }
        };
        if !ended && !client.request.contains(&b'\n') {
            return false;
        }
        let client = self.clients.remove(i);
        let (reply, stop) = match control::read_request(client.request.as_slice()) {
            Ok(Request::Stop) => (Reply::ok(), true),
            Ok(request) => (self.handle(request), false),
            Err(e) => (Reply::error(e), false),
        };
        // The reply fits in the socket's buffer; a client that went away
        // doesn't get one.
        let _ = control::write_reply(&client.stream, &reply);
        stop
    }

    fn handle(&mut self, request: Request) -> Reply {
        let result = match request {
            Request::Launch { agent, prompt } => self.launch(&agent, &prompt),
            Request::Close { agent } => self.close(&agent),
            Request::Nudge { agent } => self.nudge(&agent),
            Request::Stop => Ok(()),
        };
        result.map_or_else(Reply::error, |()| Reply::ok())
    }

    fn pane_of(&self, agent: &str) -> Option<PaneId> {
        self.panes.iter().find(|(a, _)| a == agent).map(|&(_, p)| p)
    }

    fn pane_ids(&self) -> Vec<PaneId> {
        self.panes.iter().map(|&(_, p)| p).collect()
    }

    /// Start an agent's harness in a new pane and lay the panes out again. A
    /// non-empty `prompt` starts its first turn. Fails, adding no pane, if
    /// there is no room for one.
    fn launch(&mut self, id: &str, prompt: &str) -> Result<()> {
        let state = store::load(&self.state_path)?;
        let agent = state.agent(id)?;
        if self.pane_of(&agent.id).is_some() {
            return Err(format!("{} already has a pane", agent.name).into());
        }
        let launch = harness::command(&self.exe, agent, &self.state_path, prompt)?;
        let pane = self.mux.add_pane(&harness::label(agent))?;
        let mut ids = self.pane_ids();
        ids.push(pane);
        let size = self.mux.size();
        let Some(layout) = layout(size, &ids) else {
            self.mux.remove_pane(pane);
            return Err(format!(
                "no room for another pane in a {}x{} terminal",
                size.ws_col, size.ws_row
            )
            .into());
        };
        let rect = layout[layout.len() - 1].1;
        let ws = Winsize {
            ws_col: rect.cols,
            ws_row: rect.rows,
            ..Winsize::default()
        };
        let env: Vec<_> = launch.env.iter().collect();
        match Pty::spawn(&launch.argv, &launch.cwd, &env, ws) {
            Ok(pty) => self.mux.attach(pane, pty),
            Err(e) => {
                self.mux.remove_pane(pane);
                return Err(e.into());
            }
        }
        self.panes.push((agent.id.clone(), pane));
        self.mux.set_layout(&layout)?;
        Ok(())
    }

    /// Hang up an agent's program, remove its pane, and lay the panes out
    /// again. An agent without a pane is already closed.
    fn close(&mut self, id: &str) -> Result<()> {
        let Some(i) = self.panes.iter().position(|(a, _)| a == id) else {
            return Ok(());
        };
        let (_, pane) = self.panes.remove(i);
        self.mux.remove_pane(pane);
        self.lay_out()
    }

    /// Type the nudge into an agent's pane, and press Enter `NUDGE_ENTER_MS`
    /// later.
    fn nudge(&mut self, id: &str) -> Result<()> {
        let pane = self
            .pane_of(id)
            .ok_or_else(|| format!("agent {id} has no pane"))?;
        if let Some(status) = self.mux.pane(pane).and_then(Pane::status) {
            return Err(format!("agent {id} exited with status {status}").into());
        }
        self.mux.send_text(pane, NUDGE)?;
        self.enters.push((now_ms() + NUDGE_ENTER_MS, pane));
        Ok(())
    }

    /// Lay the panes out for the terminal's size. In a terminal too small
    /// for them, the panes are hidden until it grows.
    fn lay_out(&mut self) -> Result<()> {
        let ids = self.pane_ids();
        let layout = layout(self.mux.size(), &ids)
            .unwrap_or_else(|| ids.iter().map(|&id| (id, Rect::default())).collect());
        Ok(self.mux.set_layout(&layout)?)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testutil::TempDir;
    use libghostty_vt::Terminal;
    use libghostty_vt::style::{RgbColor, StyleColor};
    use libghostty_vt::terminal::{Point, PointCoordinate};

    fn size(cols: u16, rows: u16) -> Winsize {
        Winsize {
            ws_col: cols,
            ws_row: rows,
            ..Winsize::default()
        }
    }

    fn ids(n: u32) -> Vec<PaneId> {
        (1..=n).map(PaneId).collect()
    }

    fn rect(col: u16, row: u16, cols: u16, rows: u16) -> Rect {
        Rect {
            col,
            row,
            cols,
            rows,
        }
    }

    fn rects(layout: Option<Vec<(PaneId, Rect)>>) -> Vec<Rect> {
        layout.unwrap().into_iter().map(|(_, r)| r).collect()
    }

    #[test]
    fn a_lone_pane_fills_the_terminal_above_the_status_bar() {
        assert_eq!(rects(layout(size(80, 24), &ids(1))), [rect(0, 1, 80, 22)]);
        assert_eq!(layout(size(80, 24), &[]), Some(Vec::new()));
    }

    #[test]
    fn workers_stack_right_of_the_orchestrator() {
        assert_eq!(
            rects(layout(size(100, 25), &ids(2))),
            [rect(0, 1, 40, 23), rect(41, 1, 59, 23)]
        );
        // 24 rows over three workers: 8 each, a label row and seven content rows.
        assert_eq!(
            rects(layout(size(100, 25), &ids(4))),
            [
                rect(0, 1, 40, 23),
                rect(41, 1, 59, 7),
                rect(41, 9, 59, 7),
                rect(41, 17, 59, 7),
            ]
        );
        // Uneven heights differ by at most a row and fill the column.
        let layout = rects(layout(size(81, 31), &ids(5)));
        assert_eq!(layout[0], rect(0, 1, 32, 29));
        let heights: Vec<u16> = layout[1..].iter().map(|r| r.rows + 1).collect();
        assert_eq!(heights, [7, 8, 7, 8]);
        assert_eq!(layout[4].row + layout[4].rows, 30);
    }

    #[test]
    fn a_layout_without_room_for_every_pane_fails() {
        // Each pane needs a label row and a content row.
        assert!(layout(size(80, 2), &ids(1)).is_none());
        assert!(layout(size(80, 3), &ids(1)).is_some());
        assert!(layout(size(80, 6), &ids(4)).is_none());
        assert!(layout(size(80, 7), &ids(4)).is_some());
        // And a column on each side of the rule.
        assert!(layout(size(2, 24), &ids(2)).is_none());
        assert!(layout(size(3, 24), &ids(2)).is_some());
        assert!(layout(size(0, 24), &ids(1)).is_none());
    }

    struct Screen(Terminal<'static, 'static>);

    impl Screen {
        fn draw(panes: &[PaneView], cols: u16, rows: u16) -> Screen {
            let mut chrome = TeamChrome {
                status: "~/src/team".into(),
            };
            let mut out = Vec::new();
            chrome.draw(&mut out, panes, size(cols, rows));
            assert!(out.ends_with(b"\x1b[0m"));
            let mut terminal = Terminal::new(cols, rows).unwrap();
            terminal.vt_write(&out);
            Screen(terminal)
        }

        fn cell(&self, x: u16, y: u16) -> libghostty_vt::screen::GridRef<'_> {
            let point = Point::Active(PointCoordinate { x, y: u32::from(y) });
            self.0.grid_ref(point).unwrap()
        }

        fn row(&self, y: u16) -> String {
            let cols = self.0.cols().unwrap();
            (0..cols)
                .map(|x| {
                    let mut buf = ['\0'; 4];
                    match self.cell(x, y).graphemes(&mut buf).unwrap() {
                        0 => ' ',
                        _ => buf[0],
                    }
                })
                .collect()
        }

        fn bold(&self, x: u16, y: u16) -> bool {
            self.cell(x, y).style().unwrap().bold
        }

        fn bg(&self, x: u16, y: u16) -> StyleColor {
            self.cell(x, y).style().unwrap().bg_color
        }
    }

    fn view(id: u32, label: &str, rect: Rect, focused: bool, status: Option<u8>) -> PaneView<'_> {
        PaneView {
            id: PaneId(id),
            label,
            rect,
            focused,
            status,
        }
    }

    const LIGHT_BLUE: StyleColor = StyleColor::Rgb(RgbColor {
        r: 135,
        g: 215,
        b: 255,
    });

    #[test]
    fn chrome_draws_labels_rules_and_the_status_bar() {
        let layout = rects(layout(size(30, 8), &ids(3)));
        let panes = [
            view(1, "orch", layout[0], true, None),
            view(2, "w1", layout[1], false, None),
            view(3, "w2", layout[2], false, Some(3)),
        ];
        let screen = Screen::draw(&panes, 30, 8);
        let rows: Vec<String> = (0..8).map(|y| screen.row(y)).collect();
        assert_eq!(
            rows,
            [
                "━━ orch ━━━━┳━━ w1 ━━━━━━━━━━━",
                "            ┃                 ",
                "            ┃                 ",
                "            ┣━━ w2 · exited 3 ",
                "            ┃                 ",
                "            ┃                 ",
                "            ┃                 ",
                " ~/src/team                   ",
            ]
        );
        // The focused pane's label is bold, and labels and the status bar
        // are on light blue.
        assert!(screen.bold(3, 0));
        assert!(!screen.bold(15, 0));
        assert_eq!(screen.bg(3, 0), LIGHT_BLUE);
        assert_eq!(screen.bg(0, 0), StyleColor::None);
        assert_eq!(screen.bg(29, 7), LIGHT_BLUE);
    }

    #[test]
    fn chrome_truncates_labels_and_skips_hidden_panes() {
        let panes = [view(1, "orchestrator", rect(0, 1, 8, 2), false, None)];
        let screen = Screen::draw(&panes, 8, 4);
        assert_eq!(screen.row(0), "━━ orche");
        assert_eq!(screen.row(3), " ~/src/t");

        let hidden = [
            view(1, "orch", Rect::default(), true, None),
            view(2, "w1", Rect::default(), false, None),
        ];
        let screen = Screen::draw(&hidden, 8, 2);
        assert_eq!(screen.row(0), "        ");
        assert_eq!(screen.row(1), " ~/src/t");
    }

    #[test]
    fn the_host_lock_tells_whether_a_host_runs_and_where() {
        let dir = TempDir::new();
        let state = dir.path().join("w.json");
        assert_eq!(lock_path(&state), dir.path().join("w.host.lock"));
        assert_eq!(running(&state).unwrap(), None);

        let mut lock = try_lock(&lock_path(&state)).unwrap().unwrap();
        lock.write_all(b"/dev/ttys009").unwrap();
        assert_eq!(running(&state).unwrap().as_deref(), Some("/dev/ttys009"));
        assert!(try_lock(&lock_path(&state)).unwrap().is_none());

        // A child that another test spawns at the moment shares the lock
        // until it execs, so wait for the release.
        drop(lock);
        for _ in 0..200 {
            if running(&state).unwrap().is_none() {
                break;
            }
            std::thread::sleep(Duration::from_millis(10));
        }
        assert_eq!(running(&state).unwrap(), None);
        assert!(try_lock(&lock_path(&state)).unwrap().is_some());
    }

    #[test]
    fn a_leftover_socket_is_replaced_and_removed_on_drop() {
        let dir = TempDir::new();
        let state = dir.path().join("w.json");
        let path = control::socket_path(&state).unwrap();
        fs::write(&path, "").unwrap();
        let socket = Socket::listen(&state).unwrap();
        assert!(UnixStream::connect(&path).is_ok());
        drop(socket);
        assert!(!path.exists());
    }
}
