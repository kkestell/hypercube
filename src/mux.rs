//! Per-pane Ghostty terminals, host input routing, and rendering. The caller
//! adds and removes panes, places each in a rectangle of the host terminal,
//! and draws the chrome around them through `Chrome`. Terminals can run
//! without ptys for round-trip tests.

use std::cell::{Cell, Ref, RefCell};
use std::io::Write as _;
use std::os::fd::{AsRawFd as _, OwnedFd, RawFd};
use std::process::Child;
use std::rc::Rc;

use libghostty_vt::error::{Error, Result};
use libghostty_vt::fmt::Format;
use libghostty_vt::key::{self, Action};
use libghostty_vt::mouse::{self, Button, EncoderSize, Position};
use libghostty_vt::render::{CellIterator, Cursor, CursorVisualStyle, Dirty, RowIterator};
use libghostty_vt::screen::{Cell as RawCell, CellContentTag, CellWide, Screen, TrackedGridRef};
use libghostty_vt::selection::{FormatOptions, Selection};
use libghostty_vt::style::{Palette, RgbColor, Style, StyleColor, Underline};
use libghostty_vt::terminal::{
    ConformanceLevel, DeviceAttributeFeature, DeviceAttributes, DeviceType, Mode, ModeKind, Point,
    PointCoordinate, PrimaryDeviceAttributes, ScrollViewport, SecondaryDeviceAttributes,
    SizeReportSize, TertiaryDeviceAttributes,
};
use libghostty_vt::{RenderState, Terminal};

use crate::pty::Pty;

pub use libc::winsize as Winsize;
pub use libghostty_vt::key::Key;

/// Modes mirrored onto the host terminal: the ones that change how the
/// host must encode input, and the ones that change how it draws what
/// the mux sends. Each comes with Ghostty's default, which the host is
/// assumed to share; grapheme clustering's default comes from the host.
/// Mouse modes are not mirrored: the host always reports the mouse (see
/// `Mux::input`).
pub const MIRRORED_MODES: [(u16, bool); 10] = [
    (1, false),    // DECCKM: application cursor keys
    (5, false),    // DECSCNM: reverse video
    (66, false),   // DECNKM: application keypad
    (67, false),   // DECBKM: backspace sends BS
    (1004, false), // focus in/out events
    (1035, true),  // ignore keypad with Num Lock
    (1036, true),  // Alt sends ESC
    (1039, false), // Alt sends ESC (alternate)
    (2004, false), // bracketed paste
    (2027, false), // grapheme clustering
];

/// Rows one wheel step scrolls.
const WHEEL_ROWS: isize = 3;

/// How long host input cut off by the end of a read waits for the rest.
pub const INPUT_HOLD_MS: i64 = 25;

/// The most host input held for the next read, in bytes.
const INPUT_HOLD_MAX: usize = 256;

/// The start and end of a bracketed paste.
const PASTE_START: &[u8] = b"\x1b[200~";
const PASTE_END: &[u8] = b"\x1b[201~";

/// The longest notification passed through to the host, in bytes.
const NOTIFICATION_MAX: usize = 4096;

/// The most output from outside frames held for the next frame, in bytes.
const MAX_SIDE_OUTPUT: usize = 256 * 1024;

/// How long a pane may hold drawing (mode 2026) without completing a frame.
pub const HOLD_TIMEOUT_MS: i64 = 1000;

/// How long a removed pane's program has to exit after SIGHUP before its
/// process group gets SIGKILL.
pub const HANGUP_GRACE_MS: i64 = 2000;

/// The time between frames of the chrome's animation.
pub const ANIMATION_MS: i64 = 40;

/// How long a melting pane's columns may wait before they start to fall.
const MELT_DELAY_MS: i64 = 300;

/// How long a melting column takes to fall the pane's height once it starts.
const MELT_FALL_MS: i64 = 600;

/// The most steps of a melt column's delay, one step being
/// `MELT_DELAY_MS / MELT_STEPS`.
const MELT_STEPS: i64 = 15;

/// Reports a VT220 with ANSI color.
const DEVICE_ATTRIBUTES: DeviceAttributes = DeviceAttributes {
    primary: PrimaryDeviceAttributes::new(
        ConformanceLevel::VT220,
        &[DeviceAttributeFeature::ANSI_COLOR],
    ),
    secondary: SecondaryDeviceAttributes {
        device_type: DeviceType::VT220,
        firmware_version: 0,
        rom_cartridge: 0,
    },
    tertiary: TertiaryDeviceAttributes { unit_id: 0 },
};

fn dec(value: u16) -> Mode {
    Mode::new(value, ModeKind::Dec)
}

/// A pane's identity, stable for the life of the mux.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct PaneId(pub u32);

/// The pane of no input: the mux numbers its panes from 1.
const NO_PANE: PaneId = PaneId(0);

/// A region of the host terminal in cells. `col` and `row` count from 0.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Rect {
    pub col: u16,
    pub row: u16,
    pub cols: u16,
    pub rows: u16,
}

impl Rect {
    /// The whole host terminal.
    pub fn full(ws: Winsize) -> Rect {
        Rect {
            col: 0,
            row: 0,
            cols: ws.ws_col,
            rows: ws.ws_row,
        }
    }

    fn contains(&self, col: u16, row: u16) -> bool {
        (self.col..self.col + self.cols).contains(&col)
            && (self.row..self.row + self.rows).contains(&row)
    }
}

/// Something the caller may act on, from host input or the focus moving; see
/// `Mux::take_notices`.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Notice {
    /// The focus moved to this pane.
    Focus(PaneId),
    /// Ctrl-b z was pressed while this pane had the focus.
    Zoom(PaneId),
    /// Ctrl-b s was pressed, which opens or closes the overlay.
    Overlay,
    /// A key or escape sequence arrived while input is captured.
    Key(Vec<u8>),
    /// The prefix key, Ctrl-b, was followed by this digit.
    Workspace(u8),
    /// Ctrl-b ( or ) was pressed: switch this many workspaces along the
    /// bar.
    Step(i8),
    /// Ctrl-b c was pressed, which asks for a new workspace. The input
    /// that follows is captured.
    NewWorkspace,
    /// Ctrl-b d was pressed.
    Detach,
    /// Ctrl-b a was pressed, which changes the workspace's color.
    Accent,
    /// Ctrl-b ? was pressed, which opens or closes the list of keys.
    Help,
    /// The left button was pressed outside every pane, at this column and
    /// row.
    Click(u16, u16),
    /// The left button, pressed outside every pane, was dragged to this
    /// column and row.
    Drag(u16, u16),
    /// This pane has finished melting and was removed.
    Melted(PaneId),
}

/// What the chrome sees of a pane.
pub struct PaneView<'a> {
    pub id: PaneId,
    pub label: &'a str,
    /// Text for the right end of the label row.
    pub detail: &'a str,
    pub rect: Rect,
    pub focused: bool,
    /// The program's exit status once it has been reaped.
    pub status: Option<u8>,
    /// How many rows the pane's view is above the bottom of its scrollback.
    pub scrolled: usize,
}

/// What the chrome sees of `panes`, with `focus` focused.
fn pane_views(panes: &[Pane], focus: Option<PaneId>) -> Vec<PaneView<'_>> {
    panes
        .iter()
        .map(|p| PaneView {
            id: p.id,
            label: &p.label,
            detail: &p.detail,
            rect: p.rect,
            focused: Some(p.id) == focus,
            status: p.status,
            scrolled: p.scrolled_drawn,
        })
        .collect()
}

/// Draws everything around the panes: borders and labels.
pub trait Chrome {
    /// Write the escape sequences that draw all of the chrome into `out`,
    /// leaving the SGR attributes reset. The mux calls this inside a frame
    /// whenever the panes, the focus, or the host size changed, or after
    /// `Mux::invalidate_chrome`.
    fn draw(&mut self, out: &mut Vec<u8>, panes: &[PaneView<'_>], size: Winsize);

    /// Whether the chrome has something to draw every `ANIMATION_MS`. The
    /// mux asks again after every change, and stops animating when it
    /// answers no.
    fn animated(&self, _panes: &[PaneView<'_>], _size: Winsize) -> bool {
        false
    }

    /// Write the escape sequences that draw the next frame of the animation,
    /// at `now` in `now_ms` time, leaving the SGR attributes reset. The mux
    /// calls this after `draw`, and between draws once the interval has
    /// passed, only while `animated`.
    fn animate(&mut self, _out: &mut Vec<u8>, _panes: &[PaneView<'_>], _size: Winsize, _now: i64) {}
}

/// Chrome that draws nothing.
pub struct NoChrome;

impl Chrome for NoChrome {
    fn draw(&mut self, _: &mut Vec<u8>, _: &[PaneView<'_>], _: Winsize) {}
}

/// State shared with the terminals' effect callbacks.
struct Shared {
    out: RefCell<Vec<u8>>,
    focus: Cell<Option<PaneId>>,
    /// Whether a host terminal is connected. Side output is discarded
    /// while none is.
    connected: Cell<bool>,
    /// Whether a pane rang the bell since the last frame.
    bell: Cell<bool>,
    /// Whether the focused pane's title changed since the last frame.
    title_changed: Cell<bool>,
}

impl Shared {
    /// Append output from outside a frame, a notification or a clipboard
    /// copy, whole, unless no host terminal is connected or the output would
    /// pass `MAX_SIDE_OUTPUT`.
    fn side_output(&self, data: &[u8]) {
        let mut out = self.out.borrow_mut();
        if self.connected.get() && out.len() + data.len() <= MAX_SIDE_OUTPUT {
            out.extend_from_slice(data);
        }
    }
}

/// What the mux knows of the connected host terminal, and the host input in
/// progress from it. Each connection starts afresh.
struct HostTerminal {
    /// Each mirrored mode as the host has it, `None` until set.
    modes: [Option<bool>; MIRRORED_MODES.len()],
    kitty_flags: Option<u8>,
    /// Whether the host reports mouse motion without a button held, once
    /// mouse reporting is on.
    any_motion: Option<bool>,
    /// Whether terminals cluster graphemes by default, from the host.
    grapheme_default: bool,
    /// The mouse button held since it was pressed in a pane.
    press: Option<Press>,
    /// The pane with a selection.
    selection: Option<PaneId>,
    /// Host input cut off by the end of a read, and when it was held.
    held_input: Vec<u8>,
    held_since: i64,
    /// The pane a bracketed paste in progress goes to.
    pasting: Option<PaneId>,
    /// Whether the prefix key was pressed and the next key goes with it.
    prefixed: bool,
}

impl HostTerminal {
    fn new(grapheme_default: bool) -> HostTerminal {
        HostTerminal {
            modes: [None; MIRRORED_MODES.len()],
            kitty_flags: None,
            any_motion: None,
            grapheme_default,
            press: None,
            selection: None,
            held_input: Vec::new(),
            held_since: 0,
            pasting: None,
            prefixed: false,
        }
    }
}

/// Per-pane state shared with its terminal's effect callbacks.
struct PaneState {
    pty: Cell<Option<RawFd>>,
    /// Input the program hasn't taken yet.
    input: RefCell<Vec<u8>>,
    ws: Cell<Winsize>,
    held: Cell<bool>,
    held_since: Cell<i64>,
}

impl PaneState {
    fn send(&self, data: &[u8]) {
        if self.pty.get().is_some() {
            self.input.borrow_mut().extend_from_slice(data);
            self.write_input();
        }
    }

    /// Write as much queued input as the non-blocking master takes.
    fn write_input(&self) {
        let Some(pty) = self.pty.get() else { return };
        let mut input = self.input.borrow_mut();
        while !input.is_empty() {
            let n = unsafe { libc::write(pty, input.as_ptr().cast(), input.len()) };
            if n > 0 {
                input.drain(..n as usize);
                continue;
            }
            match std::io::Error::last_os_error().raw_os_error() {
                Some(libc::EINTR) => continue,
                Some(libc::EAGAIN) => return,
                _ => return input.clear(),
            }
        }
    }
}

/// What a pane last captured from its terminal and hasn't drawn yet. A
/// capture happens before each frame, and when the program starts a hold
/// (mode 2026), so that a held pane draws the last frame the program
/// completed rather than nothing or a half-drawn one.
struct Frame {
    render_state: RenderState<'static>,
    rows: RowIterator<'static>,
    cells: CellIterator<'static>,
    /// Every row as of the last capture, by viewport row, and whether it
    /// changed since it was last drawn.
    rows_seen: Vec<(RowBuf, bool)>,
    /// Whether a capture or repaint hasn't been drawn.
    captured: bool,
    /// The cursor as of the last capture.
    cursor: Cursor,
    /// Rows to capture from the terminal itself; see `Pane::feed`.
    stale_rows: Vec<u16>,
}

impl Frame {
    fn new() -> Result<Frame> {
        Ok(Frame {
            render_state: RenderState::new()?,
            rows: RowIterator::new()?,
            cells: CellIterator::new()?,
            rows_seen: Vec::new(),
            captured: false,
            cursor: Cursor {
                viewport: None,
                visible: true,
                blinking: false,
                password_input: false,
                visual_style: CursorVisualStyle::Block,
            },
            stale_rows: Vec::new(),
        })
    }

    /// Take the rows that changed since the last capture.
    fn capture(&mut self, terminal: &Terminal<'static, '_>) -> Result<()> {
        let snapshot = self.render_state.update(terminal)?;
        self.cursor = snapshot.cursor()?;
        let dirty = snapshot.dirty()?;
        let height = usize::from(snapshot.rows()?);
        if self.rows_seen.len() != height {
            self.rows_seen = (0..height).map(|_| (RowBuf::default(), false)).collect();
        }
        let mut stale = std::mem::take(&mut self.stale_rows);
        // Stale rows are rows of the active area, which a scrolled view
        // doesn't show.
        if !terminal.viewport_active()? {
            stale.clear();
        }
        if dirty != Dirty::Clean || !stale.is_empty() {
            let mut rows = self.rows.update(&snapshot)?;
            let mut y: u16 = 0;
            while let Some(r) = rows.next() {
                let Some((row, changed)) = self.rows_seen.get_mut(usize::from(y)) else {
                    break;
                };
                let selected = r.selection()?.map(|s| (s.start_x, s.end_x));
                if stale.contains(&y) {
                    row.clear(selected);
                    for x in 0..snapshot.cols()? {
                        let grid = terminal
                            .grid_ref(Point::Active(PointCoordinate { x, y: u32::from(y) }))?;
                        let raw = grid.cell()?;
                        let style = grid.style()?;
                        row.push(raw, style, |buf| grid.graphemes(buf))?;
                    }
                    *changed = true;
                } else if dirty == Dirty::Full || r.dirty()? {
                    row.clear(selected);
                    let mut cells = self.cells.update(r)?;
                    while let Some(c) = cells.next() {
                        let raw = c.raw_cell()?;
                        let style = c.style()?;
                        row.push(raw, style, |buf| {
                            let len = c.graphemes_len()? as usize;
                            if buf.len() < len {
                                return Err(Error::OutOfSpace { required: len });
                            }
                            c.graphemes_buf(&mut buf[..len])?;
                            Ok(len)
                        })?;
                    }
                    *changed = true;
                }
                y += 1;
            }
        }
        snapshot.clean()?;
        self.captured = true;
        Ok(())
    }

    /// Draw the changed rows into `out` within `rect`.
    fn draw(&mut self, out: &mut Vec<u8>, rect: Rect) {
        for (y, (row, changed)) in self.rows_seen.iter_mut().enumerate() {
            if std::mem::take(changed) && y < usize::from(rect.rows) {
                row.write(out, y as u16, rect);
            }
        }
        self.captured = false;
    }

    /// Draw every row of the last capture again in the next draw.
    fn redraw(&mut self) {
        for (_, changed) in &mut self.rows_seen {
            *changed = true;
        }
        self.captured = true;
    }
}

/// A pane's last drawn screen falling off the bottom edge of its place, like
/// DOOM's screen wipe. The pane keeps its place and its label row until the
/// melt ends; see `Mux::melt_pane`.
struct Melt {
    /// When the melt began, in `now_ms` time.
    start: i64,
    /// How long each column waits before it starts to fall, in milliseconds.
    delays: Vec<i64>,
    /// The pane's last drawn rows, by viewport row.
    rows: Vec<RowBuf>,
}

impl Melt {
    /// How far column `x` has fallen `elapsed` ms after the melt began, in
    /// rows of a pane `rows` high.
    fn fall(&self, x: u16, elapsed: i64, rows: i64) -> i64 {
        let delay = self.delays.get(usize::from(x)).copied().unwrap_or(0);
        melt_fall(elapsed - delay, rows)
    }

    /// When the melt ends.
    fn end(&self) -> i64 {
        self.start + MELT_DELAY_MS + MELT_FALL_MS
    }
}

/// How far a column falls `elapsed` ms after its delay ran out, in rows of a
/// pane `rows` high: none until its delay, then faster and faster until it
/// has fallen the whole height.
fn melt_fall(elapsed: i64, rows: i64) -> i64 {
    if elapsed <= 0 {
        return 0;
    }
    if elapsed >= MELT_FALL_MS {
        return rows;
    }
    rows * elapsed * elapsed / (MELT_FALL_MS * MELT_FALL_MS)
}

/// The delay in milliseconds each of `cols` columns waits before it starts to
/// fall: a random 0 to 15 steps for the first, and each next one a step down,
/// a step up, or the same, so neighbouring columns form a ragged curtain
/// rather than noise. The delays come from a small xorshift64 seeded with
/// `seed`.
fn melt_delays(cols: u16, seed: u64) -> Vec<i64> {
    let step = MELT_DELAY_MS / MELT_STEPS;
    let mut state = seed | 1;
    let mut random = move || {
        state ^= state << 13;
        state ^= state >> 7;
        state ^= state << 17;
        state
    };
    let mut steps = (random() % (MELT_STEPS as u64 + 1)) as i64;
    (0..cols)
        .map(|_| {
            let delay = steps * step;
            steps = (steps + (random() % 3) as i64 - 1).clamp(0, MELT_STEPS);
            delay
        })
        .collect()
}

/// The seed of a melt's delays: the time it began and the pane's identity.
fn melt_seed(now: i64, id: PaneId) -> u64 {
    (now as u64).wrapping_mul(0x9e37_79b9_7f4a_7c15) ^ u64::from(id.0)
}

/// Per-program state.
pub struct Pane {
    id: PaneId,
    shared: Rc<Shared>,
    state: Rc<PaneState>,
    master: Option<OwnedFd>,
    child: Option<Child>,
    status: Option<u8>,
    pub label: String,
    /// Text for the right end of the label row.
    pub detail: String,
    rect: Rect,
    pub terminal: Terminal<'static, 'static>,
    frame: Rc<RefCell<Frame>>,
    /// How the pane's last drawn screen is falling as it goes.
    melt: Option<Melt>,
    dirty: bool,
    notifications: Notifications,
    /// How far the view was scrolled when the chrome was last drawn.
    scrolled_drawn: usize,
}

impl Pane {
    fn new(shared: &Rc<Shared>, id: PaneId, label: &str) -> Result<Pane> {
        let state = Rc::new(PaneState {
            pty: Cell::new(None),
            input: RefCell::new(Vec::new()),
            ws: Cell::new(Winsize {
                ws_col: 1,
                ws_row: 1,
                ws_xpixel: 0,
                ws_ypixel: 0,
            }),
            held: Cell::new(false),
            held_since: Cell::new(0),
        });
        let frame = Rc::new(RefCell::new(Frame::new()?));
        let terminal = Self::new_terminal(shared, id, &state, &frame)?;

        Ok(Pane {
            id,
            shared: shared.clone(),
            state,
            master: None,
            child: None,
            status: None,
            label: label.to_owned(),
            detail: String::new(),
            rect: Rect::default(),
            terminal,
            frame,
            melt: None,
            dirty: true,
            notifications: Notifications::default(),
            scrolled_drawn: 0,
        })
    }

    /// A terminal of one cell whose effects go to the pane's shared state.
    fn new_terminal(
        shared: &Rc<Shared>,
        id: PaneId,
        state: &Rc<PaneState>,
        frame: &Rc<RefCell<Frame>>,
    ) -> Result<Terminal<'static, 'static>> {
        let mut terminal = Terminal::new(1, 1)?;
        let s = state.clone();
        terminal.on_pty_write(move |_, data| s.send(data))?;
        let sh = shared.clone();
        terminal.on_bell(move |_| sh.bell.set(sh.connected.get()))?;
        let sh = shared.clone();
        terminal.on_title_changed(move |_| {
            if sh.focus.get() == Some(id) {
                sh.title_changed.set(true);
            }
        })?;
        terminal.on_device_attributes(|_| Some(DEVICE_ATTRIBUTES))?;
        let s = state.clone();
        terminal.on_size(move |_| {
            let ws = s.ws.get();
            let (cell_width, cell_height) = cell_pixels(&ws);
            Some(SizeReportSize {
                rows: ws.ws_row,
                columns: ws.ws_col,
                cell_width,
                cell_height,
            })
        })?;
        let s = state.clone();
        let f = frame.clone();
        terminal.on_render_hold(move |t, held| {
            s.held.set(held);
            if held {
                // Nothing after the hold's start has been processed, so the
                // terminal holds the frame the program completed.
                if let Ok(mut frame) = f.try_borrow_mut() {
                    let _ = frame.capture(t);
                }
                s.held_since.set(now_ms());
            }
        })?;
        Ok(terminal)
    }

    /// Replace the exited program's state: a new terminal of the same size,
    /// with nothing the old program left half written, a clean notification
    /// scanner, the label, and no exit status. Returns the old program's
    /// process group, hung up. The caller attaches the new program.
    fn restart(&mut self, label: &str, host_replies: &[u8]) -> Result<Option<Orphan>> {
        let mut terminal = Self::new_terminal(&self.shared, self.id, &self.state, &self.frame)?;
        let ws = self.ws();
        let (cell_width, cell_height) = cell_pixels(&ws);
        terminal.resize(ws.ws_col, ws.ws_row, cell_width, cell_height)?;
        use_host_replies_for(&mut terminal, host_replies)?;
        let orphan = self.hang_up();
        self.terminal = terminal;
        self.notifications = Notifications::default();
        self.status = None;
        self.label = label.to_owned();
        let _ = self.repaint();
        Ok(orphan)
    }

    pub fn id(&self) -> PaneId {
        self.id
    }

    pub fn rect(&self) -> Rect {
        self.rect
    }

    /// The program's exit status once it has been reaped.
    pub fn status(&self) -> Option<u8> {
        self.status
    }

    /// Whether the pane is melting away before it is removed.
    pub fn melting(&self) -> bool {
        self.melt.is_some()
    }

    /// The program's process ID, which is also its process group ID.
    pub fn pid(&self) -> Option<u32> {
        self.child.as_ref().map(Child::id)
    }

    /// Feed the program's output to the terminal, passing its desktop
    /// notifications through to the host.
    pub fn feed(&mut self, data: &[u8]) {
        let mut notifications = Vec::new();
        self.notifications.scan(data, &mut notifications);
        if !notifications.is_empty() {
            self.shared.side_output(&notifications);
        }
        // A leading zero-width character can attach to the previous cell
        // without dirtying its row, so the render state wouldn't see it.
        // Capture the cursor's row from the terminal.
        if let Ok(y) = self.terminal.cursor_y() {
            let mut frame = self.frame.borrow_mut();
            if !frame.stale_rows.contains(&y) {
                frame.stale_rows.push(y);
            }
        }
        self.terminal.vt_write(data);
        self.dirty = true;
    }

    /// The pty master, while it is open.
    pub fn pty(&self) -> Option<RawFd> {
        self.state.pty.get()
    }

    /// The pane's size, as its terminal and pty see it.
    pub fn ws(&self) -> Winsize {
        self.state.ws.get()
    }

    /// Whether the program holds drawing (mode 2026). A held pane draws
    /// only what was captured when the hold began.
    pub fn held(&self) -> bool {
        self.state.held.get()
    }

    pub fn set_held(&self, held: bool) {
        self.state.held.set(held);
    }

    /// When the program last completed a frame by starting a hold.
    pub fn held_since(&self) -> i64 {
        self.state.held_since.get()
    }

    /// End a hold that has gone on too long as if the program had ended it,
    /// so its next mode 2026 set starts a new hold.
    pub fn end_hold(&mut self) -> Result<()> {
        self.terminal.set_mode(Mode::SYNC_OUTPUT, false)?;
        self.set_held(false);
        Ok(())
    }

    /// Queue `data` for the program's input and write what the pty takes.
    pub fn send(&self, data: &[u8]) {
        self.state.send(data);
    }

    /// Whether input is queued for a pty that wasn't ready for it.
    pub fn input_pending(&self) -> bool {
        !self.state.input.borrow().is_empty()
    }

    /// Write queued input once the pty is ready for more.
    pub fn write_input(&self) {
        self.state.write_input();
    }

    /// Hang up the program and give up the pane's hold on it. Returns its
    /// process group, to be killed if it outlasts `HANGUP_GRACE_MS`.
    fn hang_up(&mut self) -> Option<Orphan> {
        self.signal(libc::SIGHUP);
        self.close_master();
        let child = self.child.take()?;
        // The program may have been reaped while others in its group, which
        // it started, still run.
        let pgid = child.id() as libc::pid_t;
        unsafe { libc::killpg(pgid, libc::SIGHUP) };
        Some(Orphan {
            pgid,
            child: self.status.is_none().then_some(child),
            since: now_ms(),
        })
    }

    /// Send `signal` to the program's process group, unless it has been
    /// reaped.
    pub fn signal(&self, signal: libc::c_int) {
        if let (Some(pid), None) = (self.pid(), self.status) {
            unsafe { libc::killpg(pid as libc::pid_t, signal) };
        }
    }

    /// Close the pty master after the program's output ended or the pane
    /// was closed. Drawing ends any hold, so the last output shows.
    pub(crate) fn close_master(&mut self) {
        self.master = None;
        self.state.pty.set(None);
        self.state.input.borrow_mut().clear();
        self.set_held(false);
    }

    /// How many rows the view is above the bottom of the scrollback. New
    /// output leaves a scrolled view where it is.
    pub fn scrolled(&self) -> usize {
        self.terminal
            .scrollbar()
            .map_or(0, |s| s.total.saturating_sub(s.offset + s.len) as usize)
    }

    /// Move the view `rows` rows through the scrollback, up when negative.
    /// Scrolling ends a hold, so the view shows what the terminal holds.
    pub fn scroll(&mut self, rows: isize) {
        self.scroll_viewport(ScrollViewport::Delta(rows));
    }

    /// Return the view to the bottom of the scrollback.
    pub fn scroll_to_bottom(&mut self) {
        self.scroll_viewport(ScrollViewport::Bottom);
    }

    fn scroll_viewport(&mut self, scroll: ScrollViewport) {
        self.terminal.scroll_viewport(scroll);
        if self.held() {
            let _ = self.end_hold();
        }
        let _ = self.repaint();
    }

    /// Whether the program reports the mouse in the pane. A scrolled view
    /// keeps the mouse for scrolling and selecting.
    fn reports_mouse(&self) -> bool {
        self.terminal.is_mouse_tracking().unwrap_or(false) && self.scrolled() == 0
    }

    /// Report a mouse event at `col` and `row` of the pane to the program,
    /// encoded for its mouse modes.
    fn report_mouse(&self, event: &MouseEvent, col: u16, row: u16) -> Result<()> {
        let ws = self.ws();
        let cell_width = u32::from(ws.ws_xpixel / ws.ws_col.max(1)).max(1);
        let cell_height = u32::from(ws.ws_ypixel / ws.ws_row.max(1)).max(1);
        let mut encoder = mouse::Encoder::new()?;
        encoder
            .set_options_from_terminal(&self.terminal)
            .set_size(EncoderSize {
                screen_width: cell_width * u32::from(ws.ws_col),
                screen_height: cell_height * u32::from(ws.ws_row),
                cell_width,
                cell_height,
                padding_top: 0,
                padding_bottom: 0,
                padding_right: 0,
                padding_left: 0,
            })
            .set_any_button_pressed(
                event.action == mouse::Action::Motion && event.button.is_some(),
            );
        let mut e = mouse::Event::new()?;
        e.set_action(event.action)
            .set_button(event.button)
            .set_mods(event.mods)
            .set_position(Position {
                x: (f32::from(col) + 0.5) * cell_width as f32,
                y: (f32::from(row) + 0.5) * cell_height as f32,
            });
        let mut buf = [0; 64];
        let n = encoder.encode(&e, &mut buf)?;
        self.send(&buf[..n]);
        Ok(())
    }

    /// Select from `anchor` to `col` and `row` of the view.
    fn select(&mut self, anchor: &TrackedGridRef, col: u16, row: u16) -> Result<()> {
        let point = Point::Viewport(PointCoordinate {
            x: col,
            y: u32::from(row),
        });
        let Some(start) = anchor.snapshot(&self.terminal)? else {
            return Ok(());
        };
        let end = self.terminal.grid_ref(point)?;
        self.terminal
            .set_selection(Some(&Selection::new(start, end, false)))?;
        self.repaint()
    }

    fn clear_selection(&mut self) -> Result<()> {
        self.terminal.set_selection(None)?;
        self.repaint()
    }

    /// The selected text, with soft-wrapped lines joined and trailing
    /// spaces trimmed.
    fn selected_text(&self) -> Result<Option<Vec<u8>>> {
        let options = FormatOptions::new()
            .with_emit_format(Format::Plain)
            .with_unwrap(true)
            .with_trim(true);
        let text = self.terminal.format_selection_alloc(None, options)?;
        Ok(text.map(|t| t.to_vec()))
    }

    /// Whether the program hasn't been reaped and the pane isn't melting.
    fn live(&self) -> bool {
        self.status.is_none() && self.melt.is_none()
    }

    fn mode(&self, mode: Mode) -> bool {
        self.terminal.mode(mode).unwrap_or(false)
    }

    fn needs_render(&self) -> bool {
        if self.held() {
            self.frame.borrow().captured
        } else {
            self.dirty
        }
    }

    /// Make the next frame draw every row.
    fn repaint(&mut self) -> Result<()> {
        // Marking the render state dirty would repaint its cached rows,
        // which can miss what `feed` captured from the terminal. Setting a
        // color default marks the terminal's palette dirty instead, which
        // makes the next capture rebuild every row.
        let cursor = self.terminal.default_cursor_color()?;
        self.terminal.set_default_cursor_color(cursor)?;
        self.dirty = true;
        // A held pane draws the frame its program completed last.
        self.frame.borrow_mut().redraw();
        Ok(())
    }

    /// Draw the rows that changed since the last frame into `out`: from
    /// the terminal, or while held, from the capture at the hold's start.
    fn render(&mut self, out: &mut Vec<u8>) -> Result<()> {
        let mut frame = self.frame.borrow_mut();
        if !self.held() {
            frame.capture(&self.terminal)?;
            self.dirty = false;
        }
        frame.draw(out, self.rect);
        Ok(())
    }

    /// Draw a melting pane: every column of its last drawn screen falls on
    /// its own, leaving blanks above it, clipped to the pane's place. A wide
    /// character is drawn only with the column after it, and its second cell
    /// is skipped while the first covers it.
    fn draw_melt(&self, out: &mut Vec<u8>, now: i64) {
        let Some(melt) = &self.melt else { return };
        let rect = self.rect;
        let elapsed = now - melt.start;
        let rows = i64::from(rect.rows);
        for y in 0..rect.rows {
            let _ = write!(out, "\x1b[{};{}H", rect.row + y + 1, rect.col + 1);
            let mut prev: Option<Style> = None;
            for x in 0..rect.cols {
                let col = rect.col + x;
                let fall = melt.fall(x, elapsed, rows);
                let Some(row) = (i64::from(y) >= fall)
                    .then(|| melt.rows.get((i64::from(y) - fall) as usize))
                    .flatten()
                else {
                    write_blank(out, &mut prev);
                    continue;
                };
                let Some(&(raw, style, start, len)) = row.cells.get(usize::from(x)) else {
                    write_blank(out, &mut prev);
                    continue;
                };
                match raw.wide().unwrap_or(CellWide::Narrow) {
                    CellWide::Wide => {
                        let right = x + 1;
                        let covered = right < rect.cols
                            && melt.fall(right, elapsed, rows) == fall
                            && usize::from(right) < row.cells.len();
                        if covered {
                            let style = cell_style(raw, style);
                            write_cell(
                                out,
                                col,
                                raw,
                                style,
                                &row.text[start..start + len],
                                &mut prev,
                            );
                        } else {
                            write_blank(out, &mut prev);
                        }
                    }
                    // The wide character before it covers this cell.
                    CellWide::SpacerTail if x > 0 && melt.fall(x - 1, elapsed, rows) == fall => {}
                    CellWide::SpacerTail => write_blank(out, &mut prev),
                    _ => {
                        let style = cell_style(raw, style);
                        write_cell(
                            out,
                            col,
                            raw,
                            style,
                            &row.text[start..start + len],
                            &mut prev,
                        );
                    }
                }
            }
        }
        out.extend_from_slice(b"\x1b[0m");
    }

    fn cursor(&self) -> Cursor {
        self.frame.borrow().cursor
    }
}

/// A removed pane's process group: SIGHUP was sent, and whatever is left
/// after `HANGUP_GRACE_MS` gets SIGKILL.
struct Orphan {
    pgid: libc::pid_t,
    /// The group's leader, the pane's program, unless it was reaped before
    /// the pane was removed.
    child: Option<Child>,
    since: i64,
}

pub struct Mux {
    ws: Winsize,
    panes: Vec<Pane>,
    next_id: u32,
    shared: Rc<Shared>,
    chrome: Box<dyn Chrome>,
    chrome_dirty: bool,
    /// Whether the last frame drew the chrome's animation, and when it
    /// drew it last.
    animated: bool,
    animation_at: i64,
    /// Whether the next frame clears the host screen and draws everything.
    clear: bool,
    /// Process groups of removed panes, until they are empty or killed.
    orphans: Vec<Orphan>,
    /// The replies of the most recently connected host terminal to
    /// `host_queries`, which every pane's terminal takes.
    host_replies: Vec<u8>,
    host: HostTerminal,
    /// Whether host input is reported as `Notice::Key` instead of routed.
    captured: bool,
    /// What host input and focus changes have told the caller since it
    /// last took them, in order.
    notices: Vec<Notice>,
}

impl Mux {
    /// A mux of size `ws` with no host terminal connected.
    pub fn new(ws: Winsize, chrome: Box<dyn Chrome>) -> Mux {
        Mux {
            ws,
            panes: Vec::new(),
            next_id: 1,
            shared: Rc::new(Shared {
                out: RefCell::new(Vec::new()),
                focus: Cell::new(None),
                connected: Cell::new(false),
                bell: Cell::new(false),
                title_changed: Cell::new(false),
            }),
            chrome,
            chrome_dirty: true,
            animated: false,
            animation_at: 0,
            clear: false,
            orphans: Vec::new(),
            host_replies: Vec::new(),
            host: HostTerminal::new(false),
            captured: false,
            notices: Vec::new(),
        }
    }

    /// The host terminal's size.
    pub fn size(&self) -> Winsize {
        self.ws
    }

    pub fn focus(&self) -> Option<PaneId> {
        self.shared.focus.get()
    }

    /// The panes in the order they were added.
    pub fn panes(&self) -> &[Pane] {
        &self.panes
    }

    pub fn pane(&self, id: PaneId) -> Option<&Pane> {
        self.panes.iter().find(|p| p.id == id)
    }

    pub fn pane_mut(&mut self, id: PaneId) -> Option<&mut Pane> {
        self.panes.iter_mut().find(|p| p.id == id)
    }

    fn index(&self, id: PaneId) -> Option<usize> {
        self.panes.iter().position(|p| p.id == id)
    }

    fn focused(&self) -> Option<&Pane> {
        self.focus().and_then(|id| self.pane(id))
    }

    /// What the mux has drawn since the output was last taken.
    pub fn output(&self) -> Ref<'_, Vec<u8>> {
        self.shared.out.borrow()
    }

    pub fn take_output(&mut self) -> Vec<u8> {
        std::mem::take(&mut *self.shared.out.borrow_mut())
    }

    /// Connect a host terminal of size `ws` whose replies to `host_queries`
    /// are `replies`. Every pane's terminal, and every pane added later,
    /// takes the replies. The next frame clears the screen, draws everything,
    /// and sets every mirrored mode, the mouse modes, the Kitty keyboard
    /// flags, and the title.
    pub fn connect_host(&mut self, replies: &[u8], ws: Winsize) -> Result<()> {
        for p in &mut self.panes {
            use_host_replies_for(&mut p.terminal, replies)?;
        }
        self.host_replies = replies.to_vec();
        self.host = HostTerminal::new(host_clusters(replies));
        self.shared.out.borrow_mut().clear();
        self.shared.connected.set(true);
        self.shared.title_changed.set(true);
        self.resize(ws);
        Ok(())
    }

    /// Let the host terminal go: its selection is cleared, and its input in
    /// progress dropped. The panes keep running, and render draws nothing
    /// until a host terminal connects again.
    pub fn disconnect_host(&mut self) {
        self.clear_selection();
        self.host = HostTerminal::new(false);
        self.shared.out.borrow_mut().clear();
        self.shared.connected.set(false);
        self.shared.bell.set(false);
    }

    pub fn connected(&self) -> bool {
        self.shared.connected.get()
    }

    /// Report host input as `Notice::Key`, one key or escape sequence each,
    /// instead of routing it to the panes, or stop. Ctrl-b s, ?, and c turn
    /// capturing on, and Ctrl-b s and ? off while captured, so input after
    /// them in the same read goes where the caller will want it, and the
    /// caller sets it to match when it handles their notices. While
    /// captured, pastes
    /// and mouse reports are dropped, the host has every mirrored mode at
    /// its default, so keys arrive in legacy form, and no cursor is shown.
    pub fn set_captured(&mut self, captured: bool) {
        if self.captured == captured {
            return;
        }
        self.captured = captured;
        if let Some(Press::Report(_) | Press::Select(..)) = self.host.press {
            self.host.press = Some(Press::Ignore);
        }
        self.host.held_input.clear();
        if self.host.pasting.is_some() {
            self.host.pasting = Some(NO_PANE);
        }
        self.chrome_dirty = true;
    }

    fn mode_default(&self, value: u16, default: bool) -> bool {
        if value == 2027 {
            self.host.grapheme_default
        } else {
            default
        }
    }

    /// Add a pane without a program. It is drawn once `set_layout` places
    /// it, and focused if no other pane is.
    pub fn add_pane(&mut self, label: &str) -> Result<PaneId> {
        let id = PaneId(self.next_id);
        let mut pane = Pane::new(&self.shared, id, label)?;
        use_host_replies_for(&mut pane.terminal, &self.host_replies)?;
        self.next_id += 1;
        self.panes.push(pane);
        if !self.focused().is_some_and(Pane::live) {
            self.set_focus(id);
        }
        self.chrome_dirty = true;
        Ok(id)
    }

    /// Give a pane its program.
    pub fn attach(&mut self, id: PaneId, pty: Pty) {
        if let Some(p) = self.pane_mut(id) {
            p.state.pty.set(Some(pty.master.as_raw_fd()));
            p.master = Some(pty.master);
            p.child = Some(pty.child);
        }
    }

    /// Hang up the pane's program: send SIGHUP to its process group and
    /// close the pty master. The pane stays until `remove_pane`, and shows
    /// the exit status once `reap` collects it.
    pub fn close_pane(&mut self, id: PaneId) {
        if let Some(p) = self.pane_mut(id) {
            p.signal(libc::SIGHUP);
            p.close_master();
        }
    }

    /// Show a pane whose program never started as exited with `status`: the
    /// focus skips it, and the chrome sees its status.
    pub fn mark_exited(&mut self, id: PaneId, status: u8) {
        let Some(index) = self.index(id) else { return };
        let p = &mut self.panes[index];
        if p.child.is_some() || !p.live() {
            return;
        }
        p.status = Some(status);
        self.chrome_dirty = true;
        self.release_focus(id);
    }

    /// Move the focus off a pane that is not to run a program again, to the
    /// next live pane. A pane whose program exits keeps the focus until the
    /// caller decides whether to start another, so a relaunched pane is
    /// still focused.
    pub fn release_focus(&mut self, id: PaneId) {
        if self.focus() == Some(id)
            && let Some(index) = self.index(id)
        {
            self.focus_from(index + 1);
        }
    }

    /// Ready a pane whose program has exited for a new one: it gets a new
    /// terminal of the same size, so nothing the old program left half
    /// written carries over, it is live again, and `label` replaces its
    /// label. The old program's process group is hung up like a removed
    /// pane's, and the host input aimed at it is dropped: a selection, a
    /// mouse press in progress, held input, and the rest of a paste. A pane
    /// whose program is running is left as it is. The caller attaches the
    /// new program, or shows why there is none with `mark_exited`.
    pub fn restart(&mut self, id: PaneId, label: &str) -> Result<()> {
        let Some(index) = self.index(id) else {
            return Ok(());
        };
        if self.panes[index].live() || self.panes[index].melting() {
            return Ok(());
        }
        let orphan = self.panes[index].restart(label, &self.host_replies)?;
        self.orphans.extend(orphan);
        self.drop_input_for(id);
        self.chrome_dirty = true;
        Ok(())
    }

    /// Drop the rest of a mouse press that a pane captured.
    fn ignore_press_of(&mut self, id: PaneId) {
        if matches!(&self.host.press, Some(Press::Report(i) | Press::Select(i, ..)) if *i == id) {
            self.host.press = Some(Press::Ignore);
        }
    }

    /// Drop the host input aimed at a pane's program that has gone: its
    /// selection, a mouse press in progress, input held for it, and the rest
    /// of a paste, through its end marker.
    fn drop_input_for(&mut self, id: PaneId) {
        if self.host.selection == Some(id) {
            self.host.selection = None;
        }
        self.ignore_press_of(id);
        if self.host.pasting == Some(id) {
            // The held start of the end marker is part of the paste.
            self.host.pasting = Some(NO_PANE);
        } else if self.focus() == Some(id) {
            self.host.held_input.clear();
        }
    }

    /// Remove a pane, hanging up its program if it is still running, and
    /// move the focus off it. The rest of the screen is drawn again. A
    /// program still running `HANGUP_GRACE_MS` later is killed.
    pub fn remove_pane(&mut self, id: PaneId) {
        let Some(index) = self.index(id) else { return };
        let mut pane = self.panes.remove(index);
        self.orphans.extend(pane.hang_up());
        if self.focus() == Some(id) {
            self.shared.focus.set(None);
            self.focus_from(index);
        }
        self.repaint();
    }

    /// Start a pane melting away: hang up its program if it is still running,
    /// drop the host input aimed at it, and move the focus off it like
    /// `remove_pane` does. The pane keeps its place and title row while its
    /// last drawn screen falls off its bottom edge, and `end_melts` removes
    /// it when the melt ends. A pane with no place on the screen is removed
    /// at once, and one already melting is left alone.
    pub fn melt_pane(&mut self, id: PaneId) {
        let Some(index) = self.index(id) else { return };
        if self.panes[index].melting() {
            return;
        }
        if self.panes[index].rect.cols == 0 || self.panes[index].rect.rows == 0 {
            self.remove_pane(id);
            return;
        }
        let now = now_ms();
        let pane = &mut self.panes[index];
        let rows = {
            let mut frame = pane.frame.borrow_mut();
            // A held pane's capture is its last completed frame.
            if !pane.held() {
                let _ = frame.capture(&pane.terminal);
            }
            std::mem::take(&mut frame.rows_seen)
                .into_iter()
                .map(|(row, _)| row)
                .collect()
        };
        let delays = melt_delays(pane.rect.cols, melt_seed(now, id));
        pane.melt = Some(Melt {
            start: now,
            delays,
            rows,
        });
        self.orphans.extend(pane.hang_up());
        self.drop_input_for(id);
        if self.focus() == Some(id) {
            self.shared.focus.set(None);
            self.focus_from(index);
        }
        self.chrome_dirty = true;
    }

    /// Collect the exit status of every pane program that has exited, and
    /// of removed panes' programs. Never blocks. A pane whose program exited
    /// keeps the focus until `release_focus`.
    pub fn reap(&mut self) -> Vec<(PaneId, u8)> {
        let now = now_ms();
        self.orphans.retain_mut(|o| {
            if now - o.since < HANGUP_GRACE_MS {
                // An unreaped leader keeps its group ID from being reused,
                // so it is reaped only after the group is killed.
                return o.child.is_some() || unsafe { libc::killpg(o.pgid, 0) } == 0;
            }
            unsafe { libc::killpg(o.pgid, libc::SIGKILL) };
            o.child
                .as_mut()
                .is_some_and(|c| matches!(c.try_wait(), Ok(None)))
        });
        let mut exited = Vec::new();
        for p in &mut self.panes {
            if !p.live() {
                continue;
            }
            if let Some(Ok(Some(status))) = p.child.as_mut().map(Child::try_wait) {
                let status = exit_status(status);
                p.status = Some(status);
                exited.push((p.id, status));
            }
        }
        if !exited.is_empty() {
            self.chrome_dirty = true;
        }
        exited
    }

    /// Whether a program has hung up or been closed without being reaped,
    /// including removed panes' programs.
    pub fn reaping(&self) -> bool {
        !self.orphans.is_empty()
            || self
                .panes
                .iter()
                .any(|p| p.live() && p.child.is_some() && p.master.is_none())
    }

    /// The host terminal changed size. The caller lays the panes out again.
    pub fn resize(&mut self, ws: Winsize) {
        self.ws = ws;
        self.repaint();
    }

    /// Place panes on the host terminal, resizing their terminals and ptys,
    /// and draw everything again. Panes not listed keep their places. A pane
    /// given an empty rectangle is hidden and keeps its size.
    pub fn set_layout(&mut self, layout: &[(PaneId, Rect)]) -> Result<()> {
        let host = self.ws;
        for &(id, rect) in layout {
            let Some(p) = self.pane_mut(id) else { continue };
            p.rect = rect;
            if rect.cols == 0 || rect.rows == 0 {
                // A pane with no place can't take the rest of a mouse press.
                self.ignore_press_of(id);
                continue;
            }
            let (cols, rows) = (rect.cols, rect.rows);
            let ws = Winsize {
                ws_col: cols,
                ws_row: rows,
                ws_xpixel: (u32::from(host.ws_xpixel) * u32::from(cols)
                    / u32::from(host.ws_col.max(1))) as u16,
                ws_ypixel: (u32::from(host.ws_ypixel) * u32::from(rows)
                    / u32::from(host.ws_row.max(1))) as u16,
            };
            let old = p.state.ws.replace(ws);
            // Resizing a terminal ends its hold, so a pane that only moves
            // keeps its size and its hold.
            if (old.ws_col, old.ws_row, old.ws_xpixel, old.ws_ypixel)
                == (ws.ws_col, ws.ws_row, ws.ws_xpixel, ws.ws_ypixel)
            {
                continue;
            }
            let (cell_width, cell_height) = cell_pixels(&ws);
            p.terminal.resize(cols, rows, cell_width, cell_height)?;
            if let Some(pty) = p.pty() {
                unsafe { libc::ioctl(pty, libc::TIOCSWINSZ, &ws) };
            }
        }
        self.repaint();
        Ok(())
    }

    /// Draw the chrome again in the next frame.
    pub fn invalidate_chrome(&mut self) {
        self.chrome_dirty = true;
    }

    /// Clear the host screen and draw everything in the next frame.
    fn repaint(&mut self) {
        for p in &mut self.panes {
            // Only fails if Ghostty can't allocate, and then the row
            // repaint is all that's lost.
            let _ = p.repaint();
        }
        self.clear = true;
        self.chrome_dirty = true;
    }

    /// End holds that have gone `HOLD_TIMEOUT_MS` without a completed frame.
    pub fn expire_holds(&mut self, now: i64) -> Result<()> {
        for p in &mut self.panes {
            if p.held() && now - p.held_since() >= HOLD_TIMEOUT_MS {
                p.end_hold()?;
            }
        }
        Ok(())
    }

    /// When the earliest hold times out.
    pub fn hold_deadline(&self) -> Option<i64> {
        self.panes
            .iter()
            .filter(|p| p.held())
            .map(|p| p.held_since() + HOLD_TIMEOUT_MS)
            .min()
    }

    /// When the earliest melt ends.
    pub fn melt_deadline(&self) -> Option<i64> {
        self.panes
            .iter()
            .filter_map(|p| p.melt.as_ref())
            .map(Melt::end)
            .min()
    }

    /// Remove every pane whose melt has ended, drawing the screen again, and
    /// report each as a `Notice::Melted`.
    pub fn end_melts(&mut self, now: i64) {
        let mut melted = Vec::new();
        self.panes.retain(|p| {
            let ended = p.melt.as_ref().is_some_and(|m| now >= m.end());
            if ended {
                melted.push(p.id);
            }
            !ended
        });
        if melted.is_empty() {
            return;
        }
        self.repaint();
        self.notices.extend(melted.into_iter().map(Notice::Melted));
    }

    /// Whether the chrome animates now: a host terminal is connected and
    /// the chrome says so.
    fn animating(&self) -> bool {
        self.connected()
            && self
                .chrome
                .animated(&pane_views(&self.panes, self.focus()), self.ws)
    }

    /// When the next frame of the chrome's animation is due, `None` while it
    /// doesn't animate.
    pub fn animation_deadline(&self) -> Option<i64> {
        self.animating().then(|| self.animation_at + ANIMATION_MS)
    }

    pub fn needs_render(&self) -> bool {
        self.chrome_dirty
            || self.clear
            || self.animation_deadline().is_some_and(|due| now_ms() >= due)
            || self.shared.bell.get()
            || self.shared.title_changed.get()
            || self
                .panes
                .iter()
                .any(|p| p.rect.cols > 0 && p.rect.rows > 0 && (p.melting() || p.needs_render()))
    }

    /// Focus a pane whose program hasn't exited, sending focus events to
    /// the panes that ask for them.
    pub fn set_focus(&mut self, id: PaneId) {
        if self.focus() == Some(id) || !self.pane(id).is_some_and(Pane::live) {
            return;
        }
        if let Some(old) = self.focused()
            && old.mode(Mode::FOCUS_EVENT)
        {
            old.send(b"\x1b[O");
        }
        self.shared.focus.set(Some(id));
        self.notices.push(Notice::Focus(id));
        let next = &self.panes[self.index(id).unwrap()];
        if next.mode(Mode::FOCUS_EVENT) {
            next.send(b"\x1b[I");
        }
        self.chrome_dirty = true;
        self.shared.title_changed.set(true);
    }

    /// Focus the next live pane after the focused one, in pane order.
    pub fn focus_next(&mut self) {
        if let Some(index) = self.focus().and_then(|id| self.index(id)) {
            self.focus_from(index + 1);
        }
    }

    /// Focus the first live pane at or after `start`, wrapping around.
    fn focus_from(&mut self, start: usize) {
        let count = self.panes.len();
        let next = (0..count)
            .map(|step| &self.panes[(start + step) % count])
            .find(|p| p.live() && Some(p.id) != self.focus())
            .map(|p| p.id);
        if let Some(id) = next {
            self.set_focus(id);
        }
    }

    /// Send host input to the focused pane. Typing clears its selection.
    fn send(&mut self, data: &[u8]) {
        if data.is_empty() {
            return;
        }
        if self.host.selection.is_some() && self.host.selection == self.focus() {
            self.clear_selection();
        }
        if let Some(p) = self.focused() {
            p.send(data);
        }
    }

    /// Type text into a pane: each character is a key press and release,
    /// encoded the way the pane's program asked for keys. Control characters
    /// are written as they are.
    pub fn send_text(&self, id: PaneId, text: &str) -> Result<()> {
        let Some(p) = self.pane(id) else {
            return Ok(());
        };
        let mut data = Vec::new();
        let mut encoder = key_encoder(p)?;
        let mut event = key::Event::new()?;
        for c in text.chars() {
            if c.is_control() {
                data.extend_from_slice(c.encode_utf8(&mut [0; 4]).as_bytes());
                continue;
            }
            let mut lower = c.to_lowercase();
            let (unshifted, mods) = match (lower.next(), lower.next()) {
                (Some(l), None) if l != c => (l, key::Mods::SHIFT),
                _ => (c, key::Mods::empty()),
            };
            event
                .set_key(Key::Unidentified)
                .set_utf8(Some(c.to_string()))
                .set_unshifted_codepoint(unshifted)
                .set_mods(mods);
            press_and_release(&mut encoder, &mut event, &mut data)?;
        }
        p.send(&data);
        Ok(())
    }

    /// Press and release a key in a pane, encoded the way the pane's
    /// program asked for keys, including the Kitty keyboard flags.
    pub fn send_key(&self, id: PaneId, key: Key) -> Result<()> {
        let Some(p) = self.pane(id) else {
            return Ok(());
        };
        let mut encoder = key_encoder(p)?;
        let mut event = key::Event::new()?;
        event.set_key(key);
        let mut data = Vec::new();
        press_and_release(&mut encoder, &mut event, &mut data)?;
        p.send(&data);
        Ok(())
    }

    /// Route host input. Ctrl-b and the key after it go to the mux (see
    /// `prefixed_key`). Mouse reports go to the pane under the pointer (see
    /// `mouse`). Shift-PageUp scrolls the focused pane up a page; while its view is scrolled, PageUp,
    /// PageDown, Up, and Down scroll it, Esc and q return it to the bottom,
    /// and any other key returns it to the bottom and goes to the program.
    /// A bracketed paste and everything else go to the focused pane as they
    /// are. A key or escape sequence cut off by the end of a read waits for
    /// the next read, or for `input_deadline`.
    pub fn input(&mut self, data: &[u8]) {
        let mut input = std::mem::take(&mut self.host.held_input);
        input.extend_from_slice(data);
        self.route(&input, false);
    }

    /// The notices since the last call, in order.
    pub fn take_notices(&mut self) -> Vec<Notice> {
        std::mem::take(&mut self.notices)
    }

    /// When held input is taken as it is: a lone Esc as the Esc key, and a
    /// cut-off sequence as typed.
    pub fn input_deadline(&self) -> Option<i64> {
        (!self.host.held_input.is_empty() && self.host.pasting.is_none())
            .then_some(self.host.held_since + INPUT_HOLD_MS)
    }

    /// Route held input as it is; see `input_deadline`.
    pub fn flush_input(&mut self) {
        let input = std::mem::take(&mut self.host.held_input);
        self.route(&input, true);
    }

    fn route(&mut self, data: &[u8], flush: bool) {
        let mut i = 0;
        let mut start = 0;
        while i < data.len() {
            if let Some(id) = self.host.pasting {
                // The paste goes to its pane through its end. An end cut off
                // by the end of the read is held, without a deadline.
                let rest = &data[i..];
                let found = find(rest, PASTE_END);
                let end = match found {
                    Some(n) => i + n + PASTE_END.len(),
                    None => {
                        let keep = (1..PASTE_END.len())
                            .rev()
                            .find(|&k| rest.ends_with(&PASTE_END[..k]))
                            .unwrap_or(0);
                        data.len() - keep
                    }
                };
                if let Some(p) = self.pane(id) {
                    p.send(&data[i..end]);
                }
                if found.is_none() {
                    self.hold(&data[end..]);
                    return;
                }
                self.host.pasting = None;
                i = end;
                start = end;
                continue;
            }
            let flush = flush || data.len() - i > INPUT_HOLD_MAX;
            let Some(end) = token_end(data, i, flush) else {
                self.send(&data[start..i]);
                self.hold(&data[i..]);
                return;
            };
            let token = &data[i..end];
            if self.host.prefixed
                && let Some(key) = prefixed_key(token)
            {
                self.send(&data[start..i]);
                self.host.prefixed = false;
                let captured = self.captured;
                match key {
                    Prefixed::Workspace(n) => self.notices.push(Notice::Workspace(n)),
                    Prefixed::Step(n) => self.notices.push(Notice::Step(n)),
                    Prefixed::Detach => self.notices.push(Notice::Detach),
                    Prefixed::Accent => self.notices.push(Notice::Accent),
                    // The overlay, the list of keys, and the prompt for a
                    // path take the input that follows.
                    Prefixed::Overlay => {
                        self.notices.push(Notice::Overlay);
                        self.set_captured(!captured);
                    }
                    Prefixed::Help => {
                        self.notices.push(Notice::Help);
                        self.set_captured(!captured);
                    }
                    Prefixed::NewWorkspace => {
                        self.notices.push(Notice::NewWorkspace);
                        self.set_captured(true);
                    }
                    _ if captured => {}
                    Prefixed::Prefix => self.send(token),
                    Prefixed::FocusNext => self.focus_next(),
                    Prefixed::Zoom => {
                        if let Some(id) = self.focus() {
                            self.notices.push(Notice::Zoom(id));
                        }
                    }
                    Prefixed::Scroll => self.scroll_focused(ScrollKey::PageUp),
                    Prefixed::Other => {}
                }
                i = end;
                start = end;
                continue;
            }
            if self.captured {
                self.capture(token);
                i = end;
                start = end;
                continue;
            }
            match self.classify(token) {
                Route::Pass => {}
                Route::Bottom => {
                    self.send(&data[start..i]);
                    start = i;
                    if let Some(p) = self.focus().and_then(|id| self.pane_mut(id)) {
                        p.scroll_to_bottom();
                    }
                }
                route => {
                    self.send(&data[start..i]);
                    start = end;
                    match route {
                        Route::Prefix => self.host.prefixed = true,
                        Route::Mouse(event) => self.mouse(event),
                        Route::Scroll(key) => self.scroll_focused(key),
                        _ => {}
                    }
                }
            }
            if token == PASTE_START {
                self.send(&data[start..end]);
                start = end;
                self.host.pasting = self.focus();
            }
            i = end;
        }
        self.send(&data[start..]);
    }

    fn hold(&mut self, input: &[u8]) {
        if !input.is_empty() {
            self.host.held_input = input.to_vec();
            self.host.held_since = now_ms();
        }
    }

    /// Report a key of captured input, or take the prefix key. A paste is
    /// dropped through its end, and so are mouse reports.
    fn capture(&mut self, token: &[u8]) {
        if token == PASTE_START {
            self.host.pasting = Some(NO_PANE);
            return;
        }
        match special_route(token) {
            Some(Route::Prefix) => self.host.prefixed = true,
            Some(_) => {}
            None => self.notices.push(Notice::Key(token.to_vec())),
        }
    }

    /// What to do with one key or escape sequence of host input.
    fn classify(&self, token: &[u8]) -> Route {
        if let Some(route) = special_route(token) {
            return route;
        }
        let key = scroll_key(token);
        if self.focused().is_some_and(|p| p.scrolled() > 0) {
            return match key {
                Some((_, _, RELEASE)) => Route::Drop,
                Some((key, _, _)) => Route::Scroll(key),
                None if released(token) => Route::Drop,
                None => Route::Bottom,
            };
        }
        match key {
            Some((ScrollKey::PageUp, SHIFT, RELEASE)) => Route::Drop,
            Some((ScrollKey::PageUp, SHIFT, _)) => Route::Scroll(ScrollKey::PageUp),
            _ => Route::Pass,
        }
    }

    fn scroll_focused(&mut self, key: ScrollKey) {
        let Some(p) = self.focus().and_then(|id| self.pane_mut(id)) else {
            return;
        };
        let page = p.rect.rows.max(1) as isize;
        match key {
            ScrollKey::LineUp => p.scroll(-1),
            ScrollKey::LineDown => p.scroll(1),
            ScrollKey::PageUp => p.scroll(-page),
            ScrollKey::PageDown => p.scroll(page),
            ScrollKey::Exit => p.scroll_to_bottom(),
        }
    }

    /// Handle a mouse event from the host. A pane whose program reports
    /// the mouse gets the event, translated to its coordinates and encoded
    /// for its mouse modes. In any other pane, the wheel scrolls the view,
    /// or on the alternate screen with alternate scroll mode sends Up and
    /// Down, and a left-button drag selects text, copied to the host's
    /// clipboard when the button is released. A button press focuses the
    /// pane under the pointer, and the button's motion and release go to
    /// that pane, clamped to it. A left-button press outside every pane is a
    /// click on the chrome, and its motion a drag of the chrome.
    fn mouse(&mut self, event: MouseEvent) {
        let wheel = matches!(
            event.button,
            Some(Button::Four | Button::Five | Button::Six | Button::Seven)
        );
        if !wheel
            && event.action != mouse::Action::Press
            && let Some(press) = self.host.press.take()
        {
            let id = match &press {
                Press::Report(id) | Press::Select(id, ..) => *id,
                Press::Chrome => {
                    if event.action == mouse::Action::Motion {
                        self.notices.push(Notice::Drag(event.col, event.row));
                        self.host.press = Some(Press::Chrome);
                    }
                    return;
                }
                Press::Ignore => {
                    if event.action != mouse::Action::Release {
                        self.host.press = Some(Press::Ignore);
                    }
                    return;
                }
            };
            let Some(p) = self.panes.iter_mut().find(|p| p.id == id) else {
                return;
            };
            let col = event
                .col
                .clamp(p.rect.col, p.rect.col + p.rect.cols.max(1) - 1)
                - p.rect.col;
            let row = event
                .row
                .clamp(p.rect.row, p.rect.row + p.rect.rows.max(1) - 1)
                - p.rect.row;
            match &press {
                Press::Report(_) => {
                    let _ = p.report_mouse(&event, col, row);
                }
                Press::Ignore | Press::Chrome => {}
                Press::Select(_, anchor, start) => {
                    // A press released where it began, without moving, is
                    // a click.
                    if (self.host.selection == Some(id) || *start != (col, row))
                        && p.select(anchor, col, row).is_ok()
                    {
                        self.host.selection = Some(id);
                    }
                    if event.action == mouse::Action::Release
                        && self.host.selection == Some(id)
                        && let Ok(Some(text)) = p.selected_text()
                    {
                        let copy = format!("\x1b]52;c;{}\x07", base64(&text));
                        self.shared.side_output(copy.as_bytes());
                    }
                }
            }
            if event.action != mouse::Action::Release {
                self.host.press = Some(press);
            }
            return;
        }

        let Some(index) = self
            .panes
            .iter()
            .position(|p| !p.melting() && p.rect.contains(event.col, event.row))
        else {
            if event.action == mouse::Action::Press && event.button == Some(Button::Left) {
                self.notices.push(Notice::Click(event.col, event.row));
                self.host.press = Some(Press::Chrome);
            }
            return;
        };
        let p = &self.panes[index];
        let (id, reports) = (p.id, p.reports_mouse());
        let (col, row) = (event.col - p.rect.col, event.row - p.rect.row);
        if wheel && !reports {
            self.wheel(index, event.button);
            return;
        }
        if event.action == mouse::Action::Press && !wheel {
            self.clear_selection();
            self.set_focus(id);
            let p = &self.panes[index];
            self.host.press = if reports {
                Some(Press::Report(id))
            } else if event.button == Some(Button::Left) {
                let point = Point::Viewport(PointCoordinate {
                    x: col,
                    y: u32::from(row),
                });
                p.terminal
                    .track_grid_ref(point)
                    .ok()
                    .map(|anchor| Press::Select(id, anchor, (col, row)))
            } else {
                None
            };
        }
        if reports {
            let _ = self.panes[index].report_mouse(&event, col, row);
        }
    }

    /// Scroll a pane that doesn't report the mouse.
    fn wheel(&mut self, index: usize, button: Option<Button>) {
        let up = match button {
            Some(Button::Four) => true,
            Some(Button::Five) => false,
            _ => return,
        };
        let p = &mut self.panes[index];
        if p.terminal.active_screen().ok() != Some(Screen::Alternate) {
            p.scroll(if up { -WHEEL_ROWS } else { WHEEL_ROWS });
        } else if p.mode(Mode::ALT_SCROLL) {
            let (id, key) = (p.id, if up { Key::ArrowUp } else { Key::ArrowDown });
            for _ in 0..WHEEL_ROWS {
                let _ = self.send_key(id, key);
            }
        }
    }

    fn clear_selection(&mut self) {
        if let Some(id) = self.host.selection.take()
            && let Some(p) = self.pane_mut(id)
        {
            let _ = p.clear_selection();
        }
    }

    /// The focused pane, unless input is captured.
    fn input_pane(&self) -> Option<&Pane> {
        if self.captured { None } else { self.focused() }
    }

    fn sync_modes(&mut self, out: &mut Vec<u8>) {
        for (i, &(value, default)) in MIRRORED_MODES.iter().enumerate() {
            let set = match self.input_pane() {
                Some(p) => p.mode(dec(value)),
                None => self.mode_default(value, default),
            };
            if Some(set) != self.host.modes[i] {
                let _ = write!(out, "\x1b[?{}{}", value, if set { 'h' } else { 'l' });
                self.host.modes[i] = Some(set);
            }
        }

        // The host reports button presses, releases, and drags in SGR form,
        // and motion without a button while the focused pane asks for it.
        // Resetting any tracking mode turns tracking off, so the mode to use
        // is set last.
        let any_motion = self.input_pane().is_some_and(|p| p.mode(Mode::ANY_MOUSE));
        if self.host.any_motion != Some(any_motion) {
            out.extend_from_slice(b"\x1b[?1006h\x1b[?1003l\x1b[?1002h");
            if any_motion {
                out.extend_from_slice(b"\x1b[?1003h");
            }
            self.host.any_motion = Some(any_motion);
        }

        let flags = self.input_pane().map_or(0, |p| {
            p.terminal.kitty_keyboard_flags().map_or(0, |f| f.bits())
        });
        if Some(flags) != self.host.kitty_flags {
            let _ = write!(out, "\x1b[={flags};1u");
            self.host.kitty_flags = Some(flags);
        }
    }

    /// Draw the chrome and the rows that changed since the last frame, the
    /// cursor, the input modes, the focused pane's title if it changed, and
    /// a bell if a pane rang one into the output. Draws nothing while no
    /// host terminal is connected.
    pub fn render(&mut self) -> Result<()> {
        if !self.connected() {
            return Ok(());
        }
        let shared = self.shared.clone();
        let mut out = shared.out.borrow_mut();
        if shared.bell.take() {
            out.push(0x07);
        }
        if shared.title_changed.take() {
            out.extend_from_slice(b"\x1b]0;");
            if let Some(p) = self.focused() {
                out.extend_from_slice(p.terminal.title().unwrap_or(b""));
            }
            out.push(0x07);
        }
        out.extend_from_slice(b"\x1b[?2026h\x1b[?25l");
        // Grapheme clustering must be synchronized before drawing rows.
        self.sync_modes(&mut out);

        // Whatever the animation drew is cleared when it ends.
        let animated = self.animating();
        if self.animated && !animated {
            self.repaint();
        }
        self.animated = animated;

        if self.clear {
            out.extend_from_slice(b"\x1b[0m\x1b[2J");
            self.clear = false;
        }
        for p in &mut self.panes {
            let scrolled = p.scrolled();
            if scrolled != p.scrolled_drawn {
                p.scrolled_drawn = scrolled;
                self.chrome_dirty = true;
            }
        }
        let now = now_ms();
        let redrawn = self.chrome_dirty;
        if redrawn {
            self.chrome
                .draw(&mut out, &pane_views(&self.panes, self.focus()), self.ws);
            self.chrome_dirty = false;
        }
        if animated && (redrawn || now >= self.animation_at + ANIMATION_MS) {
            let views = pane_views(&self.panes, self.focus());
            self.chrome.animate(&mut out, &views, self.ws, now);
            self.animation_at = now;
        }
        for p in &mut self.panes {
            if p.rect.cols == 0 || p.rect.rows == 0 {
                continue;
            }
            if p.melting() {
                p.draw_melt(&mut out, now);
            } else {
                p.render(&mut out)?;
            }
        }

        let Some(p) = self.input_pane() else {
            out.extend_from_slice(b"\x1b[?2026l");
            return Ok(());
        };
        let cursor = p.cursor();
        if let Some(pos) = cursor.viewport {
            let _ = write!(
                out,
                "\x1b[{};{}H",
                pos.y + p.rect.row + 1,
                pos.x + p.rect.col + 1
            );
        }
        let mut shape: u8 = match cursor.visual_style {
            CursorVisualStyle::Bar => 6,
            CursorVisualStyle::Underline => 4,
            _ => 2,
        };
        if cursor.blinking {
            shape -= 1;
        }
        let _ = write!(out, "\x1b[{shape} q");
        if cursor.visible && cursor.viewport.is_some() && p.rect.cols > 0 {
            out.extend_from_slice(b"\x1b[?25h");
        }
        out.extend_from_slice(b"\x1b[?2026l");
        Ok(())
    }
}

fn key_encoder(p: &Pane) -> Result<key::Encoder<'static>> {
    let mut encoder = key::Encoder::new()?;
    encoder.set_options_from_terminal(&p.terminal);
    Ok(encoder)
}

fn press_and_release(
    encoder: &mut key::Encoder,
    event: &mut key::Event,
    data: &mut Vec<u8>,
) -> Result<()> {
    for action in [Action::Press, Action::Release] {
        event.set_action(action);
        // encode_to_vec only grows an empty Vec enough for the sequence.
        let mut sequence = Vec::new();
        encoder.encode_to_vec(event, &mut sequence)?;
        data.extend_from_slice(&sequence);
    }
    Ok(())
}

/// What turns the host terminal back from anything the mux mirrored onto
/// it, leaving it on the alternate screen: every mirrored mode at its
/// default, no Kitty keyboard flags, the default cursor, default SGR
/// attributes, and synchronized output ended, in case a frame was cut off. `clusters` is whether the host clusters graphemes by default,
/// if known.
pub fn host_reset(clusters: Option<bool>) -> Vec<u8> {
    let mut out = Vec::new();
    for &(value, default) in &MIRRORED_MODES {
        let default = if value == 2027 {
            let Some(clusters) = clusters else { continue };
            clusters
        } else {
            default
        };
        let _ = write!(out, "\x1b[?{}{}", value, if default { 'h' } else { 'l' });
    }
    out.extend_from_slice(
        b"\x1b[?1003l\x1b[?1002l\x1b[?1006l\x1b[=0;1u\x1b[0 q\x1b[0m\x1b[?25h\x1b[?2026l",
    );
    out
}

/// `host_reset`, then back to the main screen.
pub fn host_restore(clusters: Option<bool>) -> Vec<u8> {
    let mut out = host_reset(clusters);
    out.extend_from_slice(b"\x1b[?1049l");
    out
}

/// An exit status as a shell reports it: the code, or 128 plus the signal.
fn exit_status(status: std::process::ExitStatus) -> u8 {
    use std::os::unix::process::ExitStatusExt as _;
    match (status.code(), status.signal()) {
        (Some(code), _) => code as u8,
        (None, Some(signal)) => 128u8.wrapping_add(signal as u8),
        (None, None) => 0,
    }
}

/// The width and height of a cell of `ws` in pixels, or 0 if unknown.
fn cell_pixels(ws: &Winsize) -> (u32, u32) {
    (
        u32::from(ws.ws_xpixel.checked_div(ws.ws_col).unwrap_or(0)),
        u32::from(ws.ws_ypixel.checked_div(ws.ws_row).unwrap_or(0)),
    )
}

/// The route of a key the mux takes whatever pane has the focus: the prefix
/// key, in legacy or Kitty form, and mouse reports.
fn special_route(token: &[u8]) -> Option<Route> {
    if token == [CTRL_B] {
        return Some(Route::Prefix);
    }
    let (&last, params) = token.strip_prefix(b"\x1b[")?.split_last()?;
    if last == b'u'
        && let Some((98, 5, event)) = csi_key(params)
    {
        // Only a press is the prefix.
        return Some(if event == 1 {
            Route::Prefix
        } else {
            Route::Drop
        });
    }
    // The host reports the mouse only in SGR form.
    if last == b'M' || last == b'm' {
        let sgr = params.strip_prefix(b"<")?;
        return Some(MouseEvent::parse(sgr, last).map_or(Route::Drop, Route::Mouse));
    }
    None
}

/// How host input is routed; see `Mux::input`.
enum Route {
    /// To the focused pane.
    Pass,
    /// Nowhere.
    Drop,
    /// The next key goes with the prefix; see `prefixed_key`.
    Prefix,
    Mouse(MouseEvent),
    Scroll(ScrollKey),
    /// To the focused pane, after returning its view to the bottom.
    Bottom,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum ScrollKey {
    LineUp,
    LineDown,
    PageUp,
    PageDown,
    /// Return to the bottom.
    Exit,
}

/// The legacy byte of Ctrl-b.
const CTRL_B: u8 = 0x02;
/// The Kitty key codes of the modifier keys, from left Shift to right Meta.
const MODIFIER_KEYS: std::ops::RangeInclusive<u32> = 57441..=57452;
/// The Kitty modifiers value of Shift alone.
const SHIFT: u16 = 2;
/// The Kitty event type of a key release.
const RELEASE: u8 = 3;
/// The bits of Caps Lock and Num Lock in a Kitty modifiers value less one.
const LOCKS: u16 = 64 | 128;

/// A key's code, Kitty modifiers value, and Kitty event type from the
/// parameters of a CSI sequence in Kitty's key;modifiers:event;... form,
/// which legacy sequences such as `CSI 5 ; 2 ~` also take. Alternate key
/// fields and the lock modifiers are ignored, and missing fields take their
/// defaults.
fn csi_key(params: &[u8]) -> Option<(u32, u16, u8)> {
    let mut fields = params.split(|&b| b == b';');
    let key = fields.next()?.split(|&b| b == b':').next()?;
    let key = if key.is_empty() { 1 } else { parse_int(key)? };
    let Some(modifiers) = fields.next() else {
        return Some((key, 1, 1));
    };
    let mut modifiers = modifiers.split(|&b| b == b':');
    let mods = parse_int::<u16>(modifiers.next()?)?;
    let mods = (mods.saturating_sub(1) & !LOCKS) + 1;
    let event = match modifiers.next() {
        Some(e) => parse_int(e)?,
        None => 1,
    };
    Some((key, mods, event))
}

/// What the key pressed after the prefix key does.
#[derive(Debug, PartialEq)]
enum Prefixed {
    /// 0 to 9: switch to the workspace numbered by this digit.
    Workspace(u8),
    /// ( and ): switch this many workspaces along the bar.
    Step(i8),
    /// c: ask for the path of a new workspace.
    NewWorkspace,
    /// s: open or close the overlay.
    Overlay,
    /// d: detach.
    Detach,
    /// a: change the workspace's color.
    Accent,
    /// ?: open or close the list of keys.
    Help,
    /// o: focus the next live pane.
    FocusNext,
    /// z: zoom the focused pane, or restore the layout.
    Zoom,
    /// [ and PageUp: scroll the focused pane up a page.
    Scroll,
    /// Ctrl-b: send the prefix key itself to the focused pane.
    Prefix,
    /// Nothing.
    Other,
}

impl Prefixed {
    /// What typing `c` after the prefix key does.
    fn typed(c: Option<char>) -> Prefixed {
        match c {
            Some(digit @ '0'..='9') => Prefixed::Workspace(digit as u8 - b'0'),
            Some('(') => Prefixed::Step(-1),
            Some(')') => Prefixed::Step(1),
            Some('c') => Prefixed::NewWorkspace,
            Some('s') => Prefixed::Overlay,
            Some('d') => Prefixed::Detach,
            Some('a') => Prefixed::Accent,
            Some('?') => Prefixed::Help,
            Some('o') => Prefixed::FocusNext,
            Some('z') => Prefixed::Zoom,
            Some('[') => Prefixed::Scroll,
            _ => Prefixed::Other,
        }
    }
}

/// What a token does after the prefix key, in legacy or Kitty form, or
/// `None` if it is no key press and is routed as usual: a mouse or focus
/// report, a paste, a key's release or repeat, or a modifier key.
fn prefixed_key(token: &[u8]) -> Option<Prefixed> {
    match token {
        [CTRL_B] => return Some(Prefixed::Prefix),
        PASTE_START | b"\x1b[I" | b"\x1b[O" => return None,
        [byte] => return Some(Prefixed::typed(Some(char::from(*byte)))),
        _ => {}
    }
    let Some((&last, params)) = token
        .strip_prefix(b"\x1b[")
        .and_then(|rest| rest.split_last())
    else {
        return Some(Prefixed::Other);
    };
    if params.starts_with(b"<") {
        return None;
    }
    let Some((code, mods, event)) = csi_key(params) else {
        return Some(Prefixed::Other);
    };
    if event != 1 || last == b'u' && MODIFIER_KEYS.contains(&code) {
        return None;
    }
    Some(match (last, code, mods) {
        (b'~', 5, 1) => Prefixed::Scroll,
        (b'u', 98, 5) => Prefixed::Prefix,
        (b'u', _, 1 | SHIFT) => Prefixed::typed(kitty_char(params, code, mods)),
        _ => Prefixed::Other,
    })
}

/// The character a Kitty key press without modifiers or with Shift types:
/// the key itself, or with Shift its shifted key if reported, or else the
/// character Shift gives the key on a US keyboard.
fn kitty_char(params: &[u8], code: u32, mods: u16) -> Option<char> {
    let c = char::from_u32(code)?;
    if mods != SHIFT {
        return Some(c);
    }
    let shifted = params
        .split(|&b| b == b';')
        .next()?
        .split(|&b| b == b':')
        .nth(1);
    if let Some(shifted) = shifted.and_then(parse_int) {
        return char::from_u32(shifted);
    }
    const UNSHIFTED: &str = "`1234567890-=[]\\;',./";
    const SHIFTED: &str = "~!@#$%^&*()_+{}|:\"<>?";
    match UNSHIFTED.find(c) {
        Some(i) => SHIFTED[i..].chars().next(),
        None => Some(c.to_ascii_uppercase()),
    }
}

/// The scrolling key a token is, with its Kitty modifiers value and event
/// type, in legacy or Kitty form.
fn scroll_key(token: &[u8]) -> Option<(ScrollKey, u16, u8)> {
    match token {
        b"\x1b" | b"q" => return Some((ScrollKey::Exit, 1, 1)),
        b"\x1bOA" => return Some((ScrollKey::LineUp, 1, 1)),
        b"\x1bOB" => return Some((ScrollKey::LineDown, 1, 1)),
        _ => {}
    }
    let (&last, params) = token.strip_prefix(b"\x1b[")?.split_last()?;
    let (code, mods, event) = csi_key(params)?;
    let key = match (last, code) {
        (b'A', 1) => ScrollKey::LineUp,
        (b'B', 1) => ScrollKey::LineDown,
        (b'~', 5) => ScrollKey::PageUp,
        (b'~', 6) => ScrollKey::PageDown,
        (b'u', 27 | 113) if mods == 1 => ScrollKey::Exit,
        _ => return None,
    };
    Some((key, mods, event))
}

/// Whether a token is a Kitty key release.
fn released(token: &[u8]) -> bool {
    token
        .strip_prefix(b"\x1b[")
        .and_then(|t| csi_key(&t[..t.len().saturating_sub(1)]))
        .is_some_and(|(_, _, event)| event == RELEASE)
}

/// A mouse event the host reported, at a cell of the host terminal.
struct MouseEvent {
    action: mouse::Action,
    button: Option<Button>,
    mods: key::Mods,
    col: u16,
    row: u16,
}

impl MouseEvent {
    /// Parse the parameters of an SGR mouse report after its `<`, ending
    /// in `last`, M for a press or motion and m for a release.
    fn parse(params: &[u8], last: u8) -> Option<MouseEvent> {
        let mut fields = params.split(|&b| b == b';');
        let mut field = || fields.next().and_then(parse_int::<u16>);
        let (code, x, y) = (field()?, field()?, field()?);
        if fields.next().is_some() || x == 0 || y == 0 {
            return None;
        }
        let button = match code & 0b1100_0011 {
            0 => Some(Button::Left),
            1 => Some(Button::Middle),
            2 => Some(Button::Right),
            3 => None,
            64 => Some(Button::Four),
            65 => Some(Button::Five),
            66 => Some(Button::Six),
            67 => Some(Button::Seven),
            128 => Some(Button::Eight),
            129 => Some(Button::Nine),
            130 => Some(Button::Ten),
            131 => Some(Button::Eleven),
            _ => return None,
        };
        let action = if last == b'm' {
            mouse::Action::Release
        } else if code & 32 != 0 {
            mouse::Action::Motion
        } else {
            mouse::Action::Press
        };
        let mut mods = key::Mods::empty();
        for (bit, m) in [
            (4, key::Mods::SHIFT),
            (8, key::Mods::ALT),
            (16, key::Mods::CTRL),
        ] {
            if code & bit != 0 {
                mods |= m;
            }
        }
        Some(MouseEvent {
            action,
            button,
            mods,
            col: x - 1,
            row: y - 1,
        })
    }
}

/// A mouse button held since it was pressed in a pane.
enum Press {
    /// Reported to the pane's program.
    Report(PaneId),
    /// Selecting text in the pane from where the button was pressed, at
    /// this column and row of the pane.
    Select(PaneId, TrackedGridRef, (u16, u16)),
    /// The pane's program was replaced; the rest of the press is dropped.
    Ignore,
    /// Pressed outside every pane: its motion drags the chrome.
    Chrome,
}

/// Finds the desktop notifications in a program's output, OSC 9 and OSC
/// 777, including those split across reads.
#[derive(Default)]
struct Notifications {
    state: OscState,
    body: Vec<u8>,
}

#[derive(Clone, Copy, Default)]
enum OscState {
    #[default]
    Ground,
    Esc,
    /// In an OSC sequence's body.
    Body,
    /// After an Esc in the body, which ends it if a backslash follows.
    BodyEsc,
}

impl Notifications {
    /// Scan output, writing each complete notification into `out`.
    fn scan(&mut self, data: &[u8], out: &mut Vec<u8>) {
        for &b in data {
            self.state = match (self.state, b) {
                (OscState::Body, 0x07) | (OscState::BodyEsc, b'\\') => {
                    self.finish(out);
                    OscState::Ground
                }
                (OscState::Body, 0x1b) => OscState::BodyEsc,
                // Other control characters end the sequence unfinished.
                (OscState::Body, 0x00..=0x1f) => OscState::Ground,
                (OscState::Body, _) => {
                    if self.body.len() <= NOTIFICATION_MAX {
                        self.body.push(b);
                    }
                    OscState::Body
                }
                (OscState::Esc | OscState::BodyEsc, b']') => {
                    self.body.clear();
                    OscState::Body
                }
                (_, 0x1b) => OscState::Esc,
                _ => OscState::Ground,
            };
        }
    }

    fn finish(&mut self, out: &mut Vec<u8>) {
        if self.body.len() <= NOTIFICATION_MAX
            && (self.body.starts_with(b"9;") || self.body.starts_with(b"777;"))
        {
            out.extend_from_slice(b"\x1b]");
            out.extend_from_slice(&self.body);
            out.push(0x07);
        }
    }
}

fn base64(data: &[u8]) -> String {
    const ALPHABET: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut s = String::with_capacity(data.len().div_ceil(3) * 4);
    for chunk in data.chunks(3) {
        let n = chunk
            .iter()
            .enumerate()
            .fold(0u32, |n, (i, &b)| n | u32::from(b) << (16 - 8 * i));
        for i in 0..4 {
            if i <= chunk.len() {
                s.push(ALPHABET[(n >> (18 - 6 * i)) as usize & 63] as char);
            } else {
                s.push('=');
            }
        }
    }
    s
}

fn parse_int<T: std::str::FromStr>(s: &[u8]) -> Option<T> {
    std::str::from_utf8(s).ok()?.parse().ok()
}

/// Where the key or escape sequence at `i` ends: a CSI or SS3 sequence, Alt
/// and a byte, Esc, or a character. Returns `None` if the end of `data` cuts
/// it off, unless `flush`, when it runs to the end of `data`.
fn token_end(data: &[u8], i: usize, flush: bool) -> Option<usize> {
    let len = data.len();
    let end = match (data[i], data.get(i + 1)) {
        (0x1b, Some(b'[')) => data[i + 2..]
            .iter()
            .position(|b| (0x40..=0x7e).contains(b))
            .map(|fin| i + 2 + fin + 1),
        (0x1b, Some(b'O')) => Some(i + 3),
        (0x1b, Some(_)) => Some(i + 2),
        (0x1b, None) => None,
        (b, _) => Some(i + utf8_sequence_length(b)),
    };
    match end {
        Some(end) if end <= len => Some(end),
        _ => flush.then_some(len),
    }
}

fn utf8_sequence_length(first: u8) -> usize {
    match first {
        0x00..=0x7f => 1,
        0xc0..=0xdf => 2,
        0xe0..=0xef => 3,
        0xf0..=0xf7 => 4,
        _ => 1,
    }
}

pub fn now_ms() -> i64 {
    let mut ts = libc::timespec {
        tv_sec: 0,
        tv_nsec: 0,
    };
    unsafe { libc::clock_gettime(libc::CLOCK_MONOTONIC, &mut ts) };
    ts.tv_sec * 1000 + ts.tv_nsec / 1_000_000
}

/// Read into `buf`, retrying when a signal interrupts the read. Returns
/// the byte count, 0 at end of input, or -1 on any other error.
pub fn read(fd: RawFd, buf: &mut [u8]) -> isize {
    loop {
        let n = unsafe { libc::read(fd, buf.as_mut_ptr().cast(), buf.len()) };
        if n >= 0 || std::io::Error::last_os_error().raw_os_error() != Some(libc::EINTR) {
            return n;
        }
    }
}

pub fn write_all(fd: RawFd, data: &[u8]) {
    let mut rest = data;
    while !rest.is_empty() {
        let n = unsafe { libc::write(fd, rest.as_ptr().cast(), rest.len()) };
        if n < 0 {
            match std::io::Error::last_os_error().raw_os_error() {
                Some(libc::EINTR | libc::EAGAIN) => continue,
                _ => return,
            }
        }
        rest = &rest[n as usize..];
    }
}

//------------------------------------------------------------------------
// Rendering

/// One row's cells, gathered from the render state or the terminal.
#[derive(Default)]
struct RowBuf {
    cells: Vec<(RawCell, Style, usize, usize)>,
    text: Vec<char>,
    /// The first and last selected columns, drawn in reverse video.
    selected: Option<(u16, u16)>,
}

impl RowBuf {
    fn clear(&mut self, selected: Option<(u16, u16)>) {
        self.cells.clear();
        self.text.clear();
        self.selected = selected;
    }

    /// Add a cell whose text `graphemes` writes into a buffer, returning
    /// its length.
    fn push(
        &mut self,
        raw: RawCell,
        style: Style,
        graphemes: impl Fn(&mut [char]) -> Result<usize>,
    ) -> Result<()> {
        let start = self.text.len();
        let mut room = 8;
        let len = loop {
            self.text.resize(start + room, '\0');
            match graphemes(&mut self.text[start..]) {
                Ok(len) => break len,
                Err(Error::OutOfSpace { required }) if required > room => room = required,
                Err(e) => return Err(e),
            }
        };
        self.text.truncate(start + len);
        self.cells.push((raw, style, start, len));
        Ok(())
    }

    /// Write the row as row `y` of `rect`, leaving out cells that don't fit,
    /// as when a held pane is drawn at a new size.
    fn write(&self, out: &mut Vec<u8>, y: u16, rect: Rect) {
        let (row, col) = (rect.row, rect.col);
        let _ = write!(out, "\x1b[{};{}H", y + row + 1, col + 1);

        let mut prev: Option<Style> = None;
        for (x, &(raw, style, start, len)) in self.cells.iter().enumerate() {
            let wide = raw.wide().unwrap_or(CellWide::Narrow);
            if wide == CellWide::SpacerTail {
                continue;
            }
            let width = if wide == CellWide::Wide { 2 } else { 1 };
            if x + width > usize::from(rect.cols) {
                break;
            }

            let mut s = cell_style(raw, style);
            if self
                .selected
                .is_some_and(|(first, last)| (usize::from(first)..=usize::from(last)).contains(&x))
            {
                s.inverse = !s.inverse;
            }
            write_cell(
                out,
                col + x as u16,
                raw,
                s,
                &self.text[start..start + len],
                &mut prev,
            );
        }
        out.extend_from_slice(b"\x1b[0m");
    }
}

/// Write one cell's text at the cursor with `style`, and a space if it has
/// none. `col` is its 0-based column on the host terminal, for re-syncing
/// after a wide character. `prev` is the style written last, so an SGR
/// sequence is written only when the style changes.
fn write_cell(
    out: &mut Vec<u8>,
    col: u16,
    raw: RawCell,
    style: Style,
    text: &[char],
    prev: &mut Option<Style>,
) {
    if *prev != Some(style) {
        write_sgr(out, style);
        *prev = Some(style);
    }
    if raw.has_text().unwrap_or(false) && !text.is_empty() {
        let mut buf = [0; 4];
        for &c in text {
            out.extend_from_slice(c.encode_utf8(&mut buf).as_bytes());
        }
    } else {
        out.push(b' ');
    }
    // Re-sync the column after a wide character in case the host terminal
    // disagrees about its width.
    if raw.wide().unwrap_or(CellWide::Narrow) == CellWide::Wide {
        let _ = write!(out, "\x1b[{}G", u32::from(col) + 3);
    }
}

/// Write a blank in the default style at the cursor, after the style written
/// last, `prev`.
fn write_blank(out: &mut Vec<u8>, prev: &mut Option<Style>) {
    let style = Style::default();
    if *prev != Some(style) {
        write_sgr(out, style);
        *prev = Some(style);
    }
    out.push(b' ');
}

/// The style a cell is drawn with. Cells cleared with a background color
/// carry it in the cell content rather than the style.
pub fn cell_style(raw: RawCell, style: Style) -> Style {
    let mut s = if raw.has_styling().unwrap_or(false) {
        style
    } else {
        Style::default()
    };
    match raw.content_tag() {
        Ok(CellContentTag::BgColorPalette) => {
            if let Ok(i) = raw.bg_color_palette() {
                s.bg_color = StyleColor::Palette(i);
            }
        }
        Ok(CellContentTag::BgColorRgb) => {
            if let Ok(rgb) = raw.bg_color_rgb() {
                s.bg_color = StyleColor::Rgb(rgb);
            }
        }
        _ => {}
    }
    s
}

fn write_color(out: &mut Vec<u8>, base: u8, color: StyleColor) {
    match color {
        StyleColor::None => {}
        StyleColor::Palette(i) => {
            let _ = write!(out, ";{base};5;{}", i.0);
        }
        StyleColor::Rgb(c) => {
            let _ = write!(out, ";{base};2;{};{};{}", c.r, c.g, c.b);
        }
    }
}

/// Write the SGR sequence that fully describes a style.
fn write_sgr(out: &mut Vec<u8>, s: Style) {
    out.extend_from_slice(b"\x1b[0");
    if s.bold {
        out.extend_from_slice(b";1");
    }
    if s.faint {
        out.extend_from_slice(b";2");
    }
    if s.italic {
        out.extend_from_slice(b";3");
    }
    match s.underline {
        Underline::None => {}
        Underline::Single => out.extend_from_slice(b";4"),
        u => {
            let _ = write!(out, ";4:{}", u as i32);
        }
    }
    if s.blink {
        out.extend_from_slice(b";5");
    }
    if s.inverse {
        out.extend_from_slice(b";7");
    }
    if s.invisible {
        out.extend_from_slice(b";8");
    }
    if s.strikethrough {
        out.extend_from_slice(b";9");
    }
    if s.overline {
        out.extend_from_slice(b";53");
    }
    write_color(out, 38, s.fg_color);
    write_color(out, 48, s.bg_color);
    write_color(out, 58, s.underline_color);
    out.push(b'm');
}

//------------------------------------------------------------------------
// Host state

/// Queries for the host's default colors, its 16 ANSI palette colors, and
/// whether it clusters graphemes (mode 2027), followed by DA1. Every
/// terminal answers DA1, so its reply marks the end of the host's replies.
/// The replies stay small enough to fit in the tty input buffer while the
/// queries are still being written.
pub fn host_queries() -> Vec<u8> {
    let mut q = b"\x1b]10;?\x1b\\\x1b]11;?\x1b\\".to_vec();
    for i in 0..16 {
        let _ = write!(q, "\x1b]4;{i};?\x1b\\");
    }
    q.extend_from_slice(b"\x1b[?2027$p\x1b[c");
    q
}

/// The pieces of `data` after each occurrence of `sep`.
fn after_each<'a>(data: &'a [u8], sep: &'a [u8]) -> impl Iterator<Item = &'a [u8]> {
    let mut rest = find(data, sep).map(|i| &data[i + sep.len()..]);
    std::iter::from_fn(move || {
        let r = rest?;
        match find(r, sep) {
            Some(i) => {
                rest = Some(&r[i + sep.len()..]);
                Some(&r[..i])
            }
            None => {
                rest = None;
                Some(r)
            }
        }
    })
}

fn find(data: &[u8], needle: &[u8]) -> Option<usize> {
    data.windows(needle.len()).position(|w| w == needle)
}

/// True once `replies` holds the host's DA1 reply.
pub fn host_replies_done(replies: &[u8]) -> bool {
    after_each(replies, b"\x1b[?").any(|reply| {
        reply
            .iter()
            .find(|b| !b.is_ascii_digit() && **b != b';')
            .is_some_and(|&b| b == b'c')
    })
}

/// Whether the host's replies say it clusters graphemes.
pub fn host_clusters(replies: &[u8]) -> bool {
    const REPLY: &[u8] = b"\x1b[?2027;";
    find(replies, REPLY)
        .and_then(|i| replies.get(i + REPLY.len()))
        .is_some_and(|&b| b == b'1' || b == b'3')
}

/// Take the host's replies to `host_queries` as the terminal's colors, so
/// it answers the program's OSC 10, 11, and 4 queries the way the host
/// would, and as its default for grapheme clustering, so the host lays out
/// rows the way the terminal does.
pub fn use_host_replies_for(t: &mut Terminal, replies: &[u8]) -> Result<()> {
    if host_clusters(replies) {
        t.set_default_mode(Mode::GRAPHEME_CLUSTER, true)?;
    }

    let mut palette = Palette::default();
    let mut have_palette = false;
    for reply in after_each(replies, b"\x1b]") {
        let Some(end) = reply.iter().position(|&b| b == 0x1b || b == 0x07) else {
            continue;
        };
        let mut fields = reply[..end].splitn(2, |&b| b == b';');
        let Some(code) = fields.next().and_then(parse_int::<u8>) else {
            continue;
        };
        let rest = fields.next().unwrap_or(b"");
        match code {
            4 => {
                let mut fields = rest.splitn(2, |&b| b == b';');
                let Some(i) = fields.next().and_then(parse_int::<u8>) else {
                    continue;
                };
                let Some(rgb) = parse_rgb(fields.next().unwrap_or(b"")) else {
                    continue;
                };
                palette.0[usize::from(i)] = rgb;
                have_palette = true;
            }
            10 | 11 => {
                let Some(rgb) = parse_rgb(rest) else { continue };
                if code == 10 {
                    t.set_default_fg_color(Some(rgb))?;
                } else {
                    t.set_default_bg_color(Some(rgb))?;
                }
            }
            _ => {}
        }
    }
    if have_palette {
        t.set_default_color_palette(Some(palette))?;
    }
    Ok(())
}

fn parse_rgb(s: &[u8]) -> Option<RgbColor> {
    RgbColor::parse(std::str::from_utf8(s).ok()?).ok()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn notifications(reads: &[&[u8]]) -> Vec<u8> {
        let mut scan = Notifications::default();
        let mut out = Vec::new();
        for read in reads {
            scan.scan(read, &mut out);
        }
        out
    }

    #[test]
    fn notifications_are_found_across_any_split() {
        let output = b"a\x1b]0;title\x07b\x1b]9;done\x1b\\c\x1b]777;notify;T;B\x07d";
        let expected = b"\x1b]9;done\x07\x1b]777;notify;T;B\x07";
        for split in 0..=output.len() {
            let (first, second) = output.split_at(split);
            assert_eq!(
                notifications(&[first, second]),
                expected,
                "split at {split}"
            );
        }
    }

    #[test]
    fn unfinished_and_oversized_notifications_are_dropped() {
        assert_eq!(notifications(&[b"\x1b]9;a\nb\x07"]), b"");
        assert_eq!(notifications(&[b"\x1b]9;a\x1b[mb\x07"]), b"");
        let long = [b"\x1b]9;".as_slice(), &[b'x'; NOTIFICATION_MAX], b"\x07"].concat();
        assert_eq!(notifications(&[&long]), b"");
        // An OSC sequence started over another's Esc is scanned on its own.
        assert_eq!(notifications(&[b"\x1b]2;x\x1b]9;y\x07"]), b"\x1b]9;y\x07");
    }

    #[test]
    fn base64_pads_to_whole_quanta() {
        for (data, encoded) in [
            ("", ""),
            ("f", "Zg=="),
            ("fo", "Zm8="),
            ("foo", "Zm9v"),
            ("ine 22\nline", "aW5lIDIyCmxpbmU="),
        ] {
            assert_eq!(base64(data.as_bytes()), encoded);
        }
    }

    #[test]
    fn scroll_keys_are_read_in_legacy_and_kitty_forms() {
        use ScrollKey::*;
        let key = scroll_key;
        let (up, page_up, page_down, exit) = (LineUp, PageUp, PageDown, Exit);
        assert_eq!(key(b"\x1b[A"), Some((up, 1, 1)));
        assert_eq!(key(b"\x1bOA"), Some((up, 1, 1)));
        assert_eq!(key(b"\x1b[5;2~"), Some((page_up, 2, 1)));
        assert_eq!(key(b"\x1b[5;2:3~"), Some((page_up, 2, 3)));
        assert_eq!(key(b"\x1b[6~"), Some((page_down, 1, 1)));
        assert_eq!(key(b"\x1b"), Some((exit, 1, 1)));
        assert_eq!(key(b"\x1b[27u"), Some((exit, 1, 1)));
        assert_eq!(key(b"\x1b[113;1:1u"), Some((exit, 1, 1)));
        assert_eq!(key(b"\x1b[113;2u"), None);
        assert_eq!(key(b"\x1b[3~"), None);
        assert!(released(b"\x1b[97;1:3u"));
        assert!(!released(b"\x1b[97u"));
    }

    #[test]
    fn tokens_end_after_whole_keys_and_sequences() {
        let ends = |data: &[u8], flush| {
            let mut i = 0;
            let mut ends = Vec::new();
            while let Some(end) = token_end(data, i, flush) {
                ends.push(end);
                i = end;
                if i == data.len() {
                    break;
                }
            }
            ends
        };
        let data = b"a\x1b[5;2~\x1bOA\xc3\xa9\x1bx\x1b";
        assert_eq!(ends(data, false), [1, 7, 10, 12, 14]);
        assert_eq!(ends(data, true), [1, 7, 10, 12, 14, 15]);
        for cut in [b"\x1b[5;2".as_slice(), b"\x1bO", b"\xc3"] {
            assert_eq!(ends(cut, false), Vec::<usize>::new());
            assert_eq!(ends(cut, true), [cut.len()]);
        }
    }

    /// A mux with two panes side by side, each 40 columns and 10 rows,
    /// whose input goes to a pipe in place of a pty.
    struct Piped {
        mux: Mux,
        ids: [PaneId; 2],
        pipes: Vec<(OwnedFd, OwnedFd)>,
    }

    impl Piped {
        fn new() -> Piped {
            let ws = Winsize {
                ws_col: 81,
                ws_row: 10,
                ws_xpixel: 0,
                ws_ypixel: 0,
            };
            let mut mux = Mux::new(ws, Box::new(NoChrome));
            mux.connect_host(&[], ws).unwrap();
            let ids = [
                mux.add_pane("left").unwrap(),
                mux.add_pane("right").unwrap(),
            ];
            let rect = |col| Rect {
                col,
                row: 0,
                cols: 40,
                rows: 10,
            };
            mux.set_layout(&[(ids[0], rect(0)), (ids[1], rect(41))])
                .unwrap();
            let mut pipes = Vec::new();
            for id in ids {
                let mut fds = [0; 2];
                assert_eq!(unsafe { libc::pipe(fds.as_mut_ptr()) }, 0);
                unsafe { libc::fcntl(fds[0], libc::F_SETFL, libc::O_NONBLOCK) };
                let (r, w) = unsafe {
                    use std::os::fd::FromRawFd as _;
                    (OwnedFd::from_raw_fd(fds[0]), OwnedFd::from_raw_fd(fds[1]))
                };
                mux.pane(id).unwrap().state.pty.set(Some(w.as_raw_fd()));
                pipes.push((r, w));
            }
            Piped { mux, ids, pipes }
        }

        /// Give pane `i` a new program, reading what the old one would have.
        fn reattach(&self, i: usize) {
            let id = self.ids[i];
            let fd = self.pipes[i].1.as_raw_fd();
            self.mux.pane(id).unwrap().state.pty.set(Some(fd));
        }

        /// What pane `i`'s program got since the last call.
        fn got(&self, i: usize) -> Vec<u8> {
            let mut got = Vec::new();
            let mut buf = [0; 4096];
            loop {
                let n = read(self.pipes[i].0.as_raw_fd(), &mut buf);
                if n <= 0 {
                    return got;
                }
                got.extend_from_slice(&buf[..n as usize]);
            }
        }

        /// Feed `data` as two reads split at `at`.
        fn input(&mut self, data: &[u8], at: usize) {
            let (a, b) = data.split_at(at);
            self.mux.input(a);
            self.mux.input(b);
        }
    }

    #[test]
    fn a_mouse_report_split_across_reads_is_routed_whole() {
        let click = b"\x1b[<0;50;5M\x1b[<0;50;5m";
        for at in 0..=click.len() {
            let mut p = Piped::new();
            assert_eq!(p.mux.focus(), Some(p.ids[0]));
            p.input(click, at);
            assert_eq!(p.mux.focus(), Some(p.ids[1]), "split at {at}");
            assert_eq!((p.got(0), p.got(1)), (vec![], vec![]), "split at {at}");
            assert_eq!(p.mux.input_deadline(), None);
        }
    }

    #[test]
    fn shift_page_up_split_across_reads_scrolls() {
        let key = b"\x1b[5;2~";
        for at in 0..=key.len() {
            let mut p = Piped::new();
            let id = p.ids[0];
            for i in 0..30 {
                p.mux
                    .pane_mut(id)
                    .unwrap()
                    .feed(format!("line {i}\r\n").as_bytes());
            }
            p.input(key, at);
            assert_eq!(p.mux.pane(id).unwrap().scrolled(), 10, "split at {at}");
            assert_eq!(p.got(0), b"", "split at {at}");
        }
    }

    #[test]
    fn held_input_is_taken_as_it_is_at_the_deadline() {
        let mut p = Piped::new();
        for held in [b"\x1b".as_slice(), b"\x1b[", b"\xc3"] {
            p.mux.input(held);
            assert_eq!(p.got(0), b"");
            assert!(p.mux.input_deadline().is_some());
            p.mux.flush_input();
            assert_eq!(p.got(0), held);
            assert_eq!(p.mux.input_deadline(), None);
        }
    }

    #[test]
    fn a_bracketed_paste_goes_to_the_focused_pane_as_it_is() {
        let paste = b"\x1b[200~a\x1b[<0;50;5M\x02o\x1b[5;2~\x1b[201~";
        for at in 0..=paste.len() {
            let mut p = Piped::new();
            p.input(paste, at);
            assert_eq!(p.mux.focus(), Some(p.ids[0]), "split at {at}");
            assert_eq!(p.got(0), paste, "split at {at}");
            // After the paste, Ctrl-b o moves the focus again.
            p.mux.input(b"\x02o");
            assert_eq!(p.mux.focus(), Some(p.ids[1]), "split at {at}");
        }
    }

    #[test]
    fn keys_split_across_reads_are_read_whole() {
        // While scrolled, Up split after its Esc scrolls a row rather than
        // returning to the bottom.
        for at in 0..=3 {
            let mut p = Piped::new();
            let id = p.ids[0];
            for i in 0..30 {
                p.mux
                    .pane_mut(id)
                    .unwrap()
                    .feed(format!("line {i}\r\n").as_bytes());
            }
            p.mux.input(b"\x1b[5;2~");
            p.input(b"\x1b[A", at);
            assert_eq!(p.mux.pane(id).unwrap().scrolled(), 11, "split at {at}");
            assert_eq!(p.got(0), b"", "split at {at}");
        }
        // A Kitty Ctrl-b press and release, o, and a character pass whole.
        let input = "\x1b[98;5u\x1b[98;5:3u\x1b[111ué".as_bytes();
        for at in 0..=input.len() {
            let mut p = Piped::new();
            p.input(input, at);
            assert_eq!(p.mux.focus(), Some(p.ids[1]), "split at {at}");
            assert_eq!(p.got(1), "é".as_bytes(), "split at {at}");
            assert_eq!(p.got(0), b"", "split at {at}");
        }
    }

    fn zooms(mux: &mut Mux) -> usize {
        let notices = mux.take_notices();
        notices
            .iter()
            .filter(|n| matches!(n, Notice::Zoom(_)))
            .count()
    }

    #[test]
    fn zoom_and_focus_notices_keep_the_order_of_the_input() {
        let mut p = Piped::new();
        let [left, right] = p.ids;
        p.mux.take_notices();
        p.mux.input(b"\x02z\x02o");
        assert_eq!(
            p.mux.take_notices(),
            [Notice::Zoom(left), Notice::Focus(right)]
        );
        p.mux.input(b"\x02o\x1b[98;5u\x1b[122u");
        assert_eq!(
            p.mux.take_notices(),
            [Notice::Focus(left), Notice::Zoom(left)]
        );
    }

    #[test]
    fn ctrl_b_z_asks_for_zoom_and_goes_to_no_pane() {
        let mut p = Piped::new();
        assert_eq!(zooms(&mut p.mux), 0);
        // Legacy, and Kitty's press.
        p.mux.input(b"\x02z");
        assert_eq!(zooms(&mut p.mux), 1);
        assert_eq!(zooms(&mut p.mux), 0);
        p.mux.input(b"\x1b[98;5u\x1b[122u");
        assert_eq!(zooms(&mut p.mux), 1);
        // z alone, and the bare Ctrl-\, go to the pane.
        p.mux.input(b"z\x1c");
        assert_eq!(zooms(&mut p.mux), 0);
        assert_eq!(p.got(0), b"z\x1c");
        // Scrolling doesn't change the key.
        let id = p.ids[0];
        for i in 0..30 {
            p.mux
                .pane_mut(id)
                .unwrap()
                .feed(format!("{i}\r\n").as_bytes());
        }
        p.mux.input(b"\x1b[5;2~\x02z");
        assert_eq!(zooms(&mut p.mux), 1);
        assert_eq!(p.mux.pane(id).unwrap().scrolled(), 10);
        assert_eq!(p.got(0), b"");
        // Every press counts, so a second one in the same read toggles back.
        p.mux.input(b"\x02z\x1b[98;5u\x1b[122u\x02z");
        assert_eq!(zooms(&mut p.mux), 3);
    }

    #[test]
    fn the_prefix_key_takes_tmux_keys_and_the_bare_keys_go_to_the_pane() {
        let mut p = Piped::new();
        p.mux.take_notices();
        p.mux.input(b"\x1d\x1c\x1e");
        assert_eq!(p.mux.take_notices(), []);
        assert_eq!(p.got(0), b"\x1d\x1c\x1e");
        // Shifted keys in legacy and Kitty form, with and without the
        // shifted key reported.
        p.mux
            .input(b"\x02a\x02(\x02)\x02d\x1b[98;5u\x1b[57;2u\x1b[98;5u\x1b[48:41;2u\x02?");
        assert_eq!(
            p.mux.take_notices(),
            [
                Notice::Accent,
                Notice::Step(-1),
                Notice::Step(1),
                Notice::Detach,
                Notice::Step(-1),
                Notice::Step(1),
                Notice::Help
            ]
        );
        assert_eq!(p.got(0), b"");
        // Ctrl-b [ and Ctrl-b PageUp scroll up a page.
        p.mux.set_captured(false);
        let id = p.ids[0];
        for i in 0..30 {
            p.mux
                .pane_mut(id)
                .unwrap()
                .feed(format!("{i}\r\n").as_bytes());
        }
        p.mux.input(b"\x02[");
        assert_eq!(p.mux.pane(id).unwrap().scrolled(), 10);
        p.mux.input(b"\x1b[98;5u\x1b[5~");
        assert_eq!(p.mux.pane(id).unwrap().scrolled(), 20);
        assert_eq!(p.got(0), b"");
    }

    #[test]
    fn the_lock_modifiers_leave_the_prefix_key_working() {
        let mut p = Piped::new();
        p.mux.take_notices();
        // Ctrl-b and 1 with Caps Lock, then Ctrl-b with Num Lock and 2.
        p.mux.input(b"\x1b[98;69u\x1b[49;65u\x1b[98;133u\x1b[50u");
        assert_eq!(
            p.mux.take_notices(),
            [Notice::Workspace(1), Notice::Workspace(2)]
        );
        assert_eq!(p.got(0), b"");
    }

    #[test]
    fn restarting_a_pane_drops_the_old_programs_output_state_and_input() {
        // An unfinished OSC doesn't swallow the new program's output.
        let mut p = Piped::new();
        let id = p.ids[0];
        p.mux.pane_mut(id).unwrap().feed(b"\x1b]2;OLD");
        p.mux.mark_exited(id, 1);
        p.mux.restart(id, "new").unwrap();
        p.mux.pane_mut(id).unwrap().feed(b"NEW\x07 VISIBLE");
        assert_eq!(p.mux.pane(id).unwrap().terminal.cursor_x().unwrap(), 11);
        // A notification cut off by the exit isn't finished by new output.
        p.mux.pane_mut(id).unwrap().feed(b"\x1b]9;half");
        p.mux.mark_exited(id, 1);
        p.mux.restart(id, "new").unwrap();
        p.mux.take_output();
        p.mux.pane_mut(id).unwrap().feed(b"rest\x07");
        assert!(find(&p.mux.output(), b"\x1b]9;").is_none());

        // The rest of a paste that began before the exit is dropped through
        // its end marker, wherever the reads end, and then keys route again.
        let rest = b"tail\x02z\x1b[201~\x02z";
        for at in 0..=rest.len() {
            let mut p = Piped::new();
            let id = p.ids[0];
            p.mux.input(b"\x1b[200~head");
            assert_eq!(p.got(0), b"\x1b[200~head");
            p.mux.mark_exited(id, 1);
            p.mux.restart(id, "new").unwrap();
            p.reattach(0);
            p.mux.set_focus(id);
            p.input(rest, at);
            assert_eq!(p.got(0), b"", "split at {at}");
            // The key inside the paste is part of it; the one after isn't.
            assert_eq!(zooms(&mut p.mux), 1, "split at {at}");
            p.mux.input(b"x");
            assert_eq!(p.got(0), b"x", "split at {at}");
        }

        // A mouse press captured by the old program ends without reaching
        // the new one, even once the new one tracks the mouse.
        let mut p = Piped::new();
        let id = p.ids[0];
        p.mux.pane_mut(id).unwrap().feed(b"\x1b[?1000h\x1b[?1006h");
        p.mux.input(b"\x1b[<0;5;5M");
        assert!(!p.got(0).is_empty());
        p.mux.mark_exited(id, 1);
        p.mux.restart(id, "new").unwrap();
        p.reattach(0);
        p.mux.pane_mut(id).unwrap().feed(b"\x1b[?1000h\x1b[?1006h");
        p.mux.input(b"\x1b[<32;6;5M\x1b[<0;6;5m");
        assert_eq!(p.got(0), b"");
        // The next press is the new program's.
        p.mux.input(b"\x1b[<0;5;5M");
        assert!(!p.got(0).is_empty());

        // Input held for the focused pane when its program exited isn't
        // sent to the new one.
        let mut p = Piped::new();
        let [first, second] = p.ids;
        p.mux.mark_exited(second, 1);
        p.mux.input(b"\x1b[");
        assert!(p.mux.input_deadline().is_some());
        p.mux.mark_exited(first, 1);
        p.mux.restart(first, "new").unwrap();
        p.reattach(0);
        assert_eq!(p.mux.input_deadline(), None);
        p.mux.flush_input();
        assert_eq!(p.got(0), b"");
        p.mux.input(b"x");
        assert_eq!(p.got(0), b"x");
    }

    #[test]
    fn restarting_a_pane_clears_its_scrollback() {
        let mut p = Piped::new();
        let id = p.ids[0];
        for i in 0..30 {
            p.mux
                .pane_mut(id)
                .unwrap()
                .feed(format!("line {i}\r\n").as_bytes());
        }
        p.mux.pane_mut(id).unwrap().scroll(-5);
        assert_eq!(p.mux.pane(id).unwrap().scrolled(), 5);
        // A pane with a running program is left alone.
        p.mux.restart(id, "again").unwrap();
        assert_eq!(p.mux.pane(id).unwrap().scrolled(), 5);

        p.mux.mark_exited(id, 1);
        assert_eq!(p.mux.pane(id).unwrap().status(), Some(1));
        p.mux.restart(id, "again").unwrap();
        let pane = p.mux.pane_mut(id).unwrap();
        assert_eq!((pane.status(), pane.label.as_str()), (None, "again"));
        pane.scroll(-100);
        assert_eq!(pane.scrolled(), 0);
    }

    #[test]
    fn a_left_press_outside_every_pane_is_a_click() {
        let mut p = Piped::new();
        p.mux.take_notices();
        // Column 41 is between the panes.
        p.mux
            .input(b"\x1b[<0;41;3M\x1b[<0;41;3m\x1b[<2;41;3M\x1b[<2;41;3m");
        assert_eq!(p.mux.take_notices(), [Notice::Click(40, 2)]);
        assert_eq!((p.got(0), p.got(1)), (b"".to_vec(), b"".to_vec()));
    }

    #[test]
    fn a_drag_from_outside_every_pane_drags_the_chrome_and_reaches_no_pane() {
        let mut p = Piped::new();
        p.mux.take_notices();
        p.mux
            .pane_mut(p.ids[1])
            .unwrap()
            .feed(b"\x1b[?1002h\x1b[?1006h");
        // Column 41 is between the panes, and the drag moves into the right
        // one, whose program reports the mouse.
        p.mux
            .input(b"\x1b[<0;41;3M\x1b[<32;43;3M\x1b[<32;45;4M\x1b[<0;45;4m");
        assert_eq!(
            p.mux.take_notices(),
            [
                Notice::Click(40, 2),
                Notice::Drag(42, 2),
                Notice::Drag(44, 3)
            ]
        );
        assert_eq!(p.got(1), b"".to_vec());
        // After the release, motion goes where it did before.
        p.mux.input(b"\x1b[<32;45;4M");
        assert_eq!(p.mux.take_notices(), []);
    }

    #[test]
    fn a_press_captured_by_a_pane_that_gets_hidden_goes_nowhere() {
        let mut p = Piped::new();
        let [left, right] = p.ids;
        p.mux.pane_mut(right).unwrap().feed(b"abcdef");
        // Press in the right pane, then it is hidden before the release.
        p.mux.input(b"\x1b[<0;43;1M");
        p.mux.take_output();
        let full = Rect {
            col: 0,
            row: 0,
            cols: 81,
            rows: 10,
        };
        p.mux
            .set_layout(&[(left, full), (right, Rect::default())])
            .unwrap();
        p.mux.take_output();
        p.mux.input(b"\x1b[<32;45;1M\x1b[<0;45;1m");
        assert!(find(&p.mux.output(), b"\x1b]52;").is_none());
        assert_eq!(p.got(1), b"");
    }

    #[test]
    fn a_hidden_pane_keeps_its_size_and_is_not_under_the_pointer() {
        let mut p = Piped::new();
        let [left, right] = p.ids;
        let size = |p: &Piped, id| {
            let ws = p.mux.pane(id).unwrap().ws();
            (ws.ws_col, ws.ws_row)
        };
        assert_eq!(size(&p, right), (40, 10));
        let full = Rect {
            col: 0,
            row: 0,
            cols: 81,
            rows: 10,
        };
        p.mux
            .set_layout(&[(left, full), (right, Rect::default())])
            .unwrap();
        assert_eq!(size(&p, left), (81, 10));
        assert_eq!(size(&p, right), (40, 10));
        // A click where the right pane was lands in the left one.
        p.mux.input(b"\x1b[<0;50;5M\x1b[<0;50;5m");
        assert_eq!(p.mux.focus(), Some(left));
    }

    #[test]
    fn a_hidden_panes_output_waits_until_it_is_shown() {
        for (cols, rows) in [(0, 0), (0, 10), (40, 0)] {
            let mut p = Piped::new();
            let [left, right] = p.ids;
            let rect = p.mux.pane(right).unwrap().rect();
            p.mux.pane_mut(right).unwrap().feed(b"before\r\nunchanged");
            frame(&mut p.mux);
            p.mux
                .set_layout(&[(right, Rect { cols, rows, ..rect })])
                .unwrap();
            frame(&mut p.mux);
            assert!(!p.mux.needs_render());

            p.mux.pane_mut(right).unwrap().feed(b"\x1b[H\x1b[2Klatest");
            assert!(!p.mux.needs_render());
            // Another pane's frame must not capture the hidden terminal.
            p.mux.pane_mut(left).unwrap().feed(b"visible");
            let out = frame(&mut p.mux);
            assert!(!has(&out, b"latest"));
            assert!(p.mux.pane(right).unwrap().dirty);
            assert!(!p.mux.needs_render());

            // Showing the same-sized pane repaints unchanged rows too.
            p.mux.set_layout(&[(right, rect)]).unwrap();
            assert!(p.mux.needs_render());
            let out = frame(&mut p.mux);
            assert!(has(&out, b"latest") && has(&out, b"unchanged"));
            assert!(!p.mux.needs_render());
        }
    }

    #[test]
    fn a_hidden_held_pane_keeps_its_completed_frame_when_shown() {
        for start_hidden in [false, true] {
            let mut p = Piped::new();
            let [left, right] = p.ids;
            let rect = p.mux.pane(right).unwrap().rect();
            p.mux.pane_mut(right).unwrap().feed(b"old");
            frame(&mut p.mux);
            if start_hidden {
                p.mux.set_layout(&[(right, Rect::default())]).unwrap();
                frame(&mut p.mux);
            }
            p.mux
                .pane_mut(right)
                .unwrap()
                .feed(b"\x1b[Hcomplete\x1b[?2026h\x1b[Hpartial!");
            if !start_hidden {
                p.mux.set_layout(&[(right, Rect::default())]).unwrap();
            }
            frame(&mut p.mux);
            assert!(p.mux.pane(right).unwrap().held());
            assert!(!p.mux.needs_render());
            p.mux.pane_mut(left).unwrap().feed(b"visible");
            frame(&mut p.mux);

            p.mux.set_layout(&[(right, rect)]).unwrap();
            assert!(p.mux.pane(right).unwrap().held());
            let out = frame(&mut p.mux);
            assert!(has(&out, b"complete") && !has(&out, b"partial!"));
            assert!(!p.mux.needs_render());
            p.mux.pane_mut(right).unwrap().feed(b"\x1b[?2026l");
            assert!(has(&frame(&mut p.mux), b"partial!"));
        }
    }

    #[test]
    fn captures_reuse_each_rows_cell_and_text_storage() {
        let mut terminal = Terminal::new(40, 2).unwrap();
        let mut frame = Frame::new().unwrap();
        terminal.vt_write(b"first\r\nsecond");
        frame.capture(&terminal).unwrap();
        let storage = |frame: &Frame| {
            frame
                .rows_seen
                .iter()
                .map(|(row, _)| {
                    (
                        row.cells.as_ptr(),
                        row.cells.capacity(),
                        row.text.as_ptr(),
                        row.text.capacity(),
                    )
                })
                .collect::<Vec<_>>()
        };
        let original = storage(&frame);
        // Exercise both the render-state and stale-row capture paths.
        terminal.vt_write(b"\x1b[Hshort");
        frame.capture(&terminal).unwrap();
        assert_eq!(storage(&frame), original);
        terminal.vt_write(b"\x1b[HX");
        frame.stale_rows.push(0);
        frame.capture(&terminal).unwrap();
        assert_eq!(storage(&frame), original);
        assert_eq!(
            frame.rows_seen[0].0.text.iter().collect::<String>(),
            "Xhort"
        );
    }

    #[test]
    fn a_melt_column_falls_after_its_delay_and_stops_at_the_pane_height() {
        let melt = Melt {
            start: 0,
            delays: vec![100],
            rows: Vec::new(),
        };
        // Before its delay, and at its start, the column hasn't moved.
        assert_eq!(melt.fall(0, 50, 10), 0);
        assert_eq!(melt.fall(0, 100, 10), 0);
        // Mid-fall, 400 ms into a 600 ms fall of 10 rows.
        assert_eq!(melt.fall(0, 500, 10), 4);
        // The whole height at the end of the fall, and no further.
        assert_eq!(melt.fall(0, 700, 10), 10);
        assert_eq!(melt.fall(0, 5000, 10), 10);
        // A column past the delays has no delay.
        assert_eq!(melt.fall(1, MELT_DELAY_MS + MELT_FALL_MS, 10), 10);
    }

    #[test]
    fn melt_delays_stay_in_range_and_neighbours_differ_by_at_most_a_step() {
        let step = MELT_DELAY_MS / MELT_STEPS;
        for seed in 1..=100 {
            let delays = melt_delays(40, seed);
            assert_eq!(delays.len(), 40);
            for &delay in &delays {
                assert!((0..=MELT_DELAY_MS).contains(&delay), "{delays:?}");
                assert_eq!(delay % step, 0, "{delays:?}");
            }
            for pair in delays.windows(2) {
                assert!((pair[0] - pair[1]).abs() <= step, "{delays:?}");
            }
        }
    }

    /// Chrome that animates while `on`, counting its frames.
    struct Ticker {
        on: Rc<Cell<bool>>,
        frames: Rc<Cell<u32>>,
        draws: Rc<Cell<u32>>,
    }

    impl Chrome for Ticker {
        fn draw(&mut self, _: &mut Vec<u8>, _: &[PaneView<'_>], _: Winsize) {
            self.draws.set(self.draws.get() + 1);
        }

        fn animated(&self, _: &[PaneView<'_>], _: Winsize) -> bool {
            self.on.get()
        }

        fn animate(&mut self, out: &mut Vec<u8>, _: &[PaneView<'_>], _: Winsize, _: i64) {
            self.frames.set(self.frames.get() + 1);
            out.push(b'~');
        }
    }

    /// A connected mux with a `Ticker` for chrome, after its first frame.
    struct Ticked {
        mux: Mux,
        on: Rc<Cell<bool>>,
        frames: Rc<Cell<u32>>,
        draws: Rc<Cell<u32>>,
    }

    fn ticker() -> Ticked {
        let ws = Winsize {
            ws_col: 20,
            ws_row: 5,
            ws_xpixel: 0,
            ws_ypixel: 0,
        };
        let (on, frames, draws) = (Rc::default(), Rc::default(), Rc::default());
        let chrome = Ticker {
            on: Rc::clone(&on),
            frames: Rc::clone(&frames),
            draws: Rc::clone(&draws),
        };
        let mut mux = Mux::new(ws, Box::new(chrome));
        mux.connect_host(&[], ws).unwrap();
        mux.render().unwrap();
        mux.take_output();
        Ticked {
            mux,
            on,
            frames,
            draws,
        }
    }

    #[test]
    fn an_animation_has_a_deadline_only_while_the_chrome_animates_and_a_host_is_connected() {
        let Ticked { mut mux, on, .. } = ticker();
        assert_eq!(mux.animation_deadline(), None);
        assert!(!mux.needs_render());
        on.set(true);
        mux.animation_at = now_ms();
        assert!(mux.animation_deadline().is_some());
        mux.disconnect_host();
        assert_eq!(mux.animation_deadline(), None);
        assert!(!mux.needs_render());
    }

    #[test]
    fn an_animation_frame_is_drawn_with_the_chrome_and_once_it_is_due() {
        let Ticked {
            mut mux,
            on,
            frames,
            draws,
        } = ticker();
        on.set(true);
        // The first frame is due at once.
        mux.animation_at = 0;
        assert!(mux.needs_render());
        mux.render().unwrap();
        assert_eq!((frames.get(), draws.get()), (1, 1));
        assert!(mux.take_output().contains(&b'~'));
        assert_eq!(
            mux.animation_deadline(),
            Some(mux.animation_at + ANIMATION_MS)
        );
        // Until the next is due, a frame drawn for another reason draws none.
        mux.animation_at = now_ms() + 60_000;
        assert!(!mux.needs_render());
        mux.shared.title_changed.set(true);
        mux.render().unwrap();
        assert_eq!(frames.get(), 1);
        // A frame that draws the chrome draws the animation after it.
        mux.invalidate_chrome();
        mux.render().unwrap();
        assert_eq!((frames.get(), draws.get()), (2, 2));
        mux.animation_at = 0;
        mux.render().unwrap();
        assert_eq!((frames.get(), draws.get()), (3, 2));
    }

    #[test]
    fn the_screen_is_cleared_when_the_animation_ends() {
        let Ticked { mut mux, on, .. } = ticker();
        on.set(true);
        mux.render().unwrap();
        mux.take_output();
        on.set(false);
        assert_eq!(mux.animation_deadline(), None);
        mux.render().unwrap();
        let out = mux.take_output();
        assert!(String::from_utf8_lossy(&out).contains("\x1b[2J"));
        assert!(!out.contains(&b'~'));
    }

    #[test]
    fn melting_a_pane_moves_the_focus_off_it_and_makes_it_not_live() {
        let mut p = Piped::new();
        let [left, right] = p.ids;
        assert_eq!(p.mux.focus(), Some(left));
        p.mux.take_notices();
        p.mux.melt_pane(left);
        assert!(p.mux.pane(left).unwrap().melting());
        assert!(!p.mux.pane(left).unwrap().live());
        assert_eq!(p.mux.pane(left).unwrap().pty(), None);
        assert_eq!(p.mux.focus(), Some(right));
        assert_eq!(p.mux.take_notices(), [Notice::Focus(right)]);
        assert!(p.mux.needs_render());
        // A second melt of the same pane leaves it alone.
        p.mux.melt_pane(left);
        assert!(p.mux.pane(left).unwrap().melting());
        assert_eq!(p.mux.take_notices(), []);
    }

    #[test]
    fn end_melts_removes_a_pane_when_its_melt_ends() {
        let mut p = Piped::new();
        let id = p.ids[0];
        p.mux.melt_pane(id);
        let end = p.mux.melt_deadline().unwrap();
        p.mux.take_notices();
        p.mux.end_melts(end - 1);
        assert!(p.mux.pane(id).is_some());
        assert_eq!(p.mux.take_notices(), []);
        p.mux.end_melts(end);
        assert!(p.mux.pane(id).is_none());
        assert_eq!(p.mux.take_notices(), [Notice::Melted(id)]);
        assert_eq!(p.mux.melt_deadline(), None);
    }

    #[test]
    fn a_hidden_pane_given_to_melt_pane_is_removed_at_once() {
        let mut p = Piped::new();
        let [left, right] = p.ids;
        let full = Rect {
            col: 0,
            row: 0,
            cols: 81,
            rows: 10,
        };
        p.mux
            .set_layout(&[(left, full), (right, Rect::default())])
            .unwrap();
        p.mux.take_notices();
        p.mux.melt_pane(right);
        assert!(p.mux.pane(right).is_none());
        assert_eq!(p.mux.take_notices(), []);
    }

    #[test]
    fn held_input_is_bounded() {
        let mut p = Piped::new();
        let long = [b"\x1b[".as_slice(), &[b'0'; INPUT_HOLD_MAX]].concat();
        p.mux.input(&long);
        assert_eq!(p.got(0), long);
        assert_eq!(p.mux.input_deadline(), None);
    }

    #[test]
    fn a_selection_ends_where_the_button_is_released() {
        let mut p = Piped::new();
        let id = p.ids[0];
        p.mux.pane_mut(id).unwrap().feed(b"abcdefgh");
        let copied = |p: &mut Piped| {
            let out = p.mux.take_output();
            let start = find(&out, b"\x1b]52;c;")?;
            let end = start + out[start..].iter().position(|&b| b == 0x07)?;
            Some(out[start + 7..end].to_vec())
        };
        // Press at a, motion to c, release at e.
        p.mux.input(b"\x1b[<0;1;1M\x1b[<32;3;1M\x1b[<0;5;1m");
        assert_eq!(copied(&mut p), Some(base64(b"abcde").into_bytes()));
        // Press at b and release at f without motion reports.
        p.mux.input(b"\x1b[<0;2;1M\x1b[<0;6;1m");
        assert_eq!(copied(&mut p), Some(base64(b"bcdef").into_bytes()));
        // A click copies nothing.
        p.mux.input(b"\x1b[<0;3;1M\x1b[<0;3;1m");
        assert_eq!(copied(&mut p), None);
    }

    #[test]
    fn a_paste_end_cut_off_by_a_read_waits_for_its_rest() {
        let mut p = Piped::new();
        p.mux.input(b"\x1b[200~hello\x1b[20");
        assert_eq!(p.mux.input_deadline(), None);
        p.mux.flush_input();
        p.mux.input(b"1~\x02o");
        assert_eq!(p.got(0), b"\x1b[200~hello\x1b[201~");
        assert_eq!(p.mux.focus(), Some(p.ids[1]));
    }

    #[test]
    fn a_paste_goes_to_the_pane_it_started_in() {
        let mut p = Piped::new();
        p.mux.input(b"\x1b[200~ab");
        p.mux.set_focus(p.ids[1]);
        p.mux.input(b"cd\x1b[201~e");
        assert_eq!(p.got(0), b"\x1b[200~abcd\x1b[201~");
        assert_eq!(p.got(1), b"e");
    }

    fn frame(mux: &mut Mux) -> Vec<u8> {
        mux.render().unwrap();
        mux.take_output()
    }

    fn has(out: &[u8], needle: &[u8]) -> bool {
        find(out, needle).is_some()
    }

    #[test]
    fn connecting_draws_everything_and_sets_every_mode() {
        let mut p = Piped::new();
        let ws = p.mux.size();
        frame(&mut p.mux);
        for _ in 0..2 {
            p.mux.disconnect_host();
            p.mux.connect_host(b"\x1b[?2027;1$y", ws).unwrap();
            let out = frame(&mut p.mux);
            assert!(has(&out, b"\x1b[2J"));
            for (value, _) in MIRRORED_MODES {
                assert!(has(&out, format!("\x1b[?{value}").as_bytes()), "{value}");
            }
            assert!(has(&out, b"\x1b[?1006h\x1b[?1003l\x1b[?1002h"));
            assert!(has(&out, b"\x1b[=0;1u"));
            assert!(has(&out, b"\x1b]0;\x07"));
            assert!(!p.mux.needs_render());
        }
    }

    #[test]
    fn nothing_is_drawn_or_passed_through_while_disconnected() {
        let mut p = Piped::new();
        let (ws, id) = (p.mux.size(), p.ids[0]);
        frame(&mut p.mux);
        p.mux.disconnect_host();
        assert!(!p.mux.connected());
        p.mux
            .pane_mut(id)
            .unwrap()
            .feed(b"\x07\x1b]0;title\x07\x1b]9;note\x07text");
        assert!(frame(&mut p.mux).is_empty());
        // Connecting draws the title and the text, and nothing that came
        // while disconnected.
        p.mux.connect_host(&[], ws).unwrap();
        let out = frame(&mut p.mux);
        assert!(has(&out, b"\x1b]0;title\x07") && has(&out, b"text"));
        assert!(!has(&out, b"note"));
        assert_eq!(out.iter().filter(|&&b| b == 0x07).count(), 1);
    }

    #[test]
    fn bells_and_titles_between_frames_are_drawn_once() {
        let mut p = Piped::new();
        let id = p.ids[0];
        frame(&mut p.mux);
        p.mux
            .pane_mut(id)
            .unwrap()
            .feed(b"\x07\x07\x1b]0;one\x07\x07\x1b]0;two\x07");
        let out = frame(&mut p.mux);
        // One bell, and the end of one title.
        assert_eq!(out.iter().filter(|&&b| b == 0x07).count(), 2);
        assert!(has(&out, b"\x1b]0;two\x07") && !has(&out, b"one"));
        assert!(!has(&frame(&mut p.mux), b"\x07"));
    }

    #[test]
    fn notifications_past_the_side_output_limit_are_dropped_whole() {
        let mut p = Piped::new();
        let id = p.ids[0];
        frame(&mut p.mux);
        let note = format!("\x1b]9;{}\x07", "n".repeat(4000));
        for _ in 0..100 {
            p.mux.pane_mut(id).unwrap().feed(note.as_bytes());
        }
        let out = p.mux.take_output();
        assert_eq!(out.len(), MAX_SIDE_OUTPUT / note.len() * note.len());
    }

    #[test]
    fn disconnecting_clears_the_selection_and_ends_the_press() {
        let mut p = Piped::new();
        let (ws, id) = (p.mux.size(), p.ids[0]);
        p.mux.pane_mut(id).unwrap().feed(b"abcdefgh");
        p.mux.input(b"\x1b[<0;1;1M\x1b[<32;3;1M");
        assert_eq!(p.mux.host.selection, Some(id));
        p.mux.disconnect_host();
        assert_eq!(p.mux.pane(id).unwrap().selected_text().unwrap(), None);
        p.mux.connect_host(&[], ws).unwrap();
        // The release of the old drag selects and copies nothing.
        p.mux.input(b"\x1b[<0;5;1m");
        assert!(!has(&p.mux.take_output(), b"\x1b]52;"));
        assert_eq!(p.mux.pane(id).unwrap().selected_text().unwrap(), None);
    }

    #[test]
    fn a_held_pane_draws_its_snapshot_after_connecting_again() {
        let mut p = Piped::new();
        let (ws, id) = (p.mux.size(), p.ids[0]);
        p.mux
            .pane_mut(id)
            .unwrap()
            .feed(b"before\x1b[?2026h\x1b[Hafter!");
        assert!(p.mux.pane(id).unwrap().held());
        frame(&mut p.mux);
        p.mux.disconnect_host();
        p.mux.connect_host(&[], ws).unwrap();
        let out = frame(&mut p.mux);
        assert!(has(&out, b"before") && !has(&out, b"after"));
    }

    #[test]
    fn captured_input_is_reported_as_keys() {
        let mut p = Piped::new();
        p.mux.set_captured(true);
        p.mux.take_notices();
        p.mux
            .input(b"j\x1b[A\x1b[<0;1;1M\x1b[200~paste\x1b[201~\r\x02s\x1b[98;5u\x1b[115u");
        let key = |k: &[u8]| Notice::Key(k.to_vec());
        assert_eq!(
            p.mux.take_notices(),
            [
                key(b"j"),
                key(b"\x1b[A"),
                key(b"\r"),
                Notice::Overlay,
                Notice::Overlay
            ]
        );
        assert_eq!(p.got(0), b"");
        // A captured frame mirrors the defaults and shows no cursor.
        let out = frame(&mut p.mux);
        assert!(!has(&out, b"\x1b[?25h"));

        p.mux.set_captured(false);
        p.mux.input(b"a");
        assert_eq!(p.got(0), b"a");
    }

    #[test]
    fn the_prefix_key_and_a_digit_choose_a_workspace() {
        let mut p = Piped::new();
        p.mux.take_notices();
        // The prefix's release, a modifier key, and a mouse report don't
        // end the prefix.
        p.mux
            .input(b"a\x022b\x1b[98;5u\x1b[98;5:3u\x1b[57442;5u\x1b[<35;1;1M\x1b[55u");
        // The prefix twice sends it; any other key goes nowhere.
        p.mux.input(b"\x02\x02\x02x\x1b[98;5u\x1b[98;5uc");
        assert_eq!(
            p.mux.take_notices(),
            [Notice::Workspace(2), Notice::Workspace(7)]
        );
        assert_eq!(p.got(0), b"ab\x1b[57442;5u\x02\x1b[98;5uc");

        // It works while input is captured, and sends nothing.
        p.mux.set_captured(true);
        p.mux.input(b"\x021\x02\x02j");
        assert_eq!(
            p.mux.take_notices(),
            [Notice::Workspace(1), Notice::Key(b"j".to_vec())]
        );
        assert_eq!(p.got(0), b"");
    }

    #[test]
    fn the_prefix_key_and_c_capture_the_input_that_follows() {
        let mut p = Piped::new();
        p.mux.take_notices();
        p.mux.input(b"a\x02c~/x");
        assert_eq!(
            p.mux.take_notices(),
            [
                Notice::NewWorkspace,
                Notice::Key(b"~".to_vec()),
                Notice::Key(b"/".to_vec()),
                Notice::Key(b"x".to_vec())
            ]
        );
        assert_eq!(p.got(0), b"a");

        p.mux.set_captured(false);
        p.mux.input(b"\x1b[98;5u\x1b[99uy");
        assert_eq!(
            p.mux.take_notices(),
            [Notice::NewWorkspace, Notice::Key(b"y".to_vec())]
        );
        assert_eq!(p.got(0), b"");
    }

    #[test]
    fn input_after_the_overlay_key_is_captured_in_order() {
        let mut p = Piped::new();
        p.mux.take_notices();
        p.mux.input(b"a\x02sd\r\x02sb\x1b[98;5u\x1b[115uq");
        p.mux.input(b"x\x02s");
        p.mux.input(b"c");
        let key = |k: &[u8]| Notice::Key(k.to_vec());
        assert_eq!(
            p.mux.take_notices(),
            [
                Notice::Overlay,
                key(b"d"),
                key(b"\r"),
                Notice::Overlay,
                Notice::Overlay,
                key(b"q"),
                key(b"x"),
                Notice::Overlay
            ]
        );
        assert_eq!(p.got(0), b"abc");
    }
}
