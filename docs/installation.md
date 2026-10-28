# Installation

Hypercube is a Rust crate built with Cargo. It runs on macOS.

## Prerequisites

- macOS on Apple silicon. The build uses an arm64 Zig toolchain.
- Rust 1.90 or later with Cargo. The crate uses the Rust 2024 edition.
- Git, and the Xcode Command Line Tools, which provide `make` and the linker.
- Network access for the first build. `make` downloads Zig and checks its
  SHA-256. The Ghostty submodule comes from GitHub, and Cargo fetches
  `libghostty-rs` from GitHub.
- The harnesses selected in `config.json` on `PATH`: `claude` for Claude
  Code, `opencode` for OpenCode 2.x. Each harness uses its own configured
  credentials. See
  [Harness support](harness-support.md) for launch details.

## Build

From a checkout of the repository:

```sh
make
```

`make` downloads Zig 0.17.0 into `.toolchain/zig-aarch64-macos-0.17.0` when
its executable is missing and verifies the archive's SHA-256. It runs
`git submodule update --init vendor/ghostty` when
`vendor/ghostty/build.zig` is missing, then runs `cargo build`. The binary is
`target/debug/hypercube`.

`.cargo/config.toml` points the libghostty-vt build at that Zig executable
and the vendored Ghostty source, with `ReleaseSafe` optimization. Ghostty's
submodule commit matches the revision pinned by the `libghostty-vt`
dependency in `Cargo.toml`. Once Zig and Ghostty are in place, Cargo commands
work directly.

`make test` runs `cargo test`. `make check` runs the full validation sequence:

```sh
cargo build
cargo clippy --all-targets -- -D warnings
cargo fmt --check
cargo test
```

## Install

```sh
make install
```

This prepares Zig and Ghostty, runs `cargo build --release --bin hypercube`,
and installs the executable as `~/.local/bin/hypercube` with mode `755`.
`make install BINDIR=<dir>` selects another destination. The destination
directory belongs on `PATH` for running `hypercube` by name.

Each agent runs the `hypercube` executable that started its session, as its MCP
server and hook command. A running host keeps the executable it started with
until it is stopped. `make reinstall` puts every running workspace on a new
build: it builds the release executable, stops every running workspace, installs
the executable, and starts the stopped workspaces again in the background, in
the bar's order, with the installed executable. Each workspace's agents launch
when a terminal attaches to it, so run `hypercube` again afterwards. Run it
outside Hypercube: it refuses inside an agent's pane, whose workspace it would
stop. `BINDIR=<dir>` selects the destination as for `make install`.

## Configuration

Hypercube reads `config.json` when a session starts, so the file must exist
before the first `hypercube` run. It is `~/.config/hypercube/config.json`, or
`$XDG_CONFIG_HOME/hypercube/config.json` when `XDG_CONFIG_HOME` is non-empty.
Its format is described in [Configuration](configuration.md).
