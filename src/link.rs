//! The attach stream between a client in the user's terminal and a host.
//! After the attach request, the connection carries `Message` values, each a
//! kind byte, a 4-byte big-endian payload length, and the payload. The host
//! reads and writes through a non-blocking `Link`; the client uses `encode`
//! and `Decoder` over a blocking stream.

use std::collections::VecDeque;
use std::io::{self, ErrorKind, Read, Write};
use std::os::fd::{AsRawFd, RawFd};
use std::os::unix::ffi::{OsStrExt, OsStringExt};
use std::os::unix::net::UnixStream;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};

use crate::mux::{Winsize, now_ms};

/// The version of the attach stream. A host refuses a client of another.
pub const PROTOCOL: u32 = 1;

/// The longest payload.
pub const MAX_PAYLOAD: usize = 1 << 20;

/// The size of the user's terminal.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct Size {
    pub cols: u16,
    pub rows: u16,
    pub xpixel: u16,
    pub ypixel: u16,
}

impl From<Winsize> for Size {
    fn from(ws: Winsize) -> Size {
        Size {
            cols: ws.ws_col,
            rows: ws.ws_row,
            xpixel: ws.ws_xpixel,
            ypixel: ws.ws_ypixel,
        }
    }
}

impl From<Size> for Winsize {
    fn from(s: Size) -> Winsize {
        Winsize {
            ws_col: s.cols,
            ws_row: s.rows,
            ws_xpixel: s.xpixel,
            ws_ypixel: s.ypixel,
        }
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Message {
    /// Client to host: what the user typed.
    Input(Vec<u8>),
    /// Client to host: the terminal's new size.
    Resize(Size),
    /// Client to host: switching to another workspace failed.
    SwitchFailed(String),
    /// Host to client: bytes for the terminal.
    Output(Vec<u8>),
    /// Host to client: attach to the workspace whose checkout is this
    /// directory instead.
    Switch(PathBuf),
    /// Host to client: the host let the client go, for this reason.
    Detached(String),
}

const INPUT: u8 = 1;
const RESIZE: u8 = 2;
const SWITCH_FAILED: u8 = 3;
const OUTPUT: u8 = 4;
const SWITCH: u8 = 5;
const DETACHED: u8 = 6;

/// The message in wire form. A payload over `MAX_PAYLOAD` is an error.
pub fn encode(message: &Message) -> io::Result<Vec<u8>> {
    let resize;
    let (kind, payload): (u8, &[u8]) = match message {
        Message::Input(data) => (INPUT, data),
        Message::Resize(s) => {
            resize = [s.cols, s.rows, s.xpixel, s.ypixel].map(u16::to_be_bytes);
            (RESIZE, resize.as_flattened())
        }
        Message::SwitchFailed(text) => (SWITCH_FAILED, text.as_bytes()),
        Message::Output(data) => (OUTPUT, data),
        Message::Switch(dir) => (SWITCH, dir.as_os_str().as_bytes()),
        Message::Detached(text) => (DETACHED, text.as_bytes()),
    };
    if payload.len() > MAX_PAYLOAD {
        return Err(invalid(format!(
            "a message of {} bytes is over the limit of {MAX_PAYLOAD}",
            payload.len()
        )));
    }
    let mut out = Vec::with_capacity(5 + payload.len());
    out.push(kind);
    out.extend_from_slice(&(payload.len() as u32).to_be_bytes());
    out.extend_from_slice(payload);
    Ok(out)
}

fn invalid(message: String) -> io::Error {
    io::Error::new(ErrorKind::InvalidData, message)
}

/// Collects bytes from the stream and yields whole messages.
#[derive(Default)]
pub struct Decoder {
    buf: Vec<u8>,
}

impl Decoder {
    pub fn push(&mut self, data: &[u8]) {
        self.buf.extend_from_slice(data);
    }

    /// The next whole message, or `None` until more bytes arrive.
    pub fn take(&mut self) -> io::Result<Option<Message>> {
        let Some(header) = self.buf.get(..5) else {
            return Ok(None);
        };
        let kind = header[0];
        let len = u32::from_be_bytes(header[1..5].try_into().unwrap()) as usize;
        if len > MAX_PAYLOAD {
            return Err(invalid(format!(
                "a message of {len} bytes is over the limit of {MAX_PAYLOAD}"
            )));
        }
        if self.buf.len() < 5 + len {
            return Ok(None);
        }
        let payload: Vec<u8> = self.buf.drain(..5 + len).skip(5).collect();
        let text = |p: Vec<u8>| String::from_utf8_lossy(&p).into_owned();
        Ok(Some(match kind {
            INPUT => Message::Input(payload),
            RESIZE => {
                if payload.len() != 8 {
                    return Err(invalid("a resize message is not 8 bytes".into()));
                }
                let n = |i: usize| u16::from_be_bytes([payload[i], payload[i + 1]]);
                Message::Resize(Size {
                    cols: n(0),
                    rows: n(2),
                    xpixel: n(4),
                    ypixel: n(6),
                })
            }
            SWITCH_FAILED => Message::SwitchFailed(text(payload)),
            OUTPUT => Message::Output(payload),
            SWITCH => Message::Switch(PathBuf::from(std::ffi::OsString::from_vec(payload))),
            DETACHED => Message::Detached(text(payload)),
            kind => return Err(invalid(format!("unknown message kind {kind}"))),
        }))
    }
}

/// Output in pieces of at most `MAX_PAYLOAD` bytes, each ending just before
/// an Esc where there is one, so every piece holds whole escape sequences:
/// the mux writes Esc only to start one, never inside one or in cell text.
pub fn split_output(data: &[u8]) -> impl Iterator<Item = &[u8]> {
    let mut rest = data;
    std::iter::from_fn(move || {
        if rest.is_empty() {
            return None;
        }
        let mut end = rest.len().min(MAX_PAYLOAD);
        if end < rest.len()
            && let Some(esc) = rest[1..=end].iter().rposition(|&b| b == 0x1b)
        {
            end = esc + 1;
        }
        let (piece, tail) = rest.split_at(end);
        rest = tail;
        Some(piece)
    })
}

/// The host's end of an attach stream: a non-blocking socket with a queue of
/// outbound bytes. Nothing on it blocks.
pub struct Link {
    stream: UnixStream,
    decoder: Decoder,
    queue: VecDeque<u8>,
    /// When the queue last shrank, or became non-empty.
    last_progress: i64,
}

impl Link {
    pub fn new(stream: UnixStream) -> io::Result<Link> {
        stream.set_nonblocking(true)?;
        Ok(Link {
            stream,
            decoder: Decoder::default(),
            queue: VecDeque::new(),
            last_progress: now_ms(),
        })
    }

    pub fn fd(&self) -> RawFd {
        self.stream.as_raw_fd()
    }

    /// Queue a message. Output over `MAX_PAYLOAD` goes as several messages;
    /// see `split_output`.
    pub fn send(&mut self, message: Message) {
        if self.queue.is_empty() {
            self.last_progress = now_ms();
        }
        let messages = match message {
            Message::Output(data) if data.len() > MAX_PAYLOAD => split_output(&data)
                .map(|c| Message::Output(c.to_vec()))
                .collect(),
            message => vec![message],
        };
        for m in &messages {
            // Every other message is far under the limit.
            if let Ok(bytes) = encode(m) {
                self.queue.extend(bytes);
            }
        }
    }

    /// Write as much of the queue as the socket takes. Returns whether any
    /// of it was written.
    pub fn flush(&mut self) -> io::Result<bool> {
        let mut progress = false;
        while !self.queue.is_empty() {
            let (front, _) = self.queue.as_slices();
            match (&self.stream).write(front) {
                Ok(n) => {
                    self.queue.drain(..n);
                    progress = true;
                }
                Err(e) if e.kind() == ErrorKind::WouldBlock => break,
                Err(e) if e.kind() == ErrorKind::Interrupted => {}
                Err(e) => return Err(e),
            }
        }
        if progress {
            self.last_progress = now_ms();
        }
        Ok(progress)
    }

    /// Whether queued bytes are waiting for the socket.
    pub fn pending(&self) -> bool {
        !self.queue.is_empty()
    }

    /// When the queue last made progress, in `now_ms` time.
    pub fn last_progress(&self) -> i64 {
        self.last_progress
    }

    /// The whole messages that have arrived. Empty when nothing has; an
    /// error at the end of the stream.
    pub fn read(&mut self) -> io::Result<Vec<Message>> {
        let mut buf = [0u8; 65536];
        loop {
            match (&self.stream).read(&mut buf) {
                Ok(0) => return Err(ErrorKind::UnexpectedEof.into()),
                Ok(n) => self.decoder.push(&buf[..n]),
                Err(e) if e.kind() == ErrorKind::WouldBlock => break,
                Err(e) if e.kind() == ErrorKind::Interrupted => {}
                Err(e) => return Err(e),
            }
        }
        let mut messages = Vec::new();
        while let Some(m) = self.decoder.take()? {
            messages.push(m);
        }
        Ok(messages)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn messages() -> Vec<Message> {
        vec![
            Message::Input(b"\x1b[A".to_vec()),
            Message::Resize(Size {
                cols: 120,
                rows: 40,
                xpixel: 960,
                ypixel: 640,
            }),
            Message::SwitchFailed("no such workspace".into()),
            Message::Output(Vec::new()),
            Message::Output(vec![0xff; 300]),
            Message::Switch(PathBuf::from("/src/bar")),
            Message::Detached("attached in another terminal".into()),
        ]
    }

    #[test]
    fn messages_round_trip_through_any_split() {
        let wire: Vec<u8> = messages().iter().flat_map(|m| encode(m).unwrap()).collect();
        for at in 0..=wire.len() {
            let mut decoder = Decoder::default();
            let mut got = Vec::new();
            for part in [&wire[..at], &wire[at..]] {
                decoder.push(part);
                while let Some(m) = decoder.take().unwrap() {
                    got.push(m);
                }
            }
            assert_eq!(got, messages(), "split at {at}");
        }
        let mut decoder = Decoder::default();
        let mut got = Vec::new();
        for &b in &wire {
            decoder.push(&[b]);
            while let Some(m) = decoder.take().unwrap() {
                got.push(m);
            }
        }
        assert_eq!(got, messages());
    }

    #[test]
    fn a_message_over_the_limit_is_an_error() {
        assert!(encode(&Message::Input(vec![0; MAX_PAYLOAD])).is_ok());
        assert!(encode(&Message::Input(vec![0; MAX_PAYLOAD + 1])).is_err());
        let mut decoder = Decoder::default();
        decoder.push(&[INPUT]);
        decoder.push(&(MAX_PAYLOAD as u32 + 1).to_be_bytes());
        assert!(decoder.take().is_err());
        let mut decoder = Decoder::default();
        decoder.push(&[99, 0, 0, 0, 0]);
        assert!(decoder.take().is_err());
    }

    #[test]
    fn large_frames_split_between_escape_sequences() {
        use crate::mux::{Mux, NoChrome, Rect, Winsize};
        // A truecolor frame of a large terminal, with a color change at
        // every cell.
        let ws = Winsize {
            ws_col: 500,
            ws_row: 150,
            ws_xpixel: 0,
            ws_ypixel: 0,
        };
        let mut mux = Mux::new(ws, Box::new(NoChrome));
        mux.connect_host(&[], ws).unwrap();
        let id = mux.add_pane("p").unwrap();
        mux.set_layout(&[(id, Rect::full(ws))]).unwrap();
        let mut text = String::new();
        for i in 0..500 * 149 {
            text.push_str(&format!("\x1b[38;2;{};{};200mx", i % 251, i % 241));
        }
        mux.pane_mut(id).unwrap().feed(text.as_bytes());
        mux.render().unwrap();
        let frame = mux.take_output();
        assert!(frame.len() > MAX_PAYLOAD);

        let pieces: Vec<&[u8]> = split_output(&frame).collect();
        assert!(pieces.len() >= 2);
        assert_eq!(pieces.concat(), frame);
        for (i, piece) in pieces.iter().enumerate() {
            assert!(piece.len() <= MAX_PAYLOAD);
            if i > 0 {
                assert_eq!(piece[0], 0x1b, "piece {i}");
            }
        }
        // Without an Esc, pieces are as long as they may be.
        let plain = vec![b'x'; MAX_PAYLOAD + 1];
        let lens: Vec<usize> = split_output(&plain).map(<[u8]>::len).collect();
        assert_eq!(lens, [MAX_PAYLOAD, 1]);
    }

    #[test]
    fn a_link_queues_what_the_socket_does_not_take() {
        let (host, client) = UnixStream::pair().unwrap();
        let mut link = Link::new(host).unwrap();
        assert!(!link.pending());
        assert!(link.read().unwrap().is_empty());
        // Far more than the socket buffers, in messages under the limit.
        let big = vec![b'x'; 3 * MAX_PAYLOAD];
        link.send(Message::Output(big.clone()));
        link.send(Message::Detached("bye".into()));
        assert!(link.flush().unwrap());
        assert!(link.pending());
        assert!(!link.flush().unwrap());

        let reader = std::thread::spawn(move || {
            let mut decoder = Decoder::default();
            let mut got = Vec::new();
            let mut buf = [0u8; 65536];
            loop {
                let n = (&client).read(&mut buf).unwrap();
                decoder.push(&buf[..n]);
                while let Some(m) = decoder.take().unwrap() {
                    match m {
                        Message::Output(data) => got.extend_from_slice(&data),
                        Message::Detached(_) => return (got, client),
                        m => panic!("{m:?}"),
                    }
                }
            }
        });
        while link.pending() {
            link.flush().unwrap();
            std::thread::sleep(std::time::Duration::from_millis(1));
        }
        let (got, client) = reader.join().unwrap();
        assert!(got == big);

        (&client)
            .write_all(&encode(&Message::Input(b"hi".to_vec())).unwrap())
            .unwrap();
        let read = loop {
            let read = link.read().unwrap();
            if !read.is_empty() {
                break read;
            }
        };
        assert_eq!(read, [Message::Input(b"hi".to_vec())]);
        // A child that another test forks at the moment may hold the
        // client's end until it execs, so the end of the stream can come
        // late.
        drop(client);
        let end = now_ms() + 2000;
        while link.read().is_ok() {
            assert!(now_ms() < end, "the stream never ended");
            std::thread::sleep(std::time::Duration::from_millis(5));
        }
    }
}
