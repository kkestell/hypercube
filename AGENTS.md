# Hypercube

Hypercube runs teams of local coding agents in one terminal. Each agent is a
native harness TUI, Claude Code or OpenCode, running in its own pty and pane.
Hypercube draws the panes itself through Ghostty's libghostty-vt. Agents
message each other through an MCP tool and receive messages through harness
hooks. A workspace holds one or more teams, each an orchestrator and the
workers it manages. Every team's orchestrator works in the original checkout,
and the screen shows one team at a time. A worker either has its own Git
worktree or works directly in an existing directory.

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
- Each workspace's session runs in a background `hypercube host` process: the
  panes, their ptys and terminals, and a control socket. `hypercube` in the
  user's terminal is the client: it attaches the terminal to a host, starting
  one if needed, and is the only process that writes to the terminal. A host
  outlives its client and ends only on `hypercube stop`, `hypercube delete`,
  the overlay's stop key, or a signal. Every other Hypercube process is
  short-lived or owned by a harness, and they share one state file per
  workspace.

## Documentation

- `docs/architecture.md`: processes, the host, the client and the attach
  stream, the control socket, the state file, the terminal layer, and message
  delivery.
- `docs/configuration.md`: `config.json`, orchestrator and worker profiles.
- `docs/installation.md`: prerequisites, building, and installing.
- `docs/harness-support.md`: how each harness is launched and hooked.
- `docs/models.md`: model and effort level recommendations for coding agents,
  from Artificial Analysis benchmark data.
- `docs/teamwork.md`: published practices for running teams of coding agents.
- `agents/plans/2026-10-27-002-rust-port.md`: the implementation plan.

Code and docs stand alone and do not refer to predecessor implementations.

## Layout

- `src/main.rs`: the `hypercube` executable and its commands.
- `src/lib.rs`: the library's module declarations. The terminal layer is:
  - `src/mux.rs`: panes and their Ghostty terminals, caller layouts and
    chrome, host input and mouse routing, scrolling, selection and copying,
    notification passthrough, mode mirroring, the melt of closed panes, and
    the renderer.
  - `src/pty.rs`: programs started in ptys.
  - `src/runtime.rs`: the step-driven event loop, which draws for the
    attached client over a link.
  - `src/link.rs`: the attach stream's messages, their encoding, and the
    host's non-blocking end of the stream.
  - `src/client.rs`: the client, which owns the user's terminal, starts and
    attaches to hosts, and proxies the terminal to them.
  - `src/signals.rs`: signals, such as SIGWINCH, delivered through pipes.
  - `src/recording.rs`: the recording format the round-trip tests replay.
- `src/store.rs`: the workspace state file: agents, queued messages,
  scheduled tasks, and locked, atomic updates.
- `src/config.rs`: loading and validating `config.json`.
- `src/workspace.rs`: the Git checkout, worker branches and worktrees, and
  worker directories.
- `src/harness.rs`: the launch command of each harness and the pane labels.
- `src/harness/opencode.js`: the OpenCode plugin, embedded in the binary and
  written for each OpenCode launch.
- `src/hook.rs`: the harness hook events and the messages they deliver.
- `src/schedule.rs`: the scheduled tasks' cron schedules, local minutes, and
  firing.
- `src/wait.rs`: `hypercube wait`, which wakes an idle agent when messages
  arrive for it.
- `src/statusline.rs`: `hypercube statusline`, Claude Code's status line.
- `src/control.rs`: the control socket's request and reply protocol, its
  client, and the socket's path.
- `src/mcp.rs`: the agents' stdio MCP server and its tools.
- `src/host.rs`: the session host, `hypercube host`: the host lock, the
  control socket, attaches, the panes, layout and chrome, the overlay's
  actions, the viewed team, creating and deleting teams, firing the
  scheduled tasks, launches, closes, and teardown.
- `src/bar.rs`: the bar of running workspaces in the last row, its prompt
  for a new workspace's path, and its drawing.
- `src/icons.rs`: the Nerd Font icons the chrome, the bar, and the overlay
  draw.
- `src/colors.rs`: the accent colors, and the palette of the header, bar,
  titles, and divider each shades from one.
- `src/splash.rs`: the splash the client draws while no workspace is
  running: the tesseract, the hint, and the prompt for a new workspace's path.
- `src/tesseract.rs`: the rotating tesseract the splash draws: its
  projection, and the braille canvas it is drawn on.
- `scratch/mockup.py`: prints a mockup of the screen, to try layouts and
  borders without running Hypercube.
- `src/overlay.rs`: the workspace overlay's list, keys, and drawing.
- `src/teamlist.rs`: the team list's rows, keys, prompt for a new team's
  name, and drawing.
- `src/help.rs`: the list of keys Ctrl-b ? shows.
- `src/testutil.rs`: helpers for unit tests.
- `src/bin/hypercube-test-child.rs`: the scripted program the live tests run in
  panes.
- `src/bin/hypercube-test-harness.rs`: the `claude` and `opencode` stand-in the
  integration tests run in panes. It runs the harness hooks and waiters,
  serves the MCP tools and asks for tool approvals on command, and records
  what it sees.
- `tests/roundtrip.rs`: feeds program output to the mux, renders frames into a
  stand-in host terminal, and compares the screens cell by cell. Inputs are the
  fixtures in `tests/support/fixtures.rs`, seeded streams from
  `tests/support/fuzz.rs` (`HYPERCUBE_FUZZ_ITERATIONS` and
  `HYPERCUBE_FUZZ_SEED`, 300 from seed 1 by default), and every `.rec` file in
  `tests/recordings`. Fixtures marked `known` track expected screen
  differences and pass while the difference remains.
- `tests/support/screen.rs`: independent render-state snapshots and cell-level
  diagnostics for the round-trip tests.
- `tests/holds.rs`: what a pane draws while its program holds rendering
  (mode 2026), and that a timed-out hold ends.
- `tests/live.rs`: the runtime with real ptys. Each test runs
  `client::proxy` on a pty whose other side feeds a Ghostty terminal standing
  in for the user's terminal, linked to the runtime by a socket pair, and runs
  the test child in the panes.
- `tests/integration.rs`: `hypercube` in a pty with isolated state and
  configuration, a temporary repository, and the harness stand-ins first on
  `PATH`: the splash, panes, worktrees, waking idle agents, agent relaunches,
  zoom, detaching and attaching again, the workspace overlay, teams and the
  team list, scheduled tasks, session restart, `stop`, `delete`, their `--all` variants, and
  `status`. The client starts
  each host in the background, and the fixture stops every host it started.
- `vendor/ghostty`: Ghostty source at the commit the libghostty-rs revision
  pins. `.cargo/config.toml` points the libghostty-vt build at it.
- `.toolchain/zig-aarch64-macos-0.17.0`: Zig 0.17, which builds libghostty-vt.
  `make` downloads it, and `.worktreeinclude` copies it into new worktrees.

## Build

```sh
make
```

`make` downloads the Zig toolchain into `.toolchain` and checks its SHA-256
when the Zig executable is missing, initializes `vendor/ghostty` when its
`build.zig` is missing, and runs `cargo build`. `.cargo/config.toml` selects
that Zig executable and Ghostty source and builds libghostty-vt in
`ReleaseSafe` mode. Once both are in place, `cargo` works directly.

`make test` runs `cargo test`. `make install` builds the release `hypercube`
binary and installs it to `~/.local/bin/hypercube`; `BINDIR=<dir>` selects
another destination. `make reinstall` (`scripts/reinstall`) also stops every
running workspace before installing and starts them again afterwards, on the
new binary.

## Development sessions

Iterate on Hypercube in an isolated dev session, never in the installed one:

```sh
make dev        # build, restart the dev session on the new binary, open it in Ghostty
make dev-stop   # stop it; its state stays, so make dev resumes the team
make dev-clean  # stop it and delete everything it left
```

`scripts/dev` runs the debug binary from this checkout with `XDG_STATE_HOME`
and `XDG_CONFIG_HOME` under `~/projects/hypercube-dev` (override with
`HYPERCUBE_DEV_DIR`), on a scratch repository there, and opens it with
`open -na Ghostty` so Kyle can watch and answer dialogs. The first `make dev`
writes a `config.json` there with Claude Code `haiku` and OpenCode
`openrouter/~deepseek/deepseek-flash-latest` profiles, and Kyle can edit it.
The installed hypercube's `status` and `stop --all` do not see this session.
Claude Code asks once to trust the scratch repository. `make dev-clean` removes
the dev agents' Claude Code transcripts and OpenCode sessions, but not that
trust entry in `~/.claude.json`.

When Kyle says the change is good and to commit it, run `make dev-clean`
before or with the commit, so no dev session outlives the work. When a change
is done and Kyle is happy with it, tell Kyle to run `make reinstall` outside
Hypercube and then start `hypercube` again. It stops every workspace, ending
every agent, so do not run it yourself, and a running host keeps the
executable it started with until then.

## Validation

Before pushing anything other than a Markdown-only change, all of this must
pass:

```sh
cargo build && cargo clippy --all-targets -- -D warnings && cargo fmt --check && cargo test
```

`make check` runs the same four commands.

## Smoke testing

For harness smoke tests, use Claude Code `haiku` and OpenCode
`openrouter/~deepseek/deepseek-flash-latest`. Use the dev session above, or
disposable local repositories and isolated `XDG_STATE_HOME` and
`XDG_CONFIG_HOME`.

When a task is done, stop every host it started with `hypercube stop`, and
delete its state file, lock files, socket, log file, repositories, and
temporary directories. `hypercube status` must not list any of them
afterward. `make dev-clean` does this for the dev session.
