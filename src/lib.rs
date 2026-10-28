//! Hypercube's terminal layer. `mux` holds the per-pane Ghostty terminals
//! and the renderer that draws them for the host terminal, and runs without
//! ptys for the round-trip tests; `pty` starts programs in ptys; `runtime`
//! is the event loop that connects them to a client over a `link`; `client`
//! owns the user's terminal and proxies it to a host; `winch` delivers
//! SIGWINCH to the client; `recording` is the format the round-trip tests
//! replay.
//!
//! The rest of the crate holds the agents' side: `store` is the workspace
//! state file, `config` the user's profiles, `workspace` the Git checkout and
//! worktrees, `harness` the launch command of each harness, `hook` the
//! harness hook events, `wait` the waiter that wakes an idle agent when
//! messages arrive, `control` the host's control socket protocol, `host` the
//! background session of a workspace, `bar` its bar of running workspaces,
//! `overlay` its workspace list, `theme` the colors and spacing of its
//! chrome, and `mcp` the agents' MCP server.

pub mod bar;
pub mod client;
pub mod config;
pub mod control;
pub mod harness;
pub mod hook;
pub mod host;
pub mod link;
pub mod mcp;
pub mod mux;
pub mod overlay;
pub mod pty;
pub mod recording;
pub mod runtime;
pub mod statusline;
pub mod store;
pub mod theme;
pub mod wait;
pub mod winch;
pub mod workspace;

#[cfg(test)]
mod testutil;
