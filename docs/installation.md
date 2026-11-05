# Installation

Hypercube runs on macOS. Install at least one authenticated coding harness:
`claude`, `opencode`, `codex`, or `pi`. Check
[harness requirements](harness-support.md) before configuring profiles.

## Download a beta

Download the ZIP and `SHA256SUMS` from a
[beta release](https://github.com/kkestell/hypercube/releases). Initial downloads
are Apple Silicon builds, checked on macOS 15. In the download folder, verify:

```sh
shasum -a 256 -c SHA256SUMS
```

Extract the ZIP and move `Hypercube.app` to `~/Applications`. Create
[your configuration](configuration.md), then open the app. Choose **Show
Hypercube** from its menu bar icon if the window is closed. It launches through
your interactive login shell so agents receive your login environment.

Beta apps are ad hoc signed and **not notarized by Apple**. If macOS blocks the
first launch, open **System Settings → Privacy & Security → Open Anyway**, then
confirm. See [Apple's instructions](https://support.apple.com/102445). You do
not need to disable Gatekeeper globally.

To run a downloaded app at login without building from source:

```sh
APPDIR="$HOME/Applications" SERVICE=com.github.kkestell.hypercube PORT=4280 \
  "$HOME/Applications/Hypercube.app/Contents/Resources/install-service"
```

For an upgrade, replace the app and run that command again. Agents and terminal
shells keep running; the plugin restart rules below still apply. Use the full
executable path for CLI commands, for example:

```sh
~/Applications/Hypercube.app/Contents/MacOS/hypercube version
```

## Build and install from a checkout

Source builds need Git and the Go version in [go.mod](../go.mod).

From this checkout:

```sh
make install
```

This installs `~/Applications/Hypercube.app` and links `hypercube` and its
plugins into `~/.local/bin`. Put that directory on your PATH. Use
`APPDIR=<dir>` and `BINDIR=<dir>` to change the destinations.

Create [your configuration](configuration.md), then run:

```sh
hypercube ui --menu
```

This starts the menu bar icon and native window. Choose **Show Hypercube** from
the icon if the window is closed. The first launch asks for notification
permission.

The same interface is available at http://localhost:4280/. Omit `--menu` to run
without the native window, or pass a port to use another address:

```sh
hypercube ui 4282
```

The native app is required for agents' browser windows and macOS notifications.

## Run at login

```sh
make install-service
```

This installs the build and loads a launchd user agent. It runs through your
interactive login shell, giving agents your login environment.

Closing the window leaves Hypercube running. **Quit Hypercube** ends the app,
agents, and terminal shells until the next login. Saved conversations remain;
[resume behavior](usage.md#resume-and-cleanup) explains when work starts again.

To remove the service, choose **Quit Hypercube**, then run:

```sh
launchctl bootout gui/$(id -u)/com.github.kkestell.hypercube
rm ~/Library/LaunchAgents/com.github.kkestell.hypercube.plist
```

## Upgrade

Run `make install-service` again. It replaces the installed build and restarts
the window and menu bar icon, while agents and terminal shells keep running.
This remains the everyday development workflow: it builds your checkout,
including uncommitted changes, without requiring a release tag or running
the full checks.

Those agents still use the plugins they launched with. Restart their sessions
to use a new plugin build. A holder-directory or plugin-protocol change requires
quitting Hypercube before installation.

State migrates on startup. Upgrading from the earlier leads/workers version
also restarts agents and panel shells once.

To preserve a way back to the previous build, take a
[backup](#backup-and-recovery) before installing.

## Backup and recovery

This procedure restores a backup on the same machine at the original paths.
Record the [build identity](#identify-a-build), harness versions, and any XDG
or harness storage overrides before starting.

1. Choose **Quit Hypercube** and wait for it to finish. For a foreground
   browser-only server, use **Ctrl-C** (SIGINT). Closing the window or stopping
   the service leaves agents running. Stop other processes writing to the
   repositories or native harness storage you will copy.
2. Copy these together into a private backup outside the directories being
   copied, preserving dotfiles and symlinks:

   - The current app bundle, including its plugins, or the current executable
     and sibling plugins for an installation without a bundle.
   - The configuration and entire saved-state directory listed below. The
     state directory includes Codex conversations and plugin files alongside
     SQLite; copying only `hypercube.db` omits them.
   - Each workspace directory, including `.git`, managed worktrees, and
     modified, untracked, and ignored files. Include the main checkout when
     backing up a linked worktree.
   - Native conversation storage outside Hypercube for the harnesses you use.
     Claude Code, OpenCode, and Pi keep their own histories. See
     [conversation storage](harness-support.md#conversation-storage) for
     Codex's account links; their source settings and credentials must remain
     available at their original paths.
3. To recover, quit the app and other writers again. Preserve any newer work
   separately, then replace the saved directories with the backup at their
   original paths. Restore the matching previous app and plugins together
   with its saved state. Start it with the same storage overrides, inspect
   the conversations and repository changes, and send a prompt to verify
   native conversation resume.

Same-path backup, upgrade, and recovery have been exercised with Codex,
including committed work and modified and untracked files in the main
checkout and a managed worktree. Recovery of Claude Code, OpenCode, and Pi
native histories has not been exercised.

## Identify a build

**Settings → Build** identifies the running app. `hypercube version` identifies
the executable on your PATH; these may differ before you reload the service.
The running server also reports its identity at http://localhost:4280/version
and writes it to the service log at startup.

Go embeds the release tag or a version containing the commit, followed by
`+dirty` when the checkout has uncommitted changes. The full commit is included
when available. A build without Git metadata reports `devel`. The app bundle
also records the full identity in `CFBundleGetInfoString`; its numeric version
fields contain the version's three-number core.

## Files on disk

| File | Default location |
| --- | --- |
| Configuration | `~/.config/hypercube/config.json` |
| Saved-state directory | `~/.local/state/hypercube/` |
| Database | `~/.local/state/hypercube/hypercube.db` |
| Service log | `~/.local/state/hypercube/ui.log` |
| Login service | `~/Library/LaunchAgents/com.github.kkestell.hypercube.plist` |

Configuration follows `XDG_CONFIG_HOME`; saved state follows `XDG_STATE_HOME`.

## Develop without replacing the installed app

```sh
make dev
```

Open http://localhost:4281/. This uses separate configuration and state, with a
scratch repository under `~/projects/hypercube-dev/repo`. Its stopped preview
session has sample files and starts no model. Use `make dev-app` when testing
native-window behavior.

Source changes rebuild the development app and reload the interface. A failed
build leaves the previous build running. Successful rebuilds preserve agents
and terminal shells.

- **Ctrl-C** or `make dev-stop` ends the development app and its processes.
- `make dev-clean` also removes disposable resources and native development
  conversations, including holders left by a crash.
- `HYPERCUBE_DEV_DIR` and `HYPERCUBE_DEV_PORT` change the directory and port.

Sessions remain marked running for the next development launch. See
[scripts/dev](../scripts/dev) for cleanup details.

For a build alone, run `make` and use `bin/hypercube` or
`bin/Hypercube.app`. Code changes require `make check`, which also needs Bun.
Tests use local harness stand-ins; live model behavior needs a smoke test.

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
