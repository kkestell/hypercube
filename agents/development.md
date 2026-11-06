# Development and releases

Start with [AGENTS.md](../AGENTS.md) for development rules and validation.
Source builds need Git, the Go version in [go.mod](../go.mod), and Bun.
Packaging downloads Electron on first use. User installation, service management,
and recovery are in [the installation guide](../docs/installation.md).

## Develop without replacing the installed app

```sh
make dev
```

Open http://localhost:4281/. This uses separate configuration and state, with a
scratch repository of sample files under `~/projects/hypercube-dev/repo`. Use
`make dev-app` when testing desktop app behavior; it also runs a development
desktop app connected to the development backend.

Source changes rebuild the development app and reload the interface. A failed
build leaves the previous build running. Successful rebuilds preserve agents
and terminal shells.

- **Ctrl-C** or `make dev-stop` ends the development backend, desktop app, and
  their processes.
- `make dev-clean` also removes disposable resources and native development
  conversations, including holders left by a crash.
- `HYPERCUBE_DEV_DIR` and `HYPERCUBE_DEV_PORT` change the directory and port.

See [scripts/dev](../scripts/dev) for cleanup details.

For a build alone, run `make` and use `bin/hypercube` or `bin/Hypercube.app`.
Follow [the validation rules](../AGENTS.md#validation) for checks, smoke tests,
and installing changes. `make install-service` builds the checkout, including
uncommitted changes, without requiring a release tag or running the full checks.

## Documentation screenshots

```sh
make screenshots
```

This regenerates `docs/images/<scene>-light.png` and `<scene>-dark.png` for
each scene in `docs/screenshots/`. A scene file lists workspaces, each a copy
of a fixture repository in `docs/screenshots/repos/` with an optional patch of
uncommitted changes, and their sessions: each turn's prompt, the transcript
items in the plugin protocol's JSON, how the turn ends (`idle`, `working`, or
`ask`), and the children it starts. A session's `view` is the page's saved
tabs and splits, `select` picks the shown session, and `size` is the window
in points. [scripts/screenshots/main.js](../scripts/screenshots/main.js) plays
each scene through a real backend with
[the scripted plugin](../plugins/scripted/main.go), opens the real desktop
window, and captures it. Everything else it creates is deleted.

The window must fit on a Retina display. On first use macOS asks to grant the
terminal the Screen Recording permission.

## Prepare an official beta

GitHub Actions runs `make check` on main-branch pushes and pull requests, using
the Go and Bun versions declared in the repository. Beta tags run those same
checks before packaging an ad hoc signed Apple Silicon app.

Once the intended changes are committed and checked, create and push an
annotated tag such as `v0.1.0-beta.1`. The **Beta release** workflow builds the
ZIP and `SHA256SUMS`, then creates a **draft prerelease** with generated notes.
Inspect the notes, test the downloaded app, and publish the draft when ready.
No Apple account or signing secrets are required. GitHub Actions must be enabled
and permitted to create releases in the repository.

To check and package locally on Apple Silicon:

```sh
make check
make release
```

Only release packaging requires a clean checkout at a beta tag. Its outputs
are under `bin/release/`; it does not install the app or restart the service.

Go embeds the release tag or commit-based version, adding `+dirty` for
uncommitted changes. Builds without Git metadata report `devel`. The bundle's
`CFBundleGetInfoString` records the full identity; its numeric version fields
contain the version's three-number core. See [build identification](../docs/installation.md#identify-a-build)
for the user-visible checks.
