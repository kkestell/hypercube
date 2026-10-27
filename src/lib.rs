//! Hypercube's terminal layer. `mux` holds the per-pane Ghostty terminals and
//! the renderer that draws them into the host terminal, and runs without ptys
//! for the round-trip tests; `winch` delivers SIGWINCH to the event loop;
//! `recording` is the format the round-trip tests replay.

pub mod mux;
pub mod recording;
pub mod winch;
