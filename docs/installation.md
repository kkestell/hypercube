# Installation

Hypercube is a Rust crate built with Cargo. It runs on macOS.

## Prerequisites

- macOS on Apple silicon. The build uses an arm64 Zig toolchain.
- Rust 1.90 or later, installed with rustup.
- Git, and the Xcode Command Line Tools, which provide `make` and the linker.
- Network access for the first build. `make` downloads Zig and checks its
  SHA-256. The Ghostty submodule comes from GitHub, and Cargo fetches
  `libghostty-rs` from GitHub.
- `claude` (Claude Code) and `pi` on `PATH`. Hypercube starts each agent by its
  command name. See `docs/harness-support.md` for how each harness is launched.

## Build

From a checkout of the repository:

```sh
make
```

`make` fetches the Zig toolchain into `.toolchain` and the `vendor/ghostty`
submodule when they are missing, then runs `cargo build`. The binary is
`target/debug/hypercube`.

## Install

```sh
make install
```

This runs `cargo install --path .`, which builds `hypercube` in release mode
and copies it to `~/.cargo/bin`, or `$CARGO_HOME/bin` when `CARGO_HOME` is set.
Once `make` has fetched the toolchain and the submodule, running
`cargo install --path .` directly does the same.

Each agent runs the `hypercube` executable that started its session, as its MCP
server and hook command.

## Configuration

Hypercube reads `config.json` when a session starts, so the file must exist
before the first `hypercube` run. It is `~/.config/hypercube/config.json`, or
`$XDG_CONFIG_HOME/hypercube/config.json` when `XDG_CONFIG_HOME` is set. Its
format is described in `docs/configuration.md`.
