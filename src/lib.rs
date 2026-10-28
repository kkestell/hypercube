//! Hypercube's terminal layer. `mux` holds the per-pane Ghostty terminals
//! and the renderer that draws them into the host terminal, and runs without
//! ptys for the round-trip tests; `pty` starts programs in ptys; `runtime`
//! is the event loop that connects them to the host terminal; `winch`
//! delivers SIGWINCH to the event loop; `recording` is the format the
//! round-trip tests replay.
//!
//! The rest of the crate holds the agents' side: `store` is the workspace
//! state file, `config` the user's profiles, `workspace` the Git checkout and
//! worktrees, `harness` the launch command of each harness, `hook` the
//! harness hook events, `wait` the waiter that wakes an idle agent when
//! messages arrive, `control` the host's control socket protocol, `host` the
//! session in the user's terminal, and `mcp` the agents' MCP server.

pub mod config;
pub mod control;
pub mod harness;
pub mod hook;
pub mod host;
pub mod mcp;
pub mod mux;
pub mod pty;
pub mod recording;
pub mod runtime;
pub mod statusline;
pub mod store;
pub mod wait;
pub mod winch;
pub mod workspace;

#[cfg(test)]
mod testutil;
