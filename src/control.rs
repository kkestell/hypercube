//! The control socket. The host listens on a Unix socket next to the state
//! file, and the MCP server and the CLI send it requests. Each connection
//! carries one JSON request line and gets one JSON reply line.

use std::error::Error;
use std::io::{BufRead, BufReader, Read, Write};
use std::os::unix::ffi::OsStrExt;
use std::os::unix::net::UnixStream;
use std::path::{Path, PathBuf};
use std::time::Duration;

use serde::{Deserialize, Serialize};

use crate::store;

type Result<T> = std::result::Result<T, Box<dyn Error>>;

/// Typed into an idle agent's pane to make it run a turn.
pub const NUDGE: &str = "Check your Hypercube messages.";

/// The longest socket path macOS accepts: `sun_path` is 104 bytes with the
/// terminating NUL.
const MAX_SOCKET_PATH: usize = 103;

/// The longest request or reply line.
const MAX_LINE: u64 = 1 << 20;

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
    /// Type the nudge into the agent's pane and press Enter.
    Nudge { agent: String },
    /// End the session.
    Stop,
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

/// Send `request` to the host listening on `socket` and wait for its reply.
/// An `Err` means the host could not be reached; a refusal is a reply with
/// `ok` unset.
pub fn request(socket: &Path, request: &Request) -> Result<Reply> {
    let mut stream = UnixStream::connect(socket).map_err(|e| {
        format!(
            "connect {}: {e}; is hypercube running for this workspace?",
            socket.display()
        )
    })?;
    stream.set_read_timeout(Some(REPLY_TIMEOUT))?;
    stream.set_write_timeout(Some(REPLY_TIMEOUT))?;
    write_line(&mut stream, request)?;
    read_line(&mut stream)?.ok_or_else(|| "the host closed the connection without a reply".into())
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
        assert_eq!(
            wire(&Request::Nudge { agent: "a1".into() }),
            r#"{"request":"nudge","agent":"a1"}"#
        );
        assert_eq!(wire(&Request::Stop), r#"{"request":"stop"}"#);
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
            Request::Nudge { .. } => Reply::error("pane is gone"),
            _ => Reply::ok(),
        });
        let requests = [
            launch(),
            Request::Close { agent: "a1".into() },
            Request::Nudge { agent: "a1".into() },
            Request::Stop,
        ];
        for r in &requests {
            let reply = request(&socket, r).unwrap();
            assert_eq!(reply.ok, !matches!(r, Request::Nudge { .. }), "{r:?}");
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
    fn a_host_that_hangs_up_is_an_error() {
        let dir = TempDir::new();
        let socket = dir.path().join("w.sock");
        let listener = std::os::unix::net::UnixListener::bind(&socket).unwrap();
        std::thread::spawn(move || drop(listener.accept()));
        let err = request(&socket, &Request::Stop).unwrap_err().to_string();
        assert!(err.contains("without a reply"), "{err}");
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
