# Contributing to Hypercube

This page is for people building or changing Hypercube. To use the released
app, follow [Install Hypercube](docs/installation.md) instead.

## Run a development copy

You need Git, the Go version declared in [go.mod](go.mod), and Bun.
Desktop packaging downloads Electron on first use. On Linux, packaging also
needs `rsvg-convert` and the
[desktop runtime libraries](docs/installation.md#install-on-linux).

1. Clone the repository and enter it:

   ```sh
   git clone https://github.com/kkestell/hypercube.git
   cd hypercube
   ```

2. Start the development copy. On macOS, open the desktop app:

   ```sh
   make dev-app
   ```

   On Linux, use `make dev` and open `http://localhost:4281/` in a browser.
   The `dev-app` script uses the macOS bundle layout; for Linux desktop
   validation, use the [Linux test environment](docs/dev/development.md#test-on-linux).

3. The development copy has separate configuration and state, using a sample
   repository at `~/projects/hypercube-dev/repo`. It does not replace your
   installed app.
4. Change a source file. Successful rebuilds reload the interface while
   preserving agents and terminal shells. A failed build leaves the last
   successful build running.
5. Press **Ctrl-C**, or run `make dev-stop`, to stop the development backend
   and its processes.

Reuse an existing development process for this checkout rather than starting
a second one. Internal development controls are in the
[development reference](docs/dev/development.md#develop-without-replacing-the-installed-app).

## Choose the right instructions

| Goal | Read |
| --- | --- |
| Find the relevant package or follow project conventions | [Development reference](docs/dev/development.md) |
| Understand backend, plugins, and process ownership | [Architecture](docs/dev/architecture.md) |
| Change guides or screenshots | [Documentation maintenance](docs/dev/documentation.md) |
| Test packaging and desktop behavior on Linux | [Linux testing](docs/dev/development.md#test-on-linux) |
| Package an official beta | [Release procedure](docs/dev/development.md#prepare-an-official-beta) |
| Work as a coding agent | [AGENTS.md](AGENTS.md) |

## Validate a change

- **Code:** run `make check`. This builds the interface, checks
  types and formatting, and runs the test suites, including Go's race detector.
- **Documentation only:** inspect the diff, links, and described behavior.
  Do not run code checks for prose changes.
- **Raw Go commands:** run `make ui` before `go build`, `go vet`, or `go test`;
  Go embeds the generated browser interface.

For focused checks and platform-specific requirements, use the
[validation reference](docs/dev/development.md#validation).

## Install your checkout

Only do this when you want to replace your installed app with this checkout,
including its uncommitted changes:

```sh
make install-service
```

This builds and installs the app, reloads its login service, and opens the
desktop app. It does not require a release tag or run the full checks. Existing
agents and shells keep running, so use **Restart Agent** to load changed
plugins. Process restart requirements are in the
[validation reference](docs/dev/development.md#validation).

`make install` installs without setting up the service. On macOS the default
destination is `~/Applications/Hypercube.app`; on Linux it is
`~/.local/share/hypercube`. Both link CLI tools into `~/.local/bin`, which
must be on your PATH. `BINDIR` overrides the links' destination. On macOS,
`APPDIR` overrides the app's parent directory; on Linux, `XDG_DATA_HOME`
controls its data directory. On Linux, `make uninstall` removes the installation.

For a local build without installation, run `make build`: the CLI is
`bin/hypercube` and the bundle is `bin/Hypercube.app` on macOS or
`bin/Hypercube` on Linux.

Discuss bugs and proposed features in
[GitHub Issues](https://github.com/kkestell/hypercube/issues).
