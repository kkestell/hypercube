//! The event loop around a `Mux`. The runtime owns the host terminal: it
//! puts it in raw mode on the alternate screen, asks it for its colors,
//! reads its input, and draws frames into it. The caller drives the loop
//! one step at a time with `Runtime::wait`, which handles pane output, host
//! input, frame pacing, and holds, and returns what the caller must act on.

use std::error::Error;
use std::os::fd::RawFd;

use crate::mux::{self, Mux, Pane, PaneId, Winsize, now_ms, read, write_all};
use crate::winch;

/// The shortest time between frames.
const FRAME_MS: i64 = 16;
/// How long to wait for the host's replies to its queries.
const HOST_REPLY_TIMEOUT_MS: i64 = 1000;
/// How often to check for the exit of a program whose pty has closed.
const REAP_MS: i64 = 20;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Event {
    /// One of the caller's descriptors is readable or hung up.
    Ready(RawFd),
    /// A pane's program exited with this status: its code, or 128 plus the
    /// signal that ended it.
    PaneExited(PaneId, u8),
    /// The host terminal changed size. The caller lays the panes out again.
    Resized,
    /// The host terminal's input ended.
    InputClosed,
    /// The deadline passed.
    Deadline,
}

pub struct Runtime {
    input: RawFd,
    output: RawFd,
    /// The host terminal's settings before raw mode, until restored.
    termios: Option<libc::termios>,
    /// Whether the host clusters graphemes by default, once it has said.
    clusters: Option<bool>,
    winch: RawFd,
    input_open: bool,
    last_render: i64,
    buf: Vec<u8>,
}

impl Runtime {
    /// Take over the host terminal whose input and output are `input` and
    /// `output`, and size `mux` to it.
    pub fn new(mux: &mut Mux, input: RawFd, output: RawFd) -> Result<Runtime, Box<dyn Error>> {
        let winch = winch::install()?;
        let mut termios = std::mem::MaybeUninit::<libc::termios>::uninit();
        if unsafe { libc::tcgetattr(input, termios.as_mut_ptr()) } != 0 {
            return Err(format!("host terminal: {}", std::io::Error::last_os_error()).into());
        }
        let termios = unsafe { termios.assume_init() };
        let mut raw = termios;
        unsafe { libc::cfmakeraw(&mut raw) };
        if unsafe { libc::tcsetattr(input, libc::TCSAFLUSH, &raw) } != 0 {
            return Err(format!("host terminal: {}", std::io::Error::last_os_error()).into());
        }
        // From here on, dropping the runtime restores the host terminal.
        let mut runtime = Runtime {
            input,
            output,
            termios: Some(termios),
            clusters: None,
            winch,
            input_open: true,
            last_render: 0,
            buf: vec![0; 65536],
        };
        write_all(output, b"\x1b[?1049h\x1b[2J");
        runtime.query_host(mux)?;
        mux.resize(runtime.host_size());
        Ok(runtime)
    }

    /// Ask the host for its colors and grapheme clustering mode; see
    /// `mux::use_host_replies_for`.
    fn query_host(&mut self, mux: &mut Mux) -> Result<(), Box<dyn Error>> {
        write_all(self.output, &mux::host_queries());
        let mut buf = [0u8; 4096];
        let mut len = 0;
        let deadline = now_ms() + HOST_REPLY_TIMEOUT_MS;
        while len < buf.len() && !mux::host_replies_done(&buf[..len]) {
            let left = deadline - now_ms();
            if left <= 0 {
                break;
            }
            // Not ready means interrupted or timed out; the deadline tells
            // which.
            if !poll(&mut [pollfd(self.input)], left)? {
                continue;
            }
            let n = read(self.input, &mut buf[len..]);
            if n <= 0 {
                break;
            }
            len += n as usize;
        }
        mux.use_host_replies(&buf[..len])?;
        self.clusters = Some(mux::host_clusters(&buf[..len]));
        Ok(())
    }

    pub fn host_size(&self) -> Winsize {
        let mut ws = Winsize {
            ws_row: 0,
            ws_col: 0,
            ws_xpixel: 0,
            ws_ypixel: 0,
        };
        unsafe { libc::ioctl(self.output, libc::TIOCGWINSZ, &mut ws) };
        ws
    }

    /// Run the loop until something needs the caller: one of `extra`
    /// becomes readable, a pane's program exits, the host resizes or its
    /// input ends, or `deadline` (in `now_ms` time) passes. Between those,
    /// pane output is drawn at most once per frame, and host input goes to
    /// the focused pane.
    pub fn wait(
        &mut self,
        mux: &mut Mux,
        extra: &[RawFd],
        deadline: Option<i64>,
    ) -> Result<Vec<Event>, Box<dyn Error>> {
        let mut events = Vec::new();
        loop {
            let exited = mux.reap();
            if !exited.is_empty() {
                // Draw everything an exited program wrote before reporting
                // the exit, in case the caller ends the session.
                for &(id, status) in &exited {
                    for _ in 0..16 {
                        let Some(fd) = mux.pane(id).and_then(Pane::pty) else {
                            break;
                        };
                        if !self.read_pane(mux, id, fd) {
                            break;
                        }
                    }
                    events.push(Event::PaneExited(id, status));
                }
                mux.render()?;
                self.last_render = now_ms();
                write_all(self.output, &mux.take_output());
            }
            let size = self.host_size();
            let old = mux.size();
            if (size.ws_col, size.ws_row, size.ws_xpixel, size.ws_ypixel)
                != (old.ws_col, old.ws_row, old.ws_xpixel, old.ws_ypixel)
            {
                mux.resize(size);
                events.push(Event::Resized);
            }
            let now = now_ms();
            if deadline.is_some_and(|d| now >= d) {
                events.push(Event::Deadline);
            }
            if !events.is_empty() {
                return Ok(events);
            }

            mux.expire_holds(now)?;
            if mux.input_deadline().is_some_and(|d| now >= d) {
                mux.flush_input();
            }
            if mux.needs_render() && now - self.last_render >= FRAME_MS {
                mux.render()?;
                self.last_render = now;
            }
            write_all(self.output, &mux.take_output());

            let mut wake = deadline;
            if mux.needs_render() {
                wake = earliest(wake, self.last_render + FRAME_MS);
            }
            if let Some(hold) = mux.hold_deadline() {
                wake = earliest(wake, hold);
            }
            if let Some(input) = mux.input_deadline() {
                wake = earliest(wake, input);
            }
            if mux.reaping() {
                wake = earliest(wake, now + REAP_MS);
            }

            let mut fds = vec![pollfd(self.winch)];
            if self.input_open {
                fds.push(pollfd(self.input));
            }
            let panes: Vec<(PaneId, RawFd)> = mux
                .panes()
                .iter()
                .filter_map(|p| Some((p.id(), p.pty()?)))
                .collect();
            for &(id, fd) in &panes {
                let mut f = pollfd(fd);
                if mux.pane(id).is_some_and(Pane::input_pending) {
                    f.events |= libc::POLLOUT;
                }
                fds.push(f);
            }
            fds.extend(extra.iter().map(|&fd| pollfd(fd)));
            let timeout = wake.map_or(-1, |w| (w - now).max(0));
            if !poll(&mut fds, timeout)? {
                continue;
            }

            let mut fds = fds.iter().map(|f| (f.fd, f.revents));
            if fds.next().is_some_and(|(_, revents)| revents != 0) {
                winch::take(self.winch);
            }
            if self.input_open && fds.next().is_some_and(|(_, revents)| revents != 0) {
                let n = read(self.input, &mut self.buf);
                if n > 0 {
                    mux.input(&self.buf[..n as usize]);
                } else {
                    self.input_open = false;
                    events.push(Event::InputClosed);
                }
            }
            for &(id, fd) in &panes {
                let revents = fds.next().map_or(0, |(_, revents)| revents);
                if revents & libc::POLLOUT != 0
                    && let Some(p) = mux.pane(id)
                {
                    p.write_input();
                }
                if revents & !libc::POLLOUT != 0 {
                    self.read_pane(mux, id, fd);
                }
            }
            events.extend(
                fds.filter(|&(_, revents)| revents != 0)
                    .map(|(fd, _)| Event::Ready(fd)),
            );
        }
    }

    /// Read once from a pane's non-blocking master, feeding the pane, or
    /// closing the master at the end of the program's output. Returns
    /// whether there was output.
    fn read_pane(&mut self, mux: &mut Mux, id: PaneId, fd: RawFd) -> bool {
        let n = read(fd, &mut self.buf);
        let would_block =
            n < 0 && std::io::Error::last_os_error().raw_os_error() == Some(libc::EAGAIN);
        let Some(p) = mux.pane_mut(id) else {
            return false;
        };
        if n > 0 {
            p.feed(&self.buf[..n as usize]);
            return true;
        }
        if !would_block {
            p.close_master();
        }
        false
    }

    /// Return the host terminal to how the runtime found it, undoing the
    /// modes the mux mirrored onto it. Dropping the runtime does this too.
    pub fn restore(&mut self) {
        if let Some(termios) = self.termios.take() {
            write_all(self.output, &mux::host_restore(self.clusters));
            unsafe { libc::tcsetattr(self.input, libc::TCSAFLUSH, &termios) };
        }
    }
}

impl Drop for Runtime {
    fn drop(&mut self) {
        self.restore();
    }
}

fn earliest(a: Option<i64>, b: i64) -> Option<i64> {
    Some(a.map_or(b, |a| a.min(b)))
}

fn pollfd(fd: RawFd) -> libc::pollfd {
    libc::pollfd {
        fd,
        events: libc::POLLIN,
        revents: 0,
    }
}

/// Poll `fds` for up to `timeout` milliseconds, or without a limit if it is
/// negative. Returns whether any is ready; an interrupted poll returns false.
fn poll(fds: &mut [libc::pollfd], timeout: i64) -> Result<bool, Box<dyn Error>> {
    let timeout = timeout.min(i64::from(libc::c_int::MAX)) as libc::c_int;
    let ready = unsafe { libc::poll(fds.as_mut_ptr(), fds.len() as libc::nfds_t, timeout) };
    if ready < 0 {
        let err = std::io::Error::last_os_error();
        if err.raw_os_error() == Some(libc::EINTR) {
            return Ok(false);
        }
        return Err(format!("poll: {err}").into());
    }
    Ok(ready > 0)
}
