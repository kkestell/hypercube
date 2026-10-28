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
use libghostty_vt::key::{self, Action};
use libghostty_vt::render::{CellIterator, Cursor, CursorVisualStyle, Dirty, RowIterator};
use libghostty_vt::screen::{Cell as RawCell, CellContentTag, CellWide};
use libghostty_vt::style::{Palette, RgbColor, Style, StyleColor, Underline};
use libghostty_vt::terminal::{
    ConformanceLevel, DeviceAttributeFeature, DeviceAttributes, DeviceType, Mode, ModeKind, Point,
    PointCoordinate, PrimaryDeviceAttributes, SecondaryDeviceAttributes, SizeReportSize,
    TertiaryDeviceAttributes,
};
use libghostty_vt::{RenderState, Terminal};

use crate::pty::Pty;

pub use libc::winsize as Winsize;
pub use libghostty_vt::key::Key;

/// Modes mirrored onto the host terminal: the ones that change how the
/// host must encode input, and the ones that change how it draws what
/// the mux sends. Each comes with Ghostty's default, which the host is
/// assumed to share; grapheme clustering's default comes from the host.
pub const MIRRORED_MODES: [(u16, bool); 19] = [
    (1, false),    // DECCKM: application cursor keys
    (5, false),    // DECSCNM: reverse video
    (9, false),    // X10 mouse reporting
    (66, false),   // DECNKM: application keypad
    (67, false),   // DECBKM: backspace sends BS
    (1000, false), // normal mouse tracking
    (1002, false), // button-event mouse tracking
    (1003, false), // any-event mouse tracking
    (1004, false), // focus in/out events
    (1005, false), // UTF-8 mouse format
    (1006, false), // SGR mouse format
    (1007, true),  // alternate scroll
    (1015, false), // urxvt mouse format
    (1016, false), // SGR pixel mouse format
    (1035, true),  // ignore keypad with Num Lock
    (1036, true),  // Alt sends ESC
    (1039, false), // Alt sends ESC (alternate)
    (2004, false), // bracketed paste
    (2027, false), // grapheme clustering
];

/// How long a pane may hold drawing (mode 2026) without completing a frame.
pub const HOLD_TIMEOUT_MS: i64 = 1000;

/// How long a removed pane's program has to exit after SIGHUP before its
/// process group gets SIGKILL.
pub const HANGUP_GRACE_MS: i64 = 2000;

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

/// What the chrome sees of a pane.
pub struct PaneView<'a> {
    pub id: PaneId,
    pub label: &'a str,
    pub rect: Rect,
    pub focused: bool,
    /// The program's exit status once it has been reaped.
    pub status: Option<u8>,
}

/// Draws everything around the panes: borders, labels, a status bar.
pub trait Chrome {
    /// Write the escape sequences that draw all of the chrome into `out`,
    /// leaving the SGR attributes reset. The mux calls this inside a frame
    /// whenever the panes, the focus, or the host size changed, or after
    /// `Mux::invalidate_chrome`.
    fn draw(&mut self, out: &mut Vec<u8>, panes: &[PaneView<'_>], size: Winsize);
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
        let stale = std::mem::take(&mut self.stale_rows);
        if dirty != Dirty::Clean || !stale.is_empty() {
            let mut rows = self.rows.update(&snapshot)?;
            let mut y: u16 = 0;
            while let Some(r) = rows.next() {
                let Some(slot) = self.rows_seen.get_mut(usize::from(y)) else {
                    break;
                };
                if stale.contains(&y) {
                    let mut row = RowBuf::default();
                    for x in 0..snapshot.cols()? {
                        let grid = terminal
                            .grid_ref(Point::Active(PointCoordinate { x, y: u32::from(y) }))?;
                        let raw = grid.cell()?;
                        let style = grid.style()?;
                        row.push(raw, style, |buf| grid.graphemes(buf))?;
                    }
                    *slot = (row, true);
                } else if dirty == Dirty::Full || r.dirty()? {
                    let mut row = RowBuf::default();
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
                    *slot = (row, true);
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

/// Per-program state.
pub struct Pane {
    id: PaneId,
    state: Rc<PaneState>,
    master: Option<OwnedFd>,
    child: Option<Child>,
    status: Option<u8>,
    pub label: String,
    rect: Rect,
    pub terminal: Terminal<'static, 'static>,
    frame: Rc<RefCell<Frame>>,
    dirty: bool,
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
        let mut terminal = Terminal::new(1, 1)?;

        let s = state.clone();
        terminal.on_pty_write(move |_, data| s.send(data))?;
        let sh = shared.clone();
        terminal.on_bell(move |_| sh.out.borrow_mut().push(0x07))?;
        let sh = shared.clone();
        terminal.on_title_changed(move |t| {
            if sh.focus.get() == Some(id) {
                write_title(&mut sh.out.borrow_mut(), t);
            }
        })?;
        terminal.on_device_attributes(|_| Some(DEVICE_ATTRIBUTES))?;
        let s = state.clone();
        terminal.on_size(move |_| {
            let ws = s.ws.get();
            Some(SizeReportSize {
                rows: ws.ws_row,
                columns: ws.ws_col,
                cell_width: u32::from(ws.ws_xpixel.checked_div(ws.ws_col).unwrap_or(0)),
                cell_height: u32::from(ws.ws_ypixel.checked_div(ws.ws_row).unwrap_or(0)),
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

        Ok(Pane {
            id,
            state,
            master: None,
            child: None,
            status: None,
            label: label.to_owned(),
            rect: Rect::default(),
            terminal,
            frame,
            dirty: true,
        })
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

    /// The program's process ID, which is also its process group ID.
    pub fn pid(&self) -> Option<u32> {
        self.child.as_ref().map(Child::id)
    }

    pub fn feed(&mut self, data: &[u8]) {
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

    /// Whether the program hasn't been reaped.
    fn live(&self) -> bool {
        self.status.is_none()
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

    fn cursor(&self) -> Cursor {
        self.frame.borrow().cursor
    }
}

pub struct Mux {
    ws: Winsize,
    panes: Vec<Pane>,
    next_id: u32,
    shared: Rc<Shared>,
    chrome: Box<dyn Chrome>,
    chrome_dirty: bool,
    /// Whether the next frame clears the host screen and draws everything.
    clear: bool,
    /// Programs of removed panes that haven't been reaped, with when they
    /// were hung up.
    orphans: Vec<(Child, i64)>,
    host_replies: Vec<u8>,
    host_modes: [bool; MIRRORED_MODES.len()],
    host_kitty_flags: u8,
    /// Whether terminals cluster graphemes by default, from the host.
    grapheme_default: bool,
}

impl Mux {
    pub fn new(ws: Winsize, chrome: Box<dyn Chrome>) -> Mux {
        let mut m = Mux {
            ws,
            panes: Vec::new(),
            next_id: 1,
            shared: Rc::new(Shared {
                out: RefCell::new(Vec::new()),
                focus: Cell::new(None),
            }),
            chrome,
            chrome_dirty: true,
            clear: false,
            orphans: Vec::new(),
            host_replies: Vec::new(),
            host_modes: [false; MIRRORED_MODES.len()],
            host_kitty_flags: 0,
            grapheme_default: false,
        };
        m.reset_host_modes();
        m
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

    /// Take the host's replies to `host_queries` for every pane's terminal,
    /// including panes added later.
    pub fn use_host_replies(&mut self, replies: &[u8]) -> Result<()> {
        for p in &mut self.panes {
            use_host_replies_for(&mut p.terminal, replies)?;
        }
        self.host_replies = replies.to_vec();
        self.grapheme_default = host_clusters(replies);
        self.reset_host_modes();
        Ok(())
    }

    fn mode_default(&self, value: u16, default: bool) -> bool {
        if value == 2027 {
            self.grapheme_default
        } else {
            default
        }
    }

    fn reset_host_modes(&mut self) {
        for (i, &(value, default)) in MIRRORED_MODES.iter().enumerate() {
            self.host_modes[i] = self.mode_default(value, default);
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

    /// Remove a pane, hanging up its program if it is still running, and
    /// move the focus off it. The rest of the screen is drawn again. A
    /// program still running `HANGUP_GRACE_MS` later is killed.
    pub fn remove_pane(&mut self, id: PaneId) {
        let Some(index) = self.index(id) else { return };
        let mut pane = self.panes.remove(index);
        pane.signal(libc::SIGHUP);
        pane.close_master();
        if let (Some(child), None) = (pane.child.take(), pane.status) {
            self.orphans.push((child, now_ms()));
        }
        if self.focus() == Some(id) {
            self.shared.focus.set(None);
            self.focus_from(index);
        }
        self.repaint();
    }

    /// Collect the exit status of every pane program that has exited, and
    /// of removed panes' programs. Never blocks.
    pub fn reap(&mut self) -> Vec<(PaneId, u8)> {
        let now = now_ms();
        self.orphans.retain_mut(|(child, since)| {
            if now - *since >= HANGUP_GRACE_MS {
                unsafe { libc::killpg(child.id() as libc::pid_t, libc::SIGKILL) };
            }
            matches!(child.try_wait(), Ok(None))
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
            if let Some(index) = self.focus().and_then(|id| self.index(id))
                && !self.panes[index].live()
            {
                self.focus_from(index + 1);
            }
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
    /// and draw everything again. Panes not listed keep their places. Ghostty
    /// and the pty need at least one cell in each dimension, so an empty
    /// rectangle still sizes the pane's terminal to one cell.
    pub fn set_layout(&mut self, layout: &[(PaneId, Rect)]) -> Result<()> {
        let host = self.ws;
        for &(id, rect) in layout {
            let Some(p) = self.pane_mut(id) else { continue };
            let cols = rect.cols.max(1);
            let rows = rect.rows.max(1);
            let ws = Winsize {
                ws_col: cols,
                ws_row: rows,
                ws_xpixel: (u32::from(host.ws_xpixel) * u32::from(cols)
                    / u32::from(host.ws_col.max(1))) as u16,
                ws_ypixel: (u32::from(host.ws_ypixel) * u32::from(rows)
                    / u32::from(host.ws_row.max(1))) as u16,
            };
            p.rect = rect;
            let old = p.state.ws.replace(ws);
            // Resizing a terminal ends its hold, so a pane that only moves
            // keeps its size and its hold.
            if (old.ws_col, old.ws_row, old.ws_xpixel, old.ws_ypixel)
                == (ws.ws_col, ws.ws_row, ws.ws_xpixel, ws.ws_ypixel)
            {
                continue;
            }
            p.terminal.resize(
                cols,
                rows,
                u32::from(ws.ws_xpixel.checked_div(cols).unwrap_or(0)),
                u32::from(ws.ws_ypixel.checked_div(rows).unwrap_or(0)),
            )?;
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

    pub fn needs_render(&self) -> bool {
        self.chrome_dirty || self.clear || self.panes.iter().any(Pane::needs_render)
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
        let next = &self.panes[self.index(id).unwrap()];
        if next.mode(Mode::FOCUS_EVENT) {
            next.send(b"\x1b[I");
        }
        self.chrome_dirty = true;
        write_title(&mut self.shared.out.borrow_mut(), &next.terminal);
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

    fn send(&self, data: &[u8]) {
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

    /// Input is parsed only within this read; partial escape sequences are
    /// forwarded without waiting for another read.
    pub fn input(&mut self, data: &[u8]) {
        let mut i = 0;
        let mut start = 0;
        while i < data.len() {
            let mut end = i + 1;
            let mut swallow = false;
            let mut toggle = false;
            if data[i] == 0x1d {
                swallow = true;
                toggle = true;
            } else if data[i..].starts_with(b"\x1b[") {
                let mut fin = i + 2;
                while fin < data.len() && !(0x40..=0x7e).contains(&data[fin]) {
                    fin += 1;
                }
                if fin < data.len() {
                    end = fin + 1;
                    let params = &data[i + 2..fin];
                    let last = data[fin];
                    if last == b'u' {
                        if let Some(press) = focus_key(params) {
                            swallow = true;
                            toggle = press;
                        }
                    } else if (params.starts_with(b"<") && (last == b'M' || last == b'm'))
                        || (last == b'M'
                            && (params.is_empty()
                                || params.iter().filter(|&&b| b == b';').count() == 2))
                    {
                        // All mouse protocols end in M/m. Only SGR cell
                        // coordinates can be translated into a pane.
                        self.send(&data[start..i]);
                        start = end;
                        if let Some(sgr) = params.strip_prefix(b"<") {
                            self.mouse(sgr, last);
                        }
                        if params.is_empty() && last == b'M' {
                            // X10/1005 encodes three characters after CSI M.
                            let utf8 = self.focused().is_some_and(|p| p.mode(Mode::UTF8_MOUSE));
                            for _ in 0..3 {
                                if end == data.len() {
                                    break;
                                }
                                let len = if utf8 {
                                    utf8_sequence_length(data[end])
                                } else {
                                    1
                                };
                                end = data.len().min(end + len);
                            }
                            start = end;
                        }
                        i = end;
                        continue;
                    }
                }
            }
            if swallow {
                self.send(&data[start..i]);
                if toggle {
                    self.focus_next();
                }
                start = end;
            }
            i = end;
        }
        self.send(&data[start..]);
    }

    fn mouse(&self, params: &[u8], last: u8) {
        let mut fields = params.split(|&b| b == b';');
        let mut field = || fields.next().and_then(parse_int::<u16>);
        let (Some(button), Some(x), Some(y)) = (field(), field(), field()) else {
            return;
        };
        if fields.next().is_some() {
            return;
        }
        let Some(p) = self.focused() else { return };
        if p.mode(Mode::SGR_PIXELS_MOUSE) || x == 0 || y == 0 || !p.rect.contains(x - 1, y - 1) {
            return;
        }
        let report = format!(
            "\x1b[<{};{};{}{}",
            button,
            x - p.rect.col,
            y - p.rect.row,
            last as char
        );
        p.send(report.as_bytes());
    }

    fn sync_modes(&mut self, out: &mut Vec<u8>) {
        for (i, &(value, default)) in MIRRORED_MODES.iter().enumerate() {
            let set = match self.focused() {
                Some(p) => p.mode(dec(value)),
                None => self.mode_default(value, default),
            };
            if set != self.host_modes[i] {
                let _ = write!(out, "\x1b[?{}{}", value, if set { 'h' } else { 'l' });
                self.host_modes[i] = set;
            }
        }

        let flags = self.focused().map_or(0, |p| {
            p.terminal.kitty_keyboard_flags().map_or(0, |f| f.bits())
        });
        if flags != self.host_kitty_flags {
            let _ = write!(out, "\x1b[={flags};1u");
            self.host_kitty_flags = flags;
        }
    }

    /// Draw the chrome and the rows that changed since the last frame, the
    /// cursor, and the input modes into the output.
    pub fn render(&mut self) -> Result<()> {
        let shared = self.shared.clone();
        let mut out = shared.out.borrow_mut();
        out.extend_from_slice(b"\x1b[?2026h\x1b[?25l");
        // Grapheme clustering must be synchronized before drawing rows.
        self.sync_modes(&mut out);

        if self.clear {
            out.extend_from_slice(b"\x1b[0m\x1b[2J");
            self.clear = false;
        }
        if self.chrome_dirty {
            let focus = self.focus();
            let views: Vec<PaneView> = self
                .panes
                .iter()
                .map(|p| PaneView {
                    id: p.id,
                    label: &p.label,
                    rect: p.rect,
                    focused: Some(p.id) == focus,
                    status: p.status,
                })
                .collect();
            self.chrome.draw(&mut out, &views, self.ws);
            self.chrome_dirty = false;
        }
        for p in &mut self.panes {
            p.render(&mut out)?;
        }

        let Some(p) = self.focused() else {
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
/// it: every mirrored mode at its default, no Kitty keyboard flags, the
/// default cursor, and the main screen. `clusters` is whether the host
/// clusters graphemes by default, if known.
pub fn host_restore(clusters: Option<bool>) -> Vec<u8> {
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
    out.extend_from_slice(b"\x1b[=0;1u\x1b[0 q\x1b[0m\x1b[?25h\x1b[?1049l");
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

fn write_title(out: &mut Vec<u8>, terminal: &Terminal) {
    out.extend_from_slice(b"\x1b]0;");
    out.extend_from_slice(terminal.title().unwrap_or(b""));
    out.push(0x07);
}

/// Kitty's key;modifiers:event;... form, including alternate key fields.
fn focus_key(params: &[u8]) -> Option<bool> {
    let mut fields = params.split(|&b| b == b';');
    let key = fields.next()?.split(|&b| b == b':').next()?;
    if parse_int::<u32>(key)? != 93 {
        return None;
    }
    let mut modifiers = fields.next()?.split(|&b| b == b':');
    if parse_int::<u16>(modifiers.next()?)? != 5 {
        return None;
    }
    let event = match modifiers.next() {
        Some(e) => parse_int::<u8>(e)?,
        None => 1,
    };
    match event {
        1 => Some(true),
        2 | 3 => Some(false),
        _ => None,
    }
}

fn parse_int<T: std::str::FromStr>(s: &[u8]) -> Option<T> {
    std::str::from_utf8(s).ok()?.parse().ok()
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
}

impl RowBuf {
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

            let s = cell_style(raw, style);
            if prev != Some(s) {
                write_sgr(out, s);
                prev = Some(s);
            }

            if raw.has_text().unwrap_or(false) && len > 0 {
                let mut buf = [0; 4];
                for &c in &self.text[start..start + len] {
                    out.extend_from_slice(c.encode_utf8(&mut buf).as_bytes());
                }
            } else {
                out.push(b' ');
            }

            // Re-sync the column after a wide character in case the host
            // terminal disagrees about its width.
            if wide == CellWide::Wide {
                let _ = write!(out, "\x1b[{}G", x + usize::from(col) + 3);
            }
        }
        out.extend_from_slice(b"\x1b[0m");
    }
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
