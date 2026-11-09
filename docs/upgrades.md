---
title: Upgrade Hypercube
weight: 15
---

# Upgrade Hypercube

Use this guide for an app installed from a beta download. For a source
installation, use the
[contributor instructions](https://github.com/kkestell/hypercube/blob/main/CONTRIBUTING.md#install-your-checkout).

## Before upgrading

1. Read the release notes for any special restart instructions.
2. Download and verify the new release using the
   [installation instructions](installation.md).

Normal upgrades preserve running agents and terminal shells. This release uses
the current saved-state format directly; conversion from historical database and
configuration formats has been retired. Follow the release notes for any future
saved-state changes. Replacing the app with an older version does not convert
saved state back to that version's format.

## Replace the app

**macOS**

1. Quit the desktop app.
2. Replace `~/Applications/Hypercube.app` with the new app.
3. In Terminal, reload the background service from the new app:

   ```sh
   APP="$HOME/Applications/Hypercube.app" SERVICE=com.github.kkestell.hypercube PORT=4280 \
     "$HOME/Applications/Hypercube.app/Contents/Resources/install-service"
   ```

4. Open the app.

**Linux**

1. Quit the desktop app and extract the new tarball.
2. Run the extracted bundle's `Hypercube/resources/install`.
3. Reload the background service from the installed app:

   ```sh
   APP="$HOME/.local/share/hypercube" SERVICE=com.github.kkestell.hypercube PORT=4280 \
     "$HOME/.local/share/hypercube/resources/install-service"
   ```

4. Open Hypercube from the application launcher.

## Load new plugins into existing sessions

An agent that survived the upgrade still uses the plugin it started with.
Choose **Restart Agent** in that session's context menu to load the installed
plugin and resume its conversation. A turn in progress is interrupted.

## Check the running version

Use **Hypercube → About Hypercube** on macOS or **Help → About Hypercube** on
Linux.

`hypercube version` identifies the executable on your PATH, which may differ
from the running backend before a service reload. See
[build identities](storage.md#build-identities) for the version format.
