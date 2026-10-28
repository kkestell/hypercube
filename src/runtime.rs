//! The event loop around a `Mux`. The runtime reads the panes' ptys and
//! draws frames for the connected client, over a `Link` to it. The caller
//! drives the loop one step at a time with `Runtime::wait`, which handles
//! pane output, the client's input, frame pacing, and holds, and returns
//! what the caller must act on.

use std::error::Error;
use std::os::fd::RawFd;

use crate::link::{Link, Message, Size};
use crate::mux::{Mux, Notice, Pane, PaneId, now_ms, read};

/// The shortest time between frames.
const FRAME_MS: i64 = 16;
/// How often to check for the exit of a program whose pty has closed.
const REAP_MS: i64 = 20;
/// How long a link may keep a queue without writing any of it before it is
/// disconnected.
const LINK_STALL_MS: i64 = 10_000;

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Event {
    /// One of the caller's descriptors is readable or hung up.
    Ready(RawFd),
    /// A pane's program exited with this status: its code, or 128 plus the
    /// signal that ended it.
    PaneExited(PaneId, u8),
    /// Ctrl-b z was pressed while this pane had the focus.
    Zoom(PaneId),
    /// The focus moved to this pane. `Zoom` and `Focus` come in the order
    /// they happened.
    Focus(PaneId),
    /// Ctrl-b s was pressed.
    Overlay,
    /// A key arrived while input is captured; see `Mux::set_captured`.
    Key(Vec<u8>),
    /// The prefix key was followed by this digit.
    Workspace(u8),
    /// Ctrl-b ( or ) was pressed: switch this many workspaces along the
    /// bar.
    Step(i8),
    /// Ctrl-b c was pressed; input is captured.
    NewWorkspace,
    /// Ctrl-b d was pressed.
    Detach,
    /// Ctrl-b a was pressed.
    Accent,
    /// Ctrl-b ? was pressed.
    Help,
    /// The left button was pressed outside every pane, at this column and
    /// row.
    Click(u16, u16),
    /// The client sent a message the runtime doesn't handle itself.
    Message(Message),
    /// The client's terminal changed size. The caller lays the panes out
    /// again.
    Resized,
    /// The link ended, failed, or stalled, and the runtime disconnected it.
    Disconnected,
    /// The deadline passed.
    Deadline,
}

pub struct Runtime {
    link: Option<Link>,
    /// Links let go of, until they have written what they had queued.
    closing: Vec<Link>,
    last_render: i64,
    stall_ms: i64,
    buf: Vec<u8>,
}

impl Default for Runtime {
    fn default() -> Runtime {
        Runtime::new()
    }
}

impl Runtime {
    /// A runtime with no client connected.
    pub fn new() -> Runtime {
        Runtime {
            link: None,
            closing: Vec::new(),
            last_render: 0,
            stall_ms: LINK_STALL_MS,
            buf: vec![0; 65536],
        }
    }

    /// Change how long a link may stall; see `LINK_STALL_MS`.
    pub fn set_stall_ms(&mut self, ms: i64) {
        self.stall_ms = ms;
    }

    /// Draw for the client at the other end of `link`, whose terminal is
    /// `size` and answered `mux::host_queries` with `replies`.
    pub fn connect(
        &mut self,
        mux: &mut Mux,
        link: Link,
        replies: &[u8],
        size: Size,
    ) -> Result<(), Box<dyn Error>> {
        self.disconnect(mux);
        mux.connect_host(replies, size.into())?;
        self.link = Some(link);
        self.last_render = 0;
        Ok(())
    }

    /// Drop the link at once.
    pub fn disconnect(&mut self, mux: &mut Mux) {
        self.link = None;
        mux.disconnect_host();
    }

    /// Tell the client it is detached, for `reason`, and let it go once it
    /// has what was queued for it.
    pub fn detach(&mut self, mux: &mut Mux, reason: &str) {
        if let Some(mut link) = self.link.take() {
            link.send(Message::Detached(reason.into()));
            self.closing.push(link);
        }
        mux.disconnect_host();
    }

    /// Queue a message for the client.
    pub fn send(&mut self, message: Message) {
        if let Some(link) = &mut self.link {
            link.send(message);
        }
    }

    pub fn connected(&self) -> bool {
        self.link.is_some()
    }

    /// Whether a detached client still has queued messages to take.
    pub fn closing(&self) -> bool {
        !self.closing.is_empty()
    }

    /// Run the loop until something needs the caller: one of `extra`
    /// becomes readable, a pane's program exits, a key the caller handles is
    /// pressed, the focus moves, the client resizes, sends a message, or
    /// goes away, or `deadline` (in `now_ms` time) passes. Between those,
    /// pane output is drawn at most once per frame, and only when the link
    /// has written the last one, and client input goes to the focused pane.
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
                // the exit.
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
                if self.link.as_ref().is_some_and(|l| !l.pending()) {
                    self.render(mux)?;
                }
            }
            events.extend(mux.take_notices().into_iter().map(|n| match n {
                Notice::Focus(id) => Event::Focus(id),
                Notice::Zoom(id) => Event::Zoom(id),
                Notice::Overlay => Event::Overlay,
                Notice::Key(key) => Event::Key(key),
                Notice::Workspace(n) => Event::Workspace(n),
                Notice::Step(n) => Event::Step(n),
                Notice::NewWorkspace => Event::NewWorkspace,
                Notice::Detach => Event::Detach,
                Notice::Accent => Event::Accent,
                Notice::Help => Event::Help,
                Notice::Click(col, row) => Event::Click(col, row),
            }));
            let now = now_ms();
            if let Some(link) = &self.link
                && link.pending()
                && now - link.last_progress() >= self.stall_ms
            {
                self.disconnect(mux);
                events.push(Event::Disconnected);
            }
            self.closing
                .retain(|l| l.pending() && now - l.last_progress() < self.stall_ms);
            if deadline.is_some_and(|d| now >= d) {
                events.push(Event::Deadline);
            }
            if !events.is_empty() {
                if self.flush().is_err() {
                    self.disconnect(mux);
                    events.push(Event::Disconnected);
                }
                return Ok(events);
            }

            mux.expire_holds(now)?;
            if mux.input_deadline().is_some_and(|d| now >= d) {
                mux.flush_input();
            }
            let idle = self.link.as_ref().is_some_and(|l| !l.pending());
            if idle {
                if mux.needs_render() && now - self.last_render >= FRAME_MS {
                    self.render(mux)?;
                } else {
                    // Output from outside frames goes as it comes.
                    let out = mux.take_output();
                    if !out.is_empty() {
                        self.send(Message::Output(out));
                    }
                }
            }
            if self.flush().is_err() {
                self.disconnect(mux);
                events.push(Event::Disconnected);
                continue;
            }

            let mut wake = deadline;
            match &self.link {
                Some(link) if link.pending() => {
                    wake = earliest(wake, link.last_progress() + self.stall_ms);
                }
                Some(_) if mux.needs_render() => {
                    wake = earliest(wake, self.last_render + FRAME_MS);
                }
                _ => {}
            }
            for link in &self.closing {
                wake = earliest(wake, link.last_progress() + self.stall_ms);
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

            let mut fds = Vec::new();
            if let Some(link) = &self.link {
                let mut f = pollfd(link.fd());
                if link.pending() {
                    f.events |= libc::POLLOUT;
                }
                fds.push(f);
            }
            for link in &self.closing {
                let mut f = pollfd(link.fd());
                f.events = libc::POLLOUT;
                fds.push(f);
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
            if self.link.is_some() {
                let revents = fds.next().map_or(0, |(_, revents)| revents);
                if revents & !libc::POLLOUT != 0 {
                    self.read_link(mux, &mut events);
                }
            }
            for _ in 0..self.closing.len() {
                fds.next();
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

    /// Draw a frame and queue it for the client.
    fn render(&mut self, mux: &mut Mux) -> Result<(), Box<dyn Error>> {
        mux.render()?;
        self.last_render = now_ms();
        let out = mux.take_output();
        if !out.is_empty() {
            self.send(Message::Output(out));
        }
        Ok(())
    }

    /// Write what the links have queued. An error ends the current link;
    /// a closing link that fails is dropped.
    fn flush(&mut self) -> std::io::Result<()> {
        self.closing
            .retain_mut(|l| l.flush().is_ok() && l.pending());
        match &mut self.link {
            Some(link) => link.flush().map(|_| ()),
            None => Ok(()),
        }
    }

    /// Take the messages the client sent: input for the panes, resizes, and
    /// the rest for the caller. The end of the stream or an error
    /// disconnects it.
    fn read_link(&mut self, mux: &mut Mux, events: &mut Vec<Event>) {
        let Some(link) = &mut self.link else { return };
        let messages = match link.read() {
            Ok(messages) => messages,
            Err(_) => {
                self.disconnect(mux);
                events.push(Event::Disconnected);
                return;
            }
        };
        for message in messages {
            match message {
                Message::Input(data) => mux.input(&data),
                Message::Resize(size) => {
                    mux.resize(size.into());
                    events.push(Event::Resized);
                }
                message => events.push(Event::Message(message)),
            }
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
}

fn earliest(a: Option<i64>, b: i64) -> Option<i64> {
    Some(a.map_or(b, |a| a.min(b)))
}

pub fn pollfd(fd: RawFd) -> libc::pollfd {
    libc::pollfd {
        fd,
        events: libc::POLLIN,
        revents: 0,
    }
}

/// Poll `fds` for up to `timeout` milliseconds, or without a limit if it is
/// negative. Returns whether any is ready; an interrupted poll returns false.
pub fn poll(fds: &mut [libc::pollfd], timeout: i64) -> Result<bool, Box<dyn Error>> {
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
