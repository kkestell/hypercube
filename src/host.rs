//! The session host: the `hypercube` process in the user's terminal. It holds
//! the workspace's host lock, runs every agent in a pane, draws the panes with
//! their labels and the status bar, and answers requests on the control
//! socket until the session ends.

use std::cell::{Cell, RefCell};
use std::error::Error;
use std::fs::{self, File, OpenOptions, TryLockError};
use std::io::{ErrorKind, Read, Write};
use std::os::fd::{AsRawFd, RawFd};
use std::os::unix::fs::MetadataExt;
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::{Path, PathBuf};
use std::rc::Rc;
use std::sync::atomic::{AtomicI32, Ordering};
use std::time::{Duration, Instant};

use crate::control::{self, NUDGE, Reply, Request};
use crate::harness;
use crate::mux::{self, Chrome, Key, Mux, Pane, PaneId, PaneView, Rect, Winsize, now_ms};
use crate::pty::Pty;
use crate::runtime::{Event, Runtime};
use crate::store::{self, Agent};
use crate::{config, limits, workspace};
use libghostty_vt::unicode;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// How long after typing the nudge the host presses Enter.
const NUDGE_ENTER_MS: i64 = 200;
/// How long a control connection has to send its request.
const REQUEST_TIMEOUT_MS: i64 = 5000;
/// The longest request the host reads.
const MAX_REQUEST: usize = 1 << 20;
/// How often the host reads the saved limits.
const LIMITS_INTERVAL_MS: i64 = 15_000;
/// A program that exits within this long of starting has exited quickly.
const QUICK_EXIT_MS: i64 = 10_000;
/// The host stops relaunching an agent after this many quick exits in a row.
const MAX_QUICK_EXITS: u32 = 3;
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
/// `delete` unlinks the lock file while holding the lock, so a lock obtained
/// on a file that is no longer at the path is released and the current file
/// locked instead.
pub fn try_lock(path: &Path) -> Result<Option<File>> {
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
        match file.try_lock() {
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

/// Where each pane goes while `zoomed` fills the pane area: where a lone pane
/// would go, with every other pane hidden at its current size. `None` if the
/// pane would get no row or column.
pub fn zoom_layout(size: Winsize, panes: &[PaneId], zoomed: PaneId) -> Option<Vec<(PaneId, Rect)>> {
    let rect = layout(size, &[zoomed])?.pop()?.1;
    Some(
        panes
            .iter()
            .map(|&id| (id, if id == zoomed { rect } else { Rect::default() }))
            .collect(),
    )
}

/// The labels above the panes, the rule between the first pane and the
/// others, and the status bar.
pub struct TeamChrome {
    /// The status bar's text on the left.
    pub status: String,
    /// The status bar's text on the right, which the host changes as the
    /// limits do.
    pub limits: Rc<RefCell<String>>,
    /// The pane that fills the pane area, which the host changes as it zooms.
    pub zoomed: Rc<Cell<Option<PaneId>>>,
}

impl Chrome for TeamChrome {
    fn draw(&mut self, out: &mut Vec<u8>, panes: &[PaneView<'_>], size: Winsize) {
        let mut s = String::new();
        // Panes are hidden while the terminal is too small for them.
        let shown: Vec<&PaneView> = panes.iter().filter(|p| p.rect.rows > 0).collect();
        for p in &shown {
            draw_label(&mut s, p, self.zoomed.get() == Some(p.id));
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
            let blank = " ".repeat(usize::from(size.ws_col));
            let left = fit(&format!(" {}", self.status), size.ws_col);
            let row = size.ws_row;
            s.push_str(&format!("\x1b[{row};1H{LABEL}{blank}\x1b[{row};1H{left}"));
            // The limits are left out when they would crowd the path.
            let limits = self.limits.borrow();
            if !limits.is_empty() {
                let right = format!("{limits} ");
                let (_, used) = clip(&right, usize::MAX);
                let room = usize::from(size.ws_col).saturating_sub(clip(&left, usize::MAX).1 + 1);
                if used <= room {
                    let col = usize::from(size.ws_col) - used + 1;
                    s.push_str(&format!("\x1b[{row};{col}H{right}"));
                }
            }
        }
        s.push_str("\x1b[0m");
        out.extend_from_slice(s.as_bytes());
    }
}

/// A pane's label row: a rule with the label near its left end, bold when the
/// pane has the focus, "zoomed" while it fills the pane area, the exit status
/// once its program has exited, and how many rows its view is scrolled up.
fn draw_label(s: &mut String, p: &PaneView, zoomed: bool) {
    let lead = p.rect.cols.min(2);
    let mut text = format!(" {}", p.label);
    if zoomed {
        text.push_str(" · zoomed");
    }
    if let Some(status) = p.status {
        text.push_str(&format!(" · exited {status}"));
    }
    if p.scrolled > 0 {
        text.push_str(&format!(" · ↑{}", p.scrolled));
    }
    text.push(' ');
    let text = fit(&text, p.rect.cols - lead);
    let bold = if p.focused { "\x1b[1m" } else { "" };
    // The label goes over the rule, which shows wherever the label is shorter
    // than measured.
    s.push_str(&format!(
        "\x1b[{row};{col}H{RULE}{}\x1b[{row};{}H{LABEL}{bold}{text}",
        "━".repeat(usize::from(p.rect.cols)),
        p.rect.col + lead + 1,
        row = p.rect.row,
        col = p.rect.col + 1,
    ));
}

/// The longest start of `text` made of whole grapheme clusters that fits in
/// `width` columns.
fn fit(text: &str, width: u16) -> String {
    clip(text, usize::from(width)).0
}

/// The longest start of `text` made of whole grapheme clusters that fits in
/// `width` columns, and the columns it takes. The host clusters graphemes or
/// not depending on the focused pane, so each cluster counts as the wider of
/// the two.
fn clip(text: &str, width: usize) -> (String, usize) {
    let chars: Vec<char> = text.chars().collect();
    let mut rest = chars.as_slice();
    let mut used = 0;
    let mut end = 0;
    while !rest.is_empty() {
        let (len, clustered) = unicode::grapheme_width(rest);
        let len = len.max(1);
        let separate: usize = rest[..len]
            .iter()
            .map(|&c| usize::from(unicode::codepoint_width(c)))
            .sum();
        let next = used + usize::from(clustered).max(separate);
        if next > width {
            break;
        }
        used = next;
        end += len;
        rest = &rest[len..];
    }
    (chars[..end].iter().collect(), used)
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
    /// Each agent's pane, in launch order.
    panes: Vec<Slot>,
    clients: Vec<Client>,
    enters: Enters,
    /// The status bar's limits text, shared with the chrome.
    limits: Rc<RefCell<String>>,
    /// The pane that fills the pane area, shared with the chrome.
    zoomed: Rc<Cell<Option<PaneId>>>,
    /// When to read the saved limits next, in `now_ms` time.
    limits_due: i64,
}

/// An agent's pane and how its program has been running.
struct Slot {
    agent: String,
    pane: PaneId,
    /// When the program started, in `now_ms` time.
    started: i64,
    /// How many times in a row the program has exited quickly.
    quick_exits: u32,
}

impl Slot {
    fn new(agent: &str, pane: PaneId) -> Slot {
        Slot {
            agent: agent.to_string(),
            pane,
            started: now_ms(),
            quick_exits: 0,
        }
    }
}

/// Nudged panes and when to press Enter in each. A pane has at most one
/// nudge pending.
#[derive(Default)]
struct Enters(Vec<(i64, PaneId)>);

impl Enters {
    /// Press Enter in `pane` `NUDGE_ENTER_MS` after `now`. Returns false,
    /// scheduling nothing, if the pane already has a nudge pending.
    fn schedule(&mut self, pane: PaneId, now: i64) -> bool {
        if self.0.iter().any(|&(_, p)| p == pane) {
            return false;
        }
        self.0.push((now + NUDGE_ENTER_MS, pane));
        true
    }

    /// When the next Enter is due.
    fn next(&self) -> Option<i64> {
        self.0.iter().map(|&(at, _)| at).min()
    }

    /// Drop the pane's pending Enter, so a program that replaces the one
    /// nudged doesn't get it.
    fn cancel(&mut self, pane: PaneId) {
        self.0.retain(|&(_, p)| p != pane);
    }

    /// Remove and return the panes whose Enter is due at `now`.
    fn take_due(&mut self, now: i64) -> Vec<PaneId> {
        let (due, later): (Vec<_>, Vec<_>) = self.0.iter().partition(|&&(at, _)| at <= now);
        self.0 = later;
        due.into_iter().map(|(_, pane)| pane).collect()
    }
}

impl Drop for Host {
    fn drop(&mut self) {
        self.end();
    }
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
    let limits = Rc::new(RefCell::new(String::new()));
    let zoomed = Rc::new(Cell::new(None));
    let chrome = TeamChrome {
        status: workspace::short(root),
        limits: Rc::clone(&limits),
        zoomed: Rc::clone(&zoomed),
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
        enters: Enters::default(),
        limits,
        zoomed,
        limits_due: 0,
    };
    host.refresh_limits();
    host.start(&agents).and_then(|()| host.serve())
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
            }
            a.activity = start_activity(a).into();
        }
        let mut agents = s.agents.clone();
        agents.sort_by_key(|a| a.role != store::ORCHESTRATOR);
        Ok(agents)
    })
}

/// The prompt an agent's harness starts with when the host launches it.
/// Workers start a turn to pick up messages queued while they were down.
fn start_prompt(agent: &Agent) -> &'static str {
    if agent.role == store::ORCHESTRATOR {
        ""
    } else {
        NUDGE
    }
}

/// What an agent is doing once the host has launched its harness.
fn start_activity(agent: &Agent) -> &'static str {
    if agent.role == store::ORCHESTRATOR {
        store::IDLE
    } else {
        store::BUSY
    }
}

/// Run a harness in a new pty of size `ws`.
fn spawn(launch: &harness::Launch, ws: Winsize) -> Result<Pty> {
    let env: Vec<_> = launch.env.iter().collect();
    Ok(Pty::spawn(&launch.argv, &launch.cwd, &env, ws)?)
}

impl Host {
    /// Launch every agent, resuming saved sessions. Workers start a turn to
    /// pick up messages queued while they were down.
    fn start(&mut self, agents: &[Agent]) -> Result<()> {
        for a in agents {
            if let Err(e) = self.launch(&a.id, start_prompt(a), true) {
                if a.role == store::ORCHESTRATOR {
                    return Err(format!("launching {}: {e}", a.name).into());
                }
                self.add_failed(a, &e.to_string())?;
            }
        }
        Ok(())
    }

    /// Give a worker whose harness failed to start a pane without a program,
    /// showing the error and exit status 127, so it keeps its place until
    /// `remove_agent` closes it.
    fn add_failed(&mut self, agent: &Agent, error: &str) -> Result<()> {
        let pane = self.mux.add_pane(&harness::label(agent))?;
        self.show_failure(pane, agent, error);
        self.panes.push(Slot::new(&agent.id, pane));
        self.lay_out()
    }

    /// Show in a pane without a program that its agent's harness failed to
    /// start, as exited with status 127.
    fn show_failure(&mut self, pane: PaneId, agent: &Agent, error: &str) {
        let error = error.replace(['\r', '\n'], " ");
        if let Some(p) = self.mux.pane_mut(pane) {
            p.label = format!("{} · failed: {error}", harness::label(agent));
            p.feed(format!("{} failed to launch: {error}\r\n", agent.name).as_bytes());
        }
        self.mux.mark_exited(pane, 127);
    }

    /// Run the session until it is stopped, every pane's program has exited
    /// for good, or the terminal closes.
    fn serve(&mut self) -> Result<()> {
        loop {
            let listener = self.socket.listener.as_raw_fd();
            let mut fds = vec![listener, self.signals];
            fds.extend(self.clients.iter().map(|c| c.stream.as_raw_fd()));
            let deadline = self
                .clients
                .iter()
                .map(|c| c.since + REQUEST_TIMEOUT_MS)
                .chain(self.enters.next())
                .chain(Some(self.limits_due))
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
                    Event::PaneExited(pane, _) => {
                        self.relaunch(pane)?;
                        if self.mux.panes().iter().all(|p| p.status().is_some()) {
                            return Ok(());
                        }
                    }
                    Event::Zoom => self.toggle_zoom()?,
                    Event::Focus => {}
                    Event::Resized => self.lay_out()?,
                    Event::InputClosed => return Ok(()),
                    Event::Deadline => {}
                }
            }
            // A change of focus ends the zoom.
            if self
                .zoomed
                .get()
                .is_some_and(|z| self.mux.focus() != Some(z))
            {
                self.unzoom()?;
            }
            let now = now_ms();
            for pane in self.enters.take_due(now) {
                self.mux.send_key(pane, Key::Enter)?;
            }
            if now >= self.limits_due {
                self.refresh_limits();
            }
            self.clients.retain(|c| now - c.since < REQUEST_TIMEOUT_MS);
        }
    }

    /// Read the saved limits, redraw the status bar if its text changed, and
    /// schedule the next read. The text counts down to each reset, so it
    /// changes between writes of the file.
    fn refresh_limits(&mut self) {
        let text = limits::load(&limits::path())
            .map(|l| limits::render(&l, limits::now()))
            .unwrap_or_default();
        if *self.limits.borrow() != text {
            *self.limits.borrow_mut() = text;
            self.mux.invalidate_chrome();
        }
        self.limits_due = now_ms() + LIMITS_INTERVAL_MS;
    }

    /// Hang up every pane's program, give them `HANGUP_GRACE_MS` to exit
    /// before they are killed, and restore the host terminal. Dropping the
    /// host does this, including when a panic unwinds it.
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
            Request::Launch { agent, prompt } => self.launch(&agent, &prompt, false),
            Request::Close { agent } => self.close(&agent),
            Request::Nudge { agent } => self.nudge(&agent),
            Request::Stop => Ok(()),
        };
        result.map_or_else(Reply::error, |()| Reply::ok())
    }

    fn pane_of(&self, agent: &str) -> Option<PaneId> {
        self.panes.iter().find(|s| s.agent == agent).map(|s| s.pane)
    }

    fn pane_ids(&self) -> Vec<PaneId> {
        self.panes.iter().map(|s| s.pane).collect()
    }

    /// Start an agent's harness in a new pane and lay the panes out again. A
    /// non-empty `prompt` starts its first turn. If there is no room for the
    /// pane, a launch at startup hides the panes until the terminal grows,
    /// and any other launch fails, adding no pane.
    fn launch(&mut self, id: &str, prompt: &str, startup: bool) -> Result<()> {
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
        let layout = layout(size, &ids).or_else(|| startup.then(|| hidden(&ids)));
        let Some(layout) = layout else {
            self.mux.remove_pane(pane);
            return Err(format!(
                "no room for another pane in a {}x{} terminal",
                size.ws_col, size.ws_row
            )
            .into());
        };
        let rect = layout[layout.len() - 1].1;
        let ws = Winsize {
            ws_col: rect.cols.max(1),
            ws_row: rect.rows.max(1),
            ..Winsize::default()
        };
        match spawn(&launch, ws) {
            Ok(pty) => self.mux.attach(pane, pty),
            Err(e) => {
                self.mux.remove_pane(pane);
                return Err(e);
            }
        }
        self.panes.push(Slot::new(&agent.id, pane));
        self.zoomed.set(None);
        self.mux.set_layout(&layout)?;
        Ok(())
    }

    /// Start the harness of an agent whose program exited again in its pane,
    /// with its saved session, as when the host launches it. An agent whose
    /// program has exited quickly `MAX_QUICK_EXITS` times in a row is left
    /// exited. If the harness fails to start, the pane shows why, and the
    /// host doesn't try again.
    fn relaunch(&mut self, pane: PaneId) -> Result<()> {
        self.enters.cancel(pane);
        let Some(slot) = self.panes.iter_mut().find(|s| s.pane == pane) else {
            return Ok(());
        };
        let now = now_ms();
        slot.quick_exits = if now - slot.started < QUICK_EXIT_MS {
            slot.quick_exits + 1
        } else {
            0
        };
        if slot.quick_exits >= MAX_QUICK_EXITS {
            return Ok(());
        }
        slot.started = now;
        let id = slot.agent.clone();
        let state = store::load(&self.state_path)?;
        let agent = state.agent(&id)?;
        self.mux.restart(pane, &harness::label(agent));
        let ws = self.mux.pane(pane).map(Pane::ws).unwrap_or_default();
        let started = harness::command(&self.exe, agent, &self.state_path, start_prompt(agent))
            .and_then(|launch| spawn(&launch, ws));
        match started {
            Ok(pty) => {
                self.mux.attach(pane, pty);
                store::update(&self.state_path, |s| {
                    s.agent_mut(&id)?.activity = start_activity(agent).into();
                    Ok(())
                })?;
            }
            Err(e) => self.show_failure(pane, agent, &e.to_string()),
        }
        Ok(())
    }

    /// Zoom the focused pane, or end the zoom.
    fn toggle_zoom(&mut self) -> Result<()> {
        if self.zoomed.get().is_some() {
            return self.unzoom();
        }
        self.zoomed.set(self.mux.focus());
        self.lay_out()
    }

    /// Lay the panes out normally if one is zoomed.
    fn unzoom(&mut self) -> Result<()> {
        if self.zoomed.take().is_none() {
            return Ok(());
        }
        self.lay_out()
    }

    /// Hang up an agent's program, remove its pane, and lay the panes out
    /// again. An agent without a pane is already closed.
    fn close(&mut self, id: &str) -> Result<()> {
        let Some(i) = self.panes.iter().position(|s| s.agent == id) else {
            return Ok(());
        };
        let slot = self.panes.remove(i);
        self.enters.cancel(slot.pane);
        self.mux.remove_pane(slot.pane);
        self.zoomed.set(None);
        self.lay_out()
    }

    /// Type the nudge into an agent's pane, and press Enter `NUDGE_ENTER_MS`
    /// later. A nudge still pending covers this one.
    fn nudge(&mut self, id: &str) -> Result<()> {
        let pane = self
            .pane_of(id)
            .ok_or_else(|| format!("agent {id} has no pane"))?;
        let p = self.mux.pane(pane).ok_or("no pane")?;
        if p.pid().is_none() {
            return Err(format!("agent {id} failed to launch").into());
        }
        if let Some(status) = p.status() {
            return Err(format!("agent {id} exited with status {status}").into());
        }
        if self.enters.schedule(pane, now_ms()) {
            self.mux.send_text(pane, NUDGE)?;
        }
        Ok(())
    }

    /// Lay the panes out for the terminal's size. In a terminal too small
    /// for them, the panes are hidden until it grows.
    fn lay_out(&mut self) -> Result<()> {
        let ids = self.pane_ids();
        let size = self.mux.size();
        let layout = match self.zoomed.get() {
            Some(zoomed) => zoom_layout(size, &ids, zoomed),
            None => layout(size, &ids),
        };
        Ok(self
            .mux
            .set_layout(&layout.unwrap_or_else(|| hidden(&ids)))?)
    }
}

/// A layout that hides every pane.
fn hidden(ids: &[PaneId]) -> Vec<(PaneId, Rect)> {
    ids.iter().map(|&id| (id, Rect::default())).collect()
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

    #[test]
    fn a_zoomed_pane_takes_the_pane_area_and_the_others_are_hidden() {
        let panes = ids(3);
        for zoomed in &panes {
            let layout = zoom_layout(size(100, 25), &panes, *zoomed).unwrap();
            let ids: Vec<PaneId> = layout.iter().map(|&(id, _)| id).collect();
            assert_eq!(ids, panes);
            for (id, r) in layout {
                let expected = if id == *zoomed {
                    rect(0, 1, 100, 23)
                } else {
                    Rect::default()
                };
                assert_eq!(r, expected);
            }
        }
        assert!(zoom_layout(size(80, 2), &ids(2), PaneId(2)).is_none());
        assert_eq!(zoom_layout(size(80, 24), &[], PaneId(1)), Some(Vec::new()));
    }

    struct Screen(Terminal<'static, 'static>);

    impl Screen {
        fn draw(panes: &[PaneView], cols: u16, rows: u16) -> Screen {
            let mut chrome = TeamChrome {
                status: "~/src/team".into(),
                limits: Rc::default(),
                zoomed: Rc::default(),
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
            scrolled: 0,
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
            PaneView {
                scrolled: 42,
                ..view(2, "w1", layout[1], false, None)
            },
            view(3, "w2", layout[2], false, Some(3)),
        ];
        let screen = Screen::draw(&panes, 30, 8);
        let rows: Vec<String> = (0..8).map(|y| screen.row(y)).collect();
        assert_eq!(
            rows,
            [
                "━━ orch ━━━━┳━━ w1 · ↑42 ━━━━━",
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
    fn chrome_marks_the_zoomed_pane_and_draws_no_rule() {
        let layout = rects(zoom_layout(size(30, 6), &ids(2), PaneId(2)));
        let panes = [
            view(1, "orch", layout[0], false, None),
            view(2, "w1", layout[1], true, Some(3)),
        ];
        let mut chrome = TeamChrome {
            status: "~/src/team".into(),
            limits: Rc::default(),
            zoomed: Rc::new(Cell::new(Some(PaneId(2)))),
        };
        let mut out = Vec::new();
        chrome.draw(&mut out, &panes, size(30, 6));
        let mut terminal = Terminal::new(30, 6).unwrap();
        terminal.vt_write(&out);
        let screen = Screen(terminal);
        assert_eq!(screen.row(0), "━━ w1 · zoomed · exited 3 ━━━━");
        assert_eq!(screen.row(1), " ".repeat(30));
        assert_eq!(screen.row(5), " ~/src/team".to_owned() + &" ".repeat(19));
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
    fn chrome_measures_wide_and_combining_characters_in_columns() {
        let panes = [view(1, "界界界界界", rect(0, 1, 9, 2), false, None)];
        let mut chrome = TeamChrome {
            status: "e\u{301}界界界界界".into(),
            limits: Rc::default(),
            zoomed: Rc::default(),
        };
        let mut out = Vec::new();
        chrome.draw(&mut out, &panes, size(9, 4));
        let mut terminal = Terminal::new(9, 4).unwrap();
        terminal.vt_write(&out);
        let screen = Screen(terminal);
        // Nothing wraps, so nothing scrolls, and spacer cells read as blanks.
        assert_eq!(screen.row(0), "━━ 界 界 界 ");
        assert_eq!(screen.row(3), " e界 界 界  ");
        assert_eq!(screen.bg(8, 3), LIGHT_BLUE);
        assert_eq!(fit("e\u{301}x", 1), "e\u{301}");
        assert_eq!(fit("界", 1), "");
    }

    fn status_bar(status: &str, limits: &str, cols: u16) -> String {
        let mut chrome = TeamChrome {
            status: status.into(),
            limits: Rc::new(RefCell::new(limits.into())),
            zoomed: Rc::default(),
        };
        let mut out = Vec::new();
        chrome.draw(&mut out, &[], size(cols, 2));
        let mut terminal = Terminal::new(cols, 2).unwrap();
        terminal.vt_write(&out);
        let screen = Screen(terminal);
        assert_eq!(screen.bg(0, 1), LIGHT_BLUE);
        assert_eq!(screen.bg(cols - 1, 1), LIGHT_BLUE);
        screen.row(1)
    }

    #[test]
    fn the_status_bar_shows_limits_on_the_right() {
        let limits = "claude 5h 22% ↻2h5m";
        assert_eq!(
            status_bar("~/src/team", limits, 40),
            " ~/src/team         claude 5h 22% ↻2h5m "
        );
        assert_eq!(
            status_bar("~/src/team", "", 40),
            format!(" ~/src/team{:29}", "")
        );
        // The limits are left out when they would touch the path.
        assert_eq!(
            status_bar("~/src/team", limits, 31),
            format!(" ~/src/team{:20}", "")
        );
        assert_eq!(
            status_bar("~/src/team", limits, 32),
            " ~/src/team claude 5h 22% ↻2h5m "
        );
        assert_eq!(status_bar("~/src/team", limits, 6), " ~/src");
    }

    #[test]
    fn the_status_bar_measures_wide_limits_in_columns() {
        assert_eq!(status_bar("a", "界界", 12), " a     界 界  ");
        assert_eq!(status_bar("a", "界界", 8), " a 界 界  ");
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

        // A child that another test forks at the moment shares the lock, or
        // the shared lock `running` takes, until it execs, so retry until the
        // lock is free.
        drop(lock);
        fn eventually(mut f: impl FnMut() -> bool) {
            for _ in 0..200 {
                if f() {
                    return;
                }
                std::thread::sleep(Duration::from_millis(10));
            }
            panic!("the host lock stayed held");
        }
        eventually(|| try_lock(&lock_path(&state)).unwrap().is_some());
        eventually(|| running(&state).unwrap().is_none());
    }

    #[test]
    fn a_lock_on_an_unlinked_file_is_not_the_host_lock() {
        let dir = TempDir::new();
        let path = dir.path().join("w.host.lock");
        let lock = try_lock(&path).unwrap().unwrap();
        assert!(at_path(&lock, &path).unwrap());
        // A host that opened the file before delete unlinked it would lock
        // the old file.
        let old = File::open(&path).unwrap();
        fs::remove_file(&path).unwrap();
        assert!(!at_path(&old, &path).unwrap());
        let new = try_lock(&path).unwrap().unwrap();
        assert!(!at_path(&old, &path).unwrap());
        assert!(at_path(&new, &path).unwrap());
        drop(lock);
    }

    #[test]
    fn a_pending_nudge_covers_the_next_one_for_its_pane() {
        let mut enters = Enters::default();
        assert_eq!(enters.next(), None);
        assert!(enters.schedule(PaneId(1), 1000));
        assert!(!enters.schedule(PaneId(1), 1100));
        assert!(enters.schedule(PaneId(2), 1100));
        assert_eq!(enters.next(), Some(1000 + NUDGE_ENTER_MS));
        assert_eq!(enters.take_due(1000 + NUDGE_ENTER_MS - 1), []);
        assert_eq!(enters.take_due(1000 + NUDGE_ENTER_MS), [PaneId(1)]);
        assert_eq!(enters.next(), Some(1100 + NUDGE_ENTER_MS));
        // Once its Enter is pressed, a pane takes a new nudge.
        assert!(enters.schedule(PaneId(1), 1250));
        assert_eq!(enters.take_due(2000), [PaneId(2), PaneId(1)]);
        assert_eq!(enters.next(), None);
    }

    #[test]
    fn a_cancelled_enter_is_not_pressed_and_the_pane_takes_a_new_nudge() {
        let mut enters = Enters::default();
        assert!(enters.schedule(PaneId(1), 1000));
        assert!(enters.schedule(PaneId(2), 1000));
        enters.cancel(PaneId(1));
        assert!(enters.schedule(PaneId(1), 1100));
        enters.cancel(PaneId(2));
        assert_eq!(enters.take_due(5000), [PaneId(1)]);
        enters.cancel(PaneId(3));
        assert_eq!(enters.next(), None);
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
