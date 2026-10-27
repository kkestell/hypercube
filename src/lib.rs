//! Hypercube's terminal layer. `mux` holds the per-pane Ghostty terminals and
//! the renderer that draws them into the host terminal, and runs without ptys
//! for the round-trip tests; `winch` delivers SIGWINCH to the event loop;
//! `recording` is the format the round-trip tests replay.
//!
//! The rest of the crate holds the agents' side: `store` is the workspace
//! state file, `config` the user's profiles, `workspace` the Git checkout and
//! worktrees, `harness` the launch command of each harness, and `hook` the
//! harness hook events.

pub mod config;
pub mod harness;
pub mod hook;
pub mod mux;
pub mod recording;
pub mod store;
pub mod winch;
pub mod workspace;

#[cfg(test)]
mod testutil;
