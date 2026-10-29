//! The session host: `hypercube host`, in the background. It holds the
//! workspace's host lock, runs every agent in a pane, draws the panes with
//! their labels for the attached client, fires the scheduled tasks, and
//! answers requests on the control socket until the session ends.

use std::cell::{Cell, RefCell};
use std::error::Error;
use std::fs::{self, File, TryLockError};
use std::io::{ErrorKind, Read, Write};
use std::os::fd::{AsRawFd, OwnedFd, RawFd};
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::{Path, PathBuf};
use std::rc::Rc;
use std::sync::atomic::AtomicI32;
use std::sync::mpsc;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use crate::bar::{self, Bar, Entry, Typed};
use crate::colors::{self, ACCENTS, Palette};
use crate::control::{self, NUDGE, Reply, Request};
use crate::harness;
use crate::help;
use crate::icons;
use crate::link::{self, Link, Message};
use crate::mcp;
use crate::mux::{
    self, Captured, Chrome, Mux, Notice, Pane, PaneId, PaneView, Rect, Winsize, now_ms,
};
use crate::overlay::{Action, Overlay};
use crate::pty::Pty;
use crate::runtime::{Event, Runtime};
use crate::schedule::{self, LocalMinute};
use crate::store::{self, Agent, State};
use crate::teamlist::{self, TeamList};
use crate::{config, signals, workspace};
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
/// How often the team list is read again while it is open.
const TEAMLIST_REFRESH_MS: i64 = 1000;

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
pub fn try_lock(path: &Path) -> Result<Option<File>> {
    store::lock_file(path, false)
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
/// The first pane, the orchestrator's, takes the share `split.0` of
/// `split.0 + split.1` of the columns after reserving the divider, rounded
/// down, or all the columns when alone. The other panes, the workers, stack
/// in the remaining columns. Each `panes` entry after the first has the worker's
/// weight, or `None` while it is collapsed to its label row. Every pane has a
/// label row above it, and every expanded worker at least one row; the rows
/// left after those go to the expanded workers by weight. A collapsed worker
/// gets its place with no rows. `None` if a pane would get no row or column.
pub fn layout(
    size: Winsize,
    panes: &[(PaneId, Option<u32>)],
    split: (u32, u32),
) -> Option<Vec<(PaneId, Rect)>> {
    let area = pane_area(size);
    let (cols, rows) = (u32::from(area.cols), u32::from(area.rows));
    let Some((&(first, _), rest)) = panes.split_first() else {
        return Some(Vec::new());
    };
    let wide = cols.saturating_sub(DIVIDER);
    let left = if rest.is_empty() {
        cols
    } else if wide < 2 {
        return None;
    } else {
        (wide * split.0 / (split.0 + split.1).max(1)).clamp(1, wide - 1)
    };
    let mut layout = vec![(
        first,
        Rect {
            row: area.row + 1,
            cols: left as u16,
            rows: area.rows.saturating_sub(1),
            ..area
        },
    )];
    let weights: Vec<u32> = rest.iter().filter_map(|&(_, w)| w).collect();
    let extra = rows.checked_sub(rest.len() as u32 + weights.len() as u32)?;
    let mut extras = shares(extra, &weights).into_iter();
    let mut top = 0;
    for &(id, weight) in rest {
        let height = weight.map_or(0, |_| 1 + extras.next().unwrap_or(0));
        let rect = Rect {
            col: area.col + (left + DIVIDER) as u16,
            row: area.row + top as u16 + 1,
            cols: cols.saturating_sub(left + DIVIDER) as u16,
            rows: height as u16,
        };
        top += 1 + height;
        layout.push((id, rect));
    }
    (layout.iter().all(|(_, r)| r.cols > 0) && layout[0].1.rows > 0).then_some(layout)
}

/// `total` split by `weights`, rounded so the parts add up to it. Weights
/// that are all 0 count as equal.
fn shares(total: u32, weights: &[u32]) -> Vec<u32> {
    let equal = weights.iter().all(|&w| w == 0);
    let weights: Vec<u64> = weights
        .iter()
        .map(|&w| if equal { 1 } else { u64::from(w) })
        .collect();
    let (total, sum) = (u64::from(total), weights.iter().sum::<u64>());
    let mut before = 0;
    weights
        .iter()
        .map(|&w| {
            let start = total * before / sum;
            before += w;
            (total * before / sum - start) as u32
        })
        .collect()
}

/// Where each pane goes while `zoomed` fills the pane area, with its label row:
/// every other pane is hidden at its current size.
/// `None` if the pane would get no row or column.
pub fn zoom_layout(size: Winsize, panes: &[PaneId], zoomed: PaneId) -> Option<Vec<(PaneId, Rect)>> {
    let area = pane_area(size);
    let rect = Rect {
        row: area.row + 1,
        rows: area.rows.saturating_sub(1),
        ..area
    };
    if rect.cols == 0 || rect.rows == 0 {
        return None;
    }
    Some(
        panes
            .iter()
            .map(|&id| (id, if id == zoomed { rect } else { Rect::default() }))
            .collect(),
    )
}

/// The header, the divider between the panes, the labels
/// above every pane, and the bar, or the workspace overlay, the list of keys,
/// or the team list while one is open.
pub struct TeamChrome {
    /// The workspace path, shortened for the header.
    pub workspace: String,
    /// The viewed team, which the host changes.
    pub view: Rc<RefCell<String>>,
    /// Every team, in `State::teams` order, and whether any of its agents
    /// waits for a tool approval, refreshed with the bar.
    pub teams: Rc<RefCell<Vec<(String, bool)>>>,
    /// How many scheduled tasks there are and how many are running,
    /// refreshed with the bar.
    pub tasks: Rc<Cell<(usize, usize)>>,
    pub teamlist: Rc<RefCell<Option<TeamList>>>,
    /// Panes whose agents wait for a tool approval, refreshed with the bar.
    pub attention: Rc<RefCell<Vec<PaneId>>>,
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
        if let Some(teamlist) = &*self.teamlist.borrow() {
            teamlist.draw(out, size, palette);
            return;
        }
        let mut s = String::new();
        // Panes are hidden while the terminal is too small for them, and a
        // collapsed worker keeps only its label.
        let shown: Vec<&PaneView> = panes.iter().filter(|p| p.rect.cols > 0).collect();
        self.draw_header(&mut s, size.ws_col, palette);
        if let [first, _, ..] = shown.as_slice() {
            let col = u32::from(first.rect.col + first.rect.cols);
            s.push_str(&colors::fg(palette.line));
            for row in 1..u32::from(size.ws_row).saturating_sub(1) {
                s.push_str(&format!("{}┃", at(row, col)));
            }
        }
        for p in &shown {
            draw_label(
                &mut s,
                p,
                self.zoomed.get() == Some(p.id),
                self.attention.borrow().contains(&p.id),
                palette,
            );
            // The top titles meet at ┳; lower worker titles branch at ┣.
            // A zoomed pane has no divider to branch from.
            if let Some(col) = p.rect.col.checked_sub(1) {
                let joint = if p.rect.row == 2 { '┳' } else { '┣' };
                s.push_str(&format!(
                    "{}{}{joint}",
                    colors::fg(palette.line),
                    at(u32::from(p.rect.row) - 1, u32::from(col))
                ));
            }
        }
        self.bar.borrow().draw(&mut s, size, palette);
        s.push_str("\x1b[0m");
        out.extend_from_slice(s.as_bytes());
    }
}

impl TeamChrome {
    /// The header: the workspace path, and while the workspace has more than
    /// one team, the viewed team's name after it. At the right, while the
    /// workspace has scheduled tasks, their counts, then each other team
    /// whose agent waits for a tool approval, with a bell.
    fn draw_header(&self, s: &mut String, cols: u16, palette: Palette) {
        let view = self.view.borrow();
        let teams = self.teams.borrow();
        let mut left = format!(" {} {}", icons::HEADER, self.workspace);
        if teams.len() > 1 {
            left.push_str(&format!(" › {view}"));
        }
        let waiting: Vec<&str> = teams
            .iter()
            .filter(|(team, attention)| *attention && *team != *view)
            .map(|(team, _)| team.as_str())
            .collect();
        let mut entries: Vec<String> = Vec::new();
        let (tasks, running) = self.tasks.get();
        if tasks > 0 {
            entries.push(task_counts(tasks, running));
        }
        let before_bells: usize = entries.iter().map(|e| text_width(&format!("{e} · "))).sum();
        entries.extend(
            waiting
                .iter()
                .map(|team| format!("{team} {}", icons::ATTENTION)),
        );
        let right = if entries.is_empty() {
            String::new()
        } else {
            format!("{} ", entries.join(" · "))
        };
        draw_row(
            s,
            &RowStyle::bar(palette.bar),
            (1, 1, cols),
            false,
            &left,
            &right,
        );
        // Recolor the bells if the right text was drawn.
        let right_width = text_width(&right);
        if right.is_empty() || text_width(&fit(&left, cols)) + 1 + right_width > usize::from(cols) {
            return;
        }
        let bell = colors::Colors {
            fg: colors::ATTENTION,
            bg: palette.bar.bg,
        };
        let mut col = usize::from(cols) - right_width + before_bells;
        for team in waiting {
            col += text_width(team) + 1;
            s.push_str(&format!(
                "{}{}{}",
                at(0, col as u32),
                bell.sgr(),
                icons::ATTENTION
            ));
            col += text_width(&format!("{} · ", icons::ATTENTION));
        }
    }
}

/// The scheduled tasks' counts, as the header and the overlay show them.
pub(crate) fn task_counts(tasks: usize, running: usize) -> String {
    let noun = if tasks == 1 { "task" } else { "tasks" };
    format!("{} {tasks} {noun}, {running} running", icons::TASKS)
}

/// Move the cursor to `row` and `col`, both from 0.
fn at(row: u32, col: u32) -> String {
    format!("\x1b[{};{}H", row + 1, col + 1)
}

/// What to note about a pane after its name: an icon while it fills the pane
/// area, the exit status once its program has exited, in the error color, and
/// how many rows its view is scrolled up.
fn notes(p: &PaneView, zoomed: bool) -> Vec<(String, Option<colors::Rgb>)> {
    let mut notes = Vec::new();
    if zoomed {
        notes.push((icons::ZOOMED.to_string(), None));
    }
    if let Some(status) = p.status {
        notes.push((format!("{} {status}", icons::EXITED), Some(colors::ERROR)));
    }
    if p.scrolled > 0 {
        notes.push((format!("↑{}", p.scrolled), None));
    }
    notes
}

/// The column of the arrow that collapses or expands a worker in its label
/// row, for a worker whose pane is at `rect`.
fn arrow_col(rect: Rect) -> u16 {
    rect.col + 2
}

/// An agent's label row above its pane, across the pane's width, with the name
/// at the left, bold when focused, a harness's failure to start in the error
/// color, an attention bell if it fits, and notes.
/// A worker beside the divider has an arrow before its name, pointing right
/// while it is collapsed and down while it is not.
/// The detail goes at the right end unless the row is too narrow for it. The
/// row is a heavy line with a space on each side of its text, and a line's
/// width to spare at each end.
fn draw_label(s: &mut String, p: &PaneView, zoomed: bool, attention: bool, palette: Palette) {
    let mut left = if p.rect.col > 0 {
        let arrow = if p.rect.rows == 0 {
            icons::COLLAPSED
        } else {
            icons::EXPANDED
        };
        format!(" {arrow} {}", p.label)
    } else {
        format!(" {}", p.label)
    };
    // Parts of the text drawn in their own color: where each starts, in
    // columns from the start of the text, the part, and its color.
    let mut spans = Vec::new();
    let failed = format!(" · {}", icons::FAILED);
    if let Some(i) = left.find(&failed) {
        let start = i + " · ".len();
        spans.push((
            text_width(&left[..start]),
            left[start..].to_string(),
            colors::ERROR,
        ));
    }
    let name_width = text_width(&left);
    if attention && name_width + 2 <= usize::from(p.rect.cols.saturating_sub(2)) {
        left.push_str(&format!(" {}", icons::ATTENTION));
        spans.push((
            name_width + 1,
            icons::ATTENTION.to_string(),
            colors::ATTENTION,
        ));
    }
    for (note, color) in notes(p, zoomed) {
        left.push_str(" · ");
        if let Some(color) = color {
            spans.push((text_width(&left), note.clone(), color));
        }
        left.push_str(&note);
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
    // Recolor the spans, as much of each as the row shows, without including
    // SGR bytes in the row's width.
    let shown = text_width(&fit(&left, p.rect.cols.saturating_sub(2)));
    for (start, text, color) in spans {
        let Some(room) = shown.checked_sub(start).filter(|&room| room > 0) else {
            continue;
        };
        s.push_str(&format!(
            "{}{}{}{}",
            at(
                u32::from(p.rect.row) - 1,
                u32::from(p.rect.col) + 1 + start as u32
            ),
            colors::fg(color),
            if p.focused { "\x1b[1m" } else { "" },
            fit(&text, u16::try_from(room).unwrap_or(u16::MAX)),
        ));
    }
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

/// The right end of an agent's label: the branch icon and its branch, without
/// the `hypercube/` prefix for workers, or else the directory icon and the
/// directory.
fn detail(agent: &Agent) -> String {
    if !agent.branch.is_empty() {
        let branch = if agent.role == store::WORKER {
            agent
                .branch
                .strip_prefix("hypercube/")
                .unwrap_or(&agent.branch)
        } else {
            &agent.branch
        };
        format!("{} {branch}", icons::BRANCH)
    } else {
        format!(
            "{} {}",
            icons::DIRECTORY,
            workspace::short(Path::new(&agent.dir))
        )
    }
}

/// Read the checkout's branch without spawning Git. Linked worktrees point
/// to their Git directory through a `gitdir:` file. Detached HEADs have no
/// branch, so their title falls back to the directory.
fn current_branch(root: &Path) -> Option<String> {
    let git = root.join(".git");
    let git = if git.is_file() {
        let contents = fs::read_to_string(&git).ok()?;
        root.join(contents.trim().strip_prefix("gitdir: ")?)
    } else {
        git
    };
    let head = fs::read_to_string(git.join("HEAD")).ok()?;
    Some(
        head.trim()
            .strip_prefix("ref: refs/heads/")
            .unwrap_or_default()
            .to_string(),
    )
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
    if signal != libc::SIGHUP {
        signals::wake(&SIGNAL_PIPE);
    }
}

/// Catch SIGINT and SIGTERM, and return a pipe that becomes readable when one
/// arrives. SIGHUP is caught and ignored. Programs started later run with the
/// default handlers.
fn catch_signals() -> Result<RawFd> {
    signals::catch(
        &[libc::SIGHUP, libc::SIGINT, libc::SIGTERM],
        on_signal,
        &SIGNAL_PIPE,
    )
    .map_err(|e| format!("pipe: {e}").into())
}

/// The reply to a request that succeeded or failed.
fn reply(result: Result<()>) -> Reply {
    result.map_or_else(Reply::error, |()| Reply::ok())
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
    /// The viewed team, whose panes the screen shows, shared with the
    /// chrome.
    view: Rc<RefCell<String>>,
    /// Every team and whether it waits for a tool approval, shared with the
    /// chrome.
    teams: Rc<RefCell<Vec<(String, bool)>>>,
    /// The scheduled tasks' counts, shared with the chrome.
    tasks: Rc<Cell<(usize, usize)>>,
    /// The last minute the scheduled tasks were checked in, in Unix
    /// minutes. The minute the host starts in is never checked.
    checked: u64,
    /// The team list while it is open, shared with the chrome.
    teamlist: Rc<RefCell<Option<TeamList>>>,
    /// When the team list was last read.
    teamlist_read: i64,
    clients: Vec<Client>,
    /// The pane that fills the pane area, shared with the chrome.
    zoomed: Rc<Cell<Option<PaneId>>>,
    /// The orchestrator's and the workers' shares of the columns beside the
    /// divider, in every team.
    split: (u32, u32),
    /// What a left-button drag outside the panes moves.
    drag: Option<Drag>,
    /// The workspace overlay while it is open, shared with the chrome.
    overlay: Rc<RefCell<Option<Overlay>>>,
    /// When the overlay's list was last read.
    overlay_read: i64,
    /// Whether the list of keys is open, shared with the chrome.
    help: Rc<Cell<bool>>,
    /// The bar, shared with the chrome.
    bar: Rc<RefCell<Bar>>,
    /// When the bar's list and pane attention flags were last read.
    bar_read: i64,
    /// Panes waiting for a tool approval, shared with the chrome.
    attention: Rc<RefCell<Vec<PaneId>>>,
    /// The workspace's colors, shared with the chrome.
    palette: Rc<Cell<Palette>>,
    /// Stops of other workspaces, which run on threads so the host keeps
    /// serving: a pipe that becomes readable when one finishes, and their
    /// results.
    stopped: (OwnedFd, OwnedFd),
    stop_results: (mpsc::Sender<StopResult>, mpsc::Receiver<StopResult>),
}

/// What a left-button drag outside the panes moves.
#[derive(Clone, Copy)]
enum Drag {
    /// The divider between the orchestrator and the workers.
    Divider,
    /// The label row of the worker in this place among the viewed team's
    /// slots, and the rows between it and the expanded workers above and
    /// below it.
    Label(usize),
}

/// An agent's pane and how its program has been running.
struct Slot {
    agent: String,
    /// The agent's team.
    team: String,
    pane: PaneId,
    /// A worker's share of the rows beside the divider, by `layout`.
    weight: u32,
    /// Whether a worker is collapsed to its label row.
    collapsed: bool,
    /// When the program started, in `now_ms` time.
    started: i64,
    /// How many times in a row the program has exited quickly.
    quick_exits: u32,
}

impl Slot {
    fn new(agent: &Agent, pane: PaneId, weight: u32) -> Slot {
        Slot {
            agent: agent.id.clone(),
            team: agent.team.clone(),
            pane,
            weight,
            collapsed: false,
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
    let mut host = Host::new(root, state_path, socket, signals, agents)?;
    let result = host.serve();
    if let Err(e) = &result {
        host.end(&e.to_string());
    }
    result
}

/// Ready the state for a session; see `ready`. Returns the agents, the
/// orchestrators first.
fn prepare(root: &Path, state_path: &Path) -> Result<Vec<Agent>> {
    let config = config::load()?;
    let branch = workspace::branch(root).ok();
    let dir = workspace::utf8(root)?;
    store::update(state_path, |s| {
        ready(s, &config.orchestrator, dir, branch.as_deref())
    })
}

/// Ready the state of the workspace whose checkout is `dir`, on `branch` if
/// it is a Git checkout, for a session: create the default team's
/// orchestrator on the first run, put agents without a team in the default
/// team, set the orchestrators' branch, and mark them idle and the workers
/// busy. Returns the agents, the orchestrators first.
fn ready(
    s: &mut State,
    spec: &config::Spec,
    dir: &str,
    branch: Option<&str>,
) -> Result<Vec<Agent>> {
    // A directory can become a checkout, so this is checked on every start.
    s.no_git = branch.is_none();
    if s.agents.is_empty() {
        s.accent = 0;
        s.agents.push(new_orchestrator(
            spec,
            "orchestrator",
            store::DEFAULT_TEAM,
            dir,
        )?);
    }
    for a in &mut s.agents {
        if a.team.is_empty() {
            a.team = store::DEFAULT_TEAM.into();
        }
        if a.role == store::ORCHESTRATOR {
            a.branch = branch.unwrap_or_default().into();
        }
        a.activity = start_activity(a).into();
    }
    let mut agents = s.agents.clone();
    agents.sort_by_key(|a| a.role != store::ORCHESTRATOR);
    Ok(agents)
}

/// An orchestrator named `name` of `team`, working in the checkout `dir`
/// with the configured orchestrator's harness, model, and effort.
fn new_orchestrator(spec: &config::Spec, name: &str, team: &str, dir: &str) -> Result<Agent> {
    Ok(Agent {
        id: store::new_id()?,
        name: name.into(),
        role: store::ORCHESTRATOR.into(),
        harness: spec.harness.clone(),
        model: spec.model.clone(),
        effort: spec.effort.clone(),
        dir: dir.into(),
        team: team.into(),
        ..Agent::default()
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
    /// The host of the workspace whose checkout is `root`, listening on
    /// `socket`, ending at a signal on the pipe `signals`, and launching
    /// `agents` on the first attach. It views the default team.
    fn new(
        root: &Path,
        state_path: PathBuf,
        socket: Socket,
        signals: RawFd,
        agents: Vec<Agent>,
    ) -> Result<Host> {
        let exe = std::env::current_exe().map_err(|e| format!("finding hypercube: {e}"))?;
        let zoomed = Rc::new(Cell::new(None));
        let overlay = Rc::new(RefCell::new(None));
        let help = Rc::new(Cell::new(false));
        let bar = Rc::new(RefCell::new(Bar::new(state_path.clone())));
        let palette = Rc::new(Cell::new(Palette::of(
            store::load(&state_path)?.accent as usize,
        )));
        let attention = Rc::new(RefCell::new(Vec::new()));
        let view = Rc::new(RefCell::new(store::DEFAULT_TEAM.to_string()));
        let teams = Rc::new(RefCell::new(Vec::new()));
        let tasks = Rc::new(Cell::new((0, 0)));
        let teamlist = Rc::new(RefCell::new(None));
        let chrome = TeamChrome {
            workspace: workspace::short(root),
            view: Rc::clone(&view),
            teams: Rc::clone(&teams),
            tasks: Rc::clone(&tasks),
            teamlist: Rc::clone(&teamlist),
            attention: Rc::clone(&attention),
            zoomed: Rc::clone(&zoomed),
            overlay: Rc::clone(&overlay),
            help: Rc::clone(&help),
            bar: Rc::clone(&bar),
            palette: Rc::clone(&palette),
        };
        Ok(Host {
            exe,
            root: root.to_path_buf(),
            state_path,
            mux: Mux::new(Winsize::default(), Box::new(chrome)),
            runtime: Runtime::new(),
            socket,
            signals,
            unstarted: Some(agents),
            panes: Vec::new(),
            view,
            teams,
            tasks,
            checked: schedule::unix_now() / 60,
            teamlist,
            teamlist_read: 0,
            clients: Vec::new(),
            zoomed,
            split: (1, 1),
            drag: None,
            overlay,
            overlay_read: 0,
            help,
            bar,
            bar_read: 0,
            attention,
            palette,
            stopped: signals::pipe().map_err(|e| format!("pipe: {e}"))?,
            stop_results: mpsc::channel(),
        })
    }

    /// Launch every agent, resuming saved sessions. Workers start a turn to
    /// pick up messages queued while they were down. Only a failure of the
    /// default team's orchestrator ends the session.
    fn start(&mut self, agents: &[Agent]) -> Result<()> {
        for a in agents {
            if let Err(e) = self.launch(&a.id, start_prompt(a), true) {
                if a.role == store::ORCHESTRATOR && a.team == store::DEFAULT_TEAM {
                    return Err(format!("launching {}: {e}", a.name).into());
                }
                self.add_failed(a, &e.to_string())?;
            }
        }
        Ok(())
    }

    /// Add a pane labeled for `agent`: its name at the left, and at the right
    /// its branch if set, or its directory. It is in the background unless
    /// its team is viewed.
    fn add_labeled_pane(&mut self, agent: &Agent) -> Result<PaneId> {
        let background = agent.team != *self.view.borrow();
        let pane = self.mux.add_pane(&agent.name, background)?;
        if let Some(p) = self.mux.pane_mut(pane) {
            p.detail = detail(agent);
        }
        Ok(pane)
    }

    /// Give an agent whose harness failed to start a pane without a
    /// program, showing the error and exit status 127, so it keeps its place
    /// until it is removed.
    fn add_failed(&mut self, agent: &Agent, error: &str) -> Result<()> {
        let pane = self.add_labeled_pane(agent)?;
        self.show_failure(pane, agent, error)?;
        let weight = self.new_weight(&agent.team);
        self.panes.push(Slot::new(agent, pane, weight));
        self.lay_out()
    }

    /// Show in a pane without a program that its agent's harness failed to
    /// start, as exited with status 127.
    fn show_failure(&mut self, pane: PaneId, agent: &Agent, error: &str) -> Result<()> {
        let error = error.replace(['\r', '\n'], " ");
        if let Some(p) = self.mux.pane_mut(pane) {
            p.label = format!("{} · {} {error}", agent.name, icons::FAILED);
            p.feed(format!("{} failed to launch: {error}\r\n", agent.name).as_bytes());
        }
        self.mux.mark_exited(pane, 127);
        self.give_up(&agent.id)
    }

    /// Record that the host has given up on running agent `id`, so the team
    /// can see that it will not answer until the session starts again.
    fn give_up(&self, id: &str) -> Result<()> {
        store::update(&self.state_path, |s| {
            s.agent_mut(id)?.activity = store::EXITED.into();
            Ok(())
        })
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
            if self.teamlist.borrow().is_some() {
                let refresh = self.teamlist_read + TEAMLIST_REFRESH_MS;
                deadline = Some(deadline.map_or(refresh, |d| d.min(refresh)));
            }
            if self.runtime.connected() {
                let refresh = self.bar_read + BAR_REFRESH_MS;
                deadline = Some(deadline.map_or(refresh, |d| d.min(refresh)));
            }
            let minute = now_ms() + next_minute_ms();
            deadline = Some(deadline.map_or(minute, |d| d.min(minute)));
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
                    Event::Notice(Notice::Melted(pane)) => self.finish_close(pane)?,
                    Event::Notice(Notice::Zoom(pane)) => self.toggle_zoom(pane)?,
                    Event::Notice(Notice::Focus(pane)) => {
                        // A change of focus ends the zoom, unless the zoomed
                        // pane is melting and keeps its place until it goes.
                        if self.zoomed.get().is_some_and(|z| {
                            z != pane && !self.mux.pane(z).is_some_and(Pane::melting)
                        }) {
                            self.unzoom()?;
                        }
                    }
                    Event::Resized => self.lay_out()?,
                    Event::Notice(Notice::Overlay) => self.toggle_overlay()?,
                    Event::Notice(Notice::Teams) => self.toggle_teamlist()?,
                    Event::Notice(Notice::Key(_)) if self.help.get() => self.toggle_help()?,
                    Event::Notice(Notice::Key(key)) if self.bar.borrow().prompt.is_some() => {
                        self.prompt_key(&key);
                    }
                    Event::Notice(Notice::Key(key)) if self.teamlist.borrow().is_some() => {
                        self.teamlist_key(&key)?;
                    }
                    Event::Notice(Notice::Key(key)) => {
                        if self.overlay_key(&key)? {
                            return Ok(());
                        }
                    }
                    Event::Notice(Notice::Workspace(n)) => self.switch_to(usize::from(n))?,
                    Event::Notice(Notice::Step(n)) => self.step(n)?,
                    Event::Notice(Notice::NewWorkspace) => self.open_prompt()?,
                    Event::Notice(Notice::Help) => self.toggle_help()?,
                    Event::Notice(Notice::Click(col, row)) => self.click(col, row)?,
                    Event::Notice(Notice::Drag(col, row)) => self.drag(col, row)?,
                    Event::Notice(Notice::Detach) => self.detach()?,
                    Event::Notice(Notice::Accent) => self.next_accent(),
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
            if self.teamlist.borrow().is_some()
                && now_ms() - self.teamlist_read >= TEAMLIST_REFRESH_MS
            {
                self.read_teamlist();
            }
            if self.runtime.connected() && now_ms() - self.bar_read >= BAR_REFRESH_MS {
                self.read_bar();
            }
            self.fire_tasks(schedule::unix_now());
            let now = now_ms();
            self.clients.retain(|c| now - c.since < REQUEST_TIMEOUT_MS);
        }
    }

    /// Fire the scheduled tasks for the minute of `now`, in Unix seconds, once
    /// a minute after the last one checked has begun. Minutes the host did not
    /// run in are not made up.
    fn fire_tasks(&mut self, now: u64) {
        if now / 60 <= self.checked {
            return;
        }
        self.checked = now / 60;
        let at = LocalMinute::at(now);
        if let Err(e) = store::update(&self.state_path, |s| Ok(schedule::fire(s, &at, now))) {
            eprintln!("hypercube: firing the scheduled tasks: {e}");
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
        *self.teamlist.borrow_mut() = None;
        self.mux.invalidate_chrome();
        let overlay = match Overlay::load(&self.state_path) {
            Ok(overlay) => overlay,
            // The overlay can't show without its list; the error goes to
            // the log.
            Err(e) => {
                eprintln!("hypercube: reading the workspaces: {e}");
                self.mux.set_captured(None);
                return Ok(());
            }
        };
        *self.overlay.borrow_mut() = Some(overlay);
        self.overlay_read = now_ms();
        self.mux.set_captured(Some(Captured::Overlay));
        self.lay_out()
    }

    fn close_overlay(&mut self) -> Result<()> {
        if self.overlay.borrow_mut().take().is_none() {
            return Ok(());
        }
        self.mux.set_captured(None);
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

    /// Refresh the bar, the attention flags of the panes and the teams, and
    /// the orchestrators' branch titles, redrawing when any changes.
    fn read_bar(&mut self) {
        let mut bar = self.bar.borrow_mut();
        let mut changed = match bar::running() {
            Ok(entries) => bar.update(entries),
            Err(e) => {
                bar.fail(e.to_string());
                true
            }
        };
        if let Ok(state) = store::load(&self.state_path) {
            let attention: Vec<PaneId> = self
                .panes
                .iter()
                .filter_map(|slot| {
                    state
                        .agent(&slot.agent)
                        .ok()
                        .filter(|a| a.attention)
                        .map(|_| slot.pane)
                })
                .collect();
            changed |= *self.attention.borrow() != attention;
            *self.attention.borrow_mut() = attention;
            let teams = team_attention(&state);
            changed |= *self.teams.borrow() != teams;
            *self.teams.borrow_mut() = teams;
            let tasks = state.task_counts();
            changed |= self.tasks.replace(tasks) != tasks;
            if let Some(branch) = current_branch(&self.root) {
                let detail = if branch.is_empty() {
                    format!("{} {}", icons::DIRECTORY, workspace::short(&self.root))
                } else {
                    format!("{} {branch}", icons::BRANCH)
                };
                for slot in &self.panes {
                    let orchestrator = state
                        .agent(&slot.agent)
                        .is_ok_and(|a| a.role == store::ORCHESTRATOR);
                    if let Some(pane) = self.mux.pane_mut(slot.pane)
                        && orchestrator
                    {
                        changed |= pane.detail != detail;
                        pane.detail = detail.clone();
                    }
                }
            }
        }
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

    /// Open the list of keys in place of the overlay, the team list, or the
    /// prompt, or close it.
    fn toggle_help(&mut self) -> Result<()> {
        let open = !self.help.get();
        self.close_screens()?;
        self.help.set(open);
        self.mux.set_captured(open.then_some(Captured::Help));
        self.lay_out()
    }

    /// Close the overlay, the list of keys, the team list, and the prompt.
    fn close_screens(&mut self) -> Result<()> {
        let covered = self.overlay.borrow_mut().take().is_some()
            | self.help.replace(false)
            | self.teamlist.borrow_mut().take().is_some();
        self.bar.borrow_mut().prompt = None;
        self.mux.set_captured(None);
        self.mux.invalidate_chrome();
        if covered { self.lay_out() } else { Ok(()) }
    }

    /// Detach the client.
    fn detach(&mut self) -> Result<()> {
        self.runtime.detach(&mut self.mux, "");
        self.close_screens()
    }

    /// Act on a left click outside the panes: switch the client to the
    /// workspace whose tab in the bar was clicked, collapse or expand the
    /// worker whose arrow was clicked, or start dragging the divider or a
    /// worker's label row.
    fn click(&mut self, col: u16, row: u16) -> Result<()> {
        self.drag = None;
        let size = self.mux.size();
        if row + 1 == size.ws_row {
            let n = self.bar.borrow().tab_at(col, size.ws_col);
            return match n {
                Some(n) => self.switch_to(n),
                None => Ok(()),
            };
        }
        if self.zoomed.get().is_some() {
            return Ok(());
        }
        let rects = self.rects();
        for (i, r) in rects.iter().enumerate().skip(1) {
            if r.cols == 0 || row + 1 != r.row || !(r.col..r.col + r.cols).contains(&col) {
                continue;
            }
            if col == arrow_col(*r) {
                let slot = self.viewed()[i];
                self.panes[slot].collapsed = !self.panes[slot].collapsed;
                return self.lay_out();
            }
            self.drag = Some(Drag::Label(i));
            return Ok(());
        }
        if let Some(first) = rects.first()
            && first.cols > 0
            && col == first.col + first.cols
            && row > 0
        {
            self.drag = Some(Drag::Divider);
        }
        Ok(())
    }

    /// Move what the left button is dragging to the pointer at `col` and
    /// `row`: the divider to its column, or a worker's label row to its row,
    /// keeping a row in each expanded worker, and lay the panes out again.
    fn drag(&mut self, col: u16, row: u16) -> Result<()> {
        if self.zoomed.get().is_some() {
            return Ok(());
        }
        match self.drag {
            None => Ok(()),
            Some(Drag::Divider) => {
                let wide = u32::from(self.mux.size().ws_col).saturating_sub(DIVIDER);
                if wide < 2 {
                    return Ok(());
                }
                let left = u32::from(col).clamp(1, wide - 1);
                self.split = (left, wide - left);
                self.lay_out()
            }
            Some(Drag::Label(i)) => {
                let rects = self.rects();
                let viewed = self.viewed();
                let expanded = |j: &usize| !self.panes[viewed[*j]].collapsed && rects[*j].rows > 0;
                let (Some(above), Some(below)) =
                    ((1..i).rev().find(expanded), (i..rects.len()).find(expanded))
                else {
                    return Ok(());
                };
                let (up, down) = (i32::from(rects[above].rows), i32::from(rects[below].rows));
                let moved = (i32::from(row) + 1 - i32::from(rects[i].row)).clamp(1 - up, down - 1);
                if moved == 0 {
                    return Ok(());
                }
                // The weights become the rows each expanded worker gets
                // beyond its first, which `layout` gives back exactly at
                // this size.
                for (j, &k) in viewed.iter().enumerate().skip(1) {
                    let slot = &mut self.panes[k];
                    if slot.collapsed {
                        continue;
                    }
                    let rows = i32::from(rects[j].rows) + if j == above { moved } else { 0 }
                        - if j == below { moved } else { 0 };
                    slot.weight = (rows - 1).max(0) as u32;
                }
                self.lay_out()
            }
        }
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
        self.mux.set_captured(Some(Captured::Prompt));
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
            Typed::Cancel => self.mux.set_captured(None),
            Typed::Enter(path) => {
                self.mux.set_captured(None);
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
            Ok(Request::Launch { agent, prompt }) => {
                (reply(self.launch(&agent, &prompt, false)), false)
            }
            Ok(Request::Close { agent }) => (reply(self.close(&agent)), false),
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
        *self.teamlist.borrow_mut() = None;
        self.bar.borrow_mut().prompt = None;
        self.help.set(false);
        self.mux.set_captured(None);
        self.runtime.connect(&mut self.mux, link, replies, size)?;
        self.read_bar();
        match self.unstarted.take() {
            Some(agents) => self.start(&agents),
            None => self.lay_out(),
        }
    }

    fn pane_of(&self, agent: &str) -> Option<PaneId> {
        self.panes.iter().find(|s| s.agent == agent).map(|s| s.pane)
    }

    /// The places in `panes` of the viewed team's slots.
    fn viewed(&self) -> Vec<usize> {
        let view = self.view.borrow();
        (0..self.panes.len())
            .filter(|&i| self.panes[i].team == *view)
            .collect()
    }

    /// Each pane of `team` with its weight for `layout`, `None` while
    /// collapsed.
    fn weights(&self, team: &str) -> Vec<(PaneId, Option<u32>)> {
        self.panes
            .iter()
            .filter(|s| s.team == team)
            .map(|s| (s.pane, (!s.collapsed).then_some(s.weight)))
            .collect()
    }

    /// The weight of a new worker of `team`: the average of its expanded
    /// workers', so it takes about as many rows as each of them.
    fn new_weight(&self, team: &str) -> u32 {
        let weights: Vec<u32> = self
            .weights(team)
            .iter()
            .skip(1)
            .filter_map(|&(_, w)| w)
            .collect();
        match weights.len() {
            0 => 1,
            n => weights.iter().sum::<u32>() / n as u32,
        }
    }

    /// Each of the viewed team's panes' place on the screen, in slot order.
    fn rects(&self) -> Vec<Rect> {
        self.viewed()
            .into_iter()
            .map(|i| {
                let pane = self.mux.pane(self.panes[i].pane);
                pane.map(Pane::rect).unwrap_or_default()
            })
            .collect()
    }

    /// Start an agent's harness in a new pane in its team and lay the panes
    /// out again. A non-empty `prompt` starts its first turn. If there is no
    /// room for the pane in its team's layout, a launch at startup hides the
    /// team's panes until the terminal grows, and any other launch fails,
    /// adding no pane.
    fn launch(&mut self, id: &str, prompt: &str, startup: bool) -> Result<()> {
        let state = store::load(&self.state_path)?;
        let agent = state.agent(id)?;
        if self.pane_of(&agent.id).is_some() {
            return Err(format!("{} already has a pane", agent.name).into());
        }
        let launch = harness::command(&self.exe, agent, &self.state_path, prompt)?;
        let pane = self.add_labeled_pane(agent)?;
        let weight = self.new_weight(&agent.team);
        let mut weights = self.weights(&agent.team);
        weights.push((pane, Some(weight)));
        let ids: Vec<PaneId> = weights.iter().map(|&(id, _)| id).collect();
        let size = self.mux.size();
        let layout = layout(size, &weights, self.split).or_else(|| startup.then(|| hidden(&ids)));
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
        self.panes.push(Slot::new(agent, pane, weight));
        if agent.team == *self.view.borrow() {
            self.zoomed.set(None);
        }
        self.lay_out()
    }

    /// Start the harness of an agent whose program exited again in its pane,
    /// with its saved session, as when the host launches it. An agent whose
    /// program has exited quickly `MAX_QUICK_EXITS` times in a row is left
    /// exited. If the harness fails to start, the pane shows why, and the
    /// host doesn't try again.
    fn relaunch(&mut self, pane: PaneId) -> Result<()> {
        if self.mux.pane(pane).is_none_or(Pane::melting) {
            return Ok(());
        }
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
            let id = slot.agent.clone();
            self.mux.release_focus(pane);
            return self.give_up(&id);
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
            Err(e) => self.show_failure(pane, &agent, &e.to_string())?,
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

    /// Melt an agent's program away, remove its pane once the melt ends, and
    /// lay the panes out again. An agent without a pane is already closed,
    /// and a second close of a melting agent does nothing.
    fn close(&mut self, id: &str) -> Result<()> {
        let Some(i) = self.panes.iter().position(|s| s.agent == id) else {
            return Ok(());
        };
        let pane = self.panes[i].pane;
        self.mux.melt_pane(pane);
        // A pane hidden by a zoom or a screen is removed at once.
        if self.mux.pane(pane).is_none() {
            self.finish_close(pane)?;
        }
        Ok(())
    }

    /// Drop a closed pane's agent slot, end the zoom if the pane was in the
    /// viewed team, and lay the panes out again. When the viewed team's last
    /// pane closes, as when `delete_task` deletes it, view the default team.
    fn finish_close(&mut self, pane: PaneId) -> Result<()> {
        let viewed = self
            .panes
            .iter()
            .any(|s| s.pane == pane && s.team == *self.view.borrow());
        self.panes.retain(|s| s.pane != pane);
        if viewed {
            self.zoomed.set(None);
            let view = self.view.borrow().clone();
            if view != store::DEFAULT_TEAM && self.panes.iter().all(|s| s.team != view) {
                return self.view_team(store::DEFAULT_TEAM);
            }
        }
        self.lay_out()
    }

    /// Lay the panes out for the terminal's size: the viewed team's, zoomed
    /// or not, and every other team's as they would be placed if it were
    /// viewed, so their programs run at the size they will be shown at. A
    /// team too big for the terminal is hidden until it grows.
    fn lay_out(&mut self) -> Result<()> {
        let size = self.mux.size();
        let view = self.view.borrow().clone();
        let mut teams: Vec<&str> = Vec::new();
        for slot in &self.panes {
            if !teams.contains(&slot.team.as_str()) {
                teams.push(&slot.team);
            }
        }
        let mut all = Vec::new();
        for team in teams {
            let weights = self.weights(team);
            let ids: Vec<PaneId> = weights.iter().map(|&(id, _)| id).collect();
            let layout = match self.zoomed.get() {
                Some(zoomed) if team == view => zoom_layout(size, &ids, zoomed),
                _ => layout(size, &weights, self.split),
            };
            all.extend(layout.unwrap_or_else(|| hidden(&ids)));
        }
        self.place(&all)
    }

    /// Size and place the panes, then hide every team's but the viewed
    /// one's, or every pane while the overlay, the list of keys, or the team
    /// list covers the screen.
    fn place(&mut self, layout: &[(PaneId, Rect)]) -> Result<()> {
        self.mux.set_layout(layout)?;
        let covered =
            self.overlay.borrow().is_some() || self.help.get() || self.teamlist.borrow().is_some();
        let view = self.view.borrow();
        let hide: Vec<PaneId> = self
            .panes
            .iter()
            .filter(|s| covered || s.team != *view)
            .map(|s| s.pane)
            .collect();
        drop(view);
        if !hide.is_empty() {
            self.mux.set_layout(&hidden(&hide))?;
        }
        Ok(())
    }

    /// View `team`: show its panes, focusing its first live one, and put
    /// every other team's in the background. Ends any zoom.
    fn view_team(&mut self, team: &str) -> Result<()> {
        *self.view.borrow_mut() = team.to_string();
        if let Some(teamlist) = &mut *self.teamlist.borrow_mut() {
            teamlist.view = team.to_string();
        }
        self.zoomed.set(None);
        self.drag = None;
        for slot in &self.panes {
            self.mux.set_background(slot.pane, slot.team != team);
        }
        for slot in self.panes.iter().filter(|s| s.team == team) {
            self.mux.set_focus(slot.pane);
            if self.mux.focus() == Some(slot.pane) {
                break;
            }
        }
        self.mux.invalidate_chrome();
        self.lay_out()
    }

    /// Open the team list in place of the overlay, the list of keys, or the
    /// prompt, or close it.
    fn toggle_teamlist(&mut self) -> Result<()> {
        if self.teamlist.borrow().is_some() {
            return self.close_screens();
        }
        self.close_screens()?;
        let view = self.view.borrow().clone();
        let teamlist = match TeamList::load(&self.state_path, &view) {
            Ok(teamlist) => teamlist,
            // The list can't show without its teams; the error goes to the
            // log.
            Err(e) => {
                eprintln!("hypercube: reading the teams: {e}");
                return Ok(());
            }
        };
        *self.teamlist.borrow_mut() = Some(teamlist);
        self.teamlist_read = now_ms();
        self.mux.set_captured(Some(Captured::Teams));
        self.lay_out()
    }

    /// Read the team list again.
    fn read_teamlist(&mut self) {
        if let Some(teamlist) = &mut *self.teamlist.borrow_mut()
            && let Err(e) = teamlist.reload()
        {
            teamlist.error = e.to_string();
        }
        self.teamlist_read = now_ms();
        self.mux.invalidate_chrome();
    }

    /// Carry out a key pressed in the team list. Viewing a team or creating
    /// one closes the list, and a failure shows in it.
    fn teamlist_key(&mut self, key: &[u8]) -> Result<()> {
        let action = match &mut *self.teamlist.borrow_mut() {
            Some(teamlist) => teamlist.key(key),
            None => return Ok(()),
        };
        let result = match action {
            teamlist::Action::None => return Ok(()),
            teamlist::Action::Move => Ok(()),
            teamlist::Action::View(team) => {
                self.close_screens()?;
                return self.view_team(&team);
            }
            teamlist::Action::Create(name) => match self.create_team(&name) {
                Ok(()) => return self.close_screens(),
                Err(e) => Err(e),
            },
            teamlist::Action::Delete(name) => self.delete_team(&name),
            teamlist::Action::Close => return self.close_screens(),
        };
        if let Err(e) = result
            && let Some(teamlist) = &mut *self.teamlist.borrow_mut()
        {
            teamlist.error = e.to_string();
        }
        self.read_teamlist();
        Ok(())
    }

    /// Create a team named `name` whose orchestrator works in the checkout,
    /// launch the orchestrator, and view the team. If the launch fails, the
    /// team is removed again.
    fn create_team(&mut self, name: &str) -> Result<()> {
        let config = config::load()?;
        let dir = workspace::utf8(&self.root)?;
        let branch = current_branch(&self.root).unwrap_or_default();
        let agent = store::update(&self.state_path, |s| {
            add_team(s, name, &config.orchestrator, dir, &branch)
        })?;
        if let Err(e) = self.launch(&agent.id, "", false) {
            store::update(&self.state_path, |s| {
                s.remove(&agent.id);
                Ok(())
            })?;
            return Err(e);
        }
        self.read_bar();
        self.view_team(name)
    }

    /// Delete the team `name` and its orchestrator, and its scheduled task if
    /// it has one, closing the orchestrator's pane. The default team and a
    /// team with workers stay. Deleting the viewed team views the default
    /// team.
    fn delete_team(&mut self, name: &str) -> Result<()> {
        let state = store::load(&self.state_path)?;
        let id = deletable(&state, name)?.id.clone();
        self.close(&id)?;
        // The orchestrator may have created a worker since the check.
        store::update(&self.state_path, |s| remove_team(s, name))?;
        self.read_bar();
        if *self.view.borrow() == name {
            self.view_team(store::DEFAULT_TEAM)?;
        }
        Ok(())
    }
}

/// Each team of `state`, in `State::teams` order, and whether any of its
/// agents waits for a tool approval.
fn team_attention(state: &State) -> Vec<(String, bool)> {
    state
        .teams()
        .into_iter()
        .map(|team| {
            let attention = state.agents.iter().any(|a| a.team == team && a.attention);
            (team.to_string(), attention)
        })
        .collect()
}

/// Add the orchestrator of a new team named `name`, working in the checkout
/// `dir` on `branch`. The name must be valid, and no team or agent may have
/// it already.
pub(crate) fn add_team(
    s: &mut State,
    name: &str,
    spec: &config::Spec,
    dir: &str,
    branch: &str,
) -> Result<Agent> {
    if !mcp::valid_name(name) {
        return Err("a team's name must be lowercase letters, digits, and hyphens".into());
    }
    if name == store::DEFAULT_TEAM || s.teams().contains(&name) {
        return Err(format!("a team named {name} already exists").into());
    }
    if s.agent(name).is_ok() {
        return Err(format!("an agent named {name} already exists").into());
    }
    let mut agent = new_orchestrator(spec, name, name, dir)?;
    agent.branch = branch.into();
    agent.activity = store::IDLE.into();
    s.agents.push(agent.clone());
    Ok(agent)
}

/// The names of the workers of `team`.
fn team_workers<'a>(s: &'a State, team: &str) -> Vec<&'a str> {
    s.agents
        .iter()
        .filter(|a| a.role == store::WORKER && a.team == team)
        .map(|a| a.name.as_str())
        .collect()
}

/// The orchestrator of `team`, if the team can be deleted: it is not the
/// default team, and it has no workers.
pub(crate) fn deletable<'a>(s: &'a State, team: &str) -> Result<&'a Agent> {
    if team == store::DEFAULT_TEAM {
        return Err("the default team can't be deleted".into());
    }
    let orchestrator = s
        .team_orchestrator(team)
        .ok_or_else(|| format!("no team {team:?}"))?;
    let workers = team_workers(s, team);
    if !workers.is_empty() {
        return Err(format!(
            "{team} has workers: {}; remove them first",
            workers.join(", ")
        )
        .into());
    }
    Ok(orchestrator)
}

/// Remove the orchestrator of `team`, whose pane has been closed, its
/// queued messages, and the team's scheduled task, unless the team has
/// workers. The team list's x and `delete_task` both delete a team this way.
pub(crate) fn remove_team(s: &mut State, team: &str) -> Result<()> {
    let workers = team_workers(s, team);
    if !workers.is_empty() {
        return Err(format!(
            "closed the pane of {team}'s orchestrator, but {team} has workers: {}; remove them first",
            workers.join(", ")
        )
        .into());
    }
    let id = deletable(s, team)?.id.clone();
    s.remove(&id);
    s.tasks.retain(|t| t.name != team);
    Ok(())
}

/// How long until the next minute of the wall clock begins, in
/// milliseconds.
fn next_minute_ms() -> i64 {
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default();
    60_000 - (now.as_millis() % 60_000) as i64
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

    /// `n` panes, the workers expanded with equal weights.
    fn even(n: u32) -> Vec<(PaneId, Option<u32>)> {
        ids(n).into_iter().map(|id| (id, Some(1))).collect()
    }

    const EVEN: (u32, u32) = (1, 1);

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
    fn a_lone_orchestrator_fills_the_area_between_the_header_and_the_bar() {
        assert_eq!(
            rects(layout(size(80, 24), &even(1), EVEN)),
            [rect(0, 2, 80, 21)]
        );
        assert_eq!(
            rects(layout(size(80, 24), &even(1), (1, 3))),
            [rect(0, 2, 80, 21)]
        );
        assert_eq!(layout(size(80, 24), &[], EVEN), Some(Vec::new()));
    }

    #[test]
    fn workers_stack_right_of_the_orchestrator() {
        // Reserve the divider, then give half the columns to the orchestrator,
        // rounded down. Every pane has a label row above its rect.
        assert_eq!(
            rects(layout(size(100, 25), &even(2), EVEN)),
            [rect(0, 2, 49, 22), rect(50, 2, 50, 22)]
        );
        // 23 rows over three workers: blocks of 7, 8 and 8 rows, each a label
        // row and the rest content.
        assert_eq!(
            rects(layout(size(100, 25), &even(4), EVEN)),
            [
                rect(0, 2, 49, 22),
                rect(50, 2, 50, 6),
                rect(50, 9, 50, 7),
                rect(50, 17, 50, 7),
            ]
        );
        // Uneven heights differ by at most a row and fill the column.
        let layout = rects(layout(size(81, 32), &even(5), EVEN));
        assert_eq!(layout[0], rect(0, 2, 40, 29));
        let heights: Vec<u16> = layout[1..].iter().map(|r| r.rows + 1).collect();
        assert_eq!(heights, [7, 8, 7, 8]);
        assert_eq!(layout[4].row + layout[4].rows, 31);
    }

    #[test]
    fn the_split_and_worker_weights_size_the_panes() {
        let panes = [
            (PaneId(1), None),
            (PaneId(2), Some(3)),
            (PaneId(3), Some(1)),
        ];
        // 99 columns beside the divider, a quarter to the orchestrator. The
        // 23 rows less two label rows and two first rows leave 19, split 14
        // to 5.
        assert_eq!(
            rects(layout(size(100, 25), &panes, (1, 3))),
            [rect(0, 2, 24, 22), rect(25, 2, 75, 15), rect(25, 18, 75, 6)]
        );
        // Workers of weight 0 get a row each, and the rest goes to the others.
        let panes = [
            (PaneId(1), None),
            (PaneId(2), Some(0)),
            (PaneId(3), Some(5)),
        ];
        assert_eq!(
            rects(layout(size(100, 25), &panes, EVEN))[1..],
            [rect(50, 2, 50, 1), rect(50, 4, 50, 20)]
        );
        // Every pane keeps a column, however far the split goes.
        assert_eq!(rects(layout(size(100, 25), &even(2), (1, 0)))[0].cols, 98);
        assert_eq!(rects(layout(size(100, 25), &even(2), (0, 1)))[0].cols, 1);
    }

    #[test]
    fn a_collapsed_worker_keeps_only_its_label_row() {
        let panes = [
            (PaneId(1), None),
            (PaneId(2), Some(1)),
            (PaneId(3), None),
            (PaneId(4), Some(1)),
        ];
        // The collapsed worker's label sits right below the first worker,
        // and the label of the next one right below it.
        assert_eq!(
            rects(layout(size(100, 25), &panes, EVEN))[1..],
            [
                rect(50, 2, 50, 10),
                rect(50, 13, 50, 0),
                rect(50, 14, 50, 10)
            ]
        );
        // A collapsed worker needs no content row.
        assert!(layout(size(80, 7), &panes, EVEN).is_some());
        assert!(layout(size(80, 6), &panes, EVEN).is_none());
    }

    #[test]
    fn shares_add_up_to_the_total() {
        assert_eq!(shares(10, &[1, 1, 1]), [3, 3, 4]);
        assert_eq!(shares(10, &[0, 0]), [5, 5]);
        assert_eq!(shares(10, &[0, 2, 3]), [0, 4, 6]);
        assert_eq!(shares(0, &[1, 2]), [0, 0]);
        assert_eq!(shares(7, &[]), Vec::<u32>::new());
    }

    #[test]
    fn a_layout_without_room_for_every_pane_fails() {
        // The orchestrator needs a label and content row between header and bar.
        assert!(layout(size(80, 3), &even(1), EVEN).is_none());
        assert!(layout(size(80, 4), &even(1), EVEN).is_some());
        // Each worker needs a label row and a content row.
        assert!(layout(size(80, 7), &even(4), EVEN).is_none());
        assert!(layout(size(80, 8), &even(4), EVEN).is_some());
        // A column for the divider, and one for each pane.
        assert!(layout(size(2, 24), &even(2), EVEN).is_none());
        assert!(layout(size(3, 24), &even(2), EVEN).is_some());
        assert!(layout(size(0, 24), &even(1), EVEN).is_none());
        assert!(layout(size(1, 24), &even(1), EVEN).is_some());
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
                    rect(0, 2, 100, 22)
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
            workspace: String::new(),
            view: Rc::new(RefCell::new(store::DEFAULT_TEAM.into())),
            teams: Rc::default(),
            tasks: Rc::default(),
            teamlist: Rc::default(),
            attention: Rc::default(),
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
    fn a_lone_orchestrator_has_a_label_row_and_no_divider() {
        let layout = rects(layout(size(20, 5), &even(1), EVEN));
        let panes = [view(1, "orch", layout[0], true, None)];
        let screen = draw_with(&mut chrome(), &panes, 20, 5);
        let rows: Vec<String> = (1..4).map(|y| screen.row(y)).collect();
        let blank = " ".repeat(20);
        assert_eq!(
            rows,
            [format!("━ orch {}", "━".repeat(13)), blank.clone(), blank]
        );
    }

    #[test]
    fn chrome_draws_the_header_rule_titles_and_a_divider_that_branches_into_them() {
        let layout = rects(layout(size(40, 10), &even(3), EVEN));
        let panes = [
            PaneView {
                detail: "\u{e725} main",
                ..view(1, "orch", layout[0], false, None)
            },
            PaneView {
                scrolled: 42,
                detail: "\u{e725} w1",
                ..view(2, "w1", layout[1], false, None)
            },
            PaneView {
                detail: "\u{e725} w2",
                ..view(3, "w2", layout[2], true, Some(3))
            },
        ];
        let mut chrome = chrome();
        chrome.workspace = "~/src/r".into();
        let screen = draw_with(&mut chrome, &panes, 40, 10);
        let rows: Vec<String> = (0..10).map(|y| screen.row(y)).collect();
        let (sp, bar) = (|n| " ".repeat(n), |n| "━".repeat(n));
        let side = format!("{}┃{}", sp(19), sp(20));
        assert_eq!(
            rows,
            [
                format!(" \u{f07c} ~/src/r{}", sp(30)),
                // A worker's detail doesn't fit beside its arrow and notes.
                format!(
                    "━ orch {} \u{e725} main ━┳━ \u{f47c} w1 · ↑42 {}",
                    bar(3),
                    bar(7)
                ),
                side.clone(),
                side.clone(),
                side.clone(),
                // The detail doesn't fit.
                format!("{}┣━ \u{f47c} w2 · \u{f057} 3 {}", sp(19), bar(7)),
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
            (19, 1),
            (19, 3),
            (20, 1),
            (34, 1),
            (39, 1),
            (19, 5),
            (38, 5),
        ] {
            assert_eq!(fg(&screen, x, y), blue, "foreground at {x},{y}");
            assert_eq!(screen.bg(x, y), StyleColor::None, "background at {x},{y}");
        }
        for (x, y) in [(22, 1), (25, 1), (22, 5), (3, 1)] {
            assert_eq!(
                fg(&screen, x, y),
                rgb(palette.text),
                "foreground at {x},{y}"
            );
        }
        assert!(!screen.bold(22, 1));
        assert!(screen.bold(22, 5));
        // Nothing but the chrome is painted: the panes' cells and the gaps
        // have the terminal's own background.
        assert_eq!(screen.bg(5, 5), StyleColor::None);
        assert_eq!(screen.bg(30, 3), StyleColor::None);
    }

    #[test]
    fn a_collapsed_workers_label_points_right_and_the_next_label_follows_it() {
        let weights = [(PaneId(1), None), (PaneId(2), None), (PaneId(3), Some(1))];
        let layout = rects(layout(size(40, 10), &weights, EVEN));
        let panes = [
            view(1, "orch", layout[0], false, None),
            view(2, "w1", layout[1], false, None),
            view(3, "w2", layout[2], false, None),
        ];
        let screen = draw_with(&mut chrome(), &panes, 40, 10);
        let bar = |n| "━".repeat(n);
        assert_eq!(
            screen.row(1),
            format!("━ orch {}┳━ \u{f460} w1 {}", bar(12), bar(13))
        );
        assert_eq!(
            screen.row(2),
            format!("{}┣━ \u{f47c} w2 {}", " ".repeat(19), bar(13))
        );
        let arrow = usize::from(arrow_col(layout[1]));
        assert_eq!(screen.row(1).chars().nth(arrow), Some('\u{f460}'));
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
        assert_eq!(screen.row(1), format!("━ w1 · \u{f065} {}", "━".repeat(21)));
        assert_eq!(screen.row(2), " ".repeat(30));
    }

    #[test]
    fn the_orchestrator_keeps_its_label_and_notes_while_zoomed() {
        let layout = rects(zoom_layout(size(30, 8), &ids(2), PaneId(1)));
        let panes = [
            PaneView {
                scrolled: 5,
                detail: "\u{e725} main",
                ..view(1, "orch", layout[0], true, Some(3))
            },
            view(2, "w1", layout[1], false, None),
        ];
        let mut chrome = chrome();
        chrome.zoomed.set(Some(PaneId(1)));
        chrome.workspace = "~/src/r".into();
        let screen = draw_with(&mut chrome, &panes, 30, 8);
        assert_eq!(
            screen.row(0),
            format!(" \u{f07c} ~/src/r{}", " ".repeat(20))
        );
        assert_eq!(
            screen.row(1),
            format!("━ orch · \u{f065} · \u{f057} 3 · ↑5 {}", "━".repeat(8))
        );
        assert!(screen.bold(2, 1));
        assert!(!screen.bold(0, 0));
    }

    #[test]
    fn chrome_marks_the_zoomed_pane() {
        let layout = rects(zoom_layout(size(23, 8), &ids(2), PaneId(2)));
        let panes = [
            view(1, "orch", layout[0], false, None),
            PaneView {
                detail: "\u{e725} w1",
                ..view(2, "w1", layout[1], true, Some(3))
            },
        ];
        let mut chrome = chrome();
        chrome.zoomed.set(Some(PaneId(2)));
        let screen = draw_with(&mut chrome, &panes, 23, 8);
        assert_eq!(screen.row(0), format!(" \u{f07c}{}", " ".repeat(21)));
        assert_eq!(
            screen.row(1),
            "━ w1 · \u{f065} · \u{f057} 3 ━ \u{e725} w1 ━"
        );
        assert_eq!(screen.row(2), " ".repeat(23));
    }

    #[test]
    fn chrome_drops_the_detail_before_clipping_the_name() {
        let row =
            |label, cols: u16| worker_row(label, "~/src/r", rect(1, 2, cols, 2), cols + 1, 6, 1);
        // The detail needs a blank column between it and the name.
        assert_eq!(row("orch", 20), "┳━ \u{f47c} orch ━ ~/src/r ━");
        assert_eq!(row("orch", 19), "┳━ \u{f47c} orch ━━━━━━━━━━");
        assert_eq!(row("orchestrator", 10), "┳━ \u{f47c} orche━");

        let hidden = [
            view(1, "orch", Rect::default(), true, None),
            view(2, "w1", Rect::default(), false, None),
        ];
        let screen = draw_with(&mut chrome(), &hidden, 8, 4);
        assert_eq!(screen.row(0), " \u{f07c}      ");
        assert_eq!(screen.row(1), "        ");
    }

    #[test]
    fn chrome_measures_wide_and_combining_characters_in_columns() {
        let row = worker_row("界界界界界", "", rect(1, 2, 11, 2), 12, 6, 1);
        // Nothing wraps, so nothing scrolls, and spacer cells read as blanks.
        assert_eq!(row, "┳━ \u{f47c} 界 界 界 ━");
        assert_eq!(fit("e\u{301}x", 1), "e\u{301}");
        assert_eq!(fit("界", 1), "");
    }

    #[test]
    fn title_attention_dots_follow_names_before_notes_and_keep_focus_style() {
        let layout = rects(layout(size(80, 12), &even(3), EVEN));
        let panes = [
            PaneView {
                detail: "\u{e725} main",
                scrolled: 2,
                ..view(1, "orch", layout[0], true, None)
            },
            PaneView {
                detail: "\u{e725} w1",
                ..view(2, "w1", layout[1], false, Some(3))
            },
            view(3, "w2", layout[2], false, None),
        ];
        let mut chrome = chrome();
        *chrome.attention.borrow_mut() = vec![PaneId(1), PaneId(2)];
        let screen = draw_with(&mut chrome, &panes, 80, 12);
        assert!(screen.row(1).starts_with("━ orch \u{f0f3} · ↑2 "));
        assert!(
            screen
                .row(1)
                .contains("┳━ \u{f47c} w1 \u{f0f3} · \u{f057} 3 ")
        );
        for (x, focused) in [(7, true), (47, false)] {
            assert_eq!(fg(&screen, x, 1), rgb(colors::ATTENTION));
            assert_eq!(screen.bold(x, 1), focused);
        }
        assert!(!screen.row(6).contains('\u{f0f3}'));
        chrome.attention.borrow_mut().clear();
        let screen = draw_with(&mut chrome, &panes, 80, 12);
        assert!(!screen.row(1).contains('\u{f0f3}'));
    }

    #[test]
    fn title_failures_and_exit_statuses_are_drawn_in_the_error_color() {
        let layout = rects(layout(size(80, 12), &even(2), EVEN));
        let label = format!("w1 · {} boom", icons::FAILED);
        let panes = [
            view(1, "orch", layout[0], true, None),
            view(2, &label, layout[1], false, Some(127)),
        ];
        let screen = draw_with(&mut chrome(), &panes, 80, 12);
        let row = screen.row(1);
        let x = |c| row.chars().position(|r| r == c).unwrap() as u16;
        for c in [icons::FAILED, 'b', icons::EXITED, '7'] {
            assert_eq!(fg(&screen, x(c), 1), rgb(colors::ERROR), "{c}");
        }
        assert_ne!(fg(&screen, x('w'), 1), rgb(colors::ERROR));
    }

    #[test]
    fn title_attention_dots_are_only_drawn_if_the_whole_dot_fits() {
        let mut chrome = chrome();
        *chrome.attention.borrow_mut() = vec![PaneId(1)];
        let row = |chrome: &mut TeamChrome, cols| {
            let panes = [view(1, "界x", rect(0, 2, cols, 1), true, None)];
            draw_with(chrome, &panes, cols, 4)
        };
        let screen = row(&mut chrome, 8);
        assert_eq!(screen.row(1), "━ 界 x \u{f0f3}━");
        assert_eq!(fg(&screen, 6, 1), rgb(colors::ATTENTION));
        let screen = row(&mut chrome, 7);
        assert_eq!(screen.row(1), "━ 界 x ━");
        assert!(!screen.row(1).contains('\u{f0f3}'));
    }

    #[test]
    fn the_header_names_the_viewed_team_and_the_others_waiting() {
        let mut chrome = chrome();
        chrome.workspace = "~/src/r".into();
        let header = |chrome: &mut TeamChrome, cols| draw_with(chrome, &[], cols, 4);
        // With one team, the header shows only the directory.
        *chrome.teams.borrow_mut() = vec![("default".into(), true)];
        let screen = header(&mut chrome, 50);
        assert_eq!(screen.row(0).trim_end(), " \u{f07c} ~/src/r");

        *chrome.teams.borrow_mut() = vec![
            ("default".into(), true),
            ("docs".into(), true),
            ("api".into(), false),
            ("review".into(), true),
        ];
        *chrome.view.borrow_mut() = "docs".into();
        let screen = header(&mut chrome, 50);
        let row = screen.row(0);
        assert_eq!(
            row,
            format!(
                " \u{f07c} ~/src/r › docs{}default \u{f0f3} · review \u{f0f3} ",
                " ".repeat(12)
            )
        );
        let palette = Palette::default();
        let x = |c, nth| {
            row.chars()
                .enumerate()
                .filter(|&(_, r)| r == c)
                .nth(nth)
                .unwrap()
                .0 as u16
        };
        for nth in 0..2 {
            let bell = x('\u{f0f3}', nth);
            assert_eq!(fg(&screen, bell, 0), rgb(colors::ATTENTION));
            assert_eq!(screen.bg(bell, 0), rgb(palette.bar.bg));
        }
        assert_eq!(fg(&screen, x('r', 1), 0), rgb(palette.bar.fg));
        // The waiting teams don't fit beside the name.
        let screen = header(&mut chrome, 30);
        assert_eq!(
            screen.row(0),
            format!(" \u{f07c} ~/src/r › docs{}", " ".repeat(13))
        );
        // The viewed team is never listed.
        *chrome.view.borrow_mut() = "default".into();
        let row = header(&mut chrome, 60).row(0);
        assert!(row.contains("docs \u{f0f3} · review \u{f0f3} "), "{row}");
        assert!(!row.contains("default \u{f0f3}"), "{row}");
    }

    #[test]
    fn the_header_shows_the_task_counts_before_the_waiting_teams() {
        let mut chrome = chrome();
        chrome.workspace = "~/src/r".into();
        *chrome.teams.borrow_mut() = vec![("default".into(), false), ("nightly".into(), false)];
        chrome.tasks.set((1, 0));
        let row = draw_with(&mut chrome, &[], 60, 4).row(0);
        assert!(row.ends_with(" \u{f017} 1 task, 0 running "), "{row}");

        *chrome.teams.borrow_mut() = vec![("default".into(), false), ("nightly".into(), true)];
        chrome.tasks.set((2, 1));
        let screen = draw_with(&mut chrome, &[], 70, 4);
        let row = screen.row(0);
        assert!(
            row.ends_with(" \u{f017} 2 tasks, 1 running · nightly \u{f0f3} "),
            "{row}"
        );
        let bell = row.chars().position(|c| c == '\u{f0f3}').unwrap() as u16;
        assert_eq!(fg(&screen, bell, 0), rgb(colors::ATTENTION));
        assert_eq!(fg(&screen, bell - 2, 0), rgb(Palette::default().bar.fg));
        // Without tasks, the counts are gone.
        chrome.tasks.set((0, 0));
        let row = draw_with(&mut chrome, &[], 70, 4).row(0);
        assert!(!row.contains("task"), "{row}");
    }

    #[test]
    fn labels_show_the_branch_and_otherwise_the_directory() {
        let mut a = Agent {
            role: store::WORKER.into(),
            branch: "hypercube/parser".into(),
            dir: "/r/.hypercube/parser".into(),
            ..Agent::default()
        };
        assert_eq!(detail(&a), "\u{e725} parser");
        a.branch = "feature/x".into();
        assert_eq!(detail(&a), "\u{e725} feature/x");
        a.branch.clear();
        assert_eq!(detail(&a), "\u{f07b} /r/.hypercube/parser");
        a.role = store::ORCHESTRATOR.into();
        a.branch = "main".into();
        a.dir = "/r".into();
        assert_eq!(detail(&a), "\u{e725} main");
        a.branch = "hypercube/main".into();
        assert_eq!(detail(&a), "\u{e725} hypercube/main");
        a.branch.clear();
        assert_eq!(detail(&a), "\u{f07b} /r");
    }

    #[test]
    fn current_branch_reads_normal_and_linked_worktree_heads() {
        let dir = TempDir::new();
        let root = dir.path();
        let git = root.join(".git");
        fs::create_dir(&git).unwrap();
        fs::write(git.join("HEAD"), "ref: refs/heads/main\n").unwrap();
        assert_eq!(current_branch(root).as_deref(), Some("main"));
        fs::write(git.join("HEAD"), "ref: refs/heads/feature/title\n").unwrap();
        assert_eq!(current_branch(root).as_deref(), Some("feature/title"));
        fs::write(git.join("HEAD"), "0123456789abcdef\n").unwrap();
        assert_eq!(current_branch(root).as_deref(), Some(""));

        let linked = root.join("linked");
        fs::create_dir(&linked).unwrap();
        for path in [git.display().to_string(), "../.git".into()] {
            fs::write(linked.join(".git"), format!("gitdir: {path}\n")).unwrap();
            fs::write(git.join("HEAD"), "ref: refs/heads/linked\n").unwrap();
            assert_eq!(current_branch(&linked).as_deref(), Some("linked"));
        }
        assert_eq!(current_branch(&root.join("missing")), None);
    }

    fn member(id: &str, role: &str, team: &str) -> Agent {
        Agent {
            id: id.into(),
            name: id.into(),
            role: role.into(),
            team: team.into(),
            ..Agent::default()
        }
    }

    /// A host of a workspace in `dir` whose state has `agents`, each in a
    /// pane without a program, in a terminal of `cols` by `rows`.
    fn host_of(dir: &TempDir, agents: &[Agent], cols: u16, rows: u16) -> Host {
        let state_path = dir.path().join("w.json");
        store::update(&state_path, |s| {
            s.agents = agents.to_vec();
            Ok(())
        })
        .unwrap();
        let socket = Socket::listen(&state_path).unwrap();
        // No signal arrives, and the loop doesn't run.
        let mut host = Host::new(dir.path(), state_path, socket, -1, Vec::new()).unwrap();
        host.mux.resize(size(cols, rows));
        for a in agents {
            host.add_failed(a, "no program").unwrap();
        }
        host
    }

    fn pane_rect(host: &Host, agent: &str) -> Rect {
        host.mux.pane(host.pane_of(agent).unwrap()).unwrap().rect()
    }

    fn pane_size(host: &Host, agent: &str) -> (u16, u16) {
        let ws = host.mux.pane(host.pane_of(agent).unwrap()).unwrap().ws();
        (ws.ws_col, ws.ws_row)
    }

    #[test]
    fn ready_puts_agents_without_a_team_in_the_default_team() {
        let spec = config::Spec {
            harness: "claude".into(),
            model: "opus".into(),
            effort: "high".into(),
        };
        let mut s = State::default();
        let agents = ready(&mut s, &spec, "/r", Some("main")).unwrap();
        assert_eq!(agents.len(), 1);
        let o = &s.agents[0];
        assert_eq!(
            (o.name.as_str(), o.role.as_str(), o.team.as_str()),
            ("orchestrator", store::ORCHESTRATOR, store::DEFAULT_TEAM)
        );
        assert_eq!((o.dir.as_str(), o.branch.as_str()), ("/r", "main"));
        assert_eq!((o.harness.as_str(), o.model.as_str()), ("claude", "opus"));

        let mut s = State {
            agents: vec![
                member("w", store::WORKER, ""),
                member("d", store::ORCHESTRATOR, "docs"),
                member("o", store::ORCHESTRATOR, ""),
            ],
            ..State::default()
        };
        let agents = ready(&mut s, &spec, "/r", None).unwrap();
        let order: Vec<&str> = agents.iter().map(|a| a.id.as_str()).collect();
        assert_eq!(order, ["d", "o", "w"]);
        let teams: Vec<&str> = s.agents.iter().map(|a| a.team.as_str()).collect();
        assert_eq!(teams, [store::DEFAULT_TEAM, "docs", store::DEFAULT_TEAM]);
        assert!(s.no_git);
        assert_eq!(s.agents[1].activity, store::IDLE);
        assert_eq!(s.agents[0].activity, store::BUSY);
    }

    #[test]
    fn background_teams_are_laid_out_at_their_own_size_and_hidden() {
        let dir = TempDir::new();
        let agents = [
            member("o", store::ORCHESTRATOR, store::DEFAULT_TEAM),
            member("w", store::WORKER, store::DEFAULT_TEAM),
            member("d", store::ORCHESTRATOR, "docs"),
        ];
        let mut host = host_of(&dir, &agents, 80, 24);
        // The default team is viewed; the docs team's lone orchestrator has
        // the size it would have at full width.
        assert_eq!(pane_rect(&host, "o"), rect(0, 2, 39, 21));
        assert_eq!(pane_rect(&host, "w"), rect(40, 2, 40, 21));
        assert_eq!(pane_rect(&host, "d"), Rect::default());
        assert_eq!(pane_size(&host, "d"), (80, 21));

        host.view_team("docs").unwrap();
        assert_eq!(pane_rect(&host, "d"), rect(0, 2, 80, 21));
        for id in ["o", "w"] {
            assert_eq!(pane_rect(&host, id), Rect::default());
        }
        assert_eq!(pane_size(&host, "o"), (39, 21));
        assert_eq!(pane_size(&host, "w"), (40, 21));
        assert_eq!(host.rects(), [rect(0, 2, 80, 21)]);

        // Every team takes the new size.
        host.mux.resize(size(100, 30));
        host.lay_out().unwrap();
        assert_eq!(pane_rect(&host, "d"), rect(0, 2, 100, 27));
        assert_eq!(pane_size(&host, "o"), (49, 27));
        // A screen hides every pane.
        host.toggle_help().unwrap();
        assert_eq!(pane_rect(&host, "d"), Rect::default());
        host.toggle_help().unwrap();
        assert_eq!(pane_rect(&host, "d"), rect(0, 2, 100, 27));
    }

    #[test]
    fn viewing_a_team_moves_the_focus_into_it() {
        let dir = TempDir::new();
        let agents = [member("o", store::ORCHESTRATOR, store::DEFAULT_TEAM)];
        let mut host = host_of(&dir, &agents, 80, 24);
        // A live pane of another team, added in the background.
        let d = member("d", store::ORCHESTRATOR, "docs");
        let pane = host.add_labeled_pane(&d).unwrap();
        host.panes.push(Slot::new(&d, pane, 1));
        assert_eq!(host.mux.focus(), host.pane_of("o"));
        host.view_team("docs").unwrap();
        assert_eq!(host.mux.focus(), Some(pane));
        host.view_team(store::DEFAULT_TEAM).unwrap();
        assert_eq!(host.mux.focus(), None);
        assert!(host.mux.pane(pane).unwrap().rect() == Rect::default());
    }

    fn spec() -> config::Spec {
        config::Spec {
            harness: "claude".into(),
            ..config::Spec::default()
        }
    }

    #[test]
    fn add_team_refuses_taken_and_invalid_names() {
        let mut s = State {
            agents: vec![
                member("orchestrator", store::ORCHESTRATOR, store::DEFAULT_TEAM),
                member("w1", store::WORKER, store::DEFAULT_TEAM),
                member("docs", store::ORCHESTRATOR, "docs"),
            ],
            ..State::default()
        };
        let before = s.clone();
        for (name, error) in [
            (
                "Docs",
                "a team's name must be lowercase letters, digits, and hyphens",
            ),
            (
                "",
                "a team's name must be lowercase letters, digits, and hyphens",
            ),
            ("default", "a team named default already exists"),
            ("docs", "a team named docs already exists"),
            ("w1", "an agent named w1 already exists"),
            ("orchestrator", "an agent named orchestrator already exists"),
        ] {
            let err = add_team(&mut s, name, &spec(), "/r", "main").unwrap_err();
            assert_eq!(err.to_string(), error);
        }
        assert_eq!(s, before);

        let agent = add_team(&mut s, "api", &spec(), "/r", "main").unwrap();
        assert_eq!(s.agents.last(), Some(&agent));
        assert_eq!(
            (
                agent.name.as_str(),
                agent.team.as_str(),
                agent.role.as_str()
            ),
            ("api", "api", store::ORCHESTRATOR)
        );
        assert_eq!((agent.dir.as_str(), agent.branch.as_str()), ("/r", "main"));
        assert_eq!(s.teams(), [store::DEFAULT_TEAM, "docs", "api"]);
    }

    #[test]
    fn a_team_with_workers_is_not_removed() {
        let mut s = State {
            agents: vec![
                member("orchestrator", store::ORCHESTRATOR, store::DEFAULT_TEAM),
                member("docs", store::ORCHESTRATOR, "docs"),
            ],
            messages: vec![store::Message {
                to: "docs".into(),
                ..store::Message::default()
            }],
            ..State::default()
        };
        assert_eq!(
            deletable(&s, store::DEFAULT_TEAM).unwrap_err().to_string(),
            "the default team can't be deleted"
        );
        assert_eq!(deletable(&s, "docs").unwrap().id, "docs");
        // A worker created between the check and the update keeps the team.
        s.agents.push(member("writer", store::WORKER, "docs"));
        let before = s.clone();
        assert_eq!(
            remove_team(&mut s, "docs").unwrap_err().to_string(),
            "closed the pane of docs's orchestrator, but docs has workers: writer; remove them first"
        );
        assert_eq!(s, before);
        assert_eq!(
            deletable(&s, "docs").unwrap_err().to_string(),
            "docs has workers: writer; remove them first"
        );
        s.remove("writer");
        remove_team(&mut s, "docs").unwrap();
        assert_eq!(s.teams(), [store::DEFAULT_TEAM]);
        assert!(s.messages.is_empty());
    }

    #[test]
    fn deleting_the_viewed_team_closes_its_pane_and_views_the_default_team() {
        let dir = TempDir::new();
        let agents = [
            member("o", store::ORCHESTRATOR, store::DEFAULT_TEAM),
            member("d", store::ORCHESTRATOR, "docs"),
            member("x", store::WORKER, "docs"),
        ];
        let mut host = host_of(&dir, &agents, 80, 24);
        host.view_team("docs").unwrap();
        let err = host.delete_team("docs").unwrap_err().to_string();
        assert_eq!(err, "docs has workers: x; remove them first");
        assert!(host.pane_of("d").is_some());
        assert!(host.delete_team(store::DEFAULT_TEAM).is_err());

        host.close("x").unwrap();
        store::update(&host.state_path, |s| {
            s.remove("x");
            Ok(())
        })
        .unwrap();
        host.toggle_teamlist().unwrap();
        host.teamlist_key(b"x").unwrap();
        assert_eq!(host.pane_of("d"), None);
        assert_eq!(*host.view.borrow(), store::DEFAULT_TEAM);
        // The open list shows the default team as viewed.
        let mut out = Vec::new();
        let teamlist = host.teamlist.borrow();
        teamlist
            .as_ref()
            .unwrap()
            .draw(&mut out, size(40, 5), Palette::default());
        drop(teamlist);
        let mut terminal = Terminal::new(40, 5).unwrap();
        terminal.vt_write(&out);
        let screen = Screen(terminal);
        assert!(screen.row(1).starts_with(" \u{f071} default  1 agent"));
        assert!(screen.bold(3, 1));
        let state = store::load(&host.state_path).unwrap();
        assert_eq!(state.teams(), [store::DEFAULT_TEAM]);
        assert_eq!(*host.teams.borrow(), [(store::DEFAULT_TEAM.into(), false)]);
    }

    #[test]
    fn the_minute_the_host_starts_in_is_not_fired() {
        let dir = TempDir::new();
        let mut host = host_of(&dir, &[member("n", store::ORCHESTRATOR, "nightly")], 80, 24);
        store::update(&host.state_path, |s| {
            s.agents[0].activity = store::IDLE.into();
            s.tasks.push(store::Task {
                name: "nightly".into(),
                schedule: "* * * * *".into(),
                prompt: "go".into(),
                last_run: 0,
            });
            Ok(())
        })
        .unwrap();
        let start = host.checked * 60;
        host.fire_tasks(start + 59);
        assert!(store::load(&host.state_path).unwrap().messages.is_empty());
        host.fire_tasks(start + 60);
        let state = store::load(&host.state_path).unwrap();
        assert_eq!(state.messages.len(), 1);
        assert_eq!(state.tasks[0].last_run, start + 60);
        assert_eq!(host.checked, start / 60 + 1);
    }

    #[test]
    fn deleting_a_task_team_deletes_its_task() {
        let dir = TempDir::new();
        let agents = [
            member("o", store::ORCHESTRATOR, store::DEFAULT_TEAM),
            member("n", store::ORCHESTRATOR, "nightly"),
        ];
        let mut host = host_of(&dir, &agents, 80, 24);
        store::update(&host.state_path, |s| {
            s.tasks.push(store::Task {
                name: "nightly".into(),
                ..store::Task::default()
            });
            Ok(())
        })
        .unwrap();
        host.toggle_teamlist().unwrap();
        host.teamlist_key(b"j").unwrap();
        host.teamlist_key(b"x").unwrap();
        assert_eq!(host.pane_of("n"), None);
        let state = store::load(&host.state_path).unwrap();
        assert_eq!(state.teams(), [store::DEFAULT_TEAM]);
        assert!(state.tasks.is_empty());
    }

    #[test]
    fn closing_the_viewed_teams_last_pane_views_the_default_team() {
        let dir = TempDir::new();
        let agents = [
            member("o", store::ORCHESTRATOR, store::DEFAULT_TEAM),
            member("n", store::ORCHESTRATOR, "nightly"),
            member("w", store::WORKER, "nightly"),
        ];
        let mut host = host_of(&dir, &agents, 80, 24);
        host.view_team("nightly").unwrap();
        // As the melts end.
        for (id, view) in [("w", "nightly"), ("n", store::DEFAULT_TEAM)] {
            let pane = host.pane_of(id).unwrap();
            host.close(id).unwrap();
            host.finish_close(pane).unwrap();
            assert_eq!(*host.view.borrow(), view, "{id}");
        }
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
