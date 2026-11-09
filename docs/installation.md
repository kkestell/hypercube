# Install Hypercube

Install a beta release, then connect it to an authenticated coding tool.
You do not need Go, Bun, or a source checkout.

## Before you begin

- **macOS:** Apple Silicon; beta builds are checked on macOS 15.
- **Linux:** AMD64 or ARM64 on Debian 13, Ubuntu 26.04, Fedora 45, or Arch.
- Install and sign in to at least one [supported harness](harness-support.md#requirements):
  Claude Code, OpenCode, or Pi. A harness is the coding tool that runs
  an agent's work.

Get the app and `SHA256SUMS` from
[GitHub Releases](https://github.com/kkestell/hypercube/releases).

## Install on macOS

1. Download the macOS ZIP and `SHA256SUMS`. In your download folder, verify it:

   ```sh
   shasum -a 256 -c --ignore-missing SHA256SUMS
   ```

2. Extract the ZIP. Move `Hypercube.app` to `~/Applications` **before opening
   it**. The background service needs the app to stay at that location.
3. Open `Hypercube.app`. If macOS blocks it, use **System Settings → Privacy &
   Security → Open Anyway**, then confirm. Betas are signed with Hypercube's
   own certificate, not notarized by Apple; do not disable Gatekeeper globally.
   See [Apple's instructions](https://support.apple.com/102445).
4. In the waiting window, choose **Set Up Background Service**. When setup
   finishes, the workspace view replaces the waiting page.

No configuration file is required. You can change preferences later in
**Settings**.

If you close the window, choose **Show Hypercube** from the menu bar icon.
The backend starts at login; the desktop app opens only when you launch it.
To open the desktop app at login too, add it in **System Settings → General →
Login Items**.

## Install on Linux

1. Install the runtime libraries for **your distribution**:

   **Debian or Ubuntu**

   ```sh
   sudo apt install libgtk-3-0t64 libnss3 libasound2t64 libgbm1 libxss1 libnotify4
   ```

   **Fedora**

   ```sh
   sudo dnf install gtk3 nss alsa-lib mesa-libgbm libXScrnSaver libnotify
   ```

   **Arch**

   ```sh
   sudo pacman -S --needed gtk3 nss alsa-lib mesa libxss libnotify
   ```

2. Download the Linux tarball for your architecture and `SHA256SUMS`.
   In the download folder, verify and extract it. Replace `<tag>` with the
   release tag; use `arm64` instead of `amd64` for ARM64:

   ```sh
   sha256sum -c --ignore-missing SHA256SUMS
   tar -xzf Hypercube-<tag>-linux-amd64.tar.gz
   Hypercube/resources/install
   ```

3. Put `~/.local/bin` on your `PATH` if it is not already there. The installer
   places the app in `~/.local/share/hypercube` and links its CLI tools into
   `~/.local/bin`.
   On Ubuntu, it asks for `sudo` to install the sandbox's AppArmor profile.
4. Open **Hypercube** from the application launcher. Choose **Set Up Background
   Service**. When setup finishes, the workspace view replaces the waiting page.
5. Delete the extracted `Hypercube` folder if you no longer need it. The
   installed copy is independent of that folder.

No configuration file is required. Closing the window quits the desktop app,
but its background service keeps running.

## Next steps

- [Your first session](getting-started.md) — a short, read-only walkthrough.
- [Upgrade Hypercube](upgrades.md) — replace an existing installation.
- [Build from source](https://github.com/kkestell/hypercube/blob/main/CONTRIBUTING.md)
  — for contributors and source installations.
