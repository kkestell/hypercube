//! Real-pty tests of the runtime. Each test hosts its panes on a pty whose
//! other side feeds a Ghostty terminal standing in for the user's terminal.
//! The programs in the panes are src/bin/hypercube-test-child.rs.
use std::io::Write as _;
use std::os::fd::{AsRawFd as _, OwnedFd, RawFd};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;

use hypercube::mux::{
    self, Chrome, Key, Mux, PaneId, PaneView, Rect, Winsize, host_replies_done, now_ms, read,
    write_all,
};
use hypercube::pty::{self, Pty};
use hypercube::runtime::{Event, Runtime};
use libghostty_vt::RenderState;
use libghostty_vt::Terminal;
use libghostty_vt::render::{CellIterator, CursorVisualStyle, RowIterator};
use libghostty_vt::screen::CellWide;
use libghostty_vt::terminal::{Mode, ModeKind};

const CHILD: &str = env!("CARGO_BIN_EXE_hypercube-test-child");

fn contains(haystack: &[u8], needle: &[u8]) -> bool {
    haystack.windows(needle.len()).any(|w| w == needle)
}
fn position(haystack: &[u8], needle: &[u8]) -> usize {
    haystack
        .windows(needle.len())
        .position(|w| w == needle)
        .unwrap()
}
/// What the stand-in host terminal answers to the runtime's queries.
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
fn winsize(cols: u16, rows: u16) -> Winsize {
    Winsize {
        ws_col: cols,
        ws_row: rows,
        ws_xpixel: cols * 8,
        ws_ypixel: rows * 16,
    }
}

/// Each pane has a label row above it. The first pane takes the left half,
/// and the others split the right half with equal heights.
fn layout(size: Winsize, ids: &[PaneId]) -> Vec<(PaneId, Rect)> {
    let (cols, rows) = (size.ws_col, size.ws_row);
    let Some((&first, rest)) = ids.split_first() else {
        return Vec::new();
    };
    if rest.is_empty() {
        return vec![(
            first,
            Rect {
                col: 0,
                row: 1,
                cols,
                rows: rows - 1,
            },
        )];
    }
    let left = (cols - 1) / 2;
    let mut result = vec![(
        first,
        Rect {
            col: 0,
            row: 1,
            cols: left,
            rows: rows - 1,
        },
    )];
    let n = rest.len() as u16;
    for (i, &id) in rest.iter().enumerate() {
        let i = i as u16;
        let (top, bottom) = (rows * i / n, rows * (i + 1) / n);
        let rect = Rect {
            col: left + 1,
            row: top + 1,
            cols: cols - left - 1,
            rows: bottom - top - 1,
        };
        result.push((id, rect));
    }
    result
}

/// Labels above the panes, "* " before the focused one's, and a divider
/// right of the first pane.
struct Labels;
impl Chrome for Labels {
    fn draw(&mut self, out: &mut Vec<u8>, panes: &[PaneView<'_>], size: Winsize) {
        for p in panes {
            let mut label = format!("{}{}", if p.focused { "* " } else { "" }, p.label);
            if let Some(status) = p.status {
                label.push_str(&format!(" (exited {status})"));
            }
            let label: String = label
                .chars()
                .chain(std::iter::repeat(' '))
                .take(p.rect.cols.into())
                .collect();
            write!(out, "\x1b[{};{}H{label}", p.rect.row, p.rect.col + 1).unwrap();
        }
        if panes.len() > 1 {
            for y in 0..size.ws_row {
                write!(out, "\x1b[{};{}H│", y + 1, panes[0].rect.cols + 1).unwrap();
            }
        }
    }
}

/// What the host side of the pty has received and not yet shown to the
/// host terminal, the total byte count, and when each read ended.
#[derive(Default)]
struct Received {
    data: Vec<u8>,
    total: usize,
    reads: Vec<(usize, i64)>,
}

/// Read the host side of the pty until it closes, answering the runtime's
/// startup queries the way a terminal would.
fn read_host(fd: RawFd, received: Arc<Mutex<Received>>) -> JoinHandle<()> {
    std::thread::spawn(move || {
        let mut buf = [0u8; 65536];
        let mut seen = Vec::new();
        loop {
            let n = read(fd, &mut buf);
            if n <= 0 {
                return;
            }
            let data = &buf[..n as usize];
            if seen.len() < 4096 {
                seen.extend_from_slice(data);
                if contains(&seen, b"\x1b[c") {
                    write_all(fd, &host_replies());
                    seen.resize(4096, 0);
                }
            }
            let mut r = received.lock().unwrap();
            r.data.extend_from_slice(data);
            r.total += data.len();
            let total = r.total;
            r.reads.push((total, now_ms()));
        }
    })
}

struct Host {
    master: OwnedFd,
    slave: Option<OwnedFd>,
    reader: Option<JoinHandle<()>>,
    received: Arc<Mutex<Received>>,
    /// The stand-in host terminal and everything it has been sent.
    terminal: Terminal<'static, 'static>,
    output: Vec<u8>,
    mux: Mux,
    runtime: Runtime,
    events: Vec<Event>,
    /// The caller's descriptors that `step` passes to `wait`.
    extra: Vec<RawFd>,
}
impl Host {
    fn new(cols: u16, rows: u16) -> Self {
        let (master, slave) = pty::openpty(winsize(cols, rows)).unwrap();
        let received = Arc::new(Mutex::new(Received::default()));
        let reader = read_host(master.as_raw_fd(), received.clone());
        let mut terminal = Terminal::new(cols, rows).unwrap();
        mux::use_host_replies_for(&mut terminal, &host_replies()).unwrap();
        let mut mux = Mux::new(winsize(cols, rows), Box::new(Labels));
        let runtime = Runtime::new(&mut mux, slave.as_raw_fd(), slave.as_raw_fd()).unwrap();
        Self {
            master,
            slave: Some(slave),
            reader: Some(reader),
            received,
            terminal,
            output: Vec::new(),
            mux,
            runtime,
            events: Vec::new(),
            extra: Vec::new(),
        }
    }
    /// Start the test child with `args` in a new pane.
    fn spawn(&mut self, label: &str, args: &[&str]) -> PaneId {
        let id = self.mux.add_pane(label).unwrap();
        self.layout();
        let mut argv = vec![CHILD];
        argv.extend_from_slice(args);
        let env: &[(&str, &str)] = &[];
        let ws = self.mux.pane(id).unwrap().ws();
        let pty = Pty::spawn(&argv, &std::env::temp_dir(), env, ws).unwrap();
        self.mux.attach(id, pty);
        id
    }
    fn layout(&mut self) {
        let ids: Vec<PaneId> = self.mux.panes().iter().map(|p| p.id()).collect();
        self.mux.set_layout(&layout(self.mux.size(), &ids)).unwrap();
    }
    fn remove(&mut self, id: PaneId) {
        self.mux.remove_pane(id);
        self.layout();
    }
    /// Run the loop for up to `ms`, laying the panes out again on resize.
    fn step(&mut self, ms: i64) {
        let extra = self.extra.clone();
        let events = self
            .runtime
            .wait(&mut self.mux, &extra, Some(now_ms() + ms))
            .unwrap();
        for event in events {
            match event {
                Event::Resized => self.layout(),
                Event::Deadline => {}
                event => self.events.push(event),
            }
        }
        self.pump();
    }
    /// Show the host terminal what it has received.
    fn pump(&mut self) {
        let data = std::mem::take(&mut self.received.lock().unwrap().data);
        self.terminal.vt_write(&data);
        self.output.extend_from_slice(&data);
    }
    fn until(&mut self, mut done: impl FnMut(&mut Self) -> bool) {
        let deadline = now_ms() + 5000;
        while !done(self) {
            assert!(
                now_ms() < deadline,
                "timed out; screen:\n{}\nevents: {:?}",
                self.screen(),
                self.events
            );
            self.step(20);
        }
    }
    fn screen(&self) -> String {
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
    fn line(&self, row: usize) -> String {
        self.screen().lines().nth(row).unwrap_or("").to_owned()
    }
    fn wait_screen(&mut self, text: &str) {
        self.until(|h| h.screen().contains(text));
    }
    /// When the host side read the end of `text`'s first occurrence.
    fn first_seen(&self, text: &str) -> i64 {
        let end = position(&self.output, text.as_bytes()) + text.len();
        let received = self.received.lock().unwrap();
        received.reads.iter().find(|(n, _)| *n >= end).unwrap().1
    }
    /// Type into the host terminal.
    fn send(&self, data: impl AsRef<[u8]>) {
        write_all(self.master.as_raw_fd(), data.as_ref());
    }
    fn resize(&mut self, cols: u16, rows: u16) {
        self.terminal.resize(cols, rows, 8, 16).unwrap();
        let ws = winsize(cols, rows);
        assert_eq!(
            unsafe { libc::ioctl(self.master.as_raw_fd(), libc::TIOCSWINSZ, &ws) },
            0
        );
    }
    fn exited(&self, id: PaneId) -> Option<u8> {
        self.events.iter().find_map(|e| match *e {
            Event::PaneExited(i, status) if i == id => Some(status),
            _ => None,
        })
    }
    fn wait_exit(&mut self, id: PaneId) -> u8 {
        self.until(|h| h.exited(id).is_some());
        self.exited(id).unwrap()
    }
    fn mode(&self, n: u16) -> bool {
        self.terminal.mode(Mode::new(n, ModeKind::Dec)).unwrap()
    }
    fn kitty(&self) -> u8 {
        self.terminal.kitty_keyboard_flags().unwrap().bits()
    }
    fn cursor_style(&self) -> CursorVisualStyle {
        let mut state = RenderState::new().unwrap();
        state
            .update(&self.terminal)
            .unwrap()
            .cursor_visual_style()
            .unwrap()
    }
    fn restore(&mut self) {
        self.runtime.restore(&mut self.mux);
        // Let the reader catch up.
        std::thread::sleep(std::time::Duration::from_millis(100));
        self.pump();
    }
}
impl Drop for Host {
    fn drop(&mut self) {
        self.runtime.restore(&mut self.mux);
        for p in self.mux.panes() {
            p.signal(libc::SIGKILL);
        }
        // Closing the runtime's side ends the reader.
        self.slave.take();
        if let Some(reader) = self.reader.take() {
            let _ = reader.join();
        }
    }
}

/// A file path unique to the test, removed afterward.
struct TempPath(std::path::PathBuf);
impl TempPath {
    fn new(name: &str) -> Self {
        Self(std::env::temp_dir().join(format!("hypercube-live-{}.{name}", std::process::id())))
    }
    fn path(&self) -> &str {
        self.0.to_str().unwrap()
    }
    fn read(&self) -> Vec<u8> {
        std::fs::read(&self.0).unwrap_or_default()
    }
}
impl Drop for TempPath {
    fn drop(&mut self) {
        let _ = std::fs::remove_file(&self.0);
    }
}

#[test]
fn exit_statuses_are_reported() {
    let mut h = Host::new(40, 10);
    let exited = h.spawn("exit", &["exit", "7", ""]);
    let killed = h.spawn("kill", &["kill"]);
    assert_eq!(h.wait_exit(exited), 7);
    assert_eq!(h.wait_exit(killed), 143);
    h.wait_screen("exit (exited 7)");
    h.wait_screen("kill (exited 143)");
}

#[test]
fn missing_command_fails_to_spawn() {
    let env: &[(&str, &str)] = &[];
    let err = Pty::spawn(
        &["/nonexistent/command"],
        &std::env::temp_dir(),
        env,
        winsize(10, 10),
    )
    .err()
    .unwrap();
    assert_eq!(err.kind(), std::io::ErrorKind::NotFound);
    assert!(
        err.to_string().starts_with("/nonexistent/command: "),
        "{err}"
    );
}

#[test]
fn short_lived_output_is_drawn() {
    let mut h = Host::new(40, 10);
    for _ in 0..10 {
        let id = h.spawn("child", &["exit", "0", "goodbye from the child"]);
        assert_eq!(h.wait_exit(id), 0);
        h.wait_screen("goodbye from the child");
        h.remove(id);
        h.until(|h| !h.screen().contains("goodbye"));
    }
}

#[test]
fn synchronized_output_holds_drawing_until_it_ends() {
    let mut h = Host::new(40, 10);
    h.spawn("child", &["hold"]);
    h.wait_screen("held-1 held-2");
    let first = position(&h.output, b"held-1");
    let second = position(&h.output, b"held-2");
    assert!(!contains(&h.output[first..second], b"\x1b[?2026l"));
    assert!(h.first_seen("held-1") - h.first_seen("before") >= 450);
}

#[test]
fn synchronized_output_hold_times_out_after_a_second() {
    let mut h = Host::new(40, 10);
    h.spawn("child", &["hold-forever"]);
    h.wait_screen("start");
    h.wait_screen("stuck");
    let elapsed = h.first_seen("stuck") - h.first_seen("start");
    assert!((1000..2500).contains(&elapsed), "held for {elapsed}ms");
}

#[test]
fn frames_are_coalesced() {
    let mut h = Host::new(40, 10);
    h.spawn("child", &["burst", "3000"]);
    h.wait_screen("done");
    let elapsed = h.first_seen("done") - h.first_seen("line ");
    let begin = position(&h.output, b"line ");
    let frames = h.output[begin..]
        .windows(8)
        .filter(|s| *s == b"\x1b[?2026h")
        .count();
    assert!(
        frames as i64 <= elapsed / 16 + 2,
        "{frames} frames in {elapsed}ms"
    );
    assert_eq!(
        h.screen(),
        "* child\nline 2992\nline 2993\nline 2994\nline 2995\nline 2996\nline 2997\nline 2998\nline 2999\ndone"
    );
}

#[test]
fn resize_reaches_children_and_redraws_host() {
    let mut h = Host::new(81, 12);
    h.spawn("left", &["size"]);
    h.spawn("right", &["size"]);
    h.until(|h| h.screen().matches("size 40x11").count() == 2);
    h.resize(100, 9);
    h.wait_screen("size 49x8");
    h.wait_screen("size 50x8");
    assert_eq!(h.screen().matches('│').count(), 9);
    assert!(!h.screen().contains("40x11"));
}

#[test]
fn input_modes_are_mirrored_and_restored() {
    let mut h = Host::new(40, 10);
    let id = h.spawn("child", &["modes"]);
    h.wait_screen("ready");
    h.until(|h| h.kitty() == 1);
    for n in [1, 66, 1000, 1004, 1006, 2004] {
        assert!(h.mode(n), "mode {n}");
    }
    assert_eq!(h.cursor_style(), CursorVisualStyle::Bar);
    h.send("a");
    h.wait_screen("got 97");
    h.send("q");
    assert_eq!(h.wait_exit(id), 0);
    h.restore();
    for n in [1, 66, 1000, 1004, 1006, 2004, 1049] {
        assert!(!h.mode(n), "mode {n}");
    }
    assert!(h.mode(25));
    assert_eq!(h.kitty(), 0);
    assert_eq!(h.cursor_style(), CursorVisualStyle::Block);
}

#[test]
fn queries_are_answered() {
    let path = TempPath::new("replies");
    let mut h = Host::new(40, 10);
    let id = h.spawn("child", &["query", path.path()]);
    assert_eq!(h.wait_exit(id), 0);
    let replies = path.read();
    let mut at = 0;
    for expected in [
        "\x1b]10;rgb:d0d0/d1d1/d2d2",
        "\x1b]11;rgb:1010/1111/1212",
        "\x1b]4;1;rgb:1111/f0f0/0a0a",
        "\x1b[3;5R",
        "\x1b[?2027;1$y",
        "\x1b[?62;22c",
    ] {
        assert!(
            contains(&replies[at..], expected.as_bytes()),
            "missing {expected:?} in {:?}",
            String::from_utf8_lossy(&replies)
        );
        at += position(&replies[at..], expected.as_bytes()) + expected.len();
    }
    assert!(host_replies_done(&host_replies()));
}

#[test]
fn input_goes_to_the_focused_pane_and_ctrl_bracket_moves_focus() {
    let mut h = Host::new(81, 12);
    h.spawn("left", &["input", "left", "plain", "0"]);
    h.spawn("right", &["input", "right", "plain", "0"]);
    h.wait_screen("left ready");
    h.wait_screen("right ready");
    h.send("a");
    h.wait_screen("left got 97");
    h.send("\x1db");
    h.wait_screen("right got 98");
    h.send("\x1dc");
    h.wait_screen("left got 99");
    // Kitty-encoded Ctrl-]: a press switches; repeats and releases are
    // consumed; other keys pass through unchanged.
    h.send("\x1b[93;5u\x1b[93;5:2;93u\x1b[93;5:3u\x1b[97;1ud");
    h.wait_screen("right got 100");
    h.send("\x1b[93:93;5:1;93ue");
    h.wait_screen("left got 101");
    for text in [
        "right got 97",
        "left got 98",
        "right got 99",
        "got 29",
        "left got 27",
    ] {
        assert!(!h.screen().contains(text), "{text}");
    }
    assert!(h.screen().contains("right got 27"));
}

#[test]
fn modes_cursor_title_and_focus_reports_follow_focus() {
    let mut h = Host::new(81, 16);
    h.spawn("left", &["input", "left", "modes", "0"]);
    h.spawn("right", &["input", "right", "plain", "0"]);
    h.wait_screen("left ready");
    h.wait_screen("right ready");
    h.until(|h| h.mode(2004) && h.kitty() == 1);
    assert_eq!(h.terminal.title().unwrap(), b"left");
    h.send("\x1d");
    h.wait_screen("left got 79"); // ESC [ O: focus out
    h.until(|h| !h.mode(2004) && h.kitty() == 0);
    for n in [1, 66, 1000, 1004, 1006, 2004] {
        assert!(!h.mode(n));
    }
    assert_eq!(h.cursor_style(), CursorVisualStyle::Block);
    assert_eq!(h.terminal.title().unwrap(), b"right");
    h.send("\x1d");
    h.wait_screen("left got 73"); // ESC [ I: focus in
    h.until(|h| h.mode(2004) && h.kitty() == 1);
    assert_eq!(h.cursor_style(), CursorVisualStyle::Bar);
    assert_eq!(h.terminal.title().unwrap(), b"left");
}

#[test]
fn exited_pane_keeps_its_place_and_loses_focus() {
    let mut h = Host::new(81, 12);
    let left = h.spawn("left", &["exit", "7", "left final"]);
    h.spawn("right", &["input", "right", "plain", "0"]);
    assert_eq!(h.wait_exit(left), 7);
    h.wait_screen("left (exited 7)");
    h.wait_screen("left final");
    h.wait_screen("* right");
    h.send("a\x1db");
    h.wait_screen("right got 98");
    assert!(h.screen().contains("right got 97"));
}

#[test]
fn sgr_mouse_is_translated_into_the_focused_pane() {
    let path = TempPath::new("mouse");
    let mut h = Host::new(81, 20);
    h.spawn("left", &["input", "left", "plain", "0"]);
    h.spawn("right", &["input", "right", "plain", "0", path.path()]);
    h.wait_screen("left ready");
    h.wait_screen("right ready");
    // The right pane starts at column 42, row 2. Reports outside it and in
    // other mouse formats are dropped.
    h.send(b"\x1d\x1b[<0;43;3M\x1b[<0;43;3m\x1b[<0;1;3M\x1b[<0;43;1M\x1b[<0;41;3M\x1b[M !!\x1b[M \xc2!\x1b[32;43;3M!");
    h.wait_screen("right got 33");
    assert_eq!(path.read(), b"\x1b[<0;2;2M\x1b[<0;2;2m!");
}

#[test]
fn other_panes_draw_during_a_hold() {
    let mut h = Host::new(81, 12);
    h.spawn("left", &["hold-forever"]);
    h.spawn("right", &["input", "right", "plain", "0"]);
    h.wait_screen("start");
    h.wait_screen("right ready");
    let deadline = now_ms() + 250;
    while now_ms() < deadline {
        h.step(20);
    }
    h.send("\x1da");
    h.wait_screen("right got 97");
    assert!(!h.screen().contains("stuck"));
    h.wait_screen("stuck");
}

#[test]
fn wide_characters_are_drawn_at_each_panes_offset() {
    let mut h = Host::new(81, 12);
    h.spawn("left", &["input", "界L", "plain", "0"]);
    h.spawn("right", &["input", "界R", "plain", "0"]);
    h.wait_screen("界L ready");
    h.wait_screen("界R ready");
    assert_eq!(
        h.line(1),
        "界L ready                               │界R ready"
    );
    h.until(|h| h.terminal.cursor_y().unwrap() == 1 && h.terminal.cursor_x().unwrap() == 9);
    h.send("\x1d");
    h.until(|h| h.terminal.cursor_x().unwrap() == 50);
}

#[test]
fn panes_are_added_while_others_run() {
    let mut h = Host::new(81, 12);
    let first = h.spawn("first", &["input", "first", "plain", "0"]);
    h.wait_screen("first ready");
    h.send("a");
    h.wait_screen("first got 97");
    h.spawn("second", &["size"]);
    h.wait_screen("size 40x11");
    // The first pane narrowed and drew again; its program keeps running.
    assert_eq!(h.line(0), "* first                                 │second");
    assert!(h.screen().contains("first got 97"));
    h.spawn("third", &["size"]);
    h.until(|h| h.screen().matches("size 40x5").count() == 2);
    h.send("b");
    h.wait_screen("first got 98");
    assert_eq!(h.mux.focus(), Some(first));
}

#[test]
fn five_panes_stack_on_the_right() {
    let mut h = Host::new(81, 17);
    for name in ["one", "two", "three", "four", "five"] {
        h.spawn(name, &["size"]);
    }
    // 17 rows split four ways: 4, 4, 4, and 5 rows, each with its label.
    h.until(|h| {
        let s = h.screen();
        s.contains("size 40x16") && s.matches("size 40x3").count() == 3 && s.contains("size 40x4")
    });
    let screen = h.screen();
    let lines: Vec<&str> = screen.lines().collect();
    for (row, label) in [(0, "two"), (4, "three"), (8, "four"), (12, "five")] {
        assert!(
            lines[row].ends_with(&format!("│{label}")),
            "row {row}: {:?}",
            lines[row]
        );
    }
    h.send("\x1d\x1d\x1d\x1d");
    h.until(|h| h.screen().contains("* five"));
}

#[test]
fn removing_a_pane_redraws_its_area_and_moves_focus() {
    let mut h = Host::new(81, 12);
    h.spawn("left", &["input", "left", "plain", "0"]);
    let middle = h.spawn("middle", &["input", "middle", "plain", "0"]);
    let right = h.spawn("right", &["input", "right", "plain", "0"]);
    h.wait_screen("middle ready");
    h.wait_screen("right ready");
    h.send("\x1d");
    h.wait_screen("* middle");
    h.remove(middle);
    // Focus moves to the next pane, and the last pane takes the whole right
    // half.
    h.until(|h| h.screen().contains("* right") && !h.screen().contains("middle"));
    assert_eq!(h.mux.focus(), Some(right));
    assert_eq!(h.mux.pane(right).unwrap().ws().ws_row, 11);
    assert_eq!(
        h.line(0),
        "left                                    │* right"
    );
    h.send("a");
    h.wait_screen("right got 97");
}

#[test]
fn send_key_encodes_enter_for_the_panes_keyboard_mode() {
    let legacy = TempPath::new("legacy-keys");
    let kitty = TempPath::new("kitty-keys");
    let mut h = Host::new(81, 12);
    let a = h.spawn("legacy", &["input", "legacy", "plain", "0", legacy.path()]);
    let b = h.spawn("kitty", &["input", "kitty", "kitty", "0", kitty.path()]);
    h.wait_screen("legacy ready");
    h.wait_screen("kitty ready");
    h.until(|h| {
        h.mux
            .pane(b)
            .unwrap()
            .terminal
            .kitty_keyboard_flags()
            .unwrap()
            .bits()
            == 11
    });
    for id in [a, b] {
        h.mux.send_text(id, "hi");
        h.mux.send_key(id, Key::Enter).unwrap();
    }
    h.until(|h| h.screen().matches("got 13").count() == 1 && h.screen().contains("got 117"));
    h.until(|_| legacy.read() == b"hi\r" && kitty.read().ends_with(b"u"));
    assert_eq!(kitty.read(), b"hi\x1b[13u\x1b[13;1:3u");
}

#[test]
fn close_pane_hangs_up_its_program() {
    let mut h = Host::new(81, 12);
    let plain = h.spawn("plain", &["input", "plain", "plain", "0"]);
    let trap = h.spawn("trap", &["trap"]);
    h.wait_screen("plain ready");
    h.wait_screen("trapping");
    h.mux.close_pane(plain);
    h.mux.close_pane(trap);
    assert_eq!(h.wait_exit(plain), 128 + libc::SIGHUP as u8);
    assert_eq!(h.wait_exit(trap), 3);
    h.wait_screen("plain (exited 129)");
    h.wait_screen("trap (exited 3)");
}

#[test]
fn extra_descriptor_wakes_wait() {
    let mut h = Host::new(40, 10);
    let mut fds = [0; 2];
    assert_eq!(unsafe { libc::pipe(fds.as_mut_ptr()) }, 0);
    let start = now_ms();
    let writer = std::thread::spawn(move || {
        std::thread::sleep(std::time::Duration::from_millis(100));
        write_all(fds[1], b"x");
    });
    let events = h
        .runtime
        .wait(&mut h.mux, &[fds[0]], Some(start + 5000))
        .unwrap();
    assert_eq!(events, [Event::Ready(fds[0])]);
    assert!(now_ms() - start < 1000);
    writer.join().unwrap();
    unsafe {
        libc::close(fds[0]);
        libc::close(fds[1]);
    }
}

#[test]
fn deadline_wakes_wait() {
    let mut h = Host::new(40, 10);
    let start = now_ms();
    let events = h.runtime.wait(&mut h.mux, &[], Some(start + 150)).unwrap();
    assert_eq!(events, [Event::Deadline]);
    assert!((150..1000).contains(&(now_ms() - start)));
}

#[test]
fn children_do_not_inherit_other_panes_masters() {
    let mut h = Host::new(81, 12);
    let first = h.spawn("first", &["size"]);
    let master = h.mux.pane(first).unwrap().pty().unwrap().to_string();
    h.spawn("second", &["fds", &master, "0"]);
    h.wait_screen(&format!("fd {master} closed"));
    h.wait_screen("fd 0 open");
}
