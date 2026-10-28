//! The client: `hypercube` in the user's terminal. It is the only process
//! that changes the terminal's settings or writes to it. It finds the
//! workspace's host, starting one if none is running, attaches to it, and
//! passes the terminal's input to the host and the host's frames to the
//! terminal until the host lets it go or the terminal goes away.

use std::error::Error;
use std::fs::{self, OpenOptions};
use std::io::{ErrorKind, Read, Write};
use std::os::fd::{AsRawFd, RawFd};
use std::os::unix::net::UnixStream;
use std::os::unix::process::CommandExt;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

use crate::link::{Decoder, Message, Size, encode};
use crate::mux::{self, Winsize, now_ms, read, write_all};
use crate::runtime::{poll, pollfd};
use crate::{control, host, store, winch, workspace};

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// How long to wait for the terminal's replies to its queries.
const HOST_REPLY_TIMEOUT_MS: i64 = 1000;
/// How long a new host has to start listening.
const SPAWN_TIMEOUT: Duration = Duration::from_secs(5);
/// How many lines of a host's log an error shows.
const LOG_LINES: usize = 10;

/// What the client works with: the user's terminal, which a `Raw` has
/// taken over, and the executable a new host runs.
pub struct Tty {
    pub input: RawFd,
    pub output: RawFd,
    /// Readable after each SIGWINCH; see `winch`.
    pub winch: RawFd,
    /// The terminal's replies to `mux::host_queries`.
    pub replies: Vec<u8>,
    pub exe: PathBuf,
}

impl Tty {
    pub fn size(&self) -> Size {
        let mut ws = Winsize {
            ws_row: 0,
            ws_col: 0,
            ws_xpixel: 0,
            ws_ypixel: 0,
        };
        unsafe { libc::ioctl(self.output, libc::TIOCGWINSZ, &mut ws) };
        ws.into()
    }

    /// Attach to the host of the workspace whose checkout is `root`,
    /// starting one if none is running.
    pub fn connect(&self, root: &Path) -> Result<UnixStream> {
        let state_path = store::path(&workspace::name(root));
        let socket = control::socket_path(&state_path)?;
        let log = host::log_path(&state_path);
        let end = Instant::now() + SPAWN_TIMEOUT;
        let mut spawned = None;
        loop {
            match control::attach(&socket, self.size(), &self.replies) {
                Ok(stream) => return Ok(stream),
                Err(e) if control::is_unreachable(&*e) => {}
                Err(e) => return Err(e),
            }
            if spawned.is_none() && !host::running(&state_path)? {
                spawned = Some(spawn_host(&self.exe, root, &log)?);
            }
            if let Some(child) = &mut spawned
                && child.try_wait()?.is_some()
            {
                return Err(host_failed("the host exited", &log));
            }
            if Instant::now() > end {
                return Err(host_failed("the host did not start listening", &log));
            }
            std::thread::sleep(Duration::from_millis(50));
        }
    }
}

/// Attach this terminal to the workspace whose checkout is `root` until the
/// host lets it go or the terminal goes away, then give the terminal back
/// and print the host's reason, if it gave one.
pub fn run(root: &Path) -> Result<()> {
    if unsafe { libc::isatty(0) == 0 || libc::isatty(1) == 0 } {
        return Err("hypercube must run in a terminal".into());
    }
    let exe = std::env::current_exe().map_err(|e| format!("finding hypercube: {e}"))?;
    let raw = Raw::take(0, 1)?;
    let tty = Tty {
        input: 0,
        output: 1,
        winch: winch::install()?,
        replies: raw.replies.clone(),
        exe,
    };
    let stream = tty.connect(root)?;
    let reason = proxy(&tty, stream)?;
    drop(raw);
    if !reason.is_empty() {
        eprintln!("hypercube: {reason}");
    }
    Ok(())
}

/// Pass the terminal's input and size to the host at the other end of
/// `stream`, and its output to the terminal, until the host detaches the
/// client, the terminal's input ends, or the host goes away. A switch moves
/// to another workspace's host. Returns the reason to show the user.
pub fn proxy(tty: &Tty, mut stream: UnixStream) -> Result<String> {
    let mut decoder = Decoder::default();
    let mut size = tty.size();
    let mut buf = vec![0u8; 65536];
    loop {
        let mut fds = [
            pollfd(tty.input),
            pollfd(tty.winch),
            pollfd(stream.as_raw_fd()),
        ];
        if !poll(&mut fds, -1)? {
            continue;
        }
        if fds[1].revents != 0 {
            winch::take(tty.winch);
            let now = tty.size();
            if now != size {
                size = now;
                if send(&stream, &Message::Resize(size)).is_err() {
                    return Ok(gone());
                }
            }
        }
        if fds[0].revents != 0 {
            let n = read(tty.input, &mut buf);
            if n <= 0 {
                return Ok(String::new());
            }
            if send(&stream, &Message::Input(buf[..n as usize].to_vec())).is_err() {
                return Ok(gone());
            }
        }
        if fds[2].revents == 0 {
            continue;
        }
        let n = match (&stream).read(&mut buf) {
            Ok(0) => return Ok(gone()),
            Ok(n) => n,
            Err(e) if e.kind() == ErrorKind::Interrupted => continue,
            Err(_) => return Ok(gone()),
        };
        decoder.push(&buf[..n]);
        while let Some(message) = decoder.take()? {
            match message {
                Message::Output(data) => write_all(tty.output, &data),
                Message::Detached(reason) => return Ok(reason),
                Message::Switch(dir) => match tty.connect(&dir) {
                    Ok(next) => {
                        // What the old host sent after the switch is dropped.
                        stream = next;
                        decoder = Decoder::default();
                        let clusters = mux::host_clusters(&tty.replies);
                        write_all(tty.output, &mux::host_reset(Some(clusters)));
                        break;
                    }
                    Err(e) => {
                        let _ = send(&stream, &Message::SwitchFailed(e.to_string()));
                    }
                },
                _ => {}
            }
        }
    }
}

fn gone() -> String {
    "the host closed the connection".into()
}

fn send(stream: &UnixStream, message: &Message) -> std::io::Result<()> {
    let mut stream = stream;
    stream.write_all(&encode(message)?)
}

/// Run `hypercube host <root>` in its own session, with no input and its
/// output appended to `log`.
fn spawn_host(exe: &Path, root: &Path, log: &Path) -> Result<std::process::Child> {
    if let Some(dir) = log.parent() {
        fs::create_dir_all(dir).map_err(|e| format!("create {}: {e}", dir.display()))?;
    }
    let file = OpenOptions::new()
        .create(true)
        .append(true)
        .open(log)
        .map_err(|e| format!("open {}: {e}", log.display()))?;
    let mut command = Command::new(exe);
    command
        .arg("host")
        .arg(root)
        .stdin(Stdio::null())
        .stdout(file.try_clone()?)
        .stderr(file);
    unsafe {
        command.pre_exec(|| {
            if libc::setsid() < 0 {
                return Err(std::io::Error::last_os_error());
            }
            Ok(())
        });
    }
    command
        .spawn()
        .map_err(|e| format!("starting the host: {e}").into())
}

/// An error about a host that failed to start, with the end of its log.
fn host_failed(what: &str, log: &Path) -> Box<dyn Error> {
    let text = fs::read_to_string(log).unwrap_or_default();
    let lines: Vec<&str> = text.lines().collect();
    let tail = lines[lines.len().saturating_sub(LOG_LINES)..].join("\n");
    format!("{what}; the end of {}:\n{tail}", log.display()).into()
}

/// The user's terminal in raw mode on the alternate screen. Dropping it
/// gives the terminal back as it was.
struct Raw {
    input: RawFd,
    output: RawFd,
    termios: libc::termios,
    /// The terminal's replies to `mux::host_queries`.
    replies: Vec<u8>,
}

impl Raw {
    fn take(input: RawFd, output: RawFd) -> Result<Raw> {
        let mut termios = std::mem::MaybeUninit::<libc::termios>::uninit();
        if unsafe { libc::tcgetattr(input, termios.as_mut_ptr()) } != 0 {
            return Err(format!("terminal: {}", std::io::Error::last_os_error()).into());
        }
        let termios = unsafe { termios.assume_init() };
        let mut raw = termios;
        unsafe { libc::cfmakeraw(&mut raw) };
        if unsafe { libc::tcsetattr(input, libc::TCSAFLUSH, &raw) } != 0 {
            return Err(format!("terminal: {}", std::io::Error::last_os_error()).into());
        }
        // From here on, dropping it restores the terminal.
        let mut raw = Raw {
            input,
            output,
            termios,
            replies: Vec::new(),
        };
        write_all(output, b"\x1b[?1049h\x1b[2J");
        raw.replies = query(input, output)?;
        Ok(raw)
    }
}

impl Drop for Raw {
    fn drop(&mut self) {
        let clusters = mux::host_clusters(&self.replies);
        write_all(self.output, &mux::host_restore(Some(clusters)));
        unsafe { libc::tcsetattr(self.input, libc::TCSAFLUSH, &self.termios) };
    }
}

/// Ask the terminal for its colors and grapheme clustering mode, and return
/// its replies; see `mux::use_host_replies_for`.
fn query(input: RawFd, output: RawFd) -> Result<Vec<u8>> {
    write_all(output, &mux::host_queries());
    let mut buf = vec![0u8; 4096];
    let mut len = 0;
    let deadline = now_ms() + HOST_REPLY_TIMEOUT_MS;
    while len < buf.len() && !mux::host_replies_done(&buf[..len]) {
        let left = deadline - now_ms();
        if left <= 0 {
            break;
        }
        // Not ready means interrupted or timed out; the deadline tells
        // which.
        if !poll(&mut [pollfd(input)], left)? {
            continue;
        }
        let n = read(input, &mut buf[len..]);
        if n <= 0 {
            break;
        }
        len += n as usize;
    }
    buf.truncate(len);
    Ok(buf)
}
