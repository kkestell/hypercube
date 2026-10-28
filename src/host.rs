//! The session host: `hypercube host`, in the background. It holds the
//! workspace's host lock, runs every agent in a pane, draws the panes with
//! their labels for the attached client, and answers requests on the control
//! socket until the session ends.

use std::cell::{Cell, RefCell};
use std::error::Error;
use std::fs::{self, File, OpenOptions, TryLockError};
use std::io::{ErrorKind, Read, Write};
use std::os::fd::{AsRawFd, FromRawFd, OwnedFd, RawFd};
use std::os::unix::fs::MetadataExt;
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::{Path, PathBuf};
use std::rc::Rc;
use std::sync::atomic::{AtomicI32, Ordering};
use std::sync::mpsc;
use std::time::{Duration, Instant};

use crate::bar::{self, Bar, Entry, Typed};
use crate::colors::{self, ACCENTS, Palette};
use crate::control::{self, NUDGE, Reply, Request};
use crate::harness;
use crate::help;
use crate::link::{self, Link, Message};
use crate::mux::{self, Chrome, Mux, Pane, PaneId, PaneView, Rect, Winsize, now_ms};
use crate::overlay::{Action, Overlay};
use crate::pty::Pty;
use crate::runtime::{Event, Runtime};
use crate::store::{self, Agent};
use crate::{config, workspace};
use libghostty_vt::unicode;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// How long a control connection has to send its request.
const REQUEST_TIMEOUT_MS: i64 = 5000;
/// The longest request the host reads.
const MAX_REQUEST: usize = 1 << 20;
/// A program that exits within this long of starting has exited quickly.
const QUICK_EXIT_MS: i64 = 10_000;
/// The host stops relaunching an agent after this many quick exits in a row.
const MAX_QUICK_EXITS: u32 = 3;
/// How long `stop` waits for the host to end.
const STOP_TIMEOUT: Duration = Duration::from_secs(10);
/// How often the overlay's list is read again while it is open.
const OVERLAY_REFRESH_MS: i64 = 1000;
/// How often the bar's list is read again while a client is attached.
const BAR_REFRESH_MS: i64 = 1000;

/// The host lock of the workspace whose state file is `state_path`. The host
/// holds it for its whole life, and writes its process ID into it.
pub fn lock_path(state_path: &Path) -> PathBuf {
    state_path.with_extension("host.lock")
}

/// The log of the workspace's hosts: their output, appended.
pub fn log_path(state_path: &Path) -> PathBuf {
    state_path.with_extension("log")
}

/// Whether a host is running for the workspace.
pub fn running(state_path: &Path) -> Result<bool> {
    let path = lock_path(state_path);
    let file = match File::open(&path) {
        Ok(file) => file,
        Err(e) if e.kind() == ErrorKind::NotFound => return Ok(false),
        Err(e) => return Err(format!("open {}: {e}", path.display()).into()),
    };
    match file.try_lock_shared() {
        Ok(()) => Ok(false),
        Err(TryLockError::WouldBlock) => Ok(true),
        Err(TryLockError::Error(e)) => Err(format!("lock {}: {e}", path.display()).into()),
    }
}

/// Ask the workspace's host to end the session, and wait until it has.
pub fn stop(state_path: &Path) -> Result<()> {
    control::request(&control::socket_path(state_path)?, &Request::Stop)?.into_result()?;
    let start = Instant::now();
    while running(state_path)? {
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

/// The area the panes share: below the header and above the bar.
fn pane_area(size: Winsize) -> Rect {
    Rect {
        col: 0,
        row: 1,
        cols: size.ws_col,
        rows: size.ws_row.saturating_sub(2),
    }
}

/// The column between the orchestrator and the workers, for the divider.
const DIVIDER: u32 = 1;

/// Where each pane goes, below the header and above the bar in the last row.
/// The first pane, the orchestrator's, takes the left 40% of the area, or all of
/// it when it is alone, and has no label. The other panes split the remaining
/// columns, after the divider's, with equal heights. Each has a label row above
/// it. `None` if a pane would get no row or column.
pub fn layout(size: Winsize, panes: &[PaneId]) -> Option<Vec<(PaneId, Rect)>> {
    let area = pane_area(size);
    let (cols, rows) = (u32::from(area.cols), u32::from(area.rows));
    let Some((&first, rest)) = panes.split_first() else {
        return Some(Vec::new());
    };
    let left = if rest.is_empty() { cols } else { cols * 2 / 5 };
    let n = rest.len() as u32;
    let mut layout = vec![(
        first,
        Rect {
            cols: left as u16,
            ..area
        },
    )];
    for (i, &id) in (0..).zip(rest) {
        let top = rows * i / n;
        let bottom = rows * (i + 1) / n;
        let rect = Rect {
            col: area.col + (left + DIVIDER) as u16,
            row: area.row + top as u16 + 1,
            cols: cols.saturating_sub(left + DIVIDER) as u16,
            rows: bottom.saturating_sub(top + 1) as u16,
        };
        layout.push((id, rect));
    }
    layout
        .iter()
        .all(|(_, r)| r.cols > 0 && r.rows > 0)
        .then_some(layout)
}

/// Where each pane goes while `zoomed` fills the pane area, with its label row
/// unless it is the first pane: every other pane is hidden at its current size.
/// `None` if the pane would get no row or column.
pub fn zoom_layout(size: Winsize, panes: &[PaneId], zoomed: PaneId) -> Option<Vec<(PaneId, Rect)>> {
    let area = pane_area(size);
    let rect = if panes.first() == Some(&zoomed) {
        area
    } else {
        Rect {
            row: area.row + 1,
            rows: area.rows.saturating_sub(1),
            ..area
        }
    };
    (rect.cols > 0 && rect.rows > 0).then(|| {
        panes
            .iter()
            .map(|&id| (id, if id == zoomed { rect } else { Rect::default() }))
            .collect()
    })
}

/// The header, the divider between the panes, the labels
/// above the workers, and the bar, or the workspace overlay while it is open.
pub struct TeamChrome {
    /// The pane that fills the pane area, which the host changes as it zooms.
    pub zoomed: Rc<Cell<Option<PaneId>>>,
    pub overlay: Rc<RefCell<Option<Overlay>>>,
    /// Whether the list of keys is open.
    pub help: Rc<Cell<bool>>,
    pub bar: Rc<RefCell<Bar>>,
    /// The workspace's colors.
    pub palette: Rc<Cell<Palette>>,
}

impl Chrome for TeamChrome {
    fn draw(&mut self, out: &mut Vec<u8>, panes: &[PaneView<'_>], size: Winsize) {
        let palette = self.palette.get();
        if let Some(overlay) = &*self.overlay.borrow() {
            overlay.draw(out, size, palette);
            return;
        }
        if self.help.get() {
            help::draw(out, size, palette);
            return;
        }
        let mut s = String::new();
        // Panes are hidden while the terminal is too small for them.
        let shown: Vec<&PaneView> = panes.iter().filter(|p| p.rect.rows > 0).collect();
        // The first pane is the orchestrator's: the header names its
        // workspace, and it has no label.
        let orchestrator = panes.first().map(|p| p.id);
        if let Some(p) = panes.first() {
            let notes = notes(p, self.zoomed.get() == Some(p.id));
            let right = if notes.is_empty() {
                String::new()
            } else {
                format!("{} ", notes.join(" · "))
            };
            let pos = (1, 1, size.ws_col);
            draw_row(
                &mut s,
                &RowStyle::bar(palette.bar),
                pos,
                false,
                &format!(" {}", p.detail),
                &right,
            );
        }
        if let [first, _, ..] = shown.as_slice() {
            let col = u32::from(first.rect.col + first.rect.cols);
            s.push_str(&colors::fg(palette.line));
            for row in 1..u32::from(size.ws_row).saturating_sub(1) {
                s.push_str(&format!("{}┃", at(row, col)));
            }
        }
        for p in &shown {
            let labeled = Some(p.id) != orchestrator;
            if labeled {
                draw_label(&mut s, p, self.zoomed.get() == Some(p.id), palette);
                // The divider, left of the pane, branches into the title's
                // line, unless the pane is zoomed and the divider is not drawn.
                if let Some(col) = p.rect.col.checked_sub(1) {
                    s.push_str(&format!(
                        "{}{}┣",
                        colors::fg(palette.line),
                        at(u32::from(p.rect.row) - 1, u32::from(col))
                    ));
                }
            }
        }
        self.bar.borrow().draw(&mut s, size, palette);
        s.push_str("\x1b[0m");
        out.extend_from_slice(s.as_bytes());
    }
}

/// Move the cursor to `row` and `col`, both from 0.
fn at(row: u32, col: u32) -> String {
    format!("\x1b[{};{}H", row + 1, col + 1)
}

/// What to note about a pane after its name: "zoomed" while it fills the pane
/// area, the exit status once its program has exited, and how many rows its
/// view is scrolled up.
fn notes(p: &PaneView, zoomed: bool) -> Vec<String> {
    let mut notes = Vec::new();
    if zoomed {
        notes.push("zoomed".to_string());
    }
    if let Some(status) = p.status {
        notes.push(format!("exited {status}"));
    }
    if p.scrolled > 0 {
        notes.push(format!("↑{}", p.scrolled));
    }
    notes
}

/// A worker's label row above its pane, across the pane's width, with the name
/// at the left, bold when the pane has the focus, and its notes. The detail
/// goes at the right end unless the row is too narrow for it. The row is a
/// heavy line with a space on each side of its text, and a line's width to
/// spare at each end.
fn draw_label(s: &mut String, p: &PaneView, zoomed: bool, palette: Palette) {
    let mut left = format!(" {}", p.label);
    for note in notes(p, zoomed) {
        left.push_str(&format!(" · {note}"));
    }
    left.push(' ');
    let style = RowStyle {
        fill_sgr: colors::fg(palette.line),
        fill: "━",
        text_sgr: colors::fg(palette.text),
        inset: 1,
    };
    let right = format!(" {} ", p.detail);
    let right = if p.detail.is_empty() {
        String::new()
    } else {
        right
    };
    // The rect is the pane's cells, so the label row is the one above it.
    let pos = (
        u32::from(p.rect.row),
        u32::from(p.rect.col) + 1,
        p.rect.cols,
    );
    draw_row(s, &style, pos, p.focused, &left, &right);
}

/// How a row of text is drawn: filled with `fill` in `fill_sgr`, and the text
/// in `text_sgr`, `inset` columns in from each end.
struct RowStyle {
    fill_sgr: String,
    fill: &'static str,
    text_sgr: String,
    inset: u16,
}

impl RowStyle {
    /// A solid bar in `colors`.
    fn bar(colors: colors::Colors) -> RowStyle {
        RowStyle {
            fill_sgr: colors.sgr(),
            fill: " ",
            text_sgr: colors.sgr(),
            inset: 0,
        }
    }
}

/// A row of `width` columns at (`row`, `col`), from 1, in `style`: `left` at
/// the start, cut to fit, and `right` at the end unless the row is too narrow,
/// keeping at least one blank column between them. Only the text is bold.
fn draw_row(
    s: &mut String,
    style: &RowStyle,
    (row, col, width): (u32, u32, u16),
    bold: bool,
    left: &str,
    right: &str,
) {
    let room = width.saturating_sub(2 * style.inset);
    let left = fit(left, room);
    let (left_width, right_width) = (text_width(&left), text_width(right));
    let right =
        (!right.is_empty() && left_width + 1 + right_width <= usize::from(room)).then_some(right);
    let bold = if bold { "\x1b[1m" } else { "" };
    let text = &style.text_sgr;
    let start = col + u32::from(style.inset);
    s.push_str(&format!(
        "\x1b[{row};{col}H{}{}\x1b[{row};{start}H{text}{bold}{left}",
        style.fill_sgr,
        style.fill.repeat(usize::from(width)),
    ));
    if let Some(right) = right {
        s.push_str(&format!(
            "\x1b[{row};{}H{text}{bold}{right}",
            (col + u32::from(width)) as usize - usize::from(style.inset) - right_width
        ));
    }
}

/// The columns `text` takes, counting each grapheme cluster as the wider of
/// how the host terminal may draw it.
pub(crate) fn text_width(text: &str) -> usize {
    let chars: Vec<char> = text.chars().collect();
    let mut rest = chars.as_slice();
    let mut used = 0;
    while !rest.is_empty() {
        let (len, width) = cluster_width(rest);
        used += width;
        rest = &rest[len..];
    }
    used
}

/// The length in chars and the width in columns of the grapheme cluster at the
/// start of `rest`. The host clusters graphemes or not depending on the
/// focused pane, so the cluster counts as the wider of the two.
fn cluster_width(rest: &[char]) -> (usize, usize) {
    let (len, clustered) = unicode::grapheme_width(rest);
    let len = len.max(1);
    let separate: usize = rest[..len]
        .iter()
        .map(|&c| usize::from(unicode::codepoint_width(c)))
        .sum();
    (len, usize::from(clustered).max(separate))
}

/// The right end of an agent's label: "⎇ " and the branch of a worker with a
/// worktree, without the `hypercube/` prefix, or else the directory.
fn detail(agent: &Agent) -> String {
    if agent.role == store::WORKER && !agent.branch.is_empty() {
        let branch = agent
            .branch
            .strip_prefix("hypercube/")
            .unwrap_or(&agent.branch);
        format!("⎇ {branch}")
    } else {
        workspace::short(Path::new(&agent.dir))
    }
}

/// The longest start of `text` made of whole grapheme clusters that fits in
/// `width` columns.
pub(crate) fn fit(text: &str, width: u16) -> String {
    let width = usize::from(width);
    let chars: Vec<char> = text.chars().collect();
    let mut rest = chars.as_slice();
    let mut used = 0;
    let mut end = 0;
    while !rest.is_empty() {
        let (len, columns) = cluster_width(rest);
        if used + columns > width {
            break;
        }
        used += columns;
        end += len;
        rest = &rest[len..];
    }
    chars[..end].iter().collect()
}

static SIGNAL_PIPE: AtomicI32 = AtomicI32::new(-1);

extern "C" fn on_signal(signal: libc::c_int) {
    if signal == libc::SIGHUP {
        return;
    }
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

/// Catch SIGINT and SIGTERM, and return a pipe that becomes readable when one
/// arrives. SIGHUP is caught and ignored. Programs started later run with the
/// default handlers.
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

/// A pipe, read end first, close-on-exec, with a non-blocking read end.
fn pipe() -> Result<(OwnedFd, OwnedFd)> {
    let mut fds = [0; 2];
    if unsafe { libc::pipe(fds.as_mut_ptr()) } != 0 {
        return Err(format!("pipe: {}", std::io::Error::last_os_error()).into());
    }
    for fd in fds {
        unsafe { libc::fcntl(fd, libc::F_SETFD, libc::FD_CLOEXEC) };
    }
    unsafe { libc::fcntl(fds[0], libc::F_SETFL, libc::O_NONBLOCK) };
    Ok(unsafe { (OwnedFd::from_raw_fd(fds[0]), OwnedFd::from_raw_fd(fds[1])) })
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

/// The name of a workspace another was asked to stop, and how it went.
type StopResult = (String, std::result::Result<(), String>);

struct Host {
    exe: PathBuf,
    root: PathBuf,
    state_path: PathBuf,
    mux: Mux,
    runtime: Runtime,
    socket: Socket,
    signals: RawFd,
    /// The agents to launch on the first attach, until then.
    unstarted: Option<Vec<Agent>>,
    /// Each agent's pane, in launch order.
    panes: Vec<Slot>,
    clients: Vec<Client>,
    /// The pane that fills the pane area, shared with the chrome.
    zoomed: Rc<Cell<Option<PaneId>>>,
    /// The workspace overlay while it is open, shared with the chrome.
    overlay: Rc<RefCell<Option<Overlay>>>,
    /// When the overlay's list was last read.
    overlay_read: i64,
    /// Whether the list of keys is open, shared with the chrome.
    help: Rc<Cell<bool>>,
    /// The bar, shared with the chrome.
    bar: Rc<RefCell<Bar>>,
    /// When the bar's list was last read.
    bar_read: i64,
    /// The workspace's colors, shared with the chrome.
    palette: Rc<Cell<Palette>>,
    /// Stops of other workspaces, which run on threads so the host keeps
    /// serving: a pipe that becomes readable when one finishes, and their
    /// results.
    stopped: (OwnedFd, OwnedFd),
    stop_results: (mpsc::Sender<StopResult>, mpsc::Receiver<StopResult>),
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

impl Drop for Host {
    fn drop(&mut self) {
        self.end("stopped");
    }
}

/// Host the workspace whose checkout is `root` until the session ends: when
/// it is stopped or deleted, or gets SIGINT or SIGTERM. The agents launch
/// when a client first attaches.
pub fn serve_workspace(root: &Path) -> Result<()> {
    let state_path = store::path(&workspace::name(root));
    let Some(mut lock) = try_lock(&lock_path(&state_path))? else {
        return Err(format!(
            "hypercube is already running for {}",
            workspace::short(root)
        )
        .into());
    };
    lock.set_len(0)?;
    lock.write_all(std::process::id().to_string().as_bytes())?;

    let agents = prepare(root, &state_path)?;
    let socket = Socket::listen(&state_path)?;
    let signals = catch_signals()?;
    let exe = std::env::current_exe().map_err(|e| format!("finding hypercube: {e}"))?;
    let zoomed = Rc::new(Cell::new(None));
    let overlay = Rc::new(RefCell::new(None));
    let help = Rc::new(Cell::new(false));
    let bar = Rc::new(RefCell::new(Bar::new(state_path.clone())));
    let palette = Rc::new(Cell::new(Palette::of(
        store::load(&state_path)?.accent as usize,
    )));
    let chrome = TeamChrome {
        zoomed: Rc::clone(&zoomed),
        overlay: Rc::clone(&overlay),
        help: Rc::clone(&help),
        bar: Rc::clone(&bar),
        palette: Rc::clone(&palette),
    };
    let mut host = Host {
        exe,
        root: root.to_path_buf(),
        state_path,
        mux: Mux::new(Winsize::default(), Box::new(chrome)),
        runtime: Runtime::new(),
        socket,
        signals,
        unstarted: Some(agents),
        panes: Vec::new(),
        clients: Vec::new(),
        zoomed,
        overlay,
        overlay_read: 0,
        help,
        bar,
        bar_read: 0,
        palette,
        stopped: pipe()?,
        stop_results: mpsc::channel(),
    };
    let result = host.serve();
    if let Err(e) = &result {
        host.end(&e.to_string());
    }
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

    /// Add a pane labeled for `agent`: its name at the left, and at the right
    /// its branch if it has a worktree, or its directory.
    fn add_labeled_pane(&mut self, agent: &Agent) -> Result<PaneId> {
        let pane = self.mux.add_pane(&agent.name)?;
        if let Some(p) = self.mux.pane_mut(pane) {
            p.detail = detail(agent);
        }
        Ok(pane)
    }

    /// Give a worker whose harness failed to start a pane without a program,
    /// showing the error and exit status 127, so it keeps its place until
    /// `remove_agent` closes it.
    fn add_failed(&mut self, agent: &Agent, error: &str) -> Result<()> {
        let pane = self.add_labeled_pane(agent)?;
        self.show_failure(pane, agent, error);
        self.panes.push(Slot::new(&agent.id, pane));
        self.lay_out()
    }

    /// Show in a pane without a program that its agent's harness failed to
    /// start, as exited with status 127.
    fn show_failure(&mut self, pane: PaneId, agent: &Agent, error: &str) {
        let error = error.replace(['\r', '\n'], " ");
        if let Some(p) = self.mux.pane_mut(pane) {
            p.label = format!("{} · failed: {error}", agent.name);
            p.feed(format!("{} failed to launch: {error}\r\n", agent.name).as_bytes());
        }
        self.mux.mark_exited(pane, 127);
    }

    /// Run the session until it is stopped or gets a signal.
    fn serve(&mut self) -> Result<()> {
        loop {
            let listener = self.socket.listener.as_raw_fd();
            let stopped = self.stopped.0.as_raw_fd();
            let mut fds = vec![listener, self.signals, stopped];
            fds.extend(self.clients.iter().map(|c| c.stream.as_raw_fd()));
            let mut deadline = self
                .clients
                .iter()
                .map(|c| c.since + REQUEST_TIMEOUT_MS)
                .min();
            if self.overlay.borrow().is_some() {
                let refresh = self.overlay_read + OVERLAY_REFRESH_MS;
                deadline = Some(deadline.map_or(refresh, |d| d.min(refresh)));
            }
            if self.runtime.connected() {
                let refresh = self.bar_read + BAR_REFRESH_MS;
                deadline = Some(deadline.map_or(refresh, |d| d.min(refresh)));
            }
            for event in self.runtime.wait(&mut self.mux, &fds, deadline)? {
                match event {
                    Event::Ready(fd) if fd == listener => self.accept(),
                    Event::Ready(fd) if fd == self.signals => return Ok(()),
                    Event::Ready(fd) if fd == stopped => self.finish_stops(),
                    Event::Ready(fd) => {
                        if self.read_client(fd)? {
                            return Ok(());
                        }
                    }
                    Event::PaneExited(pane, _) => self.relaunch(pane)?,
                    Event::Zoom(pane) => self.toggle_zoom(pane)?,
                    Event::Focus(pane) => {
                        // A change of focus ends the zoom.
                        if self.zoomed.get().is_some_and(|z| z != pane) {
                            self.unzoom()?;
                        }
                    }
                    Event::Resized => self.lay_out()?,
                    Event::Overlay => self.toggle_overlay()?,
                    Event::Key(_) if self.help.get() => self.toggle_help()?,
                    Event::Key(key) if self.bar.borrow().prompt.is_some() => {
                        self.prompt_key(&key);
                    }
                    Event::Key(key) => {
                        if self.overlay_key(&key)? {
                            return Ok(());
                        }
                    }
                    Event::Workspace(n) => self.switch_to(usize::from(n))?,
                    Event::Step(n) => self.step(n)?,
                    Event::NewWorkspace => self.open_prompt()?,
                    Event::Help => self.toggle_help()?,
                    Event::Detach => self.detach()?,
                    Event::Accent => self.next_accent(),
                    Event::Message(Message::SwitchFailed(error)) => {
                        match &mut *self.overlay.borrow_mut() {
                            Some(overlay) => overlay.error = error,
                            None => self.bar.borrow_mut().fail(error),
                        }
                        self.mux.invalidate_chrome();
                    }
                    Event::Message(_) => {}
                    Event::Disconnected => self.close_screens()?,
                    Event::Deadline => {}
                }
            }
            if self.overlay.borrow().is_some() && now_ms() - self.overlay_read >= OVERLAY_REFRESH_MS
            {
                self.read_overlay();
            }
            if self.runtime.connected() && now_ms() - self.bar_read >= BAR_REFRESH_MS {
                self.read_bar();
            }
            let now = now_ms();
            self.clients.retain(|c| now - c.since < REQUEST_TIMEOUT_MS);
        }
    }

    /// Tell the client the session ended for `reason`, hang up every pane's
    /// program, give them `HANGUP_GRACE_MS` to exit before they are killed,
    /// and give the client time to take what was queued for it. Dropping the
    /// host does this, including when a panic unwinds it.
    fn end(&mut self, reason: &str) {
        self.runtime.detach(&mut self.mux, reason);
        let ids: Vec<PaneId> = self.mux.panes().iter().map(Pane::id).collect();
        for id in ids {
            self.mux.remove_pane(id);
        }
        self.panes.clear();
        let end = now_ms() + mux::HANGUP_GRACE_MS + 1000;
        while (self.mux.reaping() || self.runtime.closing()) && now_ms() < end {
            let deadline = Some(now_ms() + 20);
            if self.runtime.wait(&mut self.mux, &[], deadline).is_err() {
                break;
            }
        }
    }

    /// Open the overlay, or close it.
    fn toggle_overlay(&mut self) -> Result<()> {
        if self.overlay.borrow().is_some() {
            return self.close_overlay();
        }
        self.bar.borrow_mut().prompt = None;
        self.help.set(false);
        self.mux.invalidate_chrome();
        let overlay = match Overlay::load(&self.state_path) {
            Ok(overlay) => overlay,
            // The overlay can't show without its list; the error goes to
            // the log.
            Err(e) => {
                eprintln!("hypercube: reading the workspaces: {e}");
                self.mux.set_captured(false);
                return Ok(());
            }
        };
        *self.overlay.borrow_mut() = Some(overlay);
        self.overlay_read = now_ms();
        self.mux.set_captured(true);
        self.lay_out()
    }

    fn close_overlay(&mut self) -> Result<()> {
        if self.overlay.borrow_mut().take().is_none() {
            return Ok(());
        }
        self.mux.set_captured(false);
        self.lay_out()
    }

    /// Stop another workspace on a thread, which wakes the loop when it is
    /// done; see `finish_stops`.
    fn start_stop(&mut self, path: PathBuf) {
        let results = self.stop_results.0.clone();
        let wake = self.stopped.1.as_raw_fd();
        std::thread::spawn(move || {
            let name = store::workspace(&path);
            let result = stop(&path).map_err(|e| e.to_string());
            let _ = results.send((name, result));
            mux::write_all(wake, b"s");
        });
    }

    /// Show the results of the stops that finished.
    fn finish_stops(&mut self) {
        let mut buf = [0u8; 64];
        while mux::read(self.stopped.0.as_raw_fd(), &mut buf) > 0 {}
        while let Ok((name, result)) = self.stop_results.1.try_recv() {
            self.read_overlay();
            if let (Err(e), Some(overlay)) = (result, &mut *self.overlay.borrow_mut()) {
                overlay.error = format!("{name}: {e}");
            }
        }
    }

    /// Read the overlay's list again.
    fn read_overlay(&mut self) {
        if let Some(overlay) = &mut *self.overlay.borrow_mut()
            && let Err(e) = overlay.reload()
        {
            overlay.error = e.to_string();
        }
        self.overlay_read = now_ms();
        self.mux.invalidate_chrome();
    }

    /// Read the bar's list again, and redraw it if it changed.
    fn read_bar(&mut self) {
        let mut bar = self.bar.borrow_mut();
        let changed = match bar::running() {
            Ok(entries) => bar.update(entries),
            Err(e) => {
                bar.fail(e.to_string());
                true
            }
        };
        if changed {
            self.mux.invalidate_chrome();
        }
        self.bar_read = now_ms();
    }

    /// Switch the client to the workspace numbered `n` in the bar. On the
    /// host's own workspace, close the overlay if it is open.
    fn switch_to(&mut self, n: usize) -> Result<()> {
        let entry = self.bar.borrow().entries.get(n).cloned();
        match entry {
            None => {}
            Some(e) if e.path == self.state_path => self.close_overlay()?,
            Some(Entry {
                root: Some(root), ..
            }) => self.runtime.send(Message::Switch(root)),
            Some(e) => {
                let error = format!("{}: the state has no orchestrator", e.name);
                self.bar.borrow_mut().fail(error);
                self.mux.invalidate_chrome();
            }
        }
        Ok(())
    }

    /// Open the list of keys in place of the overlay or the prompt, or
    /// close it.
    fn toggle_help(&mut self) -> Result<()> {
        let open = !self.help.get();
        self.close_screens()?;
        self.help.set(open);
        self.mux.set_captured(open);
        self.lay_out()
    }

    /// Close the overlay, the list of keys, and the prompt.
    fn close_screens(&mut self) -> Result<()> {
        let covered = self.overlay.borrow_mut().take().is_some() | self.help.replace(false);
        self.bar.borrow_mut().prompt = None;
        self.mux.set_captured(false);
        self.mux.invalidate_chrome();
        if covered { self.lay_out() } else { Ok(()) }
    }

    /// Detach the client.
    fn detach(&mut self) -> Result<()> {
        self.runtime.detach(&mut self.mux, "");
        self.close_screens()
    }

    /// Give the workspace the next accent color in `ACCENTS`, and keep it in
    /// the state.
    fn next_accent(&mut self) {
        let next = store::update(&self.state_path, |s| {
            s.accent = (s.accent + 1) % ACCENTS.len() as u64;
            Ok(s.accent as usize)
        });
        match next {
            Ok(n) => self.palette.set(Palette::of(n)),
            Err(e) => self.bar.borrow_mut().fail(e.to_string()),
        }
        self.mux.invalidate_chrome();
    }

    /// Switch the client `n` workspaces along the bar, wrapping around.
    fn step(&mut self, n: i8) -> Result<()> {
        let bar = self.bar.borrow();
        let count = bar.entries.len() as isize;
        let current = bar.entries.iter().position(|e| e.path == self.state_path);
        drop(bar);
        match current {
            Some(i) => self.switch_to((i as isize + isize::from(n)).rem_euclid(count) as usize),
            None => Ok(()),
        }
    }

    /// Prompt in the bar for the path of a workspace to switch to, in place
    /// of the overlay or the list of keys. The mux captured the input that
    /// follows.
    fn open_prompt(&mut self) -> Result<()> {
        self.close_screens()?;
        self.bar.borrow_mut().prompt = Some(String::new());
        self.mux.set_captured(true);
        Ok(())
    }

    /// Carry out a key typed at the bar's prompt. Enter switches the client
    /// to the workspace containing the path, relative to this workspace's
    /// root, and the client starts its host if none is running.
    fn prompt_key(&mut self, key: &[u8]) {
        let typed = self.bar.borrow_mut().key(key);
        match typed {
            Typed::None => return,
            Typed::Changed => {}
            Typed::Cancel => self.mux.set_captured(false),
            Typed::Enter(path) => {
                self.mux.set_captured(false);
                match workspace::locate(&self.root, path.trim()) {
                    Ok(root) if root == self.root => {}
                    Ok(root) => self.runtime.send(Message::Switch(root)),
                    Err(e) => self.bar.borrow_mut().fail(e.to_string()),
                }
            }
        }
        self.mux.invalidate_chrome();
    }

    /// Carry out a key pressed in the overlay. Returns whether it stopped
    /// this host's own workspace.
    fn overlay_key(&mut self, key: &[u8]) -> Result<bool> {
        let action = match &mut *self.overlay.borrow_mut() {
            Some(overlay) => overlay.key(key),
            None => return Ok(false),
        };
        match action {
            Action::None => return Ok(false),
            Action::Move => {}
            // The overlay stays until the client leaves for the other host
            // or says it couldn't.
            Action::Switch(dir) => self.runtime.send(Message::Switch(dir)),
            Action::Stop(path) if path == self.state_path => return Ok(true),
            Action::Stop(path) => self.start_stop(path),
            Action::Detach => self.detach()?,
            Action::Close => self.close_overlay()?,
        }
        self.mux.invalidate_chrome();
        Ok(false)
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
    fn read_client(&mut self, fd: RawFd) -> Result<bool> {
        let Some(i) = self.clients.iter().position(|c| c.stream.as_raw_fd() == fd) else {
            return Ok(false);
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
            return Ok(false);
        }
        let client = self.clients.remove(i);
        let (reply, stop) = match control::read_request(client.request.as_slice()) {
            Ok(Request::Stop) => (Reply::ok(), true),
            Ok(Request::Attach {
                protocol,
                size,
                replies,
            }) => {
                if protocol != link::PROTOCOL {
                    let error = format!(
                        "the host of {} runs another version of hypercube; \
                         stop it with `hypercube stop` and start it again",
                        workspace::short(&self.root)
                    );
                    (Reply::error(error), false)
                } else {
                    // The reply fits in the socket's buffer.
                    if control::write_reply(&client.stream, &Reply::ok()).is_ok() {
                        self.attach(client.stream, replies.as_bytes(), size)?;
                    }
                    return Ok(false);
                }
            }
            Ok(request) => (self.handle(request), false),
            Err(e) => (Reply::error(e), false),
        };
        // The reply fits in the socket's buffer; a client that went away
        // doesn't get one.
        let _ = control::write_reply(&client.stream, &reply);
        Ok(stop)
    }

    /// Draw for the client on `stream` from now on, detaching any other.
    /// The first attach launches the agents.
    fn attach(&mut self, stream: UnixStream, replies: &[u8], size: link::Size) -> Result<()> {
        let Ok(link) = Link::new(stream) else {
            return Ok(());
        };
        if self.runtime.connected() {
            self.runtime
                .detach(&mut self.mux, "attached in another terminal");
        }
        *self.overlay.borrow_mut() = None;
        self.bar.borrow_mut().prompt = None;
        self.help.set(false);
        self.mux.set_captured(false);
        self.runtime.connect(&mut self.mux, link, replies, size)?;
        self.read_bar();
        match self.unstarted.take() {
            Some(agents) => self.start(&agents),
            None => self.lay_out(),
        }
    }

    fn handle(&mut self, request: Request) -> Reply {
        let result = match request {
            Request::Launch { agent, prompt } => self.launch(&agent, &prompt, false),
            Request::Close { agent } => self.close(&agent),
            Request::Stop | Request::Attach { .. } => Ok(()),
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
        let pane = self.add_labeled_pane(agent)?;
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
        self.place(&layout)
    }

    /// Start the harness of an agent whose program exited again in its pane,
    /// with its saved session, as when the host launches it. An agent whose
    /// program has exited quickly `MAX_QUICK_EXITS` times in a row is left
    /// exited. If the harness fails to start, the pane shows why, and the
    /// host doesn't try again.
    fn relaunch(&mut self, pane: PaneId) -> Result<()> {
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
            self.mux.release_focus(pane);
            return Ok(());
        }
        slot.started = now;
        let id = slot.agent.clone();
        // The activity is set before the harness starts, so its hooks, which
        // change it, run after.
        let agent = store::update(&self.state_path, |s| {
            let agent = s.agent_mut(&id)?;
            agent.activity = start_activity(agent).into();
            Ok(agent.clone())
        })?;
        self.mux.restart(pane, &agent.name)?;
        let ws = self.mux.pane(pane).map(Pane::ws).unwrap_or_default();
        let started = harness::command(&self.exe, &agent, &self.state_path, start_prompt(&agent))
            .and_then(|launch| spawn(&launch, ws));
        match started {
            Ok(pty) => self.mux.attach(pane, pty),
            Err(e) => self.show_failure(pane, &agent, &e.to_string()),
        }
        Ok(())
    }

    /// Zoom a pane, or end the zoom.
    fn toggle_zoom(&mut self, pane: PaneId) -> Result<()> {
        if self.zoomed.get().is_some() {
            return self.unzoom();
        }
        // The pane may have closed or exited since the key was pressed.
        if self.mux.pane(pane).is_none_or(|p| p.status().is_some()) {
            return Ok(());
        }
        self.zoomed.set(Some(pane));
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
        self.mux.remove_pane(slot.pane);
        self.zoomed.set(None);
        self.lay_out()
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
        self.place(&layout.unwrap_or_else(|| hidden(&ids)))
    }

    /// Size and place the panes, then hide them while the overlay or the
    /// list of keys covers the screen.
    fn place(&mut self, layout: &[(PaneId, Rect)]) -> Result<()> {
        self.mux.set_layout(layout)?;
        if self.overlay.borrow().is_some() || self.help.get() {
            self.mux.set_layout(&hidden(&self.pane_ids()))?;
        }
        Ok(())
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
    fn a_lone_pane_fills_the_area_between_the_header_and_the_bar() {
        assert_eq!(rects(layout(size(80, 24), &ids(1))), [rect(0, 1, 80, 22)]);
        assert_eq!(layout(size(80, 24), &[]), Some(Vec::new()));
    }

    #[test]
    fn workers_stack_right_of_the_orchestrator() {
        // The orchestrator takes 40% of the columns, then the divider's. A
        // worker's label row is above its rect.
        assert_eq!(
            rects(layout(size(100, 25), &ids(2))),
            [rect(0, 1, 40, 23), rect(41, 2, 59, 22)]
        );
        // 23 rows over three workers: blocks of 7, 8 and 8 rows, each a label
        // row and the rest content.
        assert_eq!(
            rects(layout(size(100, 25), &ids(4))),
            [
                rect(0, 1, 40, 23),
                rect(41, 2, 59, 6),
                rect(41, 9, 59, 7),
                rect(41, 17, 59, 7),
            ]
        );
        // Uneven heights differ by at most a row and fill the column.
        let layout = rects(layout(size(81, 32), &ids(5)));
        assert_eq!(layout[0], rect(0, 1, 32, 30));
        let heights: Vec<u16> = layout[1..].iter().map(|r| r.rows + 1).collect();
        assert_eq!(heights, [7, 8, 7, 8]);
        assert_eq!(layout[4].row + layout[4].rows, 31);
    }

    #[test]
    fn a_layout_without_room_for_every_pane_fails() {
        // The orchestrator needs a content row between the header and the bar.
        assert!(layout(size(80, 2), &ids(1)).is_none());
        assert!(layout(size(80, 3), &ids(1)).is_some());
        // Each worker needs a label row and a content row.
        assert!(layout(size(80, 7), &ids(4)).is_none());
        assert!(layout(size(80, 8), &ids(4)).is_some());
        // A column for the divider, and one for each pane.
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
                // The orchestrator has no label row.
                let expected = match (id == *zoomed, id == PaneId(1)) {
                    (false, _) => Rect::default(),
                    (true, true) => rect(0, 1, 100, 23),
                    (true, false) => rect(0, 2, 100, 22),
                };
                assert_eq!(r, expected);
            }
        }
        assert!(zoom_layout(size(80, 2), &ids(2), PaneId(2)).is_none());
        assert_eq!(zoom_layout(size(80, 24), &[], PaneId(1)), Some(Vec::new()));
    }

    struct Screen(Terminal<'static, 'static>);

    impl Screen {
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
            detail: "",
            rect,
            focused,
            status,
            scrolled: 0,
        }
    }

    fn rgb((r, g, b): colors::Rgb) -> StyleColor {
        StyleColor::Rgb(RgbColor { r, g, b })
    }

    fn fg(screen: &Screen, x: u16, y: u16) -> StyleColor {
        screen.cell(x, y).style().unwrap().fg_color
    }

    fn chrome() -> TeamChrome {
        TeamChrome {
            zoomed: Rc::default(),
            overlay: Rc::default(),
            help: Rc::default(),
            palette: Rc::default(),
            bar: Rc::default(),
        }
    }

    /// `panes` drawn on a screen of `cols` by `rows`.
    fn draw_with(chrome: &mut TeamChrome, panes: &[PaneView], cols: u16, rows: u16) -> Screen {
        let mut out = Vec::new();
        chrome.draw(&mut out, panes, size(cols, rows));
        assert!(out.ends_with(b"\x1b[0m"));
        let mut terminal = Terminal::new(cols, rows).unwrap();
        terminal.vt_write(&out);
        Screen(terminal)
    }

    /// A worker's label row, with the orchestrator's pane first and hidden.
    fn worker_row(label: &str, detail: &str, rect: Rect, cols: u16, rows: u16, y: u16) -> String {
        let panes = [
            view(1, "orch", Rect::default(), false, None),
            PaneView {
                detail,
                ..view(2, label, rect, false, None)
            },
        ];
        draw_with(&mut chrome(), &panes, cols, rows).row(y)
    }

    #[test]
    fn chrome_draws_the_header_rule_titles_and_a_divider_that_branches_into_them() {
        let layout = rects(layout(size(40, 10), &ids(3)));
        let panes = [
            PaneView {
                detail: "~/src/r",
                ..view(1, "orch", layout[0], false, None)
            },
            PaneView {
                scrolled: 42,
                detail: "⎇ w1",
                ..view(2, "w1", layout[1], false, None)
            },
            PaneView {
                detail: "⎇ w2",
                ..view(3, "w2", layout[2], true, Some(3))
            },
        ];
        let screen = draw_with(&mut chrome(), &panes, 40, 10);
        let rows: Vec<String> = (0..10).map(|y| screen.row(y)).collect();
        let (sp, bar) = (|n| " ".repeat(n), |n| "━".repeat(n));
        let side = format!("{}┃{}", sp(16), sp(23));
        assert_eq!(
            rows,
            [
                format!(" ~/src/r{}", sp(32)),
                format!("{}┣━ w1 · ↑42 {} ⎇ w1 ━", sp(16), bar(5)),
                side.clone(),
                side.clone(),
                side.clone(),
                // The detail doesn't fit.
                format!("{}┣━ w2 · exited 3 {}", sp(16), bar(7)),
                side.clone(),
                side.clone(),
                side,
                sp(40),
            ]
        );
        // The header is in the bar colors. The lines and the divider are
        // the accent with no background, and the text is lighter and bold
        // when its pane has the focus.
        let palette = Palette::default();
        let blue = rgb(palette.line);
        for (x, y) in [(0, 0), (39, 0)] {
            assert_eq!(
                screen.bg(x, y),
                rgb(palette.bar.bg),
                "background at {x},{y}"
            );
            assert_eq!(
                fg(&screen, x, y),
                rgb(palette.bar.fg),
                "foreground at {x},{y}"
            );
        }
        for (x, y) in [
            (16, 1),
            (16, 3),
            (17, 1),
            (30, 1),
            (39, 1),
            (16, 5),
            (33, 5),
        ] {
            assert_eq!(fg(&screen, x, y), blue, "foreground at {x},{y}");
            assert_eq!(screen.bg(x, y), StyleColor::None, "background at {x},{y}");
        }
        for (x, y) in [(19, 1), (36, 1), (19, 5)] {
            assert_eq!(
                fg(&screen, x, y),
                rgb(palette.text),
                "foreground at {x},{y}"
            );
        }
        assert!(!screen.bold(19, 1));
        assert!(screen.bold(19, 5));
        // Nothing but the chrome is painted: the panes' cells and the gaps
        // have the terminal's own background.
        assert_eq!(screen.bg(5, 5), StyleColor::None);
        assert_eq!(screen.bg(30, 3), StyleColor::None);
    }

    #[test]
    fn a_zoomed_worker_has_no_divider_to_branch_from() {
        let layout = rects(zoom_layout(size(30, 6), &ids(2), PaneId(2)));
        let panes = [
            view(1, "orch", layout[0], false, None),
            view(2, "w1", layout[1], true, None),
        ];
        let mut chrome = chrome();
        chrome.zoomed.set(Some(PaneId(2)));
        let screen = draw_with(&mut chrome, &panes, 30, 6);
        assert_eq!(screen.row(1), format!("━ w1 · zoomed {}", "━".repeat(16)));
        assert_eq!(screen.row(2), " ".repeat(30));
    }

    #[test]
    fn the_header_notes_the_orchestrator_where_a_label_would() {
        let layout = rects(zoom_layout(size(40, 8), &ids(2), PaneId(1)));
        let panes = [
            PaneView {
                scrolled: 5,
                detail: "~/src/r",
                ..view(1, "orch", layout[0], true, Some(3))
            },
            view(2, "w1", layout[1], false, None),
        ];
        let mut chrome = chrome();
        chrome.zoomed.set(Some(PaneId(1)));
        let screen = draw_with(&mut chrome, &panes, 40, 8);
        assert_eq!(
            screen.row(0),
            format!(" ~/src/r{}zoomed · exited 3 · ↑5 ", " ".repeat(9))
        );
    }

    #[test]
    fn chrome_marks_the_zoomed_pane() {
        let layout = rects(zoom_layout(size(33, 8), &ids(2), PaneId(2)));
        let panes = [
            view(1, "orch", layout[0], false, None),
            PaneView {
                detail: "⎇ w1",
                ..view(2, "w1", layout[1], true, Some(3))
            },
        ];
        let mut chrome = chrome();
        chrome.zoomed.set(Some(PaneId(2)));
        let screen = draw_with(&mut chrome, &panes, 33, 8);
        assert_eq!(screen.row(0), " ".repeat(33));
        assert_eq!(screen.row(1), "━ w1 · zoomed · exited 3 ━ ⎇ w1 ━");
        assert_eq!(screen.row(2), " ".repeat(33));
    }

    #[test]
    fn chrome_drops_the_detail_before_clipping_the_name() {
        let row =
            |label, cols: u16| worker_row(label, "~/src/r", rect(1, 2, cols, 2), cols + 1, 6, 1);
        // The detail needs a blank column between it and the name.
        assert_eq!(row("orch", 18), "┣━ orch ━ ~/src/r ━");
        assert_eq!(row("orch", 17), "┣━ orch ━━━━━━━━━━");
        assert_eq!(row("orchestrator", 8), "┣━ orche━");

        let hidden = [
            view(1, "orch", Rect::default(), true, None),
            view(2, "w1", Rect::default(), false, None),
        ];
        let screen = draw_with(&mut chrome(), &hidden, 8, 4);
        assert_eq!(screen.row(0), "        ");
        assert_eq!(screen.row(1), "        ");
    }

    #[test]
    fn chrome_measures_wide_and_combining_characters_in_columns() {
        let row = worker_row("界界界界界", "", rect(1, 2, 9, 2), 12, 6, 1);
        // Nothing wraps, so nothing scrolls, and spacer cells read as blanks.
        assert_eq!(row, "┣━ 界 界 界 ━  ");
        assert_eq!(fit("e\u{301}x", 1), "e\u{301}");
        assert_eq!(fit("界", 1), "");
    }

    #[test]
    fn labels_show_the_branch_of_a_worktree_and_otherwise_the_directory() {
        let mut a = Agent {
            role: store::WORKER.into(),
            branch: "hypercube/parser".into(),
            dir: "/r/.hypercube/parser".into(),
            ..Agent::default()
        };
        assert_eq!(detail(&a), "⎇ parser");
        a.branch = "feature/x".into();
        assert_eq!(detail(&a), "⎇ feature/x");
        a.branch.clear();
        assert_eq!(detail(&a), "/r/.hypercube/parser");
        a.role = store::ORCHESTRATOR.into();
        a.branch = "main".into();
        a.dir = "/r".into();
        assert_eq!(detail(&a), "/r");
    }

    #[test]
    fn the_host_lock_tells_whether_a_host_runs() {
        let dir = TempDir::new();
        let state = dir.path().join("w.json");
        assert_eq!(lock_path(&state), dir.path().join("w.host.lock"));
        assert!(!running(&state).unwrap());

        let lock = try_lock(&lock_path(&state)).unwrap().unwrap();
        assert!(running(&state).unwrap());
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
        eventually(|| !running(&state).unwrap());
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
