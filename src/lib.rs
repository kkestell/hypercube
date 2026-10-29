//! Hypercube's terminal layer. `mux` holds the per-pane Ghostty terminals
//! and the renderer that draws them for the host terminal, and runs without
//! ptys for the round-trip tests; `pty` starts programs in ptys; `runtime`
//! is the event loop that connects them to a client over a `link`; `client`
//! owns the user's terminal and proxies it to a host; `signals` delivers
//! signals through pipes; `recording` is the format the round-trip tests
//! replay.
//!
//! The rest of the crate holds the agents' side: `store` is the workspace
//! state file, `config` the user's profiles, `workspace` the Git checkout and
//! worktrees, `harness` the launch command of each harness, `hook` the
//! harness hook events, `wait` the waiter that wakes an idle agent when
//! messages arrive, `control` the host's control socket protocol, `schedule`
//! the scheduled tasks' schedules and firing, `host` the background session
//! of a workspace, `bar` its bar of running workspaces,
//! `overlay` its workspace list, `teamlist`
//! its list of teams, `colors` the colors of its chrome, `icons`
//! the Nerd Font icons it draws, `splash` the client's screen while no
//! workspace is running, `tesseract` the animation on it, and `mcp` the
//! agents' MCP server.

pub mod bar;
pub mod client;
pub mod colors;
pub mod config;
pub mod control;
pub mod harness;
pub mod help;
pub mod hook;
pub mod host;
pub mod icons;
pub mod link;
pub mod mcp;
pub mod mux;
pub mod overlay;
pub mod pty;
pub mod recording;
pub mod runtime;
pub mod schedule;
pub mod signals;
pub mod splash;
pub mod statusline;
pub mod store;
pub mod teamlist;
pub mod tesseract;
pub mod wait;
pub mod workspace;

#[cfg(test)]
mod testutil;
