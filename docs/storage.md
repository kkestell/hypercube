---
title: Files and storage
weight: 33
---

# Files and storage

Hypercube saves its own transcripts and session state. Harnesses also keep
native histories needed to resume conversations.

## Hypercube files

| Item | Default location |
| --- | --- |
| Configuration | `~/.config/hypercube/config.json` |
| Saved-state directory | `~/.local/state/hypercube/` |
| Database | `~/.local/state/hypercube/hypercube.db` |
| Service log | `~/.local/state/hypercube/ui.log` |
| Desktop profile and window position | `~/.local/state/hypercube/chromium/` |
| macOS login service | `~/Library/LaunchAgents/com.github.kkestell.hypercube.plist` |
| Linux login service | `~/.config/systemd/user/com.github.kkestell.hypercube.service` |
| macOS app | `~/Applications/Hypercube.app` |
| Linux app | `~/.local/share/hypercube/` |
| Ubuntu AppArmor profile | `/etc/apparmor.d/com.github.kkestell.hypercube` |

Configuration follows `XDG_CONFIG_HOME`; saved state follows `XDG_STATE_HOME`.
The Linux service unit also follows `XDG_CONFIG_HOME`; the Linux app directory
follows `XDG_DATA_HOME`.

## Native conversations

Claude Code, OpenCode, and Pi retain histories in their own storage.

Hypercube no longer includes Codex support. Existing Codex sessions,
transcripts, and native Codex history are preserved, but Hypercube does not
resume or convert those conversations; start new work with a supported
harness.

## Session cleanup

| Action | Conversation history | Managed worktrees and branches |
| --- | --- | --- |
| Archive | Kept read-only and searchable; family leaves the active list. | Removed after Git checks. |
| Delete | Family's chats deleted. | Removed after Git checks. |
| End child | Kept read-only; child cannot resume. | Removed after Git checks. |
| Unarchive | Returns to active use. | If the session had a managed worktree, it is recreated on its branch from the original checkout's HEAD. |

Before removing a managed worktree, Hypercube checks for uncommitted files and
unmerged commits. A child's commits must be merged into its parent; a top-level
worktree's commits must be merged into the original checkout's HEAD. Removal
is refused without changes if checks fail. Checks honor Git's submodule
settings, including settings that ignore submodule changes.

Original project folders and existing directories assigned to children are
not removed. Ended children can be deleted individually.

## Build identities

| Check | Identifies |
| --- | --- |
| About Hypercube | Version reported by the running service. On macOS, in the Hypercube menu; on Linux, in Help. |
| `hypercube version` | Executable on your PATH. |
| Service log at startup | Backend build. |

The identity contains a release tag or commit, with `+dirty` for uncommitted
checkout changes. A build without Git metadata reports `devel`. Installed and
running versions can differ until the service is reloaded.
