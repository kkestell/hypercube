# Hypercube

Hypercube runs a team of local coding agents in one terminal. Each agent is a
native harness TUI, Claude Code or Pi, running in its own pty and pane. Hypercube
draws the panes itself through Ghostty's libghostty-vt. Agents message each other
through an MCP tool and receive messages through harness hooks. One
orchestrator works in the original checkout and manages workers. A worker either
has its own Git worktree or works directly in an existing directory.

Kyle is the only user. Favor small implementations that are easy to understand
and change over generalized infrastructure or speculative requirements.

## Principles

- The harnesses own their UI, transcripts, sessions, permissions, usage, and
  native delegation. Hypercube launches them, draws their panes, and moves
  messages between them.
- Hypercube supplies identities, worktrees or worker directories, messages, and
  lifecycle tools. Agents decide how to divide work, collaborate, integrate, and
  assess completion. There is no task registry, workflow, or reporting protocol.
- Configure harnesses only through per-launch flags, environment, and
  configuration overrides. The user's global and project settings, credentials,
  and `HOME` stay untouched.
- macOS only. Agents and Hypercube are trusted local processes running with the
  user's credentials.
- The `hypercube` process in the user's terminal hosts the session: the panes,
  their ptys and terminals, and a control socket. Every other Hypercube process
  is short-lived or owned by a harness, and they share one state file per
  workspace.

## Documentation

- `docs/architecture.md`: processes, the host, the control socket, the state
  file, the terminal layer, and message delivery.
- `docs/configuration.md`: `config.json`, orchestrator and worker profiles.
- `docs/installation.md`: prerequisites, building, and installing.
- `docs/harness-support.md`: how each harness is launched and hooked.
- `agents/plans/2026-10-27-002-rust-port.md`: the implementation plan.

The new code and docs stand alone. They never mention `v1/`, the Go version,
tmux, granite, or any other predecessor. `v1/` is reference material only.

## Layout

- `src/main.rs`: the `hypercube` executable and its commands.
- `src/lib.rs`: the library's modules. The terminal layer is:
  - `src/mux.rs`: panes and their Ghostty terminals, caller layouts and
    chrome, host input routing, mode mirroring, and the renderer.
  - `src/pty.rs`: programs started in ptys.
  - `src/runtime.rs`: the host terminal and the step-driven event loop.
  - `src/winch.rs`: SIGWINCH delivered through a pipe.
  - `src/recording.rs`: the recording format the round-trip tests replay.
- `src/store.rs`: the workspace state file: agents, queued messages, and
  locked, atomic updates.
- `src/config.rs`: loading and validating `config.json`.
- `src/workspace.rs`: the Git checkout, worker branches and worktrees, and
  worker directories.
- `src/harness.rs`: the launch command of each harness and the pane labels.
- `src/harness/pi.js`: the Pi extension, embedded in the binary and written for
  each Pi launch.
- `src/hook.rs`: the harness hook events and the messages they deliver.
- `src/control.rs`: the control socket's request and reply protocol, its
  client, and the socket's path.
- `src/mcp.rs`: the agents' stdio MCP server and its tools.
- `src/host.rs`: the session host: the host lock, the control socket, the
  panes, layout and chrome, launches, closes, nudges, and teardown.
- `src/testutil.rs`: helpers for unit tests.
- `src/bin/hypercube-test-child.rs`: the scripted program the live tests run in
  panes.
- `src/bin/hypercube-test-harness.rs`: the `claude` and `pi` stand-in the
  integration tests run in panes. It runs the harness hooks, serves the MCP
  tools on command, and records what it sees.
- `tests/roundtrip.rs`: feeds program output to the mux, renders frames into a
  stand-in host terminal, and compares the screens cell by cell. Inputs are the
  fixtures in `tests/support/fixtures.rs`, seeded streams from
  `tests/support/fuzz.rs` (`HYPERCUBE_FUZZ_ITERATIONS` and
  `HYPERCUBE_FUZZ_SEED`, 300 from seed 1 by default), and every `.rec` file in
  `tests/recordings`. Fixtures marked `known` cover differences the mux does not
  resolve and pass while the difference remains.
- `tests/support/screen.rs`: independent render-state snapshots and cell-level
  diagnostics for the round-trip tests.
- `tests/holds.rs`: what a pane draws while its program holds rendering
  (mode 2026), and that a timed-out hold ends.
- `tests/live.rs`: the runtime with real ptys. Each test hosts its panes on a
  pty whose other side feeds a Ghostty terminal standing in for the user's
  terminal, and runs the test child in the panes.
- `tests/integration.rs`: `hypercube` in a pty with isolated state and
  configuration, a temporary repository, and the harness stand-ins first on
  `PATH`: panes, worktrees, nudges, `stop`, `status`, restart, and `delete`.
- `vendor/ghostty`: Ghostty source at the commit the libghostty-rs revision
  pins. `.cargo/config.toml` points the libghostty-vt build at it.
- `.toolchain/zig-aarch64-macos-0.17.0`: Zig 0.17, which builds libghostty-vt.
  `make` downloads it, and `.worktreeinclude` copies it into new worktrees.

## Build

```sh
make
```

`make` downloads the Zig toolchain into `.toolchain` and checks its SHA-256 if
it is missing, runs `git submodule update --init` for `vendor/ghostty` if it is
missing, and runs `cargo build`. Once both are in place, `cargo` works
directly.

## Validation

Before pushing anything other than a Markdown-only change, all of this must
pass:

```sh
cargo build && cargo clippy --all-targets -- -D warnings && cargo fmt --check && cargo test
```

## Smoke testing

For harness smoke tests, use Claude Code `haiku` and Pi
`openrouter/~deepseek/deepseek-flash-latest`. Use disposable local
repositories and isolated `XDG_STATE_HOME` and `XDG_CONFIG_HOME`.

When a task is done, stop and destroy every demo or test session it started,
and delete its state file, lock files, socket, repositories, and temporary
directories. `hypercube status` must not list any of them afterward.
