//! Hypercube's terminal layer. `mux` holds the per-pane Ghostty terminals
//! and the renderer that draws them into the host terminal, and runs without
//! ptys for the round-trip tests; `pty` starts programs in ptys; `runtime`
//! is the event loop that connects them to the host terminal; `winch`
//! delivers SIGWINCH to the event loop; `recording` is the format the
//! round-trip tests replay.

pub mod mux;
pub mod pty;
pub mod recording;
pub mod runtime;
pub mod winch;
