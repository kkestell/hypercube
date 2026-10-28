//! The control socket. The host listens on a Unix socket next to the state
//! file, and the MCP server and the CLI send it requests. Each connection
//! carries one JSON request line and gets one JSON reply line. After an
//! accepted `attach`, the connection carries the attach stream (see `link`).

use std::error::Error;
use std::fmt;
use std::io::{BufRead, BufReader, ErrorKind, Read, Write};
use std::os::unix::ffi::OsStrExt;
use std::os::unix::net::UnixStream;
use std::path::{Path, PathBuf};
use std::time::Duration;

use serde::{Deserialize, Serialize};

use crate::link::Size;
use crate::store;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// The prompt a worker's harness starts with, so that its first turn picks up
/// its messages.
pub const NUDGE: &str = "Check your Hypercube messages.";

/// The longest socket path macOS accepts: `sun_path` is 104 bytes with the
/// terminating NUL.
const MAX_SOCKET_PATH: usize = 103;

/// The longest request or reply line.
const MAX_LINE: u64 = 1 << 20;

const HUNG_UP: &str = "the host closed the connection without a reply";

/// How long a client waits for the host's reply.
const REPLY_TIMEOUT: Duration = Duration::from_secs(30);

/// A request to the host. Agents are named by ID.
#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(tag = "request", rename_all = "snake_case")]
pub enum Request {
    /// Build the agent's command from the state, spawn it in a new pane, and
    /// lay the panes out again. A non-empty `prompt` starts its first turn.
    Launch { agent: String, prompt: String },
    /// Hang up the agent's process, close its pane, and lay the panes out
    /// again.
    Close { agent: String },
    /// End the session.
    Stop,
    /// Attach the client's terminal: `size`, whose replies to
    /// `mux::host_queries` are `replies`, speaking attach stream `protocol`.
    Attach {
        protocol: u32,
        size: Size,
        replies: String,
    },
}

/// The host's answer to a request.
#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
pub struct Reply {
    pub ok: bool,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub error: String,
}

impl Reply {
    pub fn ok() -> Reply {
        Reply {
            ok: true,
            error: String::new(),
        }
    }

    pub fn error(message: impl ToString) -> Reply {
        Reply {
            ok: false,
            error: message.to_string(),
        }
    }

    /// The reply's error, if it is one.
    pub fn into_result(self) -> Result<()> {
        if self.ok {
            return Ok(());
        }
        Err(if self.error.is_empty() {
            "the host refused the request".into()
        } else {
            self.error.into()
        })
    }
}

/// The control socket of the workspace whose state file is `state_path`. It
/// sits in the state directory and is named from a hash of the workspace
/// name, which keeps the path inside the limit macOS puts on socket paths.
pub fn socket_path(state_path: &Path) -> Result<PathBuf> {
    let name = store::workspace(state_path);
    let dir = state_path.parent().unwrap_or(Path::new(""));
    let path = dir.join(format!("{:08x}.sock", fnv1a(name.as_bytes())));
    let len = path.as_os_str().as_bytes().len();
    if len > MAX_SOCKET_PATH {
        return Err(format!(
            "socket path {} is {len} bytes, over the limit of {MAX_SOCKET_PATH}; \
             set XDG_STATE_HOME to a shorter directory",
            path.display()
        )
        .into());
    }
    Ok(path)
}

fn fnv1a(bytes: &[u8]) -> u32 {
    bytes.iter().fold(0x811c_9dc5, |hash, &b| {
        (hash ^ u32::from(b)).wrapping_mul(0x0100_0193)
    })
}

/// The error of a request that found no host: the socket is missing or
/// nothing listens on it. No host means no panes.
#[derive(Debug)]
pub struct Unreachable {
    socket: PathBuf,
    source: std::io::Error,
}

impl fmt::Display for Unreachable {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(
            f,
            "connect {}: {}; is hypercube running for this workspace?",
            self.socket.display(),
            self.source
        )
    }
}

impl Error for Unreachable {}

/// Whether `error` came from a request that found no host.
pub fn is_unreachable(error: &(dyn Error + 'static)) -> bool {
    error.is::<Unreachable>()
}

/// Send `request` to the host listening on `socket` and wait for its reply.
/// An `Err` means the host could not be reached; a refusal is a reply with
/// `ok` unset.
pub fn request(socket: &Path, request: &Request) -> Result<Reply> {
    let stream = connect(socket)?;
    let exchange = || -> Result<Option<Reply>> {
        stream.set_read_timeout(Some(REPLY_TIMEOUT))?;
        stream.set_write_timeout(Some(REPLY_TIMEOUT))?;
        write_line(&stream, request)?;
        read_line(&stream)
    };
    match exchange() {
        Ok(Some(reply)) => Ok(reply),
        Ok(None) => Err(HUNG_UP.into()),
        // A host that closes the connection early surfaces as one of these,
        // depending on when the client notices. macOS reports a socket whose
        // peer has closed as EINVAL on setsockopt.
        Err(e)
            if e.downcast_ref::<std::io::Error>().is_some_and(|e| {
                matches!(
                    e.kind(),
                    ErrorKind::BrokenPipe
                        | ErrorKind::NotConnected
                        | ErrorKind::ConnectionReset
                        | ErrorKind::InvalidInput
                )
            }) =>
        {
            Err(HUNG_UP.into())
        }
        Err(e) => Err(e),
    }
}

fn connect(socket: &Path) -> Result<UnixStream> {
    UnixStream::connect(socket).map_err(|source| -> Box<dyn Error> {
        match source.kind() {
            ErrorKind::NotFound | ErrorKind::ConnectionRefused => Box::new(Unreachable {
                socket: socket.to_path_buf(),
                source,
            }),
            _ => format!("connect {}: {source}", socket.display()).into(),
        }
    })
}

/// Attach a terminal of `size`, whose replies to `mux::host_queries` are
/// `replies`, to the host listening on `socket`. Returns the connection,
/// which carries the attach stream from here on. An `Err` that
/// `is_unreachable` means no host listens.
pub fn attach(socket: &Path, size: Size, replies: &[u8]) -> Result<UnixStream> {
    let stream = connect(socket)?;
    stream.set_read_timeout(Some(REPLY_TIMEOUT))?;
    stream.set_write_timeout(Some(REPLY_TIMEOUT))?;
    let request = Request::Attach {
        protocol: crate::link::PROTOCOL,
        size,
        replies: String::from_utf8_lossy(replies).into_owned(),
    };
    write_line(&stream, &request)?;
    // The attach stream follows the reply, so the reply is read a byte at a
    // time to leave the stream unread.
    let mut line = Vec::new();
    let mut byte = [0u8];
    while line.last() != Some(&b'\n') {
        match (&stream).read(&mut byte) {
            Ok(0) => return Err(HUNG_UP.into()),
            Ok(_) if line.len() as u64 >= MAX_LINE => {
                return Err("the host's reply is too long".into());
            }
            Ok(_) => line.push(byte[0]),
            Err(e) if e.kind() == ErrorKind::Interrupted => {}
            Err(e) => return Err(e.into()),
        }
    }
    let reply: Reply = read_line(line.as_slice())?.ok_or(HUNG_UP)?;
    reply.into_result()?;
    stream.set_read_timeout(None)?;
    stream.set_write_timeout(None)?;
    Ok(stream)
}

/// Read the request line from a connection the host accepted. The stream must
/// block.
pub fn read_request(stream: impl Read) -> Result<Request> {
    read_line(stream)?.ok_or_else(|| "empty request".into())
}

/// Write the reply line to a connection the host accepted.
pub fn write_reply(stream: impl Write, reply: &Reply) -> Result<()> {
    write_line(stream, reply)
}

fn write_line(mut stream: impl Write, value: &impl Serialize) -> Result<()> {
    let mut line = serde_json::to_string(value)?;
    line.push('\n');
    stream.write_all(line.as_bytes())?;
    stream.flush()?;
    Ok(())
}

/// The JSON value on the stream's first line, or `None` if the stream ends
/// first.
fn read_line<T: for<'de> Deserialize<'de>>(stream: impl Read) -> Result<Option<T>> {
    let mut line = String::new();
    BufReader::new(stream.take(MAX_LINE)).read_line(&mut line)?;
    if line.trim().is_empty() {
        return Ok(None);
    }
    Ok(Some(serde_json::from_str(&line).map_err(|e| {
        format!("invalid message {:?}: {e}", line.trim())
    })?))
}

/// A stand-in host for tests of the code that sends requests.
#[cfg(test)]
pub mod testing {
    use super::*;
    use std::os::unix::net::UnixListener;
    use std::sync::{Arc, Mutex};

    /// Listen on `socket` and answer each request with `reply`. The returned
    /// list collects the requests received. The listener thread ends with the
    /// process.
    pub fn serve(
        socket: &Path,
        reply: impl Fn(&Request) -> Reply + Send + 'static,
    ) -> Arc<Mutex<Vec<Request>>> {
        serve_lossy(socket, move |r| Some(reply(r)))
    }

    /// Like `serve`, but a request answered with `None` is dropped without a
    /// reply.
    pub fn serve_lossy(
        socket: &Path,
        reply: impl Fn(&Request) -> Option<Reply> + Send + 'static,
    ) -> Arc<Mutex<Vec<Request>>> {
        let listener = UnixListener::bind(socket).unwrap();
        let seen = Arc::new(Mutex::new(Vec::new()));
        let log = Arc::clone(&seen);
        std::thread::spawn(move || {
            for stream in listener.incoming() {
                let mut stream = stream.unwrap();
                let request = read_request(&mut stream).unwrap();
                let answer = reply(&request);
                log.lock().unwrap().push(request);
                if let Some(answer) = answer {
                    write_reply(&mut stream, &answer).unwrap();
                }
            }
        });
        seen
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testutil::TempDir;

    fn launch() -> Request {
        Request::Launch {
            agent: "a1".into(),
            prompt: NUDGE.into(),
        }
    }

    #[test]
    fn requests_have_a_stable_wire_form() {
        let wire = |r: &Request| serde_json::to_string(r).unwrap();
        assert_eq!(
            wire(&launch()),
            r#"{"request":"launch","agent":"a1","prompt":"Check your Hypercube messages."}"#
        );
        assert_eq!(
            wire(&Request::Close { agent: "a1".into() }),
            r#"{"request":"close","agent":"a1"}"#
        );
        assert_eq!(wire(&Request::Stop), r#"{"request":"stop"}"#);
        let attach = Request::Attach {
            protocol: 1,
            size: Size {
                cols: 80,
                rows: 24,
                xpixel: 640,
                ypixel: 384,
            },
            replies: "\x1b[?62c".into(),
        };
        assert_eq!(
            wire(&attach),
            r#"{"request":"attach","protocol":1,"size":{"cols":80,"rows":24,"xpixel":640,"ypixel":384},"replies":"\u001b[?62c"}"#
        );
        assert_eq!(
            serde_json::to_string(&Reply::ok()).unwrap(),
            r#"{"ok":true}"#
        );
        assert_eq!(
            serde_json::to_string(&Reply::error("no room")).unwrap(),
            r#"{"ok":false,"error":"no room"}"#
        );
    }

    #[test]
    fn requests_round_trip_through_the_socket() {
        let dir = TempDir::new();
        let socket = socket_path(&dir.path().join("w.json")).unwrap();
        let seen = testing::serve(&socket, |r| match r {
            Request::Close { .. } => Reply::error("pane is gone"),
            _ => Reply::ok(),
        });
        let requests = [
            launch(),
            Request::Close { agent: "a1".into() },
            Request::Stop,
        ];
        for r in &requests {
            let reply = request(&socket, r).unwrap();
            assert_eq!(reply.ok, !matches!(r, Request::Close { .. }), "{r:?}");
        }
        assert_eq!(*seen.lock().unwrap(), requests);
    }

    #[test]
    fn a_refusal_carries_its_message() {
        let dir = TempDir::new();
        let socket = socket_path(&dir.path().join("w.json")).unwrap();
        testing::serve(&socket, |_| Reply::error("no room for another pane"));
        let reply = request(&socket, &launch()).unwrap();
        assert_eq!(
            reply.into_result().unwrap_err().to_string(),
            "no room for another pane"
        );
        assert!(Reply::ok().into_result().is_ok());
        let silent = Reply {
            ok: false,
            error: String::new(),
        };
        assert!(silent.into_result().is_err());
    }

    #[test]
    fn an_unreachable_host_is_an_error() {
        let dir = TempDir::new();
        let socket = dir.path().join("missing.sock");
        let err = request(&socket, &Request::Stop).unwrap_err().to_string();
        assert!(
            err.contains("missing.sock") && err.contains("running"),
            "{err}"
        );
    }

    #[test]
    fn a_socket_nobody_listens_on_is_unreachable() {
        let dir = TempDir::new();
        let socket = dir.path().join("w.sock");
        let err = request(&socket, &Request::Stop).unwrap_err();
        assert!(is_unreachable(&*err));
        assert!(!is_unreachable(&*Box::<dyn Error>::from("other")));

        // The file stays behind when its listener goes away. A child that
        // another test spawns at the moment the listener is created inherits
        // its descriptor and keeps it open for a while, so retry until the
        // connection is refused.
        drop(std::os::unix::net::UnixListener::bind(&socket).unwrap());
        let mut last = String::new();
        for _ in 0..200 {
            let err = request(&socket, &Request::Stop).unwrap_err();
            if is_unreachable(&*err) {
                return;
            }
            last = err.to_string();
            std::thread::sleep(Duration::from_millis(10));
        }
        panic!("the connection was never refused: {last}");
    }

    #[test]
    fn a_host_that_hangs_up_is_an_error() {
        let dir = TempDir::new();
        let socket = dir.path().join("w.sock");
        let listener = std::os::unix::net::UnixListener::bind(&socket).unwrap();
        // Hang up after the request arrives, then before it is read.
        let host = std::thread::spawn(move || {
            let (mut stream, _) = listener.accept().unwrap();
            read_request(&mut stream).unwrap();
            drop(stream);
            drop(listener.accept().unwrap());
        });
        for _ in 0..2 {
            let err = request(&socket, &Request::Stop).unwrap_err();
            assert!(!is_unreachable(&*err));
            assert_eq!(err.to_string(), HUNG_UP);
        }
        host.join().unwrap();
    }

    #[test]
    fn read_request_rejects_bad_input() {
        let read = |s: &str| read_request(s.as_bytes()).map_err(|e| e.to_string());
        assert_eq!(read("{\"request\":\"stop\"}\n").unwrap(), Request::Stop);
        assert_eq!(read("{\"request\":\"stop\"}").unwrap(), Request::Stop);
        assert!(read("").unwrap_err().contains("empty"));
        assert!(read("not json\n").unwrap_err().contains("invalid"));
        assert!(
            read("{\"request\":\"explode\"}\n")
                .unwrap_err()
                .contains("invalid")
        );
        assert!(
            read("{\"request\":\"close\"}\n")
                .unwrap_err()
                .contains("invalid")
        );
    }

    #[test]
    fn the_socket_sits_next_to_the_state_file() {
        let state = Path::new("/state/hypercube/repo-1a2b3c.json");
        let socket = socket_path(state).unwrap();
        assert_eq!(socket.parent().unwrap(), Path::new("/state/hypercube"));
        assert_eq!(socket.extension().unwrap(), "sock");
        assert_eq!(socket, socket_path(state).unwrap());
        let other = socket_path(Path::new("/state/hypercube/repo-4d5e6f.json")).unwrap();
        assert_ne!(socket, other);
        // The name of the file is the hash, not the workspace.
        assert!(!socket.to_string_lossy().contains("repo"));
    }

    #[test]
    fn a_socket_path_over_the_limit_is_an_error() {
        // Around the directory's name, the path has "/", "/", 8 hex digits, and ".sock".
        let fits = format!("/{}/w.json", "d".repeat(MAX_SOCKET_PATH - 15));
        assert_eq!(
            socket_path(Path::new(&fits)).unwrap().as_os_str().len(),
            MAX_SOCKET_PATH
        );
        let long = format!("/{}/w.json", "d".repeat(MAX_SOCKET_PATH - 14));
        let err = socket_path(Path::new(&long)).unwrap_err().to_string();
        assert!(err.contains("104") || err.contains("limit"), "{err}");
        assert!(
            err.contains(&format!("{} bytes", MAX_SOCKET_PATH + 1)),
            "{err}"
        );
    }
}
