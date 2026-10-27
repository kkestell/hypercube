//! The event loop around a `Mux`. The runtime owns the host terminal: it
//! puts it in raw mode on the alternate screen, asks it for its colors,
//! reads its input, and draws frames into it. The caller drives the loop
//! one step at a time with `Runtime::wait`, which handles pane output, host
//! input, frame pacing, and holds, and returns what the caller must act on.

use std::error::Error;
use std::os::fd::RawFd;

use crate::mux::{self, Mux, PaneId, Winsize, now_ms, read, write_all};
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
    winch: RawFd,
    input_open: bool,
    last_render: i64,
    buf: Vec<u8>,
}

impl Runtime {
    /// Take over the host terminal whose input and output are `input` and
    /// `output`, and size `mux` to it.
    pub fn new(mux: &mut Mux, input: RawFd, output: RawFd) -> Result<Runtime, Box<dyn Error>> {
        let mut termios = std::mem::MaybeUninit::<libc::termios>::uninit();
        if unsafe { libc::tcgetattr(input, termios.as_mut_ptr()) } != 0 {
            return Err(format!("host terminal: {}", std::io::Error::last_os_error()).into());
        }
        let termios = unsafe { termios.assume_init() };
        let mut raw = termios;
        unsafe {
            libc::cfmakeraw(&mut raw);
            libc::tcsetattr(input, libc::TCSAFLUSH, &raw);
        }
        let mut runtime = Runtime {
            input,
            output,
            termios: Some(termios),
            winch: winch::install()?,
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
            if left <= 0 || !poll(&mut [pollfd(self.input)], left)? {
                break;
            }
            let n = read(self.input, &mut buf[len..]);
            if n <= 0 {
                break;
            }
            len += n as usize;
        }
        mux.use_host_replies(&buf[..len])?;
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
            for (id, status) in mux.reap() {
                events.push(Event::PaneExited(id, status));
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
            fds.extend(panes.iter().map(|&(_, fd)| pollfd(fd)));
            fds.extend(extra.iter().map(|&fd| pollfd(fd)));
            let timeout = wake.map_or(-1, |w| (w - now).max(0));
            if !poll(&mut fds, timeout)? {
                continue;
            }

            let mut fds = fds.iter().map(|f| (f.fd, f.revents != 0));
            if fds.next().is_some_and(|(_, ready)| ready) {
                winch::take(self.winch);
            }
            if self.input_open && fds.next().is_some_and(|(_, ready)| ready) {
                let n = read(self.input, &mut self.buf);
                if n > 0 {
                    mux.input(&self.buf[..n as usize]);
                } else {
                    self.input_open = false;
                    events.push(Event::InputClosed);
                }
            }
            for &(id, fd) in &panes {
                if !fds.next().is_some_and(|(_, ready)| ready) {
                    continue;
                }
                let n = read(fd, &mut self.buf);
                let Some(p) = mux.pane_mut(id) else { continue };
                if n > 0 {
                    p.feed(&self.buf[..n as usize]);
                } else {
                    p.close_master();
                }
            }
            events.extend(
                fds.filter(|&(_, ready)| ready)
                    .map(|(fd, _)| Event::Ready(fd)),
            );
        }
    }

    /// Return the host terminal to how the runtime found it.
    pub fn restore(&mut self, mux: &mut Mux) {
        mux.write_restore();
        write_all(self.output, &mux.take_output());
        if let Some(termios) = self.termios.take() {
            unsafe { libc::tcsetattr(self.input, libc::TCSAFLUSH, &termios) };
        }
    }
}

impl Drop for Runtime {
    /// Leave the alternate screen and raw mode if `restore` wasn't called.
    fn drop(&mut self) {
        if let Some(termios) = self.termios.take() {
            write_all(self.output, b"\x1b[0m\x1b[?25h\x1b[?1049l");
            unsafe { libc::tcsetattr(self.input, libc::TCSAFLUSH, &termios) };
        }
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
