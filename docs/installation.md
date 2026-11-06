---
title: Installation
weight: 1
---

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

Extract the ZIP and move `Hypercube.app` to `~/Applications` before opening it,
so the background service names a stable location. Create
[your configuration](configuration.md), then open the app. The first time, its
window says it is waiting for the Hypercube service. Choose **Set Up Background
Service**: it installs the login service that runs the app's backend at login
through your interactive login shell, so agents receive your login environment,
and the window then shows Hypercube. Choose **Show Hypercube** from the menu bar
icon if the window is closed.

The backend and the desktop app are separate. The backend starts at login and
keeps agents running; the desktop app, which shows the window, menu bar icon,
notifications, and agents' browsers, starts when you open it. Hypercube does not
add itself to Login Items; add it in **System Settings → General → Login Items**
to have the desktop app open at login too. Quitting the desktop app leaves the
backend and its agents running.

Beta apps are ad hoc signed and **not notarized by Apple**. If macOS blocks the
first launch, open **System Settings → Privacy & Security → Open Anyway**, then
confirm. See [Apple's instructions](https://support.apple.com/102445). You do
not need to disable Gatekeeper globally.

The button runs the app's installer, which you can also run by hand:

```sh
APP="$HOME/Applications/Hypercube.app" SERVICE=com.github.kkestell.hypercube PORT=4280 \
  "$HOME/Applications/Hypercube.app/Contents/Resources/install-service"
```

For an upgrade, quit the desktop app, replace the app, run that command again,
and open the app. Agents and terminal shells keep running; the plugin restart
rules below still apply. Use the full executable path for CLI commands, for
example:

```sh
~/Applications/Hypercube.app/Contents/MacOS/hypercube version
```

## Build and install from a checkout

Source builds need Git, the Go version in
[go.mod](https://github.com/kkestell/hypercube/blob/main/go.mod), and Bun.
Packaging downloads the Electron runtime the first time. The Go build embeds
the browser view, so run `make ui` before `go build`, `go vet`, or `go test`.

From this checkout:

```sh
make install
```

This installs `~/Applications/Hypercube.app` and links `hypercube` and its
plugins into `~/.local/bin`. Put that directory on your PATH. Use
`APPDIR=<dir>` and `BINDIR=<dir>` to change the destinations.

Create [your configuration](configuration.md), then follow
[Run at login](#run-at-login). For development and release packaging, see
[the development reference](https://github.com/kkestell/hypercube/blob/main/agents/development.md).

## Run at login

```sh
make install-service
```

This installs the build, loads a launchd user agent that runs the backend at
login through your interactive login shell, and opens the desktop app. The
service gives agents your login environment.

Closing the window leaves Hypercube running, and **Quit Hypercube** quits only
the desktop app. To start the backend again if it is stopped, run
`launchctl kickstart gui/$(id -u)/com.github.kkestell.hypercube`.

To end every agent and terminal shell, send the service SIGINT:

```sh
launchctl kill SIGINT gui/$(id -u)/com.github.kkestell.hypercube
```

Saved conversations remain; [resume behavior](usage.md#resume-and-cleanup)
explains when work starts again. To remove the service, run:

```sh
launchctl bootout gui/$(id -u)/com.github.kkestell.hypercube
rm ~/Library/LaunchAgents/com.github.kkestell.hypercube.plist
```

To run without a login service, start `hypercube ui` in a terminal, then open
`~/Applications/Hypercube.app`. The interface is also available at
http://localhost:4280/. The desktop app provides agents' browser windows and
macOS notifications; its first launch asks for notification permission.

To use another port, pass it to both processes:

```sh
hypercube ui 4282
~/Applications/Hypercube.app/Contents/MacOS/Hypercube\ Desktop --port=4282
```

## Upgrade

Run `make install-service` again. It quits the desktop app if it is open,
replaces the installed build, restarts the backend, and opens the app, while
agents and terminal shells keep running.

Those agents still use the plugins they launched with. Choose **Restart Agent**
from a session's context menu to use a new plugin build. A holder-directory or plugin-protocol change requires
ending the agents with SIGINT before installation.

State migrates on startup. Upgrading from the earlier leads/workers version
also restarts agents and panel shells once.

To preserve a way back to the previous build, take a
[backup](#backup-and-recovery) before installing.

## Backup and recovery

This procedure restores a backup on the same machine at the original paths.
Record the [build identity](#identify-a-build), harness versions, and any XDG
or harness storage overrides before starting.

1. Quit the desktop app, then end the backend and its agents: run the SIGINT
   command above for the service, or use **Ctrl-C** (SIGINT) for a foreground
   `hypercube ui`, and wait for it to finish. Quitting the desktop app,
   closing the window, or stopping the service with SIGTERM leaves agents
   running. Stop other processes writing to the
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
     Codex storage. Keep harness settings and credentials available at their
     original paths.
3. To recover, quit the app and other writers again. Preserve any newer work
   separately, then replace the saved directories with the backup at their
   original paths. Restore the matching previous app and plugins together
   with its saved state. Start it with the same storage overrides, inspect
   the conversations and repository changes, and send a prompt to verify
   native conversation resume.

## Identify a build

**Hypercube → About Hypercube** identifies the running app. `hypercube version` identifies
the executable on your PATH; these may differ before you reload the service.
The running server also reports its identity at http://localhost:4280/version
and writes it to the service log at startup.

Build identities contain the release tag or commit, followed by `+dirty` for
uncommitted checkout changes. A build without Git metadata reports `devel`.

## Files on disk

| File | Default location |
| --- | --- |
| Configuration | `~/.config/hypercube/config.json` |
| Saved-state directory | `~/.local/state/hypercube/` |
| Database | `~/.local/state/hypercube/hypercube.db` |
| Service log | `~/.local/state/hypercube/ui.log` |
| Desktop app profile: website logins and window position | `~/.local/state/hypercube/chromium/` |
| Login service | `~/Library/LaunchAgents/com.github.kkestell.hypercube.plist` |

Configuration follows `XDG_CONFIG_HOME`; saved state follows `XDG_STATE_HOME`.
