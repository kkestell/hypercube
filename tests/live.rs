//! Real-pty tests of the runtime. Each test runs `client::proxy` in a thread
//! on a pty whose other side feeds a Ghostty terminal standing in for the
//! user's terminal, linked to the runtime by a socket pair. The programs in
//! the panes are src/bin/hypercube-test-child.rs.
use std::io::Write as _;
use std::os::fd::{AsRawFd as _, FromRawFd as _, OwnedFd, RawFd};
use std::os::unix::net::UnixStream;
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;

use hypercube::client::{self, Tty};
use hypercube::link::{self, Link, Message};
use hypercube::mux::{
    self, Chrome, Key, Mux, PaneId, PaneView, Rect, Winsize, host_replies_done, now_ms, read,
    write_all,
};
use hypercube::pty::{self, Pty};
use hypercube::runtime::{Event, Runtime};
use libghostty_vt::RenderState;
use libghostty_vt::Terminal;
use libghostty_vt::mouse;
use libghostty_vt::render::{CellIterator, CursorVisualStyle, RowIterator};
use libghostty_vt::screen::CellWide;
use libghostty_vt::terminal::{Mode, ModeKind, Point, PointCoordinate};

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
            if p.scrolled > 0 {
                label.push_str(&format!(" (scrolled {})", p.scrolled));
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

/// Read the host side of the pty until it closes.
fn read_host(fd: RawFd, received: Arc<Mutex<Received>>) -> JoinHandle<()> {
    std::thread::spawn(move || {
        let mut buf = [0u8; 65536];
        loop {
            let n = read(fd, &mut buf);
            if n <= 0 {
                return;
            }
            let data = &buf[..n as usize];
            let mut r = received.lock().unwrap();
            r.data.extend_from_slice(data);
            r.total += data.len();
            let total = r.total;
            r.reads.push((total, now_ms()));
        }
    })
}

/// A non-blocking pipe, read end first, as `winch` makes.
fn pipe() -> (OwnedFd, OwnedFd) {
    let mut fds = [0; 2];
    assert_eq!(unsafe { libc::pipe(fds.as_mut_ptr()) }, 0);
    unsafe { libc::fcntl(fds[0], libc::F_SETFL, libc::O_NONBLOCK) };
    unsafe { (OwnedFd::from_raw_fd(fds[0]), OwnedFd::from_raw_fd(fds[1])) }
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
    /// The client running `client::proxy` on the pty, until it returns its
    /// reason.
    proxy: Option<JoinHandle<String>>,
    /// The client's SIGWINCH pipe, written after a resize.
    winch: (OwnedFd, OwnedFd),
    /// The client's hangup pipe, never written.
    hangup: (OwnedFd, OwnedFd),
    events: Vec<Event>,
    /// The caller's descriptors that `step` passes to `wait`.
    extra: Vec<RawFd>,
}
impl Host {
    fn new(cols: u16, rows: u16) -> Self {
        let (master, slave) = pty::openpty(winsize(cols, rows)).unwrap();
        let mut termios = std::mem::MaybeUninit::<libc::termios>::uninit();
        unsafe {
            libc::tcgetattr(slave.as_raw_fd(), termios.as_mut_ptr());
            let mut termios = termios.assume_init();
            libc::cfmakeraw(&mut termios);
            libc::tcsetattr(slave.as_raw_fd(), libc::TCSANOW, &termios);
        }
        let received = Arc::new(Mutex::new(Received::default()));
        let reader = read_host(master.as_raw_fd(), received.clone());
        let mut terminal = Terminal::new(cols, rows).unwrap();
        mux::use_host_replies_for(&mut terminal, &host_replies()).unwrap();
        let mut h = Self {
            master,
            slave: Some(slave),
            reader: Some(reader),
            received,
            terminal,
            output: Vec::new(),
            mux: Mux::new(winsize(cols, rows), Box::new(Labels)),
            runtime: Runtime::new(),
            proxy: None,
            winch: pipe(),
            hangup: pipe(),
            events: Vec::new(),
            extra: Vec::new(),
        };
        h.attach();
        h
    }
    fn tty(&self) -> Tty {
        let slave = self.slave.as_ref().unwrap().as_raw_fd();
        Tty {
            input: slave,
            output: slave,
            winch: self.winch.0.as_raw_fd(),
            hangup: self.hangup.0.as_raw_fd(),
            replies: host_replies(),
            exe: std::path::PathBuf::new(),
        }
    }
    /// Link a new client on the pty to the runtime, as attaching does.
    fn attach(&mut self) {
        let (host, client) = UnixStream::pair().unwrap();
        let size = winsize(self.terminal.cols().unwrap(), self.terminal.rows().unwrap());
        self.runtime
            .connect(
                &mut self.mux,
                Link::new(host).unwrap(),
                &host_replies(),
                size.into(),
            )
            .unwrap();
        self.layout();
        let tty = self.tty();
        self.proxy = Some(std::thread::spawn(move || {
            client::proxy(&tty, client).unwrap()
        }));
    }
    /// Detach the client, run the loop until it has the message, and return
    /// the reason it got.
    fn detach(&mut self, reason: &str) -> String {
        self.runtime.detach(&mut self.mux, reason);
        while self.runtime.closing() {
            self.step(20);
        }
        self.proxy.take().unwrap().join().unwrap()
    }
    /// Start the test child with `args` in a new pane.
    fn spawn(&mut self, label: &str, args: &[&str]) -> PaneId {
        let mut argv = vec![CHILD];
        argv.extend_from_slice(args);
        self.spawn_argv(label, &argv)
    }
    fn spawn_argv(&mut self, label: &str, argv: &[&str]) -> PaneId {
        let id = self.mux.add_pane(label).unwrap();
        self.layout();
        let env: &[(&str, &str)] = &[];
        let ws = self.mux.pane(id).unwrap().ws();
        let pty = Pty::spawn(argv, &std::env::temp_dir(), env, ws).unwrap();
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
        write_all(self.winch.1.as_raw_fd(), b"w");
    }
    fn exited(&self, id: PaneId) -> Option<u8> {
        self.events.iter().find_map(|e| match e {
            Event::PaneExited(i, status) if *i == id => Some(*status),
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
    /// Whether the host screen shows `text` once the reader has caught up
    /// with what the runtime already sent, without running the loop.
    fn sent(&mut self, text: &str) -> bool {
        let deadline = now_ms() + 1000;
        while now_ms() < deadline {
            self.pump();
            if self.screen().contains(text) {
                return true;
            }
            std::thread::sleep(std::time::Duration::from_millis(10));
        }
        false
    }
    /// Detach, and restore the terminal as the client does when it exits.
    fn restore(&mut self) {
        self.detach("");
        self.restore_terminal();
    }
    fn restore_terminal(&mut self) {
        write_all(
            self.slave.as_ref().unwrap().as_raw_fd(),
            &mux::host_restore(Some(true)),
        );
        // Let the reader catch up.
        std::thread::sleep(std::time::Duration::from_millis(100));
        self.pump();
    }
}
impl Drop for Host {
    fn drop(&mut self) {
        for p in self.mux.panes() {
            p.signal(libc::SIGKILL);
        }
        // Dropping the link ends the client.
        self.runtime.disconnect(&mut self.mux);
        if let Some(proxy) = self.proxy.take() {
            let _ = proxy.join();
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
        // The runtime had sent the output when it reported the exit.
        assert!(h.sent("goodbye from the child"), "{}", h.screen());
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
    for n in [1, 66, 1004, 2004] {
        assert!(h.mode(n), "mode {n}");
    }
    // The host reports the mouse in SGR form whatever the pane asks for.
    assert!(h.mode(1002) && h.mode(1006) && !h.mode(1000));
    assert_eq!(h.cursor_style(), CursorVisualStyle::Bar);
    h.send("a");
    h.wait_screen("got 97");
    h.send("q");
    assert_eq!(h.wait_exit(id), 0);
    h.restore();
    for n in [1, 66, 1000, 1002, 1003, 1004, 1006, 2004, 1049] {
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
    // The loop reports the move to the caller.
    assert!(h.events.iter().any(|e| matches!(e, Event::Focus(_))));
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
    for n in [1, 66, 1004, 2004] {
        assert!(!h.mode(n));
    }
    assert!(h.mode(1002) && h.mode(1006));
    assert_eq!(h.cursor_style(), CursorVisualStyle::Block);
    assert_eq!(h.terminal.title().unwrap(), b"right");
    h.send("\x1d");
    h.wait_screen("left got 73"); // ESC [ I: focus in
    h.until(|h| h.mode(2004) && h.kitty() == 1);
    assert_eq!(h.cursor_style(), CursorVisualStyle::Bar);
    assert_eq!(h.terminal.title().unwrap(), b"left");
}

#[test]
fn exited_pane_keeps_its_place_and_keeps_the_focus_until_it_is_released() {
    let mut h = Host::new(81, 12);
    let left = h.spawn("left", &["exit", "7", "left final"]);
    h.spawn("right", &["input", "right", "plain", "0"]);
    assert_eq!(h.wait_exit(left), 7);
    h.wait_screen("left (exited 7)");
    h.wait_screen("left final");
    // The caller may start another program in the pane, so it is still
    // focused until the caller says it is not.
    h.wait_screen("* left");
    h.mux.release_focus(left);
    h.wait_screen("* right");
    h.send("a\x1db");
    h.wait_screen("right got 98");
    assert!(h.screen().contains("right got 97"));
}

#[test]
fn the_zoom_key_is_reported_and_goes_to_no_pane() {
    let mut h = Host::new(81, 12);
    h.spawn("left", &["input", "left", "plain", "0"]);
    h.wait_screen("left ready");
    let zooms = |h: &Host| {
        h.events
            .iter()
            .filter(|e| matches!(e, Event::Zoom(_)))
            .count()
    };
    h.send("\x1ca");
    h.wait_screen("left got 97");
    assert_eq!(zooms(&h), 1);
    // Kitty's press is reported; its repeat and release are consumed.
    h.send("\x1b[92;5u\x1b[92;5:2u\x1b[92;5:3ub");
    h.wait_screen("left got 98");
    assert_eq!(zooms(&h), 2);
    assert!(!h.screen().contains("got 28"));
}

#[test]
fn a_hidden_pane_keeps_its_size_while_the_others_resize() {
    let mut h = Host::new(81, 12);
    let left = h.spawn("left", &["size"]);
    let right = h.spawn("right", &["size"]);
    h.wait_screen("size 40x11");
    let rects = layout(h.mux.size(), &[left, right]);
    let full = Rect {
        col: 0,
        row: 1,
        cols: 81,
        rows: 11,
    };
    h.mux
        .set_layout(&[(left, full), (right, Rect::default())])
        .unwrap();
    h.wait_screen("size 81x11");
    assert!(!h.screen().contains("right"));
    // The hidden program saw no resize.
    h.step(200);
    assert!(h.screen().lines().all(|l| !l.contains("size 40x")));
    assert_eq!(h.mux.pane(right).unwrap().ws().ws_col, rects[1].1.cols);
    // It keeps running, and is drawn again once it has a place.
    h.layout();
    h.wait_screen("right");
    h.wait_screen("size 40x11");
}

#[test]
fn a_restarted_pane_runs_a_new_program_in_the_same_place() {
    let mut h = Host::new(81, 12);
    let left = h.spawn("left", &["exit", "7", "old output"]);
    let right = h.spawn("right", &["input", "right", "plain", "0"]);
    assert_eq!(h.wait_exit(left), 7);
    h.wait_screen("left (exited 7)");
    h.wait_screen("old output");
    let place = h.mux.pane(left).unwrap().rect();

    // A pane whose program is running is left as it is.
    h.mux.restart(right, "ignored").unwrap();
    assert_eq!(h.mux.pane(right).unwrap().label, "right");

    h.mux.restart(left, "left again").unwrap();
    let ws = h.mux.pane(left).unwrap().ws();
    let env: &[(&str, &str)] = &[];
    let pty = Pty::spawn(
        &[CHILD, "input", "left", "plain", "0"],
        &std::env::temp_dir(),
        env,
        ws,
    )
    .unwrap();
    h.mux.attach(left, pty);
    h.wait_screen("left ready");
    let screen = h.screen();
    assert!(screen.contains("left again"), "{screen}");
    assert!(!screen.contains("exited"), "{screen}");
    assert!(!screen.contains("old output"), "{screen}");
    assert_eq!(h.mux.pane(left).unwrap().rect(), place);
    assert_eq!(h.mux.pane(left).unwrap().status(), None);

    // The pane kept the focus through the restart.
    h.send("a");
    h.wait_screen("left got 97");
}

#[test]
fn focus_skips_a_pane_marked_exited() {
    let mut h = Host::new(81, 12);
    h.spawn("left", &["input", "left", "plain", "0"]);
    let failed = h.mux.add_pane("failed").unwrap();
    h.layout();
    h.mux.mark_exited(failed, 127);
    h.spawn("right", &["input", "right", "plain", "0"]);
    h.wait_screen("failed (exited 127)");
    h.send("\x1db");
    h.wait_screen("right got 98");
    h.send("\x1da");
    h.wait_screen("left got 97");
    assert_eq!(h.mux.pane(failed).unwrap().status(), Some(127));
}

#[test]
fn mouse_reports_go_to_the_pane_under_the_pointer() {
    let path = TempPath::new("mouse");
    let mut h = Host::new(81, 20);
    h.spawn("left", &["input", "left", "plain", "0"]);
    h.spawn("right", &["input", "right", "modes", "0", path.path()]);
    h.wait_screen("left ready");
    h.wait_screen("right ready");
    h.send("\x1d");
    h.wait_screen("* left");
    // The right pane starts at column 42, row 2, and asks for focus events
    // and normal tracking in SGR form. A click there focuses it and is
    // reported in its coordinates; motion is not reported, a report on a
    // label row is dropped, and the release of a drag that left the pane is
    // clamped to it.
    h.send(b"\x1b[<0;43;3M\x1b[<0;43;3m\x1b[<32;44;3M\x1b[<0;43;1M");
    h.wait_screen("* right");
    h.send(b"\x1b[<0;44;4M\x1b[<32;10;5M\x1b[<0;10;5m!");
    h.wait_screen("right got 33");
    assert_eq!(
        path.read(),
        b"\x1b[I\x1b[<0;2;2M\x1b[<0;2;2m\x1b[<0;3;3M\x1b[<0;1;4m!"
    );
    assert!(!h.screen().contains("left got"));
}

/// What the host terminal reports for a left-button press at its first
/// cell, as its mouse modes encode it.
fn host_click(h: &Host) -> Vec<u8> {
    let mut encoder = mouse::Encoder::new().unwrap();
    encoder
        .set_options_from_terminal(&h.terminal)
        .set_size(mouse::EncoderSize {
            screen_width: 81 * 8,
            screen_height: 12 * 16,
            cell_width: 8,
            cell_height: 16,
            padding_top: 0,
            padding_bottom: 0,
            padding_right: 0,
            padding_left: 0,
        });
    let mut event = mouse::Event::new().unwrap();
    event
        .set_action(mouse::Action::Press)
        .set_button(Some(mouse::Button::Left))
        .set_position(mouse::Position { x: 4.0, y: 8.0 });
    let mut buf = [0; 64];
    let n = encoder.encode(&event, &mut buf).unwrap();
    buf[..n].to_vec()
}

#[test]
fn the_host_reports_clicks_whatever_the_focused_pane_tracks() {
    let mut h = Host::new(81, 12);
    h.spawn("left", &["input", "left", "plain", "0"]);
    h.spawn_argv(
        "any",
        &["/bin/sh", "-c", "printf '\\033[?1003hany ready'; sleep 30"],
    );
    h.wait_screen("left ready");
    h.wait_screen("any ready");
    h.until(|h| host_click(h) == b"\x1b[<0;1;1M");
    // Focus moves to the pane with any-event tracking, and back.
    h.send("\x1d");
    h.until(|h| h.mode(1003) && host_click(h) == b"\x1b[<0;1;1M");
    h.send("\x1d");
    h.until(|h| !h.mode(1003) && host_click(h) == b"\x1b[<0;1;1M");
}

#[test]
fn a_click_focuses_a_pane_that_does_not_track_the_mouse() {
    let mut h = Host::new(81, 12);
    h.spawn("left", &["input", "left", "plain", "0"]);
    h.spawn("right", &["input", "right", "plain", "0"]);
    h.wait_screen("left ready");
    h.wait_screen("right ready");
    h.send("\x1b[<0;50;5M\x1b[<0;50;5ma");
    h.wait_screen("right got 97");
    h.send("\x1b[<2;5;5M\x1b[<2;5;5mb");
    h.wait_screen("left got 98");
    // The program doesn't track the mouse, so it got no reports.
    assert!(!h.screen().contains("got 27"));
}

/// The first row of the pane in a single-pane host.
fn top_row(h: &Host) -> String {
    h.line(1)
}

#[test]
fn the_wheel_scrolls_a_pane_and_typing_returns_it_to_the_bottom() {
    let mut h = Host::new(40, 10);
    let id = h.spawn("lines", &["lines", "30"]);
    h.wait_screen("ready");
    assert_eq!(top_row(&h), "line 22");
    h.send("\x1b[<64;5;5M");
    h.wait_screen("lines (scrolled 3)");
    assert_eq!(top_row(&h), "line 19");
    // New output leaves the view where it is.
    h.mux.send_text(id, "x").unwrap();
    h.wait_screen("lines (scrolled 4)");
    assert_eq!(top_row(&h), "line 19");
    h.send("\x1b[<65;5;5M");
    h.wait_screen("lines (scrolled 1)");
    assert_eq!(top_row(&h), "line 22");
    // Typing returns to the bottom and reaches the program.
    h.send("y");
    h.wait_screen("got 121");
    h.until(|h| !h.screen().contains("scrolled"));
    assert_eq!(top_row(&h), "line 24");
}

#[test]
fn scroll_mode_keys_scroll_and_esc_returns_to_the_bottom() {
    let mut h = Host::new(40, 10);
    h.spawn("lines", &["lines", "30"]);
    h.wait_screen("ready");
    // Shift-PageUp scrolls a page of nine rows.
    h.send("\x1b[5;2~");
    h.wait_screen("lines (scrolled 9)");
    assert_eq!(top_row(&h), "line 13");
    h.send("\x1b[A");
    h.wait_screen("lines (scrolled 10)");
    h.send("\x1b[6~");
    h.wait_screen("lines (scrolled 1)");
    h.send("\x1b[5~");
    h.wait_screen("lines (scrolled 10)");
    h.send("\x1b");
    h.until(|h| !h.screen().contains("scrolled"));
    // Scrolling took none of the keys, and q leaves scroll mode too.
    h.send("\x1b[5;2~");
    h.wait_screen("lines (scrolled 9)");
    h.send("q");
    h.until(|h| !h.screen().contains("scrolled"));
    h.send("z");
    h.wait_screen("got 122");
    assert!(!h.screen().contains("got 27"), "{}", h.screen());
    assert!(!h.screen().contains("got 113"), "{}", h.screen());
}

fn copied(text: &str) -> Vec<u8> {
    const ALPHABET: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = b"\x1b]52;c;".to_vec();
    for chunk in text.as_bytes().chunks(3) {
        let mut n = 0u32;
        for (i, &b) in chunk.iter().enumerate() {
            n |= u32::from(b) << (16 - 8 * i);
        }
        for i in 0..4 {
            out.push(if i <= chunk.len() {
                ALPHABET[(n >> (18 - 6 * i)) as usize & 63]
            } else {
                b'='
            });
        }
    }
    out.push(0x07);
    out
}

#[test]
fn a_drag_selects_text_and_copies_it_on_release() {
    let mut h = Host::new(40, 10);
    h.spawn("lines", &["lines", "30"]);
    h.wait_screen("ready");
    // From the "i" of "line 22" to the "e" of "line 23", rows 2 and 3.
    h.send("\x1b[<0;2;2M\x1b[<32;4;3M");
    h.until(|h| {
        let style = |x, y| {
            h.terminal
                .grid_ref(Point::Active(PointCoordinate { x, y }))
                .unwrap()
                .style()
                .unwrap()
                .inverse
        };
        style(1, 1) && style(3, 2) && !style(0, 1) && !style(4, 2)
    });
    h.send("\x1b[<0;4;3m");
    let expected = copied("ine 22\nline");
    h.until(|h| contains(&h.output, &expected));
    // A click clears the selection without copying.
    h.send("\x1b[<0;9;6M\x1b[<0;9;6m");
    h.until(|h| {
        !h.terminal
            .grid_ref(Point::Active(PointCoordinate { x: 1, y: 1 }))
            .unwrap()
            .style()
            .unwrap()
            .inverse
    });
    // A scrolled view selects what it shows.
    h.send("\x1b[<64;5;5M");
    h.wait_screen("lines (scrolled 3)");
    h.send("\x1b[<0;1;2M\x1b[<32;7;2M\x1b[<0;7;2m");
    let expected = copied("line 19");
    h.until(|h| contains(&h.output, &expected));
    assert_eq!(
        h.output.windows(7).filter(|w| w == b"\x1b]52;c;").count(),
        2
    );
}

#[test]
fn notifications_pass_through_to_the_host() {
    let mut h = Host::new(81, 12);
    h.spawn("left", &["input", "left", "plain", "0"]);
    h.spawn("notify", &["notify"]);
    h.wait_screen("notified");
    h.until(|h| {
        contains(&h.output, b"\x1b]9;first\x07")
            && contains(&h.output, b"\x1b]777;notify;Title;Body\x07")
    });
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
fn send_text_and_send_key_encode_for_the_panes_keyboard_mode() {
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
        h.mux.send_text(id, "hI").unwrap();
        h.mux.send_key(id, Key::Enter).unwrap();
    }
    h.until(|_| legacy.read() == b"hI\r" && kitty.read().ends_with(b"\x1b[13;1:3u"));
    // Report-all-keys mode gets every key as an escape code.
    assert_eq!(
        String::from_utf8(kitty.read()).unwrap(),
        "\x1b[104u\x1b[104;1:3u\x1b[105;2u\x1b[105;2:3u\x1b[13u\x1b[13;1:3u"
    );
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
fn removed_pane_that_ignores_sighup_is_killed() {
    let mut h = Host::new(40, 10);
    let id = h.spawn("child", &["ignore-hup"]);
    h.wait_screen("ignoring");
    let pid = h.mux.pane(id).unwrap().pid().unwrap() as libc::pid_t;
    let removed = now_ms();
    h.remove(id);
    // Reaping clears the zombie, so the pid disappears.
    h.until(|_| unsafe { libc::kill(pid, 0) } != 0);
    assert!(now_ms() - removed >= mux::HANGUP_GRACE_MS);
    assert!(!h.mux.reaping());
}

#[test]
fn removed_panes_process_group_is_killed_after_its_leader_exits() {
    let mut h = Host::new(40, 10);
    // The shell exits on SIGHUP; the sleep it started ignores it.
    let script = "trap '' HUP; sleep 30 & trap - HUP; echo sleeper $!; wait";
    let id = h.spawn_argv("shell", &["/bin/sh", "-c", script]);
    h.wait_screen("sleeper ");
    let line = h
        .screen()
        .lines()
        .find(|l| l.starts_with("sleeper "))
        .unwrap()
        .to_owned();
    let sleeper: libc::pid_t = line["sleeper ".len()..].trim().parse().unwrap();
    let removed = now_ms();
    h.remove(id);
    h.until(|_| unsafe { libc::kill(sleeper, 0) } != 0);
    assert!(now_ms() - removed >= mux::HANGUP_GRACE_MS);
    h.until(|h| !h.mux.reaping());
}

#[test]
fn input_a_program_does_not_read_does_not_block_the_loop() {
    let mut h = Host::new(40, 10);
    let id = h.spawn("child", &["ignore-hup"]);
    h.wait_screen("ignoring");
    h.mux.pane(id).unwrap().send(&vec![b'x'; 1 << 20]);
    assert!(h.mux.pane(id).unwrap().input_pending());
    let start = now_ms();
    let events = h.runtime.wait(&mut h.mux, &[], Some(start + 100)).unwrap();
    assert_eq!(events, [Event::Deadline]);
    assert!(now_ms() - start < 1000);
}

#[test]
fn attaching_again_shows_the_same_screen_and_modes() {
    let mut h = Host::new(81, 12);
    h.spawn("left", &["input", "left", "modes", "0"]);
    h.spawn("right", &["lines", "30"]);
    h.wait_screen("left ready");
    h.wait_screen("ready");
    h.until(|h| h.kitty() == 1 && h.mode(2004));
    h.step(100);
    let (screen, kitty, cursor) = (h.screen(), h.kitty(), h.cursor_style());
    let modes: Vec<bool> = [1, 66, 1002, 1004, 1006, 2004].map(|n| h.mode(n)).to_vec();
    let title = h.terminal.title().unwrap().to_vec();

    assert_eq!(h.detach(""), "");
    h.restore_terminal();
    // A new client starts from a clear screen.
    write_all(h.slave.as_ref().unwrap().as_raw_fd(), b"\x1b[2J");
    h.until(|h| !h.screen().contains("ready"));
    assert_eq!(h.kitty(), 0);
    h.attach();
    h.until(|h| h.screen() == screen);
    h.until(|h| h.kitty() == kitty);
    assert_eq!(h.cursor_style(), cursor);
    assert_eq!(
        [1, 66, 1002, 1004, 1006, 2004].map(|n| h.mode(n)).to_vec(),
        modes
    );
    assert_eq!(h.terminal.title().unwrap(), title);
    h.send("a");
    h.wait_screen("left got 97");
}

#[test]
fn a_client_that_stops_reading_does_not_stop_the_panes() {
    let input = TempPath::new("unread");
    let mut h = Host::new(40, 10);
    h.runtime.set_stall_ms(60_000);
    // A client that reads nothing, with more queued than the socket holds.
    let (host, client) = UnixStream::pair().unwrap();
    h.runtime
        .connect(
            &mut h.mux,
            Link::new(host).unwrap(),
            &host_replies(),
            winsize(40, 10).into(),
        )
        .unwrap();
    h.proxy.take().unwrap().join().unwrap();
    h.runtime.send(Message::Output(vec![b' '; 1 << 20]));
    let id = h.spawn("child", &["input", "child", "plain", "0", input.path()]);
    let exits = h.spawn("exit", &["exit", "7", "exit output"]);
    assert_eq!(h.wait_exit(exits), 7);
    // Input still reaches the panes.
    (&client)
        .write_all(&link::encode(&Message::Input(b"xyz".to_vec())).unwrap())
        .unwrap();
    h.until(|_| input.read() == b"xyz");
    h.mux.send_text(id, "w").unwrap();
    h.until(|_| input.read() == b"xyzw");

    // Once it reads, it gets the latest screen.
    let tty = h.tty();
    h.proxy = Some(std::thread::spawn(move || {
        client::proxy(&tty, client).unwrap()
    }));
    h.wait_screen("child got 119");
    h.wait_screen("exit (exited 7)");
}

#[test]
fn a_stalled_link_is_disconnected() {
    let mut h = Host::new(40, 10);
    h.runtime.set_stall_ms(200);
    let (host, client) = UnixStream::pair().unwrap();
    h.runtime
        .connect(
            &mut h.mux,
            Link::new(host).unwrap(),
            &host_replies(),
            winsize(40, 10).into(),
        )
        .unwrap();
    h.proxy.take().unwrap().join().unwrap();
    h.runtime.send(Message::Output(vec![b' '; 1 << 20]));
    let start = now_ms();
    h.until(|h| h.events.contains(&Event::Disconnected));
    assert!(now_ms() - start >= 200);
    assert!(!h.runtime.connected() && !h.mux.connected());
    drop(client);
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
